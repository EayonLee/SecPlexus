package iamclient

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// TestTransportFailureKeepsCauseAndSafeResult 保留真实 TLS 故障链，前端仍只得到依赖不可用。
func TestTransportFailureKeepsCauseAndSafeResult(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	client, err := New(server.URL, strings.Repeat("x", 32), "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	request := httptest.NewRequest("GET", "/items?token=secret-query", nil)
	request.Header.Set("Cookie", "secret-cookie")
	_, err = client.Authorize(logging.With(context.Background(), "request_id", "caller-one"), request, "iam.organizations.read", "", nil)
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != result.CodeServiceUnavailable || failure.Cause == nil || failure.Reason != "transport" {
		t.Fatal("transport cause or public classification lost")
	}
	if strings.Contains(output.String(), "secret-") || !strings.Contains(output.String(), "caller-one") || !strings.Contains(output.String(), "transport") {
		t.Fatal("unsafe or uncorrelated RPC diagnostic")
	}
}

// TestGeneratedInternalClientKeepsResultAndResourceBoundaries 通过真实TLS校验生成客户端和项目策略的组合。
func TestGeneratedInternalClientKeepsResultAndResourceBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		payload string
		want    int
	}{
		{"valid", 200, `{"code":"ok","message":"ok","request_id":"9b843a09-850c-4194-b3f7-733b62ac5eb9","data":{"allowed":true,"decision_id":"decision","principal":{"user_id":"user","session_id":"session"},"scope":{"kind":"organization","org_id":"org"}}}`, 200},
		{"denied", 403, `{"code":"permission_denied","message":"denied","data":null,"request_id":"9b843a09-850c-4194-b3f7-733b62ac5eb9"}`, 403},
		{"mismatch", 403, `{"code":"conflict","message":"denied","data":null,"request_id":"9b843a09-850c-4194-b3f7-733b62ac5eb9"}`, 503},
		{"trailing", 200, `{"code":"ok","data":{}} {}`, 503},
		{"oversized", 200, strings.Repeat(" ", 64*1024+1), 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			credential := strings.Repeat("x", 32)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/internal/v1/iam/authorize" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer "+credential || r.Header.Get("Cookie") != "" {
					t.Error("generated request violated internal boundary")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.payload))
			}))
			defer server.Close()
			client, err := New(server.URL, credential, "")
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			client.http.Transport = server.Client().Transport
			original := httptest.NewRequest("GET", "/items", nil)
			original.Header.Set("Cookie", "browser-cookie-stays-in-authorize-body")
			_, err = client.Authorize(context.Background(), original, "iam.organizations.read", "org", nil)
			if tc.want == 200 {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var failure *Error
			if !errors.As(err, &failure) || failure.Status != tc.want {
				t.Fatalf("want status %d, got %v", tc.want, err)
			}
		})
	}
}
