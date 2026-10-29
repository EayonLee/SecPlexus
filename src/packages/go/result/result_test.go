package result

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestErrorResponse(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("X-Request-ID", "req-1")
	Fail(w, CodeInvalidRequest)
	if w.Code != http.StatusBadRequest || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("状态或缓存头不符合约定：%d %q", w.Code, w.Header().Get("Cache-Control"))
	}

	var result struct {
		Code      Code   `json:"code"`
		Message   string `json:"message"`
		Data      any    `json:"data"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}

	if result.Code != CodeInvalidRequest || result.Message == "" || result.Data != nil || result.RequestID != "req-1" {
		t.Fatalf("错误响应不符合 Result 约定：%+v", result)
	}
}

func TestSuccessResult(t *testing.T) {
	w := httptest.NewRecorder()
	Success(w, []string{})
	var result map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}

	if string(result["code"]) != `"ok"` || string(result["data"]) != `[]` || result["request_id"] == nil {
		t.Fatalf("成功响应不符合 Result 约定：%s", w.Body.String())
	}
}

func TestEncodingFailureDoesNotCommitSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	Success(w, make(chan int))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("编码失败不应返回 %d", w.Code)
	}
}

func TestDefaultAndCustomFailure(t *testing.T) {
	for _, input := range []struct {
		name    string
		write   func(http.ResponseWriter)
		status  int
		message string
	}{
		{"default", func(w http.ResponseWriter) { Fail(w) }, 500, ""},
		{"registered", func(w http.ResponseWriter) { Fail(w, CodePermissionDenied) }, 403, ""},
		{"custom message", func(w http.ResponseWriter) { FailWithMessage(w, CodeConflict, "角色仍有成员") }, 409, "角色仍有成员"},
	} {
		t.Run(input.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			input.write(w)
			var value Result[any]
			if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
				t.Fatal(err)
			}

			if w.Code != input.status || value.Data != nil || value.RequestID == "" {
				t.Fatalf("invalid failure: %s", w.Body.String())
			}

			if input.message != "" && value.Message != input.message {
				t.Fatal("custom public message was not preserved")
			}
		})
	}
}
