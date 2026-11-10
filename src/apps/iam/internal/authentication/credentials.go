package authentication

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/account"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// attemptKey 认证请求上下文的类型化键。
type attemptKey struct{}

// Attempt 保存单次请求里刚建立的会话，供会话提交失败时补偿撤销。
// 它只承载会话结果：request_id 与来源由调用方显式传入。
type Attempt struct {
	reauthenticate bool                 // 是否为当前账号的密码确认。
	principal      *iamclient.Principal // 已验证的账号投影。
}

// NewAttempt 为每个请求创建空的认证状态；认证成功后才写入 Principal。
func NewAttempt() *Attempt { return &Attempt{} }

// WithAttempt 将本次认证请求状态放入请求上下文。
func WithAttempt(ctx context.Context, a *Attempt) context.Context {
	return context.WithValue(ctx, attemptKey{}, a)
}

// AttemptFrom 读取本次请求的认证状态；读取方只用于会话提交与失败补偿。
func AttemptFrom(ctx context.Context) *Attempt {
	a, _ := ctx.Value(attemptKey{}).(*Attempt)
	return a
}

// SetPrincipal 记录本次请求已建立的会话，供中间件判断是否需要补偿。
func (a *Attempt) SetPrincipal(p iamclient.Principal, reauthenticate bool) {
	a.principal, a.reauthenticate = &p, reauthenticate
}

// Session 返回本次请求新建的会话，以及它是否属于密码确认。
func (a *Attempt) Session() (*iamclient.Principal, bool) { return a.principal, a.reauthenticate }

// ErrInvalidCredentials 统一表示账号或密码不可用于登录，不暴露账号状态。
var ErrInvalidCredentials = errors.New("invalid login credentials")

// Credentials 是已经通过 HTTP 协议边界校验的本次登录凭据。
type Credentials struct {
	Username string
	Password string
}

// RequestMetadata 只传递审计所需的请求信息，不包含 HTTP 对象。
type RequestMetadata struct {
	RequestID string // 关联本次请求的标识，也是登录审计的定位键。
	ClientIP  string // 来源地址，写入登录审计供溯源。
}

// Authenticator 编排 IAM 密码登录业务，密码与会话机制由成熟组件承担。
type Authenticator struct {
	sessions  *Sessions
	limiter   *Limiter
	slots     chan struct{}
	dummyHash string
}

// NewAuthenticator 显式接收会话、限流与密码计算资源，不创建第二套账号状态。
func NewAuthenticator(sessions *Sessions, limiter *Limiter, slots chan struct{}) (*Authenticator, error) {
	hash, err := (account.PasswordHasher{}).GenerateHash("unused comparison password")
	if err != nil {
		return nil, err
	}
	return &Authenticator{sessions: sessions, limiter: limiter, slots: slots, dummyHash: hash}, nil
}

// Login 验证账号凭据并创建可撤销会话，不读取请求或输出响应。
func (s *Authenticator) Login(ctx context.Context, credentials Credentials, metadata RequestMetadata) (iamclient.Principal, error) {
	return s.authenticate(ctx, credentials, metadata, false)
}

// Reauthenticate 只确认当前会话账号，保留原始绝对到期时间。
func (s *Authenticator) Reauthenticate(ctx context.Context, password string, metadata RequestMetadata) (iamclient.Principal, error) {
	principal, err := s.sessions.Resolve(ctx)
	if err != nil {
		return iamclient.Principal{}, err
	}
	return s.authenticate(ctx, Credentials{Username: principal.Username, Password: password}, metadata, true)
}

