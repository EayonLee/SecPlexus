package middleware

import (
	"bytes"
	"context"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authentication"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	httpmiddleware "github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// Session 等待 SCS 会话持久化完成后再发送响应；
// 持久化失败时不能向浏览器发送成功响应或认证 Cookie。
func Session(sessions *authentication.Sessions) gin.HandlerFunc {
	return func(c *gin.Context) {
		w, r := c.Writer, c.Request
		defer func() { c.Writer = w }()
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		id := httpmiddleware.RequestID(r)
		if id == "" {
			id = uuid.NewString()
		}

		w.Header().Set("X-Request-ID", id)
		var token string
		if cookie, err := r.Cookie(sessions.Manager.Cookie.Name); err == nil {
			token = cookie.Value
		}

		loaded, err := sessions.Manager.Load(ctx, token)
		if err != nil {
			logging.From(ctx).Error("Could not load session data", "event", "session.load_failed", "error", logging.Error(err))
			result.Fail(w, result.CodeServiceUnavailable)
			c.Abort()
			return
		}

		a := authentication.NewAttempt()
		committed := false
		defer func() {
			// 逻辑会话已落库但响应/缓存未提交时，包括 panic 路径，必须撤销该次新会话。
			if principal, _ := a.Session(); principal != nil && !committed {
				cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				if err := repository.CompensateSession(cleanup, sessions.Store.DB, principal.SessionID); err != nil {
					logging.From(loaded).Error("Could not roll back an uncommitted session", "event", "session.compensation_failed", "session_id", principal.SessionID, "error", logging.Error(err))
				}
			}
		}()
		loaded = authentication.WithAttempt(loaded, a)
		r = r.WithContext(loaded)
		buffer := NewBuffer()
		buffer.Header().Set("X-Request-ID", id)
		c.Request = r
		c.Writer = &sessionWriter{ResponseWriter: w, buffer: buffer}
		c.Next()
		c.Writer = w
		manager := sessions.Manager
		if buffer.Status < 400 {
			switch manager.Status(loaded) {
			case scs.Modified:
				token, expiry, err := manager.Commit(loaded)
				if err != nil {
					logging.From(loaded).Error("Could not save session; success response refused", "event", "session.commit_failed", "error", logging.Error(err))

					result.Fail(w, result.CodeServiceUnavailable)
					return
				}
				committed = true
				manager.WriteSessionCookie(loaded, buffer, token, expiry)
				if principal, reauthenticated := a.Session(); principal != nil {
					event := "auth.login_succeeded"
					if reauthenticated {
						event = "auth.reauthenticated"
					}
					logging.From(loaded).Info("Session saved", "event", event, "user_id", principal.UserID, "session_id", principal.SessionID)
				}
			case scs.Destroyed:
				manager.WriteSessionCookie(loaded, buffer, "", time.Time{})
			}
		}

		for key, values := range buffer.header {
			w.Header()[key] = values
		}

		w.Header().Set("X-Request-ID", id)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(buffer.Status)
		_, _ = w.Write(buffer.Body.Bytes())
	}
}

// Buffer 暂存 IAM 的非流式响应，直到会话状态成功提交。
type Buffer struct {
	header  http.Header
	Status  int // 最终 HTTP 状态。
	written bool
	Body    bytes.Buffer // 已编码且尚未提交的普通响应。
}

// Header 返回待提交响应头。
func (b *Buffer) Header() http.Header { return b.header }

// WriteHeader 只记录第一次状态提交。
func (b *Buffer) WriteHeader(status int) {
	if !b.written {
		b.Status = status
		b.written = true
	}
}

// Write 暂存响应体，等待会话存储完成。
func (b *Buffer) Write(data []byte) (int, error) {
	if !b.written {
		b.WriteHeader(200)
	}

	return b.Body.Write(data)
}

// NewBuffer 创建 IAM 普通响应的临时缓冲。
func NewBuffer() *Buffer {
	return &Buffer{header: make(http.Header),
		Status: http.StatusOK}
}

// sessionWriter 仅在 IAM 非流式请求内暂存响应；Gin 保留原生路由和异常边界。
type sessionWriter struct {
	gin.ResponseWriter
	buffer *Buffer
}

func (w *sessionWriter) Header() http.Header                  { return w.buffer.Header() }
func (w *sessionWriter) WriteHeader(status int)               { w.buffer.WriteHeader(status) }
func (w *sessionWriter) Write(data []byte) (int, error)       { return w.buffer.Write(data) }
func (w *sessionWriter) WriteString(data string) (int, error) { return w.buffer.Write([]byte(data)) }
func (w *sessionWriter) Status() int                          { return w.buffer.Status }
func (w *sessionWriter) Size() int                            { return w.buffer.Body.Len() }
func (w *sessionWriter) Written() bool                        { return w.buffer.written }
func (w *sessionWriter) WriteHeaderNow()                      { w.buffer.WriteHeader(w.buffer.Status) }
