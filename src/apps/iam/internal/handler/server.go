// Package handler 处理 IAM 认证与账号请求，管理接口由 management 子包负责。
package handler

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/EayonLee/SecPlexus/src/packages/go/logging"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authentication"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/identity"
	iammiddleware "github.com/EayonLee/SecPlexus/src/apps/iam/internal/middleware"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/router"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/service"
	"github.com/EayonLee/SecPlexus/src/contracts/common"
	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// Server 提供 IAM 的公开和内部 HTTP 接口。
type Server struct {
	Service    *service.Service                // IAM 业务用例。
	Auth       *authentication.Authenticator   // 登录认证适配器。
	Security   *iammiddleware.Security         // CSRF 和内部调用安全检查。
	Limiter    *authentication.Limiter         // 登录请求限流。
	hashSlots  chan struct{}                   // 限制同时执行的密码计算数量。
	validate   func(http.Handler) http.Handler // 启动时编译的请求契约校验。
	readyCheck func(context.Context) error     // 启动装配提供的真实依赖就绪检查。
}

// New 创建 IAM 服务并校验接口契约。
func New(service *service.Service, security *iammiddleware.Security, limiter *authentication.Limiter, readyCheck func(context.Context) error) (*Server, error) {
	validator, err := iammiddleware.ContractValidation()
	if err != nil {
		return nil, errors.New("invalid published IAM API contract")
	}

	auth, err := authentication.NewAuthenticator(service.Sessions)
	if err != nil {
		return nil, err
	}

	return &Server{
		Service:    service,
		Auth:       auth,
		Security:   security,
		Limiter:    limiter,
		hashSlots:  make(chan struct{}, 4),
		validate:   validator,
		readyCheck: readyCheck,
	}, nil
}

// handle 统一绑定路由参数并映射用例错误。
func (s *Server) handle(fn httpx.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for index, key := range chi.RouteContext(r.Context()).URLParams.Keys {
			r.SetPathValue(key, chi.RouteContext(r.Context()).URLParams.Values[index])
		}

		// 路由模板不含用户输入，向下传递用于关联业务和依赖日志。
		r = r.WithContext(logging.With(r.Context(), "route", chi.RouteContext(r.Context()).RoutePattern()))
		started := time.Now()
		if err := fn(w, r); err != nil {
			s.writeError(w, r, err)
		}
		logging.From(r.Context()).Debug("IAM 用例处理完成", "event", "iam.handled", "duration_ms", time.Since(started).Milliseconds())
	}
}

// writeError 将业务错误集中映射为公共注册表中的结果码。
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	code := common.CodeInternalError
	switch {
	case errors.Is(err, authentication.ErrUnavailable), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled), errors.Is(err, sql.ErrConnDone):
		code = common.CodeServiceUnavailable
	case errors.Is(err, identity.ErrPasswordMismatch):
		code = common.CodeInvalidCurrentPassword
	case errors.Is(err, repository.ErrUnauthenticated):
		code = common.CodeUnauthenticated
	case errors.Is(err, authorization.ErrRecentAuthRequired):
		code = common.CodeRecentAuthRequired
	case errors.Is(err, authorization.ErrDenied):
		code = common.CodePermissionDenied
	case errors.Is(err, repository.ErrInvalid):
		code = common.CodeInvalidRequest
	case errors.Is(err, repository.ErrConflict):
		code = common.CodeConflict
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, sql.ErrNoRows):
		code = common.CodeNotFound
	case errors.Is(err, authentication.ErrRateLimited):
		code = common.CodeRateLimited
		w.Header().Set("Retry-After", "60")
	default:
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) {
			switch pgerr.Code {
			case "08000", "08001", "08003", "08004", "08006", "08007", "08P01", "57P01", "57P02", "57P03":
				code = common.CodeServiceUnavailable
			case "23505", "23503":
				code = common.CodeConflict
			case "23514", "22P02":
				code = common.CodeInvalidRequest
			}
		}
	}

	var network *net.OpError
	if code == common.CodeInternalError && errors.As(err, &network) {
		code = common.CodeServiceUnavailable
	}

	level := slog.LevelWarn
	if code == common.CodeInternalError || code == common.CodeServiceUnavailable {
		level = slog.LevelError
	}
	if errors.Is(err, context.Canceled) && r.Context().Err() != nil {
		// 浏览器导航取消请求属于正常中断，不作为内部故障告警。
		level = slog.LevelDebug
	}
	logging.From(r.Context()).Log(r.Context(), level, "IAM 请求未完成", "event", "iam.rejected", "code", code, "error", logging.Error(err))

	result.Fail(w, code)
}

// requestID 读取入口生成的请求标识。
func requestID(r *http.Request) string { return middleware.RequestID(r) }

// PublicHandler 返回包含会话和安全中间件的公开接口。
func (s *Server) PublicHandler() http.Handler {
	mux := chi.NewRouter()
	router.RegisterPublic(mux, s.routeHandlers())
	return middleware.Boundary(iammiddleware.Session(s.Service.Sessions, s.clientIP)(s.Security.Protect(s.validate(mux))), mux)
}

