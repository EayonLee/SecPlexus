package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/gorilla/csrf"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/config"
	"github.com/EayonLee/SecPlexus/src/contracts/common"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// Security 管理公开接口的 CSRF 防护及内部服务认证。
type Security struct {
	host    string                          // 安全读取请求使用的配置主机。
	scheme  string                          // 公开入口协议，决定 CSRF 与 Cookie 的 TLS 策略。
	protect func(http.Handler) http.Handler // 真实 CSRF Cookie 和令牌校验器。
	origins map[string]string               // 允许的完整来源及其主机，不接受任意转发地址。
	secure  bool                            // 公开入口是否要求 HTTPS。
	clients []InternalClient                // 内部调用方的凭证摘要与权限。
}

// InternalClient 保存内部调用方的凭证摘要和允许的操作。
type InternalClient struct {
	ID        string
	TokenHash string
	Actions   []string
}

// NewSecurity 根据配置初始化安全检查和内部调用方。
func NewSecurity(cfg config.Config) (*Security, error) {
	key := cfg.Auth.CSRFKey
	if len(key) != 32 {
		return nil, errors.New("CSRF signing key must contain exactly 32 bytes")
	}

	origin := strings.TrimSuffix(cfg.App.PublicURL, "/")
	parsed, err := url.Parse(origin)
	if err != nil {
		return nil, err
	}

	security := &Security{
		origins: map[string]string{origin: parsed.Host},
		secure:  parsed.Scheme == "https",
		host:    parsed.Host,
		scheme:  parsed.Scheme,
	}
	// 本机入口接受明确的回环别名，但协议和端口仍来自同一公开配置。
	switch strings.ToLower(parsed.Hostname()) {
	case "localhost", "127.0.0.1", "::1":
		for _, host := range []string{"localhost", "127.0.0.1", "[::1]"} {
			if port := parsed.Port(); port != "" {
				host += ":" + port
			}
			security.origins[parsed.Scheme+"://"+host] = host
		}
	}

	security.protect = csrf.Protect(
		[]byte(key),
		csrf.Secure(security.secure),
		csrf.HttpOnly(true),
		csrf.SameSite(csrf.SameSiteLaxMode),
		csrf.Path("/"),
		csrf.CookieName("spx_csrf"),
		csrf.RequestHeader("X-CSRF-Token"),
		csrf.ErrorHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logging.From(r.Context()).Warn("CSRF 校验拒绝请求", "event", "security.csrf_rejected")
			result.Fail(w, common.CodeCSRFInvalid)
		})),
	)
	for id, client := range cfg.Internal.Clients {
		if id == "" || len(client.Token) < 32 || len(client.Token) > 512 || len(client.Actions) == 0 {
			return nil, errors.New("internal clients require a unique ID, 32–512 byte token and allowed actions")
		}

		digest := sha256.Sum256([]byte(client.Token))
		security.clients = append(security.clients, InternalClient{
			ID:        id,
			TokenHash: hex.EncodeToString(digest[:]),
			Actions:   client.Actions,
		})
	}

	if len(security.clients) == 0 {
		return nil, errors.New("at least one internal client is required")
	}

	return security, nil
}

// Protect 校验请求来源并应用 CSRF 防护。
func (s *Security) Protect(next http.Handler) http.Handler {
	protected := s.protect(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := s.host
		if !safeMethod(r.Method) {
			allowedHost, ok := s.origins[r.Header.Get("Origin")]
			if !ok {
				logging.From(r.Context()).Warn("请求来源不符合配置", "event", "security.origin_rejected")
				result.Fail(w, common.CodeOriginInvalid)
				return
			}
			host = allowedHost
		}

		// CSRF 按本次已允许的来源校验；不能改写 Origin 或信任代理转发头来绕过。
		r = r.Clone(r.Context())
		r.Host = host
		r.URL.Scheme = s.scheme
		r.URL.Host = host
		// Cookie 的 TLS 策略依据公开访问地址，不依赖代理到服务的本地链路。
		if !s.secure {
			r = csrf.PlaintextHTTPRequest(r)
		}

		protected.ServeHTTP(w, r)
	})
}

// InternalClient 验证 TLS、调用凭证和调用方允许的操作。
func (s *Security) InternalClient(r *http.Request, action string) bool {
	if r.TLS == nil {
		logging.From(r.Context()).Warn("内部调用必须使用 TLS", "event", "security.client_rejected", "reason", "tls_required")
		return false
	}

	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || len(token) < 32 || len(token) > 512 {
		logging.From(r.Context()).Warn("内部调用凭证格式不符合要求", "event", "security.client_rejected", "reason", "credential_invalid")
		return false
	}

	digest := sha256.Sum256([]byte(token))
	for _, client := range s.clients {
		expected, err := hex.DecodeString(client.TokenHash)
		if err == nil && subtle.ConstantTimeCompare(digest[:], expected) == 1 {
			allowed := slices.Contains(client.Actions, action)
			if !allowed {
				logging.From(r.Context()).Warn("内部调用方无权执行目标能力", "event", "security.client_action_rejected", "client_id", client.ID, "action", action)
			}
			return allowed
		}
	}

	logging.From(r.Context()).Warn("内部调用凭证未匹配", "event", "security.client_rejected", "reason", "credential_unknown")
	return false
}

// ClearCSRF 清除浏览器中的 CSRF Cookie。
func (s *Security) ClearCSRF(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "spx_csrf",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// safeMethod 判断不应产生业务写入的 HTTP 方法。
func safeMethod(method string) bool {
	return method == "GET" || method == "HEAD" || method == "OPTIONS"
}

// GinProtect 只适配成熟 CSRF 组件，业务处理保持原生 Gin 调用链。
func (s *Security) GinProtect() gin.HandlerFunc {
	return func(c *gin.Context) {
		invoked := false
		s.Protect(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			invoked = true
			c.Request = r
			c.Next()
		})).ServeHTTP(c.Writer, c.Request)
		if !invoked {
			c.Abort()
		}
	}
}
