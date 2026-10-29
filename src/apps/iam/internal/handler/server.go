// Package handler 处理 IAM 的原生 Gin 协议边界。
package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/csrf"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authentication"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authorization"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/handler/management"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/identity"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/service"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
	iammiddleware "github.com/EayonLee/SecPlexus/src/apps/iam/internal/middleware"
)

// Server 显式装配 IAM 用例与 HTTP 安全边界。
type Server struct {
	*management.Handler
	Service    *service.Service
	Auth       *authentication.Authenticator
	Security   *iammiddleware.Security
	Limiter    *authentication.Limiter
	hashSlots  chan struct{}
	ReadyCheck func(context.Context) error
}

// New 在启动时装配有界认证资源和显式 HTTP 依赖。
func New(svc *service.Service, security *iammiddleware.Security, limiter *authentication.Limiter, ReadyCheck func(context.Context) error) (*Server, error) {
	slots := make(chan struct{}, 4)
	auth, err := authentication.NewAuthenticator(svc.Sessions, limiter, slots)
	if err != nil {
		return nil, err
	}
	return &Server{Handler: &management.Handler{Service: svc, HashSlots: slots}, Service: svc, Auth: auth, Security: security, Limiter: limiter, hashSlots: slots, ReadyCheck: ReadyCheck}, nil
}

// Errors 将 Gin 收集的业务错误在响应及会话提交前统一映射。
func (s *Server) Errors(c *gin.Context) {
	c.Next()
	if len(c.Errors) > 0 && !c.Writer.Written() {
		s.writeError(c.Writer, c.Request, c.Errors.Last().Err)
	}
}

// writeError 将业务错误集中映射为公共注册表中的结果码。
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	code := result.CodeInternalError
	switch {
	case errors.Is(err, authentication.ErrUnavailable), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled), errors.Is(err, sql.ErrConnDone):
		code = result.CodeServiceUnavailable
	case errors.Is(err, authentication.ErrInvalidCredentials):
		code = result.CodeInvalidCredentials
	case errors.Is(err, identity.ErrPasswordMismatch):
		code = result.CodeInvalidCurrentPassword
	case errors.Is(err, repository.ErrUnauthenticated):
		code = result.CodeUnauthenticated
	case errors.Is(err, authorization.ErrRecentAuthRequired):
		code = result.CodeRecentAuthRequired
	case errors.Is(err, authorization.ErrDenied):
		code = result.CodePermissionDenied
	case errors.Is(err, repository.ErrInvalid):
		code = result.CodeInvalidRequest
	case errors.Is(err, repository.ErrConflict):
		code = result.CodeConflict
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, sql.ErrNoRows):
		code = result.CodeNotFound
	case errors.Is(err, authentication.ErrRateLimited):
		code = result.CodeRateLimited
		w.Header().Set("Retry-After", "1")
		var limited *authentication.RateLimitError
		if errors.As(err, &limited) {
			w.Header().Set("Retry-After", strconv.FormatInt(limited.RetryAfter, 10))
		}
	default:
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) {
			switch pgerr.Code {
			case "08000", "08001", "08003", "08004", "08006", "08007", "08P01", "57P01", "57P02", "57P03":
				code = result.CodeServiceUnavailable
			case "23505", "23503":
				code = result.CodeConflict
			case "23514", "22P02":
				code = result.CodeInvalidRequest
			}
		}
	}

	var network *net.OpError
	if code == result.CodeInternalError && errors.As(err, &network) {
		code = result.CodeServiceUnavailable
	}

	level := slog.LevelWarn
	if code == result.CodeInternalError || code == result.CodeServiceUnavailable {
		level = slog.LevelError
	}
	if errors.Is(err, context.Canceled) && r.Context().Err() != nil {
		// 浏览器导航取消请求属于正常中断，不作为内部故障告警。
		level = slog.LevelDebug
	}
	logging.From(r.Context()).Log(r.Context(), level, "IAM 请求未完成", "event", "iam.rejected", "code", code, "error", logging.Error(err))

	result.Fail(w, code)
}

// requestID 读取入口生成的关联标识。
func requestID(r *http.Request) string { return middleware.RequestID(r) }

