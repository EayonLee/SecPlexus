package httpx

import (
	"github.com/gin-gonic/gin"
	"github.com/oapi-codegen/runtime"
)

// Bind 复用 OpenAPI 已验证的值，避免再次解析或维护另一套请求规则。
func Bind(c *gin.Context, target any) error {
	if value, ok := c.Request.Body.(interface{ ValidatedValue() any }); ok {
		return runtime.BindJSONValue(value.ValidatedValue(), target)
	}
	return c.ShouldBindJSON(target)
}

// Reject 交给 Gin 统一错误中间件输出，终止余下处理。
func Reject(c *gin.Context, err error) { _ = c.Error(err); c.Abort() }