// authenticate 共用密码比较与失败审计，成功提交仍检查最新账号及凭证版本。
func (s *Authenticator) authenticate(ctx context.Context, credentials Credentials, metadata RequestMetadata, reauthenticate bool) (iamclient.Principal, error) {
	normalized, err := account.NormalizeUsername(credentials.Username)
	if err != nil {
		normalized = ""
	}
	if err := s.limiter.CheckAccount(ctx, normalized); err != nil {
		if errors.Is(err, ErrRateLimited) {
			logAuthFailure(ctx, slog.LevelWarn, "auth.login_rejected", "rate_limited", nil)
		} else {
			logAuthFailure(ctx, slog.LevelError, "auth.login_failed", "dependency_unavailable", nil)
		}
		return iamclient.Principal{}, err
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		logAuthFailure(ctx, slog.LevelWarn, "auth.login_rejected", "capacity_exhausted", nil)
		return iamclient.Principal{}, ErrRateLimited
	}
	user, err := repository.GetLoginUser(ctx, s.sessions.Store.DB, normalized)
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
		_ = (account.PasswordHasher{}).CompareHashAndPassword(s.dummyHash, credentials.Password)
		logAuthFailure(ctx, slog.LevelWarn, "auth.login_failed", "invalid_credentials", nil)
		if err := s.failure(ctx, nil, metadata); err != nil {
			return iamclient.Principal{}, err
		}
		return iamclient.Principal{}, ErrInvalidCredentials
	}
	if err != nil {
		return iamclient.Principal{}, err
	}
	now, err := repository.DatabaseTime(ctx, s.sessions.Store.DB)
	if err != nil {
		return iamclient.Principal{}, err
	}

	passwordErr := (account.PasswordHasher{}).CompareHashAndPassword(user.PasswordHash, credentials.Password)
	locked := user.LockedUntil != nil && user.LockedUntil.After(now)
	if passwordErr != nil || user.Status != enum.UserStatusActive || locked {
		// 密码确认失败不累计登录失败：操作者已持有有效会话，
		// 计入锁定会让一次输错的 sudo 确认把账号锁在门外。
		if !reauthenticate {
			if err := s.failure(ctx, &user, metadata); err != nil {
				return iamclient.Principal{}, err
			}
		}

		if reauthenticate {
			logAuthFailure(ctx, slog.LevelWarn, "auth.reauthenticate_failed", "password_mismatch", &user.ID)
			return iamclient.Principal{}, account.ErrPasswordMismatch
		}

		logAuthFailure(ctx, slog.LevelWarn, "auth.login_failed", "invalid_credentials", &user.ID)
		return iamclient.Principal{}, ErrInvalidCredentials
	}

	return s.sessions.Create(ctx, user, metadata.RequestID, reauthenticate)
}

// logAuthFailure 只记录稳定原因和已解析的用户标识，不记录账号或密码。
func logAuthFailure(ctx context.Context, level slog.Level, event, reason string, userID *uuid.UUID) {
	args := []any{"event", event, "reason", reason}
	if userID != nil {
		args = append(args, "user_id", userID.String())
	}
	logging.From(ctx).Log(ctx, level, "Authentication did not complete", args...)
}

// failure 记录一次登录失败：累计失败次数并写登录审计。
func (s *Authenticator) failure(ctx context.Context, user *repository.LoginUser, metadata RequestMetadata) error {
	return s.sessions.Store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		audit := repository.Audit{
			ActorType:  "system",
			Action:     "iam.auth.login",
			TargetType: "user",
			Outcome:    enum.AuditOutcomeDenied,
			Reason:     "invalid_credentials",
			RequestID:  metadata.RequestID,
			Metadata:   loginMetadata(metadata),
		}

		if user != nil {
			if _, err = repository.LockedAccount(ctx, tx, user.ID); err != nil {
				return err
			}

			err = repository.RecordLoginFailure(ctx, tx, user.ID, s.sessions.Config.Auth.LoginMaxFailures, s.sessions.Config.Auth.LoginLockDuration)
			if err != nil {
				return err
			}

			audit.ActorID = &user.ID
			audit.ActorName = user.Username
		}

		if err = repository.AppendAudit(ctx, tx, audit); err != nil {
			return err
		}

		return nil
	})
}

// loginMetadata 把登录来源写进审计：会话表只保留认证状态。
func loginMetadata(metadata RequestMetadata) map[string]any {
	if metadata.ClientIP == "" {
		return nil
	}

	return map[string]any{"client_ip": metadata.ClientIP}
}