// Login 将原生绑定后的凭据传入认证用例。
func (s *Server) Login(c *gin.Context) {
	var input model.APILoginInput
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}
	p, err := s.Auth.Login(c.Request.Context(), authentication.Credentials{Username: input.Username, Password: input.Password}, s.metadata(c))
	if err != nil {
		httpx.Reject(c, err)
		return
	}
	s.authenticated(c, p, false)
}

// Reauthenticate 仅校验当前账号，不重新构造 HTTP 登录请求。
func (s *Server) Reauthenticate(c *gin.Context) {
	var input model.APIReauthenticateInput
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}
	p, err := s.Auth.Reauthenticate(c.Request.Context(), input.Password, s.metadata(c))
	if err != nil {
		httpx.Reject(c, err)
		return
	}
	s.authenticated(c, p, true)
}

func (s *Server) metadata(c *gin.Context) authentication.RequestMetadata {
	return authentication.RequestMetadata{RequestID: requestID(c.Request), ClientIP: c.ClientIP(), UserAgent: c.Request.UserAgent()}
}

func (s *Server) authenticated(c *gin.Context, p iamclient.Principal, reauth bool) {
	a := authentication.CurrentAttempt(c.Request.Context())
	a.Principal = &p
	a.Reauthenticate = reauth
	s.Security.ClearCSRF(c.Writer)
	result.Success(c.Writer, model.APILoginResponse{Principal: p})
}

// Ready 检查真实依赖，不触发迁移或初始化。
func (s *Server) Ready(c *gin.Context) {
	if err := s.ReadyCheck(c.Request.Context()); err != nil {
		httpx.Reject(c, authentication.ErrUnavailable)
		return
	}
	result.Success(c.Writer, map[string]bool{"ready": true})
}

// AuthContext 返回当前浏览器的 CSRF 令牌。
func (s *Server) AuthContext(c *gin.Context) {
	result.Success(c.Writer, map[string]string{"csrf_token": csrf.Token(c.Request)})
}

// AuthorizeInternal 先确认服务身份，再按转发的原始会话执行权限检查。
func (s *Server) AuthorizeInternal(c *gin.Context) {
	r := c.Request
	var input iamclient.AuthorizeRequest
	if err := httpx.Bind(c, &input); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}
	if !s.Security.InternalClient(r, input.Action) {
		httpx.Reject(c, authorization.ErrDenied)
		return
	}
	if input.Resource != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}
	parsed, err := url.ParseRequestURI(input.Path)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Path == "" || input.Method == "" || len(input.Cookie) > 8192 {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}
	original := (&http.Request{Method: input.Method, URL: parsed, Header: make(http.Header), RemoteAddr: r.RemoteAddr}).WithContext(r.Context())
	original.Header.Set("Cookie", input.Cookie)
	original.Header.Set("Origin", input.Origin)
	original.Header.Set("X-CSRF-Token", input.CSRFToken)
	var decision iamclient.CapabilityDecision
	var failure error
	buffer := iammiddleware.NewBuffer()
	s.Security.Protect(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		var token string
		if cookie, err := req.Cookie(s.Service.Sessions.Manager.Cookie.Name); err == nil {
			token = cookie.Value
		}
		loaded, err := s.Service.Sessions.Manager.Load(req.Context(), token)
		if err != nil {
			failure = authentication.ErrUnavailable
			return
		}
		decision, failure = s.Service.Authorize(loaded, input.Action, input.OrgID)
	})).ServeHTTP(buffer, original)
	if failure != nil {
		httpx.Reject(c, failure)
		return
	}
	if buffer.Status >= 400 {
		// 保留成熟CSRF组件的公开错误码；仅此HTTP适配边界解开它自己的错误响应。
		var rejected result.Result[any]
		if buffer.Status != http.StatusForbidden || json.Unmarshal(buffer.Body.Bytes(), &rejected) != nil {
			httpx.Reject(c, errors.New("invalid security component response"))
			return
		}
		result.Fail(c.Writer, rejected.Code)
		return
	}
	result.Success(c.Writer, decision)
}
