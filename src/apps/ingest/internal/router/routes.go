// Package router 用原生 Gin 注册 Ingest 路由及其输入与认证边界。
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx/middleware"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// Deps 提供各业务路由所需的处理器和鉴权客户端。
type Deps struct {
	EventRead   gin.HandlerFunc      // 事件回查处理器。
	Push        gin.HandlerFunc      // 设备推送处理器，拥有独立的认证、体积和媒体类型规则。
	Admin       *handler.Admin       // 管理面处理器；为空时不注册管理接口。
	EventSchema *handler.EventSchema // 事件字段目录处理器；为空时不注册目录接口。
	IAM         *iamclient.Client    // 管理面鉴权与审计客户端。
}

// Handler 在同一个 Gin 引擎中注册设备、管理和探针路由。
func Handler(server *handler.Server, deps Deps) http.Handler {
	engine := middleware.NewEngine()
	engine.GET("/health/ready", server.ReadyHandler)
	// 指标端点由反向代理限制抓取来源。
	inputBoundary := middleware.InputBoundary(nil)
	engine.GET("/metrics", inputBoundary, server.Metrics)

	api := engine.Group("/api/v1/ingest")
	if deps.Push != nil {
		api.POST("/events/:input_id", deps.Push)
	}
	if deps.EventRead != nil && deps.IAM != nil {
		api.GET("/events/:event_id", inputBoundary, middleware.Guard(deps.IAM, iamclient.PermissionIngestEventsRead, "event"), deps.EventRead)
	}
	if deps.Admin != nil && deps.IAM != nil {
		management := api.Group("", inputBoundary)
		registerInputs(management.Group("/inputs"), deps.Admin, deps.IAM)
		registerMappings(management.Group("/parsing-rules"), deps.Admin, deps.IAM)
		if deps.EventSchema != nil {
			schema := management.Group("/event-schema", middleware.Guard(deps.IAM, iamclient.PermissionIngestParsingRuleRead, "mapping"))
			registerEventSchema(schema, deps.EventSchema)
		}
	}

	return engine
}
