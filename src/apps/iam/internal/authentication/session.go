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
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/config"
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

// Resolve 验证当前会话并返回用户身份。
func (s *Sessions) Resolve(ctx context.Context) (iamclient.Principal, error) {
	id, err := uuid.Parse(s.Manager.GetString(ctx, LogicalSessionKey))
	if err != nil {
		return iamclient.Principal{}, repository.ErrUnauthenticated
	}

	token := s.Manager.Token(ctx)
	if token == "" {
		return iamclient.Principal{}, repository.ErrUnauthenticated
	}

	tx := s.Store.DB.WithContext(ctx).Begin()
	err = tx.Error
	if err != nil {
		return iamclient.Principal{}, err
	}

	defer tx.Rollback()

	principal, err := s.ResolveTx(ctx, tx, id, token)
	if err != nil {
		return iamclient.Principal{}, err
	}

	if err = tx.Commit().Error; err != nil {
		return iamclient.Principal{}, err
	}

	return principal, nil
}

// ResolveTx 在事务中先锁定用户再锁定会话。数据库在取得锁后判断过期时间，
// 防止延迟请求重新激活已空闲过期的会话。
func (s *Sessions) ResolveTx(ctx context.Context, tx *gorm.DB, id uuid.UUID, token string) (iamclient.Principal, error) {
	var userID uuid.UUID
	if err := repository.SessionUser(ctx, tx, id, &userID); err != nil {
		return iamclient.Principal{}, sessionError(err)
	}

	q := repository.NewQueries(tx)
	user, err := q.LockUser(ctx, userID)
	if err != nil {
		return iamclient.Principal{}, sessionError(err)
	}

	if user.Status != "active" {
		return iamclient.Principal{}, repository.ErrUnauthenticated
	}

	session, err := q.LockSession(ctx, id)
	if err != nil {
		return iamclient.Principal{}, sessionError(err)
	}

	if session.UserID != user.ID || session.AuthVersion != user.AuthVersion || session.TokenHash != TokenHash(token) {
		return iamclient.Principal{}, repository.ErrUnauthenticated
	}

	var stored bool
	if err = repository.HasStoredSession(ctx, tx, session.TokenHash, &stored); err != nil {
		return iamclient.Principal{}, err
	}

	if !stored {
		return iamclient.Principal{}, repository.ErrUnauthenticated
	}

	session, err = q.TouchSession(ctx, repository.TouchSessionParams{
		ID:          id,
		TokenHash:   session.TokenHash,
		AuthVersion: user.AuthVersion,
		IdleTimeout: interval(s.Config.Auth.SessionIdleTimeout),
	})
	if err != nil {
		return iamclient.Principal{}, sessionError(err)
	}

	p := iamclient.Principal{
		UserID:          user.ID.String(),
		SessionID:       session.ID.String(),
		Username:        user.Username,
		DisplayName:     user.DisplayName,
		IsSuperAdmin:    user.IsSuperAdmin,
		AuthVersion:     user.AuthVersion,
		AuthenticatedAt: session.AuthenticatedAt,
	}
	if session.ReauthenticatedAt.Valid {
		p.ReauthenticatedAt = &session.ReauthenticatedAt.Time
	}

	return p, nil
}

