package middleware

import (
	"net/http"
)

// requestKey 共享 HTTP 请求标识的上下文键。
type requestKey struct{}

// RequestID 返回入口生成的请求标识，不直接信任外部请求头。
func RequestID(r *http.Request) string {
	id, _ := r.Context().Value(requestKey{}).(string)
	return id
}
