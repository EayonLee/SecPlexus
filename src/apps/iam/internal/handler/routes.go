package handler

import (
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/handler/management"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/router"
)

// routeHandlers 将请求处理函数交给路由层，统一错误处理仍由 handler 负责。
func (s *Server) routeHandlers() router.Handlers {
	handlers := (&management.Handler{
		Service:   s.Service,
		HashSlots: s.hashSlots,
		Handle:    s.handle,
	}).Routes()
	handlers.Ready = s.handle(s.ready)
	handlers.AuthContext = s.authContext
	handlers.Login = s.handle(s.login)
	handlers.Reauthenticate = s.handle(s.reauthenticate)
	handlers.Me = s.handle(s.me)
	handlers.ChangePassword = s.handle(s.changePassword)
	handlers.Logout = s.handle(s.logout)
	handlers.LogoutAll = s.handle(s.logoutAll)
	handlers.ListSessions = s.handle(s.listSessions)
	handlers.RevokeSession = s.handle(s.revokeSession)
	handlers.AuthorizeInternal = s.handle(s.authorizeInternal)
	return handlers
}
