package router_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/handler"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/router"
)

// fakeStore 记录调用并返回固定结果。
type fakeStore struct {
	created   repository.Source
	token     string
	createErr error
	deletedID string
}

func (f *fakeStore) CreateSource(context.Context, repository.CreateSourceInput) (repository.Source, string, error) {
	return f.created, f.token, f.createErr
}

func (f *fakeStore) ListSources(context.Context) ([]repository.Source, error) {
	return []repository.Source{f.created}, nil
}

func (f *fakeStore) UpdateSource(context.Context, string, repository.UpdateSourceInput) (repository.Source, string, error) {
	return f.created, "", nil
}

func (f *fakeStore) DeleteSource(_ context.Context, id string) error {
	f.deletedID = id

	return nil
}

func (f *fakeStore) CreateRule(context.Context, repository.CreateRuleInput) (repository.ParseRule, error) {
	return repository.ParseRule{}, nil
}

func (f *fakeStore) ListRules(context.Context) ([]repository.ParseRule, error) { return nil, nil }

func (f *fakeStore) UpdateRule(context.Context, string, repository.CreateRuleInput) (repository.ParseRule, error) {
	return repository.ParseRule{}, nil
}

func (f *fakeStore) DeleteRule(context.Context, string) error { return nil }

// 安全回归：没有会话守卫时，管理路由不得注册。
// 宁可管理面不可用，也不能暴露一个任何人都能建接入点、拿令牌的写接口。
func TestAdminRoutesAbsentWithoutGuard(t *testing.T) {
	store := &fakeStore{}
	server := httptest.NewServer(router.Handler(&handler.Server{}, router.Deps{Admin: store}))
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/api/v1/ingest/sources")
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
		created: repository.Source{ID: "s1", Name: "fw", Transport: "http", Enabled: true},
		token:   "spx_onetime",
	}

	guard := func(c *gin.Context) { c.Next() }
	server := httptest.NewServer(router.Handler(&handler.Server{}, router.Deps{Admin: store, Guard: guard}))
	defer server.Close()

	payload := `{"name":"fw","transport":"http"}`
	response, err := server.Client().Post(server.URL+"/api/v1/ingest/sources", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("状态 = %d", response.StatusCode)
	}

	var body struct {
		Data model.CreateSourceResponse `json:"data"`
	}
	if err = json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Token != "spx_onetime" {
		t.Fatalf("令牌没有返回: %#v", body.Data)
	}
	if body.Data.Source.ID != "s1" {
		t.Fatalf("接入点没有返回: %#v", body.Data.Source)
	}
}

// 守卫必须真的挡在前面：守卫拒绝时创建不得发生。
func TestAdminGuardBlocksRequest(t *testing.T) {
	store := &fakeStore{}

	guard := func(c *gin.Context) {
		c.AbortWithStatus(http.StatusUnauthorized)
	}

	server := httptest.NewServer(router.Handler(&handler.Server{}, router.Deps{Admin: store, Guard: guard}))
	defer server.Close()

	response, err := server.Client().Post(server.URL+"/api/v1/ingest/sources", "application/json",
		strings.NewReader(`{"name":"fw","transport":"http"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("状态 = %d，期望 401", response.StatusCode)
	}
}
