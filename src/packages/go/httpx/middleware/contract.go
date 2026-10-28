package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/gin-gonic/gin"
	ginmiddleware "github.com/oapi-codegen/gin-middleware"

	"github.com/EayonLee/SecPlexus/src/contracts/common"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// ContractValidation 使用官方 Gin 集成验证请求，业务权限仍由 IAM 用例执行。
func ContractValidation(source []byte, limits map[string]int64) (gin.HandlerFunc, error) {
	strictJSON.Do(registerJSONDecoder)
	document, err := openapi3.NewLoader().LoadFromData(source)
	if err != nil {
		return nil, err
	}
	if err = document.Validate(context.Background()); err != nil {
		return nil, err
	}
	validator := ginmiddleware.OapiRequestValidatorWithOptions(document, &ginmiddleware.Options{
		Options: openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
		ErrorHandler: func(c *gin.Context, _ string, _ int) {
			code := common.CodeInvalidRequest
			if c.GetBool("spx.body-too-large") {
				code = common.CodePayloadTooLarge
			}
			result.Fail(c.Writer, code)
			c.Abort()
		},
	})
	return func(c *gin.Context) {
		if c.FullPath() == "" || c.FullPath() == "/health/ready" {
			c.Next()
			return
		}
		if c.Query("return_to") != "" || c.Query("redir") != "" {
			result.Fail(c.Writer, common.CodeInvalidRequest)
			c.Abort()
			return
		}
		limit := int64(32 * 1024)
		if custom, ok := limits[c.FullPath()]; ok {
			limit = custom
		}
		if c.Request.ContentLength > limit {
			result.Fail(c.Writer, common.CodePayloadTooLarge)
			c.Abort()
			return
		}
		if c.Request.Body != nil && c.Request.Body != http.NoBody && c.Request.ContentLength != 0 {
			media, _, err := mime.ParseMediaType(c.ContentType())
			if err != nil || media != "application/json" {
				result.Fail(c.Writer, common.CodeUnsupportedMediaType)
				c.Abort()
				return
			}
		}
		c.Request.Body = &boundedBody{ReadCloser: http.MaxBytesReader(c.Writer, c.Request.Body, limit), context: c}
		validator(c)
	}, nil
}

// boundedBody 保留 MaxBytesReader 的结构化溢出信息；官方校验器的错误 Hook 仅提供字符串。
type boundedBody struct {
	io.ReadCloser
	context *gin.Context
}

func (b *boundedBody) Read(data []byte) (int, error) {
	n, err := b.ReadCloser.Read(data)
	var oversized *http.MaxBytesError
	if errors.As(err, &oversized) {
		b.context.Set("spx.body-too-large", true)
	}
	return n, err
}

var strictJSON sync.Once

// registerJSONDecoder 通过官方扩展点限制为一个 JSON 值；默认解码器允许尾随第二个值。
func registerJSONDecoder() {
	openapi3filter.RegisterBodyDecoder("application/json", func(body io.Reader, _ http.Header, _ *openapi3.SchemaRef, _ openapi3filter.EncodingFn) (any, error) {
		decoder := json.NewDecoder(body)
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return nil, errors.New("request body must contain one JSON value")
		}
		return value, nil
	})
}
