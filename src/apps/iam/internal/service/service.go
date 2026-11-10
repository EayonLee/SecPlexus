// Package service 实现 IAM 用例、授权及事务边界。
package service

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authentication"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/config"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// Service 协调 IAM 用例、会话校验和数据存储。
type Service struct {
	Store    *repository.Store        // IAM 数据访问。
	Sessions *authentication.Sessions // 会话校验与撤销。
	Config   config.Config            // 认证策略与服务配置。
}

// Authorize 在事务中验证当前会话并判断指定权限。
// 授权只有一条路径：用户通过角色持有的权限键；没有组织或作用域分支。
func (s *Service) Authorize(ctx context.Context, action string) (decision iamclient.CapabilityDecision, err error) {
	ctx = logging.With(ctx, "operation", "iam.authorize", "action", action)
	started := time.Now()
	defer func() {
		if err != nil || !decision.Allowed {
			logging.From(ctx).Warn("Permission check denied", "event", "iam.authorization_rejected", "duration_ms", time.Since(started).Milliseconds(), "error", logging.Error(err))
		} else {
			logging.From(ctx).Debug(
				"Permission check allowed",
				"event", "iam.authorization_allowed",
				"decision_id", decision.DecisionID,
				"user_id", decision.Principal.UserID,
				"duration_ms", time.Since(started).Milliseconds(),
			)
		}
	}()
	err = s.Store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := s.resolve(ctx, tx)
		if err != nil {
			return err
		}

		if err := s.authorizeTx(ctx, tx, p, action); err != nil {
			return err
		}

		decision = iamclient.CapabilityDecision{Allowed: true, DecisionID: uuid.NewString(), Principal: p}
		return nil
	})
	if err != nil {
		return iamclient.CapabilityDecision{}, err
	}

	return decision, nil
}

// resolve 在当前事务内解析并复核逻辑会话。
func (s *Service) resolve(ctx context.Context, tx *gorm.DB) (iamclient.Principal, error) {
	id, err := uuid.Parse(s.Sessions.Manager.GetString(ctx, authentication.LogicalSessionKey))
	if err != nil {
		return iamclient.Principal{}, repository.ErrUnauthenticated
	}

	return s.Sessions.ResolveTx(ctx, tx, id, s.Sessions.Manager.Token(ctx))
}

// authorizeTx 按最新账号状态与权限键判断动作。
func (s *Service) authorizeTx(ctx context.Context, tx *gorm.DB, p iamclient.Principal, action string) error {
	permission, err := repository.ReadPermission(ctx, tx, action)
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
		return authorization.ErrDenied
	}

	if err != nil {
		return err
	}

	userID, err := uuid.Parse(p.UserID)
	if err != nil {
		return repository.ErrUnauthenticated
	}

	permissions, err := repository.EffectivePermissions(ctx, tx, userID)
	if err != nil {
		return err
	}

	var now time.Time
	if err = repository.DatabaseTime(ctx, tx, &now); err != nil {
		return err
	}

	return authorization.Evaluate(authorization.Snapshot{Principal: p, Permissions: permissions}, permission, now, s.Config.Auth.ReauthWindow)
}

// Manage 串行执行低频管理变更，在同一事务内重新校验操作者并写入审计。
func (s *Service) Manage(ctx context.Context, action, requestID, targetType string, targetID *uuid.UUID, change func(*gorm.DB, iamclient.Principal, map[string]any) error, additionalUsers ...uuid.UUID) (err error) {
	// 诊断只记录稳定动作和资源标识，不记录请求参数或审计扩展值。
	ctx = logging.With(ctx, "operation", action, "target_type", targetType, "target_id", targetID)
	started := time.Now()
	phase := "begin"
	var actorID uuid.UUID
	defer func() {
		level, outcome := slog.LevelInfo, "committed"
		if err != nil {
			outcome, level = "failed", slog.LevelWarn
			// 只有基础设施和数据库故障升级为 error；业务拒绝保持 warn，避免审计噪声。
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, sql.ErrConnDone) {
				level = slog.LevelError
			}
		}
		logging.From(ctx).Log(
			ctx,
			level,
			"Management transaction finished",
			"event", "iam.management.completed",
			"phase", phase,
			"outcome", outcome,
			"actor_id", actorID,
			"duration_ms", time.Since(started).Milliseconds(),
			"error", logging.Error(err),
		)
	}()

	err = s.Store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		phase = "lock_management"
		if err = repository.LockManagement(ctx, tx, repository.ManagementLock); err != nil {
			return err
		}

		phase = "resolve_actor"
		sessionID, err := uuid.Parse(s.Sessions.Manager.GetString(ctx, authentication.LogicalSessionKey))
		if err != nil {
			return repository.ErrUnauthenticated
		}

		if err = repository.SessionUser(ctx, tx, sessionID, &actorID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
				return repository.ErrUnauthenticated
			}

			return err
		}

		userLocks := append([]uuid.UUID{actorID}, additionalUsers...)
		if targetType == "user" && targetID != nil {
			userLocks = append(userLocks, *targetID)
		}

		phase = "lock_users"
		if err = repository.LockUsers(ctx, tx, userLocks); err != nil {
			return err
		}

		phase = "resolve_session"
		p, err := s.resolve(ctx, tx)
		if err != nil {
			return err
		}

		phase = "authorize"
		if err = s.authorizeTx(ctx, tx, p, action); err != nil {
			return err
		}

		phase = "apply_change"
		metadata := map[string]any{}
		if err = change(tx, p, metadata); err != nil {
			return err
		}

		actorID, err = uuid.Parse(p.UserID)
		if err != nil {
			return err
		}

		phase = "append_audit"
		if err = repository.AppendAudit(ctx, tx, repository.Audit{
			ActorID:    &actorID,
			ActorName:  p.Username,
			Action:     action,
			TargetType: targetType,
			TargetID:   targetID,
			RequestID:  requestID,
			Metadata:   metadata,
		}); err != nil {
			return err
		}

		phase = "commit"
		return nil
	})
	return err
}
