// Package service 实现 IAM 用例、授权及事务边界。
package service

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/EayonLee/SecPlexus/src/packages/go/logging"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authentication"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/config"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
)

// Service 协调 IAM 用例、会话校验和数据存储。
type Service struct {
	Store    *repository.Store        // IAM 数据访问。
	Sessions *authentication.Sessions // 会话校验与撤销。
	Config   config.Config            // 认证策略与服务配置。
}

// Authorize 在事务中验证当前会话并计算指定能力的授权范围。
func (s *Service) Authorize(ctx context.Context, action, orgID string) (decision contract.CapabilityDecision, err error) {
	ctx = logging.With(ctx, "operation", "iam.authorize", "action", action, "org_id", orgID)
	started := time.Now()
	defer func() {
		if err != nil || !decision.Allowed {
			logging.From(ctx).Warn("能力校验未通过", "event", "iam.authorization_rejected", "duration_ms", time.Since(started).Milliseconds(), "error", logging.Error(err))
		} else {
			logging.From(ctx).Debug(
				"能力校验通过",
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
		scope, err := s.authorizeTx(ctx, tx, p, action, orgID)
		if err != nil {
			return err
		}
		decision = contract.CapabilityDecision{Allowed: true, DecisionID: uuid.NewString(), Principal: p, Scope: scope}
		return nil
	})
	if err != nil {
		return contract.CapabilityDecision{}, err
	}
	return decision, nil
}

// resolve 在当前事务内解析并复核逻辑会话。
func (s *Service) resolve(ctx context.Context, tx *gorm.DB) (contract.Principal, error) {
	id, err := uuid.Parse(s.Sessions.Manager.GetString(ctx, authentication.LogicalSessionKey))
	if err != nil {
		return contract.Principal{}, repository.ErrUnauthenticated
	}

	return s.Sessions.ResolveTx(ctx, tx, id, s.Sessions.Manager.Token(ctx))
}

// authorizeTx 根据最新组织、成员和权限状态计算动作范围。
func (s *Service) authorizeTx(ctx context.Context, tx *gorm.DB, p contract.Principal, action, orgID string) (contract.AuthorizedScope, error) {
	permission, err := repository.ReadPermission(ctx, tx, action)
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
		return contract.AuthorizedScope{}, authorization.ErrDenied
	}

	if err != nil {
		return contract.AuthorizedScope{}, err
	}

	snapshot := authorization.Snapshot{Principal: p}
	if permission.Scope == "organization" {
		org, err := uuid.Parse(orgID)
		if err != nil {
			return contract.AuthorizedScope{}, authorization.ErrDenied
		}

		organization, err := repository.NewQueries(tx).GetOrganization(ctx, org)
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
			return contract.AuthorizedScope{}, authorization.ErrDenied
		}

		if err != nil {
			return contract.AuthorizedScope{}, err
		}

		snapshot.OrganizationID = org.String()
		snapshot.OrganizationActive = organization.Status == "active"
		userID, err := uuid.Parse(p.UserID)
		if err != nil {
			return contract.AuthorizedScope{}, repository.ErrUnauthenticated
		}

		if err = repository.ActiveMembership(ctx, tx, org, userID, &snapshot.MembershipActive); err != nil {
			return contract.AuthorizedScope{}, err
		}

		snapshot.Permissions, err = repository.NewQueries(tx).GetOrganizationPermissions(ctx, repository.GetOrganizationPermissionsParams{OrgID: org,
			UserID: userID})
		if err != nil {
			return contract.AuthorizedScope{}, err
		}
	}

	var now time.Time
	if err = repository.DatabaseTime(ctx, tx, &now); err != nil {
		return contract.AuthorizedScope{}, err
	}

	return authorization.Evaluate(snapshot, permission, orgID, now, s.Config.Auth.ReauthWindow)
}

// Manage 串行执行低频管理变更，在同一事务内重新校验操作者并写入审计。
func (s *Service) Manage(ctx context.Context, action, orgID, requestID, targetType string, targetID *uuid.UUID, change func(*gorm.DB, contract.Principal, map[string]any) error, additionalUsers ...uuid.UUID) (err error) {
	// 诊断只记录稳定动作和资源标识，不记录请求参数或审计扩展值。
	ctx = logging.With(ctx, "operation", action, "org_id", orgID, "target_type", targetType, "target_id", targetID)
	started := time.Now()
	phase := "begin"
	committed := false
	var actorID uuid.UUID
	defer func() {
		level, outcome := slog.LevelInfo, "committed"
		if !committed {
			level, outcome = slog.LevelError, "aborted"
		}
		if err != nil {
			outcome = "failed"
			level = slog.LevelWarn
			if !errors.Is(err, authorization.ErrDenied) && !errors.Is(err, repository.ErrInvalid) && !errors.Is(err, repository.ErrConflict) && !errors.Is(err, repository.ErrNotFound) && !errors.Is(err, repository.ErrUnauthenticated) && !errors.Is(err, gorm.ErrRecordNotFound) {
				level = slog.LevelError
			}
		}
		logging.From(ctx).Log(
			ctx,
			level,
			"管理事务结束",
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

		phase = "lock_organization"
		if orgID != "" {
			var lockedID uuid.UUID
			if err = repository.LockOrganization(ctx, tx, orgID, &lockedID); err != nil {
				return err
			}
		}

		phase = "authorize"
		if _, err = s.authorizeTx(ctx, tx, p, action, orgID); err != nil {
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

		phase = "bump_authorization_version"
		var org *uuid.UUID
		if orgID != "" {
			parsed, err := uuid.Parse(orgID)
			if err != nil {
				return repository.ErrInvalid
			}

			org = &parsed
			if err = repository.BumpOrganizationVersion(ctx, tx, parsed); err != nil {
				return err
			}
		}

		phase = "append_audit"
		if err = repository.AppendAudit(ctx, tx, repository.Audit{
			OrgID:      org,
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
	committed = err == nil
	return err
}