// login 限制密码计算与来源频率后调用统一认证适配器。
func (s *Server) login(w http.ResponseWriter, r *http.Request) error {
	select {
	case s.hashSlots <- struct{}{}:
		defer func() { <-s.hashSlots }()
	default:
		return authentication.ErrRateLimited
	}

	data, err := io.ReadAll(io.LimitReader(r.Body, 2049))
	if err != nil || len(data) > 2048 {
		return repository.ErrInvalid
	}
	// 来源计数也约束无效请求；账号标识在进入 Redis 前取摘要。
	if err = s.Limiter.Check(r.Context(), s.clientIP(r), stringAccount(data)); err != nil {
		return err
	}

	r.Body = io.NopCloser(bytes.NewReader(data))
	err = s.Auth.Login(w, r)
	if err == nil && authentication.CurrentAttempt(r.Context()).Principal != nil {
		s.Security.ClearCSRF(w)
	}

	return err
}

// clientIP 仅依据明确配置的可信代理解析来源地址。
func (s *Server) clientIP(r *http.Request) string {
	prefixes := make([]netip.Prefix, 0, len(s.Service.Config.App.TrustedProxyCIDRs))
	for _, value := range s.Service.Config.App.TrustedProxyCIDRs {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
		if err == nil {
			prefixes = append(prefixes, prefix)
		}
	}

	return identity.ClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"), prefixes)
}

// reauthenticate 确认当前账号的密码并保留原会话绝对到期时间。
func (s *Server) reauthenticate(w http.ResponseWriter, r *http.Request) error {
	p, err := s.Service.Sessions.Resolve(r.Context())
	if err != nil {
		return err
	}

	var input struct {
		Password string `json:"password"`
	}
	if err = httpx.Decode(w, r, &input); err != nil {
		return repository.ErrInvalid
	}

	body, err := encodeLogin(p.Username, input.Password)
	if err != nil {
		return err
	}

	r.Body = io.NopCloser(bytes.NewReader(body))
	// 密码确认仅针对当前账号，不能借重新认证延长原会话绝对期限。
	a := authentication.CurrentAttempt(r.Context())
	a.Reauthenticate = true
	return s.login(w, r)
}

// InternalHandler 返回需要服务身份认证的内部接口。
func (s *Server) InternalHandler() http.Handler {
	mux := chi.NewRouter()
	router.RegisterInternal(mux, s.routeHandlers())
	return middleware.Boundary(s.validate(mux), mux)
}

// ready 检查 IAM 所依赖的数据库和 Redis 是否可访问。
func (s *Server) ready(w http.ResponseWriter, r *http.Request) error {
	if err := s.readyCheck(r.Context()); err != nil {
		logging.From(r.Context()).Error("IAM 依赖就绪检查失败", "event", "dependency.unready", "error", logging.Error(err))
		result.Fail(w, common.CodeServiceUnavailable)
		return nil
	}

	result.Success(w, map[string]bool{"ready": true})
	return nil
}

// authContext 返回当前会话对应的 CSRF 令牌。
func (s *Server) authContext(w http.ResponseWriter, r *http.Request) {
	result.Success(w, map[string]string{"csrf_token": csrf.Token(r)})
}

// authorizeInternal 验证调用方，并使用原始会话计算能力权限。
func (s *Server) authorizeInternal(w http.ResponseWriter, r *http.Request) error {
	var input contract.AuthorizeRequest
	if err := httpx.Decode(w, r, &input); err != nil {
		return repository.ErrInvalid
	}

	if !s.Security.InternalClient(r, input.Action) {
		return authorization.ErrDenied
	}
	// v1 只返回能力范围，不把未支持的对象上下文当作已完成对象授权。
	if input.Resource != nil {
		return repository.ErrInvalid
	}

	parsed, err := url.ParseRequestURI(input.Path)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Path == "" || input.Method == "" || len(input.Cookie) > 8192 {
		return repository.ErrInvalid
	}

	original := &http.Request{
		Method:     input.Method,
		URL:        parsed,
		Header:     make(http.Header),
		RemoteAddr: r.RemoteAddr,
	}

	original = original.WithContext(r.Context())
	original.Header.Set("Cookie", input.Cookie)
	original.Header.Set("Origin", input.Origin)
	original.Header.Set("X-CSRF-Token", input.CSRFToken)
	var failure error

	callback := iammiddleware.Session(s.Service.Sessions, s.clientIP)(s.Security.Protect(http.HandlerFunc(func(inner http.ResponseWriter, req *http.Request) {
		decision, err := s.Service.Authorize(req.Context(), input.Action, input.OrgID)
		if err != nil {
			failure = err
			return
		}

		logging.From(req.Context()).Debug("内部能力校验通过", "event", "iam.authorized", "decision_id", decision.DecisionID, "action", input.Action, "org_id", input.OrgID, "user_id", decision.Principal.UserID)
		result.Success(inner, decision)
	})))

	buffer := iammiddleware.NewBuffer()
	callback.ServeHTTP(buffer, original)
	if failure != nil {
		return failure
	}
	// 会话续期发生在服务端，内部授权响应不转发浏览器 Cookie。
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(buffer.Status)
	_, _ = w.Write(buffer.Body.Bytes())
	return nil
}
