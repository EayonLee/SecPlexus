package router

import (
	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/handler"
)

func registerAuth(routes gin.IRouter, server *handler.Server) {
	routes.GET("/context", server.AuthContext)
	routes.POST("/login", server.Login)
	routes.POST("/reauthenticate", server.Reauthenticate)
	routes.POST("/logout", server.Logout)
}

func registerMe(routes gin.IRouter, server *handler.Server) {
	routes.GET("", server.Me)
	routes.PATCH("", server.UpdateProfile)
	routes.POST("/password", server.ChangePassword)
}
