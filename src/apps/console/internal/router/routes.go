// Package router 集中定义 Console 的 HTTP 路由。
package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
)

// Handlers 包含 Console 路由需要的请求处理函数。
type Handlers struct {
	Ready   http.HandlerFunc // 就绪检查。
	Context http.HandlerFunc // 获取当前组织上下文。
	Events  http.HandlerFunc // 订阅组织事件。
}

// Register 注册 Console 的公开接口。
func Register(mux chi.Router, h Handlers) {
	mux = mux.With(middleware.CaptureRoute)
	httpx.RouterErrors(mux)
	mux.MethodFunc("GET", "/health/ready", h.Ready)
	mux.Route("/api/v1/console", func(r chi.Router) {
		r.MethodFunc("GET", "/orgs/{org_id}/context", h.Context)
		r.MethodFunc("GET", "/orgs/{org_id}/events", h.Events)
	})
}
