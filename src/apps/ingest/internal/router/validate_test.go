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

	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/ingest/parsing-rules/validate", strings.NewReader(payload))
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
	server := httptest.NewServer(router.Handler(&handler.Server{}, router.Deps{Admin: &handler.Admin{Store: &fakeStore{}}, IAM: client}))
	defer server.Close()

	for _, tc := range []struct{ mode, sample string }{
		{"json", `{"src":"10.0.0.1","port":0,"ok":false}`},
		{"kv", "src=10.0.0.1 port=0 ok=false"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			payload, err := json.Marshal(map[string]any{
				"definition": map[string]any{"contract_version": "v1", "parse": map[string]string{"mode": tc.mode}, "branches": []map[string]any{{"id": "network", "name": "Network", "class_uid": 4001, "field_map": map[string]string{"src": "src_endpoint.ip"}}}},
				"samples":    []string{tc.sample, "invalid"},
			})
			if err != nil {
				t.Fatal(err)
			}

			response := postValidate(t, server, string(payload))
			if response.StatusCode != http.StatusOK {
				t.Fatalf("状态 = %d", response.StatusCode)
			}

			var body struct {
				Data struct {
					Compiled bool `json:"compiled"`
					Total    int  `json:"total"`
					Matched  int  `json:"matched"`
					Previews []struct {
						Fields       map[string]any `json:"fields"`
						Index        int            `json:"index"`
						Status       string         `json:"status"`
						PartitionKey string         `json:"partition_key"`
						Event        struct {
							ClassUID int `json:"class_uid"`
							TypeUID  int `json:"type_uid"`
							Source   struct {
								IP string `json:"ip"`
							} `json:"src_endpoint"`
						} `json:"event"`
					} `json:"previews"`
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
			if len(body.Data.Failures) != 0 {
				t.Fatalf("失败明细 = %+v", body.Data.Failures)
			}
			if len(body.Data.Previews) != 2 || body.Data.Previews[1].Status != "error" || body.Data.Previews[1].Event.ClassUID != 0 {
				t.Fatalf("预览 = %+v，期望有命中和未分类预览", body.Data.Previews)
			}
			preview := body.Data.Previews[0]
			if preview.Fields["src"] != "10.0.0.1" {
				t.Fatalf("源字段缺失: %v", preview.Fields)
			}
			if preview.Index != 0 || preview.Event.ClassUID != 4001 || preview.Event.TypeUID != 400100 || preview.Event.Source.IP != "10.0.0.1" || preview.PartitionKey != "10.0.0.1" {
				t.Fatalf("预览未还原发布事件的字段与分区键: %+v", preview)
			}
			if tc.mode == "json" && (preview.Fields["port"] != float64(0) || preview.Fields["ok"] != false) {
				t.Fatalf("源字段类型不符: %v", preview.Fields)
			}
		})
	}
}

// 编译不过的规则要返回原因，而不是 500。
func TestValidateMappingReportsCompileError(t *testing.T) {
	client := fakeIAM(t, http.StatusOK)
	server := httptest.NewServer(router.Handler(&handler.Server{}, router.Deps{Admin: &handler.Admin{Store: &fakeStore{}}, IAM: client}))
	defer server.Close()

	payload := `{"definition":{"contract_version":"v1","parse":{"mode":"protobuf"},"branches":[{"id":"network","name":"Network","class_uid":4001}]},"samples":["x"]}`

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
	server := httptest.NewServer(router.Handler(&handler.Server{}, router.Deps{Admin: &handler.Admin{Store: &fakeStore{}}, IAM: client}))
	defer server.Close()

	payload := `{"definition":{"contract_version":"v1","parse":{"mode":"json"},"branches":[{"id":"network","name":"Network","class_uid":4001}]},"samples":[]}`

	response := postValidate(t, server, payload)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("状态 = %d，期望 400", response.StatusCode)
	}
}

