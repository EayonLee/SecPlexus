package router

import (
	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

func registerMappings(routes gin.IRouter, admin *handler.Admin, iam *iamclient.Client) {
	preview := middleware.Guard(iam, iamclient.PermissionIngestParsingRuleWrite, "mapping")
	routes.POST("/validate", preview, admin.ValidateMapping)
	routes.POST("/extract", preview, admin.ExtractMapping)
	read := middleware.Guard(iam, iamclient.PermissionIngestParsingRuleRead, "mapping")
	routes.GET("", read, admin.ListMappings)
	routes.GET("/:id", read, admin.ReadMapping)
	routes.GET("/:id/revisions", read, admin.ListMappingRevisions)
	routes.GET("/:id/revisions/:version", read, admin.ReadMappingRevision)
	write := middleware.GuardWithDurableAudit(iam, iamclient.PermissionIngestParsingRuleWrite, "mapping")
	routes.POST("", write, admin.CreateMapping)
	routes.PATCH("/:id", write, admin.UpdateMapping)
	routes.POST("/:id/revisions/:version/restore", write, admin.RestoreMappingRevision)
	routes.DELETE("/:id", write, admin.DeleteMapping)
}

func registerEventSchema(routes gin.IRouter, schema *handler.EventSchema) {
	routes.GET("", schema.List)
	routes.GET("/classes/:class_uid", schema.GetClass)
	routes.GET("/fields", schema.Fields)
}
