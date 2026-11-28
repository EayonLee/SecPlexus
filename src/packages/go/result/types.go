package result

// Result 是普通 JSON API 的统一结果，data 的业务类型由具体接口确定。
type Result[T any] struct {
	Code        Code         `json:"code"`
	Message     string       `json:"message"`
	Data        T            `json:"data"`
	RequestID   string       `json:"request_id"`
	FieldErrors []FieldError `json:"field_errors,omitempty"`
}

// FieldError 定位可纠正的请求字段，仅包含稳定错误码及长度上限。
type FieldError struct {
	Field string `json:"field"`
	Code  string `json:"code"`
	Limit int    `json:"limit,omitempty"`
}