func TestRuleWritesValidateTheSubmittedSamples(t *testing.T) {
	client := fakeIAM(t, http.StatusOK)
	server := httptest.NewServer(router.Handler(&handler.Server{}, router.Deps{Admin: &handler.Admin{Store: &fakeStore{}}, IAM: client}))
	t.Cleanup(server.Close)
	const definition = `{"contract_version":"v1","description":"Network activity","parse":{"mode":"json"},"branches":[{"id":"network","name":"Network","class_uid":4001,"field_map":{"src":"src_endpoint.ip"},"constants":{"activity_id":6}}]}`
	for _, test := range []struct {
		samples string
		want    int
	}{
		{`[]`, http.StatusBadRequest},
		{`["invalid JSON"]`, http.StatusBadRequest},
		{`["{\"src\":\"192.0.2.1\"}"]`, http.StatusCreated},
	} {
		request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/ingest/parsing-rules", strings.NewReader(`{"name":"Network","vendor":"Example","product":"Firewall","definition":`+definition+`,"samples":`+test.samples+`}`))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-CSRF-Token", "csrf-token")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != test.want {
			t.Errorf("samples %s: got HTTP %d, want %d", test.samples, response.StatusCode, test.want)
		}
	}
}

func TestExtractMappingWithoutClassification(t *testing.T) {
	client := fakeIAM(t, http.StatusOK)
	server := httptest.NewServer(router.Handler(&handler.Server{}, router.Deps{Admin: &handler.Admin{Store: &fakeStore{}}, IAM: client}))
	defer server.Close()

	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/ingest/parsing-rules/extract", strings.NewReader(`{
		"parse":{"mode":"json"},"processors":[{"operation":"trim","source":"value","target":"trimmed"}],
		"samples":["{\"value\":\" test \",\"password\":1234,\"count\":42,\"active\":false,\"items\":[1],\"detail\":{\"key\":\"private\"},\"empty\":null}","invalid"]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", "csrf-token")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status %d", response.StatusCode)
	}
	var body struct {
		Data struct {
			Previews []struct {
				Fields map[string]any
				Reason string
				Types  map[string]string `json:"field_types"`
			}
		}
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.Previews) != 2 || body.Data.Previews[0].Fields["trimmed"] != "test" ||
		body.Data.Previews[0].Fields["password"] == float64(1234) || body.Data.Previews[1].Reason != "format_mismatch" {
		t.Fatalf("unexpected extraction: %+v", body.Data)
	}
	preview := body.Data.Previews[0]
	for field, want := range map[string]string{"value": "string", "trimmed": "string", "count": "number", "active": "boolean", "items": "array", "detail.key": "string", "empty": "null"} {
		if preview.Types[field] != want {
			t.Fatalf("type of %s = %q, want %q", field, preview.Types[field], want)
		}
	}
	if _, shown := preview.Types["password"]; shown || preview.Fields["items"] != "[REDACTED]" {
		t.Fatal("preview exposed hidden values or a sensitive field type")
	}
}

func TestMappingSampleTextRemainsUTF8(t *testing.T) {
	client := fakeIAM(t, http.StatusOK)
	server := httptest.NewServer(router.Handler(&handler.Server{}, router.Deps{Admin: &handler.Admin{Store: &fakeStore{}}, IAM: client}))
	defer server.Close()

	for endpoint, payload := range map[string]string{
		"extract":  `{"encoding":"GBK","parse":{"mode":"json"},"samples":["{\"label\":\"登录成功\"}"]}`,
		"validate": `{"definition":{"contract_version":"v1","encoding":"GBK","parse":{"mode":"json"}},"samples":["{\"label\":\"登录成功\"}"]}`,
	} {
		t.Run(endpoint, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/ingest/parsing-rules/"+endpoint, strings.NewReader(payload))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-CSRF-Token", "csrf-token")
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			var body struct {
				Data struct {
					Compiled bool
					Previews []struct {
						Fields map[string]any
					}
				}
			}
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusOK || !body.Data.Compiled || len(body.Data.Previews) != 1 ||
				body.Data.Previews[0].Fields["label"] != "登录成功" {
				t.Fatalf("sample text changed: %+v", body.Data)
			}
		})
	}
}
