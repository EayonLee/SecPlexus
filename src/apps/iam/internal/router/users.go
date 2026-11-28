package router

import (
	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/handler"
)

func registerUsers(routes gin.IRouter, server *handler.Server) {
	routes.GET("", server.ListUsers)
	routes.POST("", server.CreateUser)
	routes.GET("/:id", server.GetUser)
	routes.PATCH("/:id", server.UpdateUser)
	routes.DELETE("/:id", server.DeleteUser)
	routes.PUT("/:id/roles", server.SetUserRoles)
	routes.POST("/:id/reset-password", server.ResetUserPassword)
}
