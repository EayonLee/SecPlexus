package middleware

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/EayonLee/SecPlexus/src/contracts/common"
	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// ContractValidation 在 CSRF 校验后验证请求结构；身份与权限仍由用例校验。
func ContractValidation() (func(http.Handler) http.Handler, error) {
	loader := openapi3.NewLoader()
	document, err := loader.LoadFromData(contract.OpenAPI)
	if err != nil {
		return nil, err
	}

	if err = document.Validate(context.Background()); err != nil {
		return nil, err
	}

	router, err := legacy.NewRouter(document)
	if err != nil {
		return nil, err
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/v1/iam/") && !strings.HasPrefix(r.URL.Path, "/internal/v1/iam/") {
				next.ServeHTTP(w, r)
				return
			}

			if r.URL.Query().Has("return_to") || r.URL.Query().Has("redir") {
				result.Fail(w, common.CodeInvalidRequest)
				return
			}

			route, pathParams, err := router.FindRoute(r)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			if route.Operation.RequestBody != nil {
				media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if err != nil || media != "application/json" {
					result.Fail(w, common.CodeUnsupportedMediaType)
					return
				}
			}

			r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
			input := &openapi3filter.RequestValidationInput{
				Request:    r,
				PathParams: pathParams,
				Route:      route,
				Options:    &openapi3filter.Options{AuthenticationFunc: func(context.Context, *openapi3filter.AuthenticationInput) error { return nil }},
			}
			if err = openapi3filter.ValidateRequest(r.Context(), input); err != nil {
				// 校验器错误可能包含请求值，不直接写入响应或日志。
				code := common.CodeInvalidRequest
				var oversized *http.MaxBytesError
				if errors.As(err, &oversized) {
					code = common.CodePayloadTooLarge
				}

				logging.From(r.Context()).Warn("请求不符合接口契约", "event", "http.validation_failed", "operation", route.Operation.OperationID, "code", code)
				result.Fail(w, code)
				return
			}

			next.ServeHTTP(w, httpx.ValidatedBody(r))
		})
	}, nil
}
