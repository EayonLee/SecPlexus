package authentication

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/identity"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// attemptKey 认证请求上下文的类型化键。
type attemptKey struct{}

// Attempt 保存单次请求的认证状态，不写入 Cookie 或共享缓存。
type Attempt struct {
	RequestID      string               // 本次请求的关联标识。
	Reauthenticate bool                 // 是否为当前账号的密码确认。
	Principal      *iamclient.Principal // 已验证的身份投影。
}

// WithAttempt 将本次认证请求状态放入请求上下文。
func WithAttempt(ctx context.Context, a *Attempt) context.Context {
	return context.WithValue(ctx, attemptKey{}, a)
}

// attempt 读取本次请求的认证状态，不作为应用依赖容器。
func attempt(ctx context.Context) *Attempt {
	a, _ := ctx.Value(attemptKey{}).(*Attempt)
	return a
}

// ErrInvalidCredentials 统一表示账号或密码不可用于登录，不暴露账号状态。
var ErrInvalidCredentials = errors.New("invalid login credentials")

// Credentials 是已经通过 HTTP 协议边界校验的本次登录凭据。
type Credentials struct {
	Username string
	Password string
}

// RequestMetadata 只传递审计和会话所需的请求信息，不包含 HTTP 对象。
type RequestMetadata struct {
	RequestID string
	ClientIP  string
	UserAgent string
}

// Authenticator 编排 IAM 密码登录业务，密码与会话机制由成熟组件承担。
type Authenticator struct {
	sessions  *Sessions
	limiter   *Limiter
	slots     chan struct{}
	dummyHash string
}

// NewAuthenticator 显式接收会话、限流与密码计算资源，不创建第二套身份状态。
func NewAuthenticator(sessions *Sessions, limiter *Limiter, slots chan struct{}) (*Authenticator, error) {
	hash, err := (identity.PasswordHasher{}).GenerateHash("unused comparison password")
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
	normalized, err := identity.NormalizeUsername(credentials.Username)
	if err != nil {
		normalized = ""
	}
	if err := s.limiter.CheckAccount(ctx, normalized); err != nil {
		return iamclient.Principal{}, err
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return iamclient.Principal{}, ErrRateLimited
	}
	user, err := repository.NewQueries(s.sessions.Store.DB).GetLoginUser(ctx, normalized)
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
		_ = (identity.PasswordHasher{}).CompareHashAndPassword(s.dummyHash, credentials.Password)
		if err := s.failure(ctx, nil); err != nil {
			return iamclient.Principal{}, err
		}
		return iamclient.Principal{}, ErrInvalidCredentials
	}
	if err != nil {
		return iamclient.Principal{}, err
	}
	passwordErr := (identity.PasswordHasher{}).CompareHashAndPassword(user.PasswordHash, credentials.Password)
	if passwordErr != nil || user.Status != "active" || (user.LockedUntil.Valid && user.LockedUntil.Time.After(time.Now())) {
		if err := s.failure(ctx, &user); err != nil {
			return iamclient.Principal{}, err
		}
		if reauthenticate {
			return iamclient.Principal{}, identity.ErrPasswordMismatch
		}
		return iamclient.Principal{}, ErrInvalidCredentials
	}
	return s.sessions.Create(ctx, user, metadata.RequestID, metadata.UserAgent, metadata.ClientIP, reauthenticate)
}

func (s *Authenticator) failure(ctx context.Context, user *repository.LoginUser) error {
	return s.sessions.Store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		audit := repository.Audit{
			ActorType:  "system",
			Action:     "iam.auth.login",
			TargetType: "user",
			Outcome:    "denied",
			Reason:     "invalid_credentials",
		}
		if a := attempt(ctx); a != nil {
			audit.RequestID = a.RequestID
		}

		if user != nil {
			if _, err = repository.NewQueries(tx).LockUser(ctx, user.ID); err != nil {
				return err
			}

			err = repository.RecordLoginFailure(ctx, tx, user.ID, s.sessions.Config.Auth.LoginMaxFailures, interval(s.sessions.Config.Auth.LoginLockDuration))
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

// CurrentAttempt 返回用于会话提交及失败补偿的请求状态。
func CurrentAttempt(ctx context.Context) *Attempt { return attempt(ctx) }
