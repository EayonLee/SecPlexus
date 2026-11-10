package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/console/internal/handler"
	"github.com/EayonLee/SecPlexus/src/apps/console/internal/router"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// fakeIAM 提供最小授权端点，按用例决定允许、拒绝或返回畸形范围。
func fakeIAM(t *testing.T, respond func(w http.ResponseWriter)) *iamclient.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		respond(w)
	}))
	t.Cleanup(server.Close)

	client, err := iamclient.New(server.URL, strings.Repeat("c", 32))
	if err != nil {
		t.Fatal(err)
	}

	return client
}

const upstreamRequestID = "11111111-2222-4333-8444-555555555555"

func decisionBody(t *testing.T, decision iamclient.CapabilityDecision) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"code": "ok", "message": "", "data": decision, "request_id": upstreamRequestID,
	})
	if err != nil {
		t.Fatal(err)
	}

	return body
}

func rejectionBody(code result.Code) []byte {
	return []byte(`{"code":"` + string(code) + `","message":"rejected","data":null,"request_id":"` + upstreamRequestID + `"}`)
}

func get(t *testing.T, server *handler.Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	router.Handler(server).ServeHTTP(recorder, request)
	return recorder
}

func TestContextReturnsVerifiedPrincipal(t *testing.T) {
	client := fakeIAM(t, func(w http.ResponseWriter) {
		_, _ = w.Write(decisionBody(t, iamclient.CapabilityDecision{Allowed: true, DecisionID: "decision-1",
			Principal: iamclient.Principal{UserID: "user-1", SessionID: "session-1"}}))
	})

	recorder := get(t, &handler.Server{IAM: client}, "/api/v1/console/context")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}

	var body struct {
		Data handler.APIConsoleContext `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}

	if body.Data.UserID != "user-1" || body.Data.DecisionID != "decision-1" {
		t.Fatalf("unexpected context %+v", body.Data)
	}
}

// IAM 拒绝必须原样传递，不能被当成服务故障或成功。
func TestContextPropagatesDenial(t *testing.T) {
	client := fakeIAM(t, func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write(rejectionBody(result.CodePermissionDenied))
	})

	recorder := get(t, &handler.Server{IAM: client}, "/api/v1/console/context")

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}

	var body struct {
		Code result.Code `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}

	if body.Code == result.CodeServiceUnavailable {
		t.Fatal("denial must not be reported as service unavailable")
	}
}

// 限流响应必须带 Retry-After，客户端据此退避。
func TestContextRateLimitCarriesRetryAfter(t *testing.T) {
	client := fakeIAM(t, func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write(rejectionBody(result.CodeRateLimited))
	})

	recorder := get(t, &handler.Server{IAM: client}, "/api/v1/console/context")

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", recorder.Code)
	}

	if recorder.Header().Get("Retry-After") != "60" {
		t.Fatalf("Retry-After = %q, want 60", recorder.Header().Get("Retry-After"))
	}
}

// 上游不可达必须是 503，而不是允许或 500。
func TestContextUnavailableIAMReturnsServiceUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close()

	client, err := iamclient.New(url, strings.Repeat("c", 32))
	if err != nil {
		t.Fatal(err)
	}

	if code := get(t, &handler.Server{IAM: client}, "/api/v1/console/context").Code; code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", code)
	}
}

// 健康检查不经过 IAM，必须独立可用。
func TestReadyDoesNotDependOnIAM(t *testing.T) {
	client := fakeIAM(t, func(w http.ResponseWriter) {
		t.Error("readiness must not call IAM")
		w.WriteHeader(http.StatusInternalServerError)
	})

	if code := get(t, &handler.Server{IAM: client}, "/health/ready").Code; code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
}
