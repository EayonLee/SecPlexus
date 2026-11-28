package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/channel/httpchannel"
)

// DevicePush 把设备请求交给通道的认证与报文处理边界。
func DevicePush(channel *httpchannel.Channel) gin.HandlerFunc {
	return func(c *gin.Context) {
		channel.Handle(c.Writer, c.Request, c.Param("input_id"))
	}
}
