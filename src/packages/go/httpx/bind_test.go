package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeRejectsUnknownOrMultipleValues(t *testing.T) {
	for _, body := range []string{`{"value":"ok","unknown":1}`, `{"value":"ok"} {"value":"other"}`} {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		var value struct {
			Value string `json:"value"`
		}
		if err := Decode(httptest.NewRecorder(), r, &value); err == nil {
			t.Fatalf("应拒绝请求体 %s", body)
		}
	}
}
