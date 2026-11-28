package router_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/router"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
)

// fakeStore 记录调用并返回固定结果。
type fakeStore struct {
	handler.InputStore
	created   repository.Input
	token     string
	createErr error
	deletedID string
}

func (f *fakeStore) CreateInput(context.Context, repository.CreateInputParams) (repository.Input, string, error) {
	return f.created, f.token, f.createErr
}

func (f *fakeStore) UpdateInput(context.Context, string, repository.UpdateInputParams) (repository.Input, string, error) {
	return f.created, "", nil
}

func (f *fakeStore) DeleteInput(_ context.Context, id string) error {
	f.deletedID = id

	return nil
}

func (f *fakeStore) CreateMapping(context.Context, repository.CreateMappingParams) (repository.Mapping, error) {
	return repository.Mapping{}, nil
}

func (f *fakeStore) UpdateMapping(context.Context, string, repository.CreateMappingParams) (repository.Mapping, error) {
	return repository.Mapping{}, nil
}

func (f *fakeStore) DeleteMapping(context.Context, string) error { return nil }

// 安全回归：没有会话守卫时，管理路由不得注册。
// 宁可管理面不可用，也不能暴露一个任何人都能建接入点、拿令牌的写接口。
func TestAdminRoutesAbsentWithoutGuard(t *testing.T) {
	store := &fakeStore{}
	server := httptest.NewServer(router.Handler(&handler.Server{}, router.Deps{Admin: &handler.Admin{Store: store}}))
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/api/v1/ingest/inputs")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("状态 = %d，期望 404（管理面未启用）", response.StatusCode)
	}
}

// 装了守卫之后路由才可用，且创建响应必须带回一次性令牌。
func TestAdminCreateReturnsTokenOnce(t *testing.T) {
	store := &fakeStore{
		created: repository.Input{ID: "s1", Name: "fw", Protocol: "http", Enabled: true},
		token:   "spx_onetime",
	}

	client := fakeIAM(t, http.StatusOK)
	server := httptest.NewServer(router.Handler(&handler.Server{}, router.Deps{Admin: &handler.Admin{Store: store}, IAM: client}))
	defer server.Close()

	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/ingest/inputs", strings.NewReader(`{"name":"fw","protocol":"http"}`))
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

	if response.StatusCode != http.StatusCreated {
		t.Fatalf("状态 = %d，期望 201", response.StatusCode)
	}

	var body struct {
		Data model.CreateInputResponse `json:"data"`
	}
	if err = json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Token != "spx_onetime" {
		t.Fatalf("令牌没有返回: %#v", body.Data)
	}
}

// 守卫必须真的挡在前面：守卫拒绝时创建不得发生。
func TestAdminGuardBlocksRequest(t *testing.T) {
	store := &fakeStore{}
	client := fakeIAM(t, http.StatusForbidden)

	server := httptest.NewServer(router.Handler(&handler.Server{}, router.Deps{Admin: &handler.Admin{Store: store}, IAM: client}))
	defer server.Close()

	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/ingest/inputs", strings.NewReader(`{"name":"fw","protocol":"http"}`))
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

	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("状态 = %d，期望 403", response.StatusCode)
	}
}

// fakeIAM 起一个按给定状态答复 /authorize 的假 IAM；审计写入始终成功。
func fakeIAM(t *testing.T, status int) *iamclient.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "audit") {
			w.WriteHeader(http.StatusNoContent)

			return
		}

		if status != http.StatusOK {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": "permission_denied", "message": "denied", "data": nil,
				"request_id": "00000000-0000-4000-8000-000000000001",
			})

			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "ok", "message": "ok",
			"data": map[string]any{
				"allowed": true, "decision_id": "00000000-0000-4000-8000-000000000002",
				"principal": map[string]any{"user_id": "u1", "session_id": "s1", "username": "alice"},
			},
			"request_id": "00000000-0000-4000-8000-000000000003",
		})
	}))
	t.Cleanup(server.Close)

	client, err := iamclient.New(server.URL, strings.Repeat("c", 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	return client
}
