package handler

import (
	"net/http"

	"github.com/EayonLee/SecPlexus/src/apps/console/internal/router"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// routeHandlers 将 Console 的处理函数交给路由层。
func (s *Server) routeHandlers() router.Handlers {
	return router.Handlers{
		Ready: func(w http.ResponseWriter, r *http.Request) {
			result.Success(w, map[string]bool{"ready": true})
		},
		Context: s.context,
		Events:  s.events,
	}
}
