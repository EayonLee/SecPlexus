package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/contracts/common"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// requestKey 共享 HTTP 请求标识的上下文键。
type requestKey struct{}

// requestState 仅记录入口标识和匹配后的路由，不保存原始 URL 或查询值。
type requestState struct{ id, route string }

// RequestID 返回入口生成的请求标识，不直接信任外部请求头。
func RequestID(r *http.Request) string {
	state, _ := r.Context().Value(requestKey{}).(*requestState)
	if state == nil {
		return ""
	}
	return state.id
}

// Boundary 统一请求标识、异常恢复和访问日志，保留流式响应能力。
func Boundary(next http.Handler, routers ...chi.Router) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.NewString()
		state := &requestState{id: id, route: "unmatched"}
		// 只匹配路由、不执行处理器；CSRF/会话拒绝发生在 mux 之前也能定位接口。
		for _, router := range routers {
			route := chi.NewRouteContext()
			if router.Match(route, r.Method, r.URL.Path) {
				state.route = route.RoutePattern()
				break
			}
		}
		ctx := logging.With(r.Context(), "request_id", id, "method", r.Method)
		r = r.WithContext(context.WithValue(ctx, requestKey{}, state))
		w.Header().Set("X-Request-ID", id)
		writer := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		started := time.Now()
		defer func() {
			if failure := recover(); failure != nil {
				logging.From(r.Context()).Error("HTTP 请求异常", "event", "http.panic", "panic_kind", fmt.Sprintf("%T", failure), "stack", logging.Stack())
				if writer.Status() == 0 {
					result.Fail(writer, common.CodeInternalError)
				}
			}

			status := writer.Status()
			if status == 0 {
				status = http.StatusOK
			}
			level := slog.LevelInfo
			if state.route == "/health/ready" && status < 400 {
				level = slog.LevelDebug
			}
			logging.From(r.Context()).Log(
				r.Context(),
				level,
				"HTTP 请求完成",
				"event", "http.completed",
				"route", state.route,
				"status", status,
				"bytes", writer.BytesWritten(),
				"duration_ms", time.Since(started).Milliseconds(),
			)
		}()

		next.ServeHTTP(writer, r)
	})
}

// CaptureRoute 由真实路由器提供规范路径；未知路径不把用户输入原样写入日志。
func CaptureRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if state, ok := r.Context().Value(requestKey{}).(*requestState); ok {
				if pattern := chi.RouteContext(r.Context()).RoutePattern(); pattern != "" {
					state.route = pattern
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}
