package router_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/router"
)

// postValidate 发一次样本验证请求。
func postValidate(t *testing.T, server *httptest.Server, payload string) *http.Response {
	t.Helper()

	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/ingest/mappings/validate", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", "csrf-token")

	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = response.Body.Close() })

	return response
}

// 回归：能解析的样本要计入命中数，不能解析的样本要给出原因。
func TestValidateMappingReportsPerSampleResults(t *testing.T) {
	client := fakeIAM(t, http.StatusOK)
	server := httptest.NewServer(router.Handler(&handler.Server{}, router.Deps{Admin: &fakeStore{}, IAM: client}))
	defer server.Close()

	payload := `{"definition":{"id":"r","name":"r","parse":{"mode":"json"},"field_map":{"src":"src_endpoint.ip"},"constants":{"class_uid":2004}},
	  "samples":["{\"src\":\"10.0.0.1\"}","not json at all"]}`

	response := postValidate(t, server, payload)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("状态 = %d", response.StatusCode)
	}

	var body struct {
		Data struct {
			Compiled bool `json:"compiled"`
			Total    int  `json:"total"`
			Matched  int  `json:"matched"`
			Failures []struct {
				Index  int    `json:"index"`
				Reason string `json:"reason"`
			} `json:"failures"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	if !body.Data.Compiled || body.Data.Total != 2 || body.Data.Matched != 1 {
		t.Fatalf("结果 = %+v，期望 2 条样本命中 1 条", body.Data)
	}
	if len(body.Data.Failures) != 1 || body.Data.Failures[0].Index != 1 || body.Data.Failures[0].Reason == "" {
		t.Fatalf("失败明细 = %+v", body.Data.Failures)
	}
}

// 编译不过的规则要返回原因，而不是 500。
func TestValidateMappingReportsCompileError(t *testing.T) {
	client := fakeIAM(t, http.StatusOK)
	server := httptest.NewServer(router.Handler(&handler.Server{}, router.Deps{Admin: &fakeStore{}, IAM: client}))
	defer server.Close()

	payload := `{"definition":{"id":"b","name":"b","parse":{"mode":"protobuf"},"field_map":{},"constants":{"class_uid":2004}},"samples":["x"]}`

	response := postValidate(t, server, payload)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("状态 = %d", response.StatusCode)
	}

	var body struct {
		Data struct {
			Compiled   bool   `json:"compiled"`
			CompileErr string `json:"compile_error"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	if body.Data.Compiled || body.Data.CompileErr == "" {
		t.Fatalf("编译错误没有返回: %+v", body.Data)
	}
}

// 没有样本时拒绝：验证必须基于真实报文才有意义。
func TestValidateMappingRejectsEmptySamples(t *testing.T) {
	client := fakeIAM(t, http.StatusOK)
	server := httptest.NewServer(router.Handler(&handler.Server{}, router.Deps{Admin: &fakeStore{}, IAM: client}))
	defer server.Close()

	payload := `{"definition":{"id":"r","name":"r","parse":{"mode":"json"},"field_map":{},"constants":{"class_uid":2004}},"samples":[]}`

	response := postValidate(t, server, payload)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("状态 = %d，期望 400", response.StatusCode)
	}
}
