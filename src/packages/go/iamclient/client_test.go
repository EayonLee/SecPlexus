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

	"github.com/EayonLee/SecPlexus/src/contracts/common"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
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
	if !errors.As(err, &failure) || failure.Code != common.CodeServiceUnavailable || failure.Cause == nil || failure.Reason != "transport" {
		t.Fatal("transport cause or public classification lost")
	}
	if strings.Contains(output.String(), "secret-") || !strings.Contains(output.String(), "caller-one") || !strings.Contains(output.String(), "transport") {
		t.Fatal("unsafe or uncorrelated RPC diagnostic")
	}
}
