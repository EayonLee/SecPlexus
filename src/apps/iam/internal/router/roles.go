package router

import (
	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/handler"
)

func registerRoles(routes gin.IRouter, server *handler.Server) {
	routes.GET("", server.ListRoles)
	routes.POST("", server.CreateRole)
	routes.GET("/:id", server.GetRole)
	routes.PATCH("/:id", server.UpdateRole)
	routes.DELETE("/:id", server.DeleteRole)
	routes.PUT("/:id/permissions", server.SetRolePermissions)
}
