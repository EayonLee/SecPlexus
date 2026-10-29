package middleware

import (
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// InputBoundary 只保护 HTTP 体积、媒体和路径边界，字段规则由原生 DTO 绑定拥有。
func InputBoundary(limits map[string]int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.FullPath() == "" || c.FullPath() == "/health/ready" {
			c.Next()
			return
		}
		var path struct {
			ID    string `uri:"id" binding:"omitempty,uuid"`
			OrgID string `uri:"org_id" binding:"omitempty,uuid"`
		}
		if err := c.ShouldBindUri(&path); err != nil || c.Query("return_to") != "" || c.Query("redir") != "" {
			result.Fail(c.Writer, result.CodeInvalidRequest)
			c.Abort()
			return
		}
		limit := int64(32 * 1024)
		if custom, ok := limits[c.FullPath()]; ok {
			limit = custom
		}
		if c.Request.ContentLength > limit {
			result.Fail(c.Writer, result.CodePayloadTooLarge)
			c.Abort()
			return
		}
		if c.Request.Body != nil && c.Request.Body != http.NoBody && c.Request.ContentLength != 0 {
			media, _, err := mime.ParseMediaType(c.ContentType())
			if err != nil || media != "application/json" {
				result.Fail(c.Writer, result.CodeUnsupportedMediaType)
				c.Abort()
				return
			}
		}
		if c.Request.Body != nil {
			c.Request.Body = &boundedBody{ReadCloser: http.MaxBytesReader(c.Writer, c.Request.Body, limit), context: c}
		}
		c.Next()
	}
}

// boundedBody 保留流式 JSON 解码器可能包装掉的结构化超限信息。
type boundedBody struct {
	io.ReadCloser
	context *gin.Context
}

func (body *boundedBody) Read(data []byte) (int, error) {
	n, err := body.ReadCloser.Read(data)
	var oversized *http.MaxBytesError
	if errors.As(err, &oversized) {
		body.context.Set("spx.body-too-large", true)
	}
	return n, err
}
