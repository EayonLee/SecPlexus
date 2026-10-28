// Package httpx 提供请求绑定和路由协议边界。
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// Decode 在请求绑定边界读取单个 JSON 值；字段规则由契约验证统一维护。
func Decode(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	decoder := json.NewDecoder(r.Body)
	if !BodyValidated(r) {
		decoder.DisallowUnknownFields()
	}

	if err := decoder.Decode(target); err != nil {
		return err
	}

	if BodyValidated(r) {
		return nil
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("request must contain one JSON value")
	}

	return nil
}

// validatedBodyKey 标记请求结构已通过统一 schema 校验。
type validatedBodyKey struct{}

// ValidatedBody 标记统一绑定入口已经完成 schema 校验，内部不重复验证结构规则。
func ValidatedBody(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), validatedBodyKey{}, true))
}

// BodyValidated 供认证适配器与普通绑定器遵守同一校验边界。
func BodyValidated(r *http.Request) bool {
	value, _ := r.Context().Value(validatedBodyKey{}).(bool)
	return value
}

// Handler 返回业务错误，由注册入口统一映射为失败 Result。
type Handler func(http.ResponseWriter, *http.Request) error
