// Package authentication 将 SCS 接入可撤销的 IAM 会话。
package authentication

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/config"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

const LogicalSessionKey = "iam_session_id"

// Sessions 管理浏览器会话及数据库中可撤销的会话记录。
type Sessions struct {
	Manager *scs.SessionManager // Cookie 与会话令牌管理。
	Store   *repository.Store   // 可撤销会话的持久化存储。
	Config  config.Config       // 会话有效期等配置。
}

// NewSessions 初始化会话存储、Cookie 策略和最长有效时间。
func NewSessions(store *repository.Store, cfg config.Config) *Sessions {
	manager := scs.New()
	// 过期记录由显式维护入口清理；此存储未启用后台清理任务。
	manager.Store = pgxstore.NewWithConfig(store.Pool, pgxstore.Config{TableName: "iam.scs_sessions"})
	manager.HashTokenInStore = true
	manager.Lifetime = cfg.Auth.SessionAbsoluteTimeout
	manager.IdleTimeout = 0
	manager.Cookie.Name = "spx_session"
	manager.Cookie.Secure = strings.HasPrefix(cfg.Auth.WebOrigin, "https://")
	if manager.Cookie.Secure {
		manager.Cookie.Name = "__Host-spx_session"
	}

	manager.Cookie.HttpOnly = true
	manager.Cookie.Path = "/"
	manager.Cookie.SameSite = http.SameSiteLaxMode
	manager.Cookie.Persist = false
	return &Sessions{
		Manager: manager,
		Store:   store,
		Config:  cfg,
	}
}

// TokenHash 与当前 SCS 的令牌哈希实现保持一致。
func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// Resolve 验证当前会话并返回登录用户。
func (s *Sessions) Resolve(ctx context.Context) (iamclient.Principal, error) {
	id, err := uuid.Parse(s.Manager.GetString(ctx, LogicalSessionKey))
	if err != nil {
		return iamclient.Principal{}, repository.ErrUnauthenticated
	}

	token := s.Manager.Token(ctx)
	if token == "" {
		return iamclient.Principal{}, repository.ErrUnauthenticated
	}

	var principal iamclient.Principal
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		resolved, resolveErr := s.ResolveTx(ctx, tx, id, token)
		if resolveErr != nil {
			return resolveErr
		}

		principal = resolved

		return nil
	})
	if err != nil {
		return iamclient.Principal{}, err
	}

	return principal, nil
}

// ResolveTx 在事务中先锁定用户再锁定会话。数据库在取得锁后判断过期时间，
// 防止延迟请求重新激活已空闲过期的会话。
func (s *Sessions) ResolveTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, token string) (iamclient.Principal, error) {
	userID, err := repository.SessionUser(ctx, tx, id)
	if err != nil {
		return iamclient.Principal{}, sessionError(err)
	}

	user, err := repository.LockedAccount(ctx, tx, userID)
	if err != nil {
		return iamclient.Principal{}, sessionError(err)
	}

	if user.Status != enum.UserStatusActive {
		return iamclient.Principal{}, repository.ErrUnauthenticated
	}

	session, err := repository.LockedSession(ctx, tx, id)
	if err != nil {
		return iamclient.Principal{}, sessionError(err)
	}

	if session.UserID != user.ID || session.AuthEpoch != user.AuthEpoch || session.TokenHash != TokenHash(token) {
		return iamclient.Principal{}, repository.ErrUnauthenticated
	}

	stored, err := repository.HasStoredSession(ctx, tx, session.TokenHash)
	if err != nil {
		return iamclient.Principal{}, err
	}

	if !stored {
		return iamclient.Principal{}, repository.ErrUnauthenticated
	}

	session, err = repository.TouchSession(ctx, tx, repository.TouchSessionParams{
		ID:          id,
		TokenHash:   session.TokenHash,
		AuthEpoch:   user.AuthEpoch,
		IdleTimeout: s.Config.Auth.SessionIdleTimeout,
	})
	if err != nil {
		return iamclient.Principal{}, sessionError(err)
	}

	p := iamclient.Principal{
		UserID:          user.ID.String(),
		SessionID:       session.ID.String(),
		Username:        user.Username,
		DisplayName:     user.DisplayName,
		Configured:      user.Configured,
		AuthEpoch:       user.AuthEpoch,
		AuthenticatedAt: session.AuthenticatedAt,
	}
	p.ReauthenticatedAt = session.ReauthenticatedAt

	return p, nil
}

