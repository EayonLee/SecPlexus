package authentication

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aarondl/authboss/v3"
	"github.com/aarondl/authboss/v3/auth"
	"github.com/aarondl/authboss/v3/defaults"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/identity"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/contracts/common"
	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// attemptKey 认证请求上下文的类型化键。
type attemptKey struct{}

// Attempt 保存单次请求的认证状态，不写入 Cookie 或共享缓存。
type Attempt struct {
	RequestID      string              // 本次请求的关联标识。
	ClientIP       string              // 经过可信代理规则解析的来源地址。
	Reauthenticate bool                // 是否为当前账号的密码确认。
	Principal      *contract.Principal // 已验证的身份投影。
	Username       string              // 本次输入的账号，不写日志。
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

// authUser 认证库使用的内部账号快照，不对外序列化凭据。
type authUser struct{ repository.LoginUser }

// GetPID 读取 Authboss 所需的登录标识。
func (u *authUser) GetPID() string { return u.Username }

// PutPID 接收认证库更新的登录标识。
func (u *authUser) PutPID(pid string) { u.Username = pid }

// GetPassword 仅向密码校验器提供内部哈希，不返回给浏览器。
func (u *authUser) GetPassword() string { return u.PasswordHash }

// PutPassword 接收认证库的内存哈希状态，不直接持久化账号快照。
func (u *authUser) PutPassword(hash string) { u.PasswordHash = hash }

var _ authboss.AuthableUser = (*authUser)(nil)

// authStore 在 IAM 存储和认证库之间适配账号与失败审计。
type authStore struct {
	sessions  *Sessions
	dummyHash string
}

var _ authboss.ServerStorer = (*authStore)(nil)

// Load 读取内部账号快照；未知账号仍执行密码计算以降低枚举风险。
func (s *authStore) Load(ctx context.Context, username string) (authboss.User, error) {
	normalized, err := identity.NormalizeUsername(username)
	if err != nil {
		normalized = ""
	}

	user, err := repository.NewQueries(s.sessions.Store.DB).GetLoginUser(ctx, normalized)
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
		// 未知或锁定账号仍承担密码计算成本，降低账号枚举风险。
		_ = (identity.PasswordHasher{}).CompareHashAndPassword(s.dummyHash, "unrelated authentication input")
		if err = s.failure(ctx, nil); err != nil {
			return nil, err
		}

		return nil, authboss.ErrUserNotFound
	}

	if err != nil {
		return nil, err
	}

	return &authUser{user}, nil
}

// Save 拒绝通用账号覆盖写入；密码和锁定状态必须通过 IAM 事务修改。
func (s *authStore) Save(context.Context, authboss.User) error {
	// 密码和锁定变更必须经过带版本的 IAM 事务，不覆盖写入认证库账号快照。
	return errors.New("use the IAM password or lock transaction")
}

// failure 在用户锁内更新登录失败次数并原子写入审计。
func (s *authStore) failure(ctx context.Context, user *authUser) error {
	tx := s.sessions.Store.DB.WithContext(ctx).Begin()
	err := tx.Error
	if err != nil {
		return err
	}

	defer tx.Rollback()

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

	return tx.Commit().Error
}

// Authenticator 将登录认证适配到 IAM 会话管理。
type Authenticator struct {
	Boss  *authboss.Authboss
	login *auth.Auth
}

// NewAuthenticator 初始化登录认证适配器。
func NewAuthenticator(sessions *Sessions) (*Authenticator, error) {
	dummy, err := (identity.PasswordHasher{}).GenerateHash("unused random comparison password")
	if err != nil {
		return nil, err
	}

	store := &authStore{sessions: sessions, dummyHash: dummy}
	boss := authboss.New()
	boss.Core.ViewRenderer = jsonRenderer{}
	defaults.SetCore(&boss.Config, true, true)
	boss.Core.Logger = defaults.NewLogger(io.Discard)
	boss.Core.Hasher = identity.PasswordHasher{}
	boss.Core.BodyReader = loginReader{}
	boss.Core.Responder = invalidResponder{}
	boss.Storage.Server = store
	boss.Storage.SessionState = sessionBridge{sessions: sessions}
	boss.Events.After(authboss.EventAuthFail, func(w http.ResponseWriter, r *http.Request, handled bool) (bool, error) {
		user, ok := r.Context().Value(authboss.CTXKeyUser).(*authUser)
		if !ok {
			return false, repository.ErrUnauthenticated
		}

		return false, store.failure(r.Context(), user)
	})
	boss.Events.Before(authboss.EventAuth, func(w http.ResponseWriter, r *http.Request, handled bool) (bool, error) {
		user := r.Context().Value(authboss.CTXKeyUser).(*authUser)
		if user.Status != "active" || (user.LockedUntil.Valid && user.LockedUntil.Time.After(time.Now())) {
			if err := store.failure(r.Context(), user); err != nil {
				return false, err
			}

			invalidCredentials(w, r)
			return true, nil
		}

		return false, nil
	})
	boss.Events.After(authboss.EventAuth, func(w http.ResponseWriter, r *http.Request, handled bool) (bool, error) {
		user := r.Context().Value(authboss.CTXKeyUser).(*authUser)
		a := attempt(r.Context())
		if a == nil {
			return false, errors.New("missing authentication request context")
		}

		p, err := sessions.Create(r.Context(), user.LoginUser, a.RequestID, r.UserAgent(), a.ClientIP, a.Reauthenticate)
		if err != nil {
			return false, err
		}

		a.Principal = &p
		result.Success(w, map[string]any{"principal": p})
		return true, nil
	})
	if err = boss.Init("auth"); err != nil {
		return nil, err
	}

	return &Authenticator{Boss: boss, login: &auth.Auth{Authboss: boss}}, nil
}

