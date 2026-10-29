package httpx

import (
	"errors"
	"io"
	"net/http"
	"reflect"

	"github.com/aarondl/opt/omit"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	jsoniter "github.com/json-iterator/go"

	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// init 在请求处理前注册可选字段，避免首次并发请求修改验证器。
func init() {
	engine := binding.Validator.Engine().(*validator.Validate)
	engine.RegisterCustomTypeFunc(func(field reflect.Value) any {
		value, present := field.Interface().(omit.Val[string]).Get()
		if !present {
			return nil
		}
		return value
	}, omit.Val[string]{})
}

// Bind 在 Gin 正式绑定扩展点单次解码 JSON，并保留严格对象和单值边界。
func Bind(c *gin.Context, target any) error {
	err := c.ShouldBindWith(target, strictJSON{})
	var oversized *http.MaxBytesError
	if errors.As(err, &oversized) || c.GetBool("spx.body-too-large") {
		result.Fail(c.Writer, result.CodePayloadTooLarge)
		c.Abort()
	}
	return err
}

var requestJSON = jsoniter.Config{CaseSensitive: true, DisallowUnknownFields: true, UseNumber: true}.Froze()

type strictJSON struct{}

func (strictJSON) Name() string { return "json" }

func (strictJSON) Bind(request *http.Request, target any) error {
	decoder := requestJSON.NewDecoder(request.Body)
	if err := decoder.Decode(&target); err != nil {
		return err
	}
	if target == nil {
		return errors.New("request must be a JSON object")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return err
		}
		return errors.New("request must contain exactly one JSON value")
	}
	return binding.Validator.ValidateStruct(target)
}

// Reject 交给 Gin 统一错误中间件输出，终止余下处理。
func Reject(c *gin.Context, err error) { _ = c.Error(err); c.Abort() }
