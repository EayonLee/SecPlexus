// Package result 按公共契约输出成功与失败结果。
package result

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin/render"
	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// Success 使用默认成功码和消息返回业务数据。
func Success(w http.ResponseWriter, data any) { success(w, http.StatusOK, data) }

// Created 返回资源创建成功结果。
func Created(w http.ResponseWriter, data any) { success(w, http.StatusCreated, data) }

// NoContent 返回 204，不发送 JSON 结果体。
func NoContent(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

// success 构造默认成功结果，状态由公开成功入口确定。
func success(w http.ResponseWriter, status int, data any) {
	definition, _ := Lookup(CodeOK)
	write(w, status, Result[any]{
		Code:      CodeOK,
		Message:   definition.Message,
		Data:      data,
		RequestID: responseID(w),
	})
}

// Fail 默认返回内部错误；指定已登记的结果码时自动匹配状态与公开消息。
func Fail(w http.ResponseWriter, codes ...Code) {
	code := CodeInternalError
	if len(codes) == 1 {
		code = codes[0]
	}

	FailWithMessage(w, code, "")
}

// FailWithMessage 仅在确需特定公开提示时覆盖默认消息，状态仍由结果码决定。
// 禁止传入原始数据库错误、凭据或内部地址。
func FailWithMessage(w http.ResponseWriter, code Code, message string) {
	definition, ok := Lookup(code)
	if !ok || code == CodeOK {
		code = CodeInternalError
		definition, _ = Lookup(code)
		message = ""
	}

	if message == "" {
		message = definition.Message
	}

	write(w, definition.Status, Result[any]{
		Code:      code,
		Message:   message,
		Data:      nil,
		RequestID: responseID(w),
	})
}

// responseID 复用中间件请求标识，独立调用时生成新标识。
func responseID(w http.ResponseWriter) string {
	id := w.Header().Get("X-Request-ID")
	if id == "" {
		id = uuid.NewString()
	}

	w.Header().Set("X-Request-ID", id)
	return id
}

// write 编码成功后再提交状态，编码失败转为内部错误结果。
func write(w http.ResponseWriter, status int, value Result[any]) {
	data, err := json.Marshal(value)
	if err != nil {
		slog.Error("响应数据编码失败", "event", "http.encode_failed", "request_id", value.RequestID, "error", logging.Error(err), "source", logging.Stack())
		definition, _ := Lookup(CodeInternalError)
		status = definition.Status
		// 此分支只包含已知字符串与 nil，不能再次触发业务值的编码错误。
		data, _ = json.Marshal(Result[any]{
			Code:      CodeInternalError,
			Message:   definition.Message,
			Data:      nil,
			RequestID: value.RequestID,
		})
	}

	encoded := render.Data{ContentType: "application/json; charset=utf-8", Data: data}
	encoded.WriteContentType(w)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := encoded.Render(w); err != nil {
		slog.Warn("响应已提交但发送失败", "event", "http.write_failed", "request_id", value.RequestID, "error", logging.Error(err))
	}
}
