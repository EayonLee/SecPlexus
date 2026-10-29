package middleware

import (
	"bytes"
	"context"
	"net/http"
	"time"

	"github.com/EayonLee/SecPlexus/src/packages/go/logging"

	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/authentication"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/contracts/common"
	httpmiddleware "github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
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
			logging.From(ctx).Error("会话数据加载失败", "event", "session.load_failed", "error", logging.Error(err))
			result.Fail(w, common.CodeServiceUnavailable)
			c.Abort()
			return
		}

		a := &authentication.Attempt{RequestID: id}
		committed := false
		defer func() {
			// 逻辑会话已落库但响应/缓存未提交时，包括 panic 路径，必须撤销该次新会话。
			if a.Principal != nil && !committed {
				cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				if err := repository.CompensateSession(cleanup, sessions.Store.DB, a.Principal.SessionID); err != nil {
					logging.From(loaded).Error("未提交会话补偿失败", "event", "session.compensation_failed", "session_id", a.Principal.SessionID, "error", logging.Error(err))
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
					logging.From(loaded).Error("会话保存失败，拒绝发送成功响应", "event", "session.commit_failed", "error", logging.Error(err))

					result.Fail(w, common.CodeServiceUnavailable)
					return
				}
				committed = true
				manager.WriteSessionCookie(loaded, buffer, token, expiry)
				if a.Principal != nil {
					event := "auth.login_succeeded"
					if a.Reauthenticate {
						event = "auth.reauthenticated"
					}
					logging.From(loaded).Info("认证会话已持久化", "event", event, "user_id", a.Principal.UserID, "session_id", a.Principal.SessionID)
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
