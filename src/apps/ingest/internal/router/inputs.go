package router

import (
	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

func registerInputs(routes gin.IRouter, admin *handler.Admin, iam *iamclient.Client) {
	read := middleware.Guard(iam, iamclient.PermissionIngestInputRead, "input")
	routes.GET("", read, admin.ListInputs)
	routes.GET("/counts-by-rule", read, admin.MappingInputCounts)
	routes.GET("/:id", read, admin.ReadInput)
	write := middleware.GuardWithDurableAudit(iam, iamclient.PermissionIngestInputWrite, "input")
	routes.POST("", write, admin.CreateInput)
	routes.PATCH("/:id", write, admin.UpdateInput)
	routes.DELETE("/:id", write, admin.DeleteInput)
}