// Login 使用请求中的凭证执行登录认证。
func (a *Authenticator) Login(w http.ResponseWriter, r *http.Request) error {
	var response error
	a.Boss.LoadClientStateMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { response = a.login.LoginPost(w, r) })).ServeHTTP(w, r)
	return response
}

// loginValues 本次登录凭据，仅在请求期间使用。
type loginValues struct {
	Username string `json:"username"` // 本次输入的账号，不写日志。
	Password string `json:"password"` // 仅在请求期间使用的密码。
}

// GetPID 读取 Authboss 所需的登录标识。
func (v loginValues) GetPID() string { return strings.TrimSpace(v.Username) }

// GetPassword 仅在本次认证期间向密码校验器提供请求密码，不保存或记录。
func (v loginValues) GetPassword() string { return v.Password }

// Validate 结构校验由统一请求边界负责，此方法用于满足认证库接口。
func (v loginValues) Validate() []error { return nil }

// loginReader 适配统一 JSON 登录输入。
type loginReader struct{}

// Read 绑定认证输入并限制直接调用时的请求大小。
func (loginReader) Read(_ string, r *http.Request) (authboss.Validator, error) {
	var v loginValues
	decoder := json.NewDecoder(io.LimitReader(r.Body, 2048))
	if !httpx.BodyValidated(r) {
		decoder.DisallowUnknownFields()
	}

	if err := decoder.Decode(&v); err != nil {
		return nil, repository.ErrInvalid
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, repository.ErrInvalid
	}

	if len(v.Username) > 64 || len(v.Password) > 512 {
		return nil, repository.ErrInvalid
	}

	if a := attempt(r.Context()); a != nil {
		a.Username = v.Username
	}

	return v, nil
}

// jsonRenderer 满足认证库的 JSON 渲染接口。
type jsonRenderer struct{}

// Load 满足认证库渲染接口，本适配器不加载 HTML 模板。
func (jsonRenderer) Load(...string) error { return nil }

// Render 为认证库提供 JSON 渲染，具体响应由结果边界管理。
func (jsonRenderer) Render(_ context.Context, _ string, data authboss.HTMLData) ([]byte, string, error) {
	b, err := json.Marshal(data)
	return b, "application/json", err
}

// invalidResponder 将认证库失败统一映射为安全的 Result。
type invalidResponder struct{}

// Respond 统一返回安全的凭据错误，不透露账号是否存在。
func (invalidResponder) Respond(w http.ResponseWriter, r *http.Request, _ int, _ string, _ authboss.HTMLData) error {
	invalidCredentials(w, r)
	return nil
}

// invalidCredentials 区分登录失败与密码确认失败，使用已登记结果码。
func invalidCredentials(w http.ResponseWriter, r *http.Request) {
	logging.From(r.Context()).Warn("认证凭据校验未通过", "event", "auth.credentials_rejected")
	if a := attempt(r.Context()); a != nil && a.Reauthenticate {
		result.Fail(w, common.CodeInvalidCurrentPassword)
		return
	}

	result.Fail(w, common.CodeInvalidCredentials)
}

// sessionBridge 把 SCS 会话适配给认证库。
type sessionBridge struct{ sessions *Sessions }

// sessionState 只持有当前请求的会话上下文。
type sessionState struct {
	ctx      context.Context
	sessions *Sessions
}

// Get 从当前会话中读取认证库状态。
func (s sessionState) Get(key string) (string, bool) {
	value := s.sessions.Manager.GetString(s.ctx, key)
	return value, value != ""
}

// ReadState 将当前 SCS 会话适配为认证库的会话接口。
func (s sessionBridge) ReadState(r *http.Request) (authboss.ClientState, error) {
	return sessionState{ctx: r.Context(), sessions: s.sessions}, nil
}

// WriteState 把认证库状态变更写入当前 SCS 请求会话。
func (s sessionBridge) WriteState(_ http.ResponseWriter, state authboss.ClientState, events []authboss.ClientStateEvent) error {
	current, ok := state.(sessionState)
	if !ok {
		return errors.New("missing Authboss session state")
	}

	for _, event := range events {
		switch event.Kind {
		case authboss.ClientStateEventPut:
			s.sessions.Manager.Put(current.ctx, event.Key, event.Value)
		case authboss.ClientStateEventDel:
			s.sessions.Manager.Remove(current.ctx, event.Key)
		case authboss.ClientStateEventDelAll:
			whitelist := strings.Split(event.Key, ",")
			for _, key := range s.sessions.Manager.Keys(current.ctx) {
				keep := false
				for _, allowed := range whitelist {
					if key == allowed {
						keep = true
					}
				}

				if !keep {
					s.sessions.Manager.Remove(current.ctx, key)
				}
			}
		default:
			return errors.New("unknown Authboss session event")
		}
	}

	return nil
}

// CurrentAttempt 返回当前请求的认证状态。
func CurrentAttempt(ctx context.Context) *Attempt { return attempt(ctx) }