// Create 轮换会话令牌并将其绑定到会话记录。
// 会话只保存认证状态与期限；来源信息属于登录审计，不在这里重复存一份。
func (s *Sessions) Create(ctx context.Context, user repository.LoginUser, requestID string, reauthenticate bool) (iamclient.Principal, error) {
	oldID, _ := uuid.Parse(s.Manager.GetString(ctx, LogicalSessionKey))
	oldToken := s.Manager.Token(ctx)
	// 先生成新随机令牌，再把摘要绑定到数据库逻辑会话。
	if err := s.Manager.RenewToken(ctx); err != nil {
		return iamclient.Principal{}, err
	}

	token := s.Manager.Token(ctx)
	var principal iamclient.Principal
	var newID uuid.UUID
	var err error
	txErr := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		userLocks := []uuid.UUID{user.ID}
		if oldID != uuid.Nil {
			previousUser, lookup := repository.SessionUser(ctx, tx, oldID)
			if lookup == nil {
				userLocks = append(userLocks, previousUser)
			} else if !errors.Is(lookup, pgx.ErrNoRows) {
				return lookup
			}
		}

		if err = repository.LockUsers(ctx, tx, userLocks); err != nil {
			return err
		}

		locked, err := repository.LockedAccount(ctx, tx, user.ID)
		if err != nil {
			return sessionError(err)
		}

		if locked.Status != enum.UserStatusActive || locked.AuthEpoch != user.AuthEpoch {
			return repository.ErrUnauthenticated
		}

		valid, err := repository.VerifyLoginCredential(ctx, tx, user.ID, user.PasswordHash)
		if err != nil {
			return err
		}

		if !valid {
			return repository.ErrUnauthenticated
		}

		now, err := repository.DatabaseTime(ctx, tx)
		if err != nil {
			return err
		}

		authenticated, absolute := now, now.Add(s.Config.Auth.SessionAbsoluteTimeout)
		var reauthenticated *time.Time
		if reauthenticate {
			previous, err := repository.LockedSession(ctx, tx, oldID)
			if err != nil || previous.State != enum.SessionStateActive || previous.UserID != user.ID || previous.TokenHash != TokenHash(oldToken) || previous.AuthEpoch != locked.AuthEpoch || !previous.IdleExpiresAt.After(now) || !previous.AbsoluteExpiresAt.After(now) {
				return repository.ErrUnauthenticated
			}

			authenticated, absolute = previous.AuthenticatedAt, previous.AbsoluteExpiresAt
			reauthenticated = &now
		}

		if oldID != uuid.Nil {
			if err = repository.RevokeRotatedSession(ctx, tx, oldID, TokenHash(oldToken)); err != nil {
				return err
			}
		}

		newID = uuid.New()
		idle := now.Add(s.Config.Auth.SessionIdleTimeout)
		if idle.After(absolute) {
			idle = absolute
		}

		if err = repository.InsertSession(ctx, tx, repository.InsertSessionParams{
			ID:                newID,
			UserID:            user.ID,
			TokenHash:         TokenHash(token),
			AuthEpoch:         user.AuthEpoch,
			AuthenticatedAt:   authenticated,
			ReauthenticatedAt: reauthenticated,
			AbsoluteExpiresAt: absolute,
			IdleExpiresAt:     idle,
			Now:               now,
		}); err != nil {
			return err
		}

		if err = repository.ResetLoginFailures(ctx, tx, user.ID); err != nil {
			return err
		}

		if err = repository.AppendAudit(ctx, tx, repository.Audit{
			ActorID:    &user.ID,
			ActorName:  user.Username,
			Action:     "iam.auth.login",
			TargetType: "session",
			TargetID:   &newID,
			RequestID:  requestID,
		}); err != nil {
			return err
		}

		principal = iamclient.Principal{
			UserID:            user.ID.String(),
			SessionID:         newID.String(),
			Username:          user.Username,
			DisplayName:       user.DisplayName,
			Configured:        user.Configured,
			AuthEpoch:         user.AuthEpoch,
			AuthenticatedAt:   authenticated,
			ReauthenticatedAt: reauthenticated}

		return nil
	})
	if txErr != nil {
		return iamclient.Principal{}, txErr
	}

	s.Manager.Put(ctx, LogicalSessionKey, newID.String())

	return principal, nil
}

// Revoke 撤销本人当前会话。
func (s *Sessions) Revoke(ctx context.Context, p iamclient.Principal, sessionID string, reason, requestID string) error {
	userID, err := uuid.Parse(p.UserID)
	if err != nil {
		return repository.ErrUnauthenticated
	}

	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err = repository.LockedAccount(ctx, tx, userID); err != nil {
			return err
		}

		actorSession, parseErr := uuid.Parse(p.SessionID)
		if parseErr != nil {
			return repository.ErrUnauthenticated
		}

		fresh, err := s.ResolveTx(ctx, tx, actorSession, s.Manager.Token(ctx))
		if err != nil {
			return err
		}

		if fresh.UserID != p.UserID {
			return repository.ErrUnauthenticated
		}

		id, err := uuid.Parse(sessionID)
		if err != nil {
			return repository.ErrInvalid
		}

		if err = repository.RevokeOwnSession(ctx, tx, id, userID, reason); err != nil {
			return err
		}

		return repository.AppendAudit(ctx, tx, repository.Audit{
			ActorID:    &userID,
			ActorName:  p.Username,
			Action:     "iam.sessions.revoke",
			TargetType: "session",
			RequestID:  requestID,
		})
	})
	if err != nil {
		return err
	}

	logging.From(ctx).Info("Session revoked", "event", "session.revoked", "user_id", p.UserID, "session_id", sessionID, "reason", reason)

	return nil
}

// sessionError 将未找到会话映射为未认证，保留真实存储故障。
func sessionError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return repository.ErrUnauthenticated
	}

	return err
}