// Create 轮换会话令牌并将其绑定到会话记录。
func (s *Sessions) Create(ctx context.Context, user repository.LoginUser, requestID, userAgent, clientIP string, reauthenticate bool) (iamclient.Principal, error) {
	oldID, _ := uuid.Parse(s.Manager.GetString(ctx, LogicalSessionKey))
	oldToken := s.Manager.Token(ctx)
	// 先生成新随机令牌，再把摘要绑定到数据库逻辑会话。
	if err := s.Manager.RenewToken(ctx); err != nil {
		return iamclient.Principal{}, err
	}

	token := s.Manager.Token(ctx)
	tx := s.Store.DB.WithContext(ctx).Begin()
	err := tx.Error
	if err != nil {
		return iamclient.Principal{}, err
	}

	defer tx.Rollback()

	userLocks := []uuid.UUID{user.ID}
	if oldID != uuid.Nil {
		var previousUser uuid.UUID
		lookup := repository.SessionUser(ctx, tx, oldID, &previousUser)
		if lookup == nil {
			userLocks = append(userLocks, previousUser)
		} else if !errors.Is(lookup, pgx.ErrNoRows) {
			return iamclient.Principal{}, lookup
		}
	}

	if err = repository.LockUsers(ctx, tx, userLocks); err != nil {
		return iamclient.Principal{}, err
	}

	q := repository.NewQueries(tx)
	locked, err := q.LockUser(ctx, user.ID)
	if err != nil {
		return iamclient.Principal{}, sessionError(err)
	}

	if locked.Status != "active" || locked.AuthVersion != user.AuthVersion {
		return iamclient.Principal{}, repository.ErrUnauthenticated
	}

	var valid bool
	if err = repository.VerifyLoginCredential(ctx, tx, user.ID, user.PasswordHash, &valid); err != nil {
		return iamclient.Principal{}, err
	}

	if !valid {
		return iamclient.Principal{}, repository.ErrUnauthenticated
	}

	var now time.Time
	if err = repository.DatabaseTime(ctx, tx, &now); err != nil {
		return iamclient.Principal{}, err
	}

	authenticated, absolute := now, now.Add(s.Config.Auth.SessionAbsoluteTimeout)
	var reauthenticated *time.Time
	if reauthenticate {
		previous, err := q.LockSession(ctx, oldID)
		if err != nil || previous.State != "active" || previous.UserID != user.ID || previous.TokenHash != TokenHash(oldToken) || previous.AuthVersion != locked.AuthVersion || !previous.IdleExpiresAt.After(now) || !previous.AbsoluteExpiresAt.After(now) {
			return iamclient.Principal{}, repository.ErrUnauthenticated
		}

		authenticated, absolute = previous.AuthenticatedAt, previous.AbsoluteExpiresAt
		reauthenticated = &now
	}

	if oldID != uuid.Nil {
		if err = repository.RevokeRotatedSession(ctx, tx, oldID, TokenHash(oldToken)); err != nil {
			return iamclient.Principal{}, err
		}
	}

	newID := uuid.New()
	idle := now.Add(s.Config.Auth.SessionIdleTimeout)
	if idle.After(absolute) {
		idle = absolute
	}

	if len(userAgent) > 512 {
		userAgent = string([]rune(userAgent)[:min(len([]rune(userAgent)), 128)])
	}

	var ip any
	if clientIP != "" {
		ip = clientIP
	}

	if err = repository.InsertSession(ctx, tx, newID, user.ID, TokenHash(token), user.AuthVersion, authenticated, reauthenticated, absolute, idle, now, ip, userAgent); err != nil {
		return iamclient.Principal{}, err
	}

	if err = repository.ResetLoginFailures(ctx, tx, user.ID); err != nil {
		return iamclient.Principal{}, err
	}

	if err = repository.AppendAudit(ctx, tx, repository.Audit{
		ActorID:    &user.ID,
		ActorName:  user.Username,
		Action:     "iam.auth.login",
		TargetType: "session",
		TargetID:   &newID,
		RequestID:  requestID,
	}); err != nil {
		return iamclient.Principal{}, err
	}

	if err = tx.Commit().Error; err != nil {
		return iamclient.Principal{}, err
	}

	s.Manager.Put(ctx, LogicalSessionKey, newID.String())
	return iamclient.Principal{
		UserID:            user.ID.String(),
		SessionID:         newID.String(),
		Username:          user.Username,
		DisplayName:       user.DisplayName,
		IsSuperAdmin:      user.IsSuperAdmin,
		AuthVersion:       user.AuthVersion,
		AuthenticatedAt:   authenticated,
		ReauthenticatedAt: reauthenticated,
	}, nil
}

// Revoke 撤销本人当前会话。
func (s *Sessions) Revoke(ctx context.Context, p iamclient.Principal, sessionID string, reason, requestID string) error {
	tx := s.Store.DB.WithContext(ctx).Begin()
	err := tx.Error
	if err != nil {
		return err
	}

	defer tx.Rollback()

	userID, err := uuid.Parse(p.UserID)
	if err != nil {
		return repository.ErrUnauthenticated
	}

	if _, err = repository.NewQueries(tx).LockUser(ctx, userID); err != nil {
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

	response := tx.WithContext(ctx).Exec(`UPDATE iam.user_sessions SET state='revoked',revoked_at=clock_timestamp(),revoke_reason=$3 WHERE id=$1 AND user_id=$2 AND state='active'`, id, userID, reason)
	if err = response.Error; err != nil {
		return err
	}

	if response.RowsAffected == 0 {
		return repository.ErrNotFound
	}

	if err = repository.AppendAudit(ctx, tx, repository.Audit{
		ActorID:    &userID,
		ActorName:  p.Username,
		Action:     "iam.sessions.revoke",
		TargetType: "session",
		RequestID:  requestID,
	}); err != nil {
		return err
	}

	if err = tx.Commit().Error; err != nil {
		return err
	}
	logging.From(ctx).Info("逻辑会话撤销已提交", "event", "session.revoked", "user_id", p.UserID, "session_id", sessionID, "reason", reason)
	return nil
}

// sessionError 将未找到会话映射为未认证，保留真实存储故障。
func sessionError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
		return repository.ErrUnauthenticated
	}

	return err
}
