package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// SourceStore 是管理面需要的存取能力。
// 接口存在是为了让 handler 用假实现测试，不依赖数据库。
type SourceStore interface {
	CreateSource(ctx context.Context, input repository.CreateSourceInput) (repository.Source, string, error)
	ListSources(ctx context.Context) ([]repository.Source, error)
	UpdateSource(ctx context.Context, id string, input repository.UpdateSourceInput) (repository.Source, string, error)
	DeleteSource(ctx context.Context, id string) error

	CreateRule(ctx context.Context, input repository.CreateRuleInput) (repository.ParseRule, error)
	ListRules(ctx context.Context) ([]repository.ParseRule, error)
	UpdateRule(ctx context.Context, id string, input repository.CreateRuleInput) (repository.ParseRule, error)
	DeleteRule(ctx context.Context, id string) error
}

// Admin 处理接入点与解析规则的管理请求。
type Admin struct {
	Store SourceStore
}

// ListSources 返回全部接入点。
func (a *Admin) ListSources(c *gin.Context) {
	items, err := a.Store.ListSources(c.Request.Context())
	if err != nil {
		writeRepositoryError(c.Writer, err)

		return
	}

	out := make([]model.Source, 0, len(items))
	for _, item := range items {
		out = append(out, toModel(item))
	}

	result.Success(c.Writer, out)
}

// CreateSource 创建接入点并返回一次性令牌。
func (a *Admin) CreateSource(c *gin.Context) {
	var request model.CreateSourceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		result.Fail(c.Writer, result.CodeInvalidRequest)

		return
	}

	created, token, err := a.Store.CreateSource(c.Request.Context(), repository.CreateSourceInput{
		Name:         request.Name,
		Transport:    request.Transport,
		ParseRuleID:  request.ParseRuleID,
		SyslogPort:   request.SyslogPort,
		KafkaBrokers: request.KafkaBrokers,
		KafkaTopic:   request.KafkaTopic,
	})
	if err != nil {
		writeRepositoryError(c.Writer, err)

		return
	}

	result.Success(c.Writer, model.CreateSourceResponse{Source: toModel(created), Token: token})
}

// UpdateSource 修改接入点；换令牌时同样只返回一次明文。
func (a *Admin) UpdateSource(c *gin.Context) {
	var request model.UpdateSourceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		result.Fail(c.Writer, result.CodeInvalidRequest)

		return
	}

	input := repository.UpdateSourceInput{
		Name:        request.Name,
		Enabled:     request.Enabled,
		RotateToken: request.RotateToken,
	}

	// 解绑与换绑是两种意图，必须能区分：显式解绑才置空。
	if request.UnbindRule {
		empty := (*string)(nil)
		input.ParseRuleID = &empty
	} else if request.ParseRuleID != nil {
		bound := request.ParseRuleID
		input.ParseRuleID = &bound
	}

	updated, token, err := a.Store.UpdateSource(c.Request.Context(), c.Param("id"), input)
	if err != nil {
		writeRepositoryError(c.Writer, err)

		return
	}

	result.Success(c.Writer, model.CreateSourceResponse{Source: toModel(updated), Token: token})
}

// DeleteSource 删除接入点。
func (a *Admin) DeleteSource(c *gin.Context) {
	if err := a.Store.DeleteSource(c.Request.Context(), c.Param("id")); err != nil {
		writeRepositoryError(c.Writer, err)

		return
	}

	c.Status(http.StatusNoContent)
}

// ListRules 返回全部解析规则。
func (a *Admin) ListRules(c *gin.Context) {
	items, err := a.Store.ListRules(c.Request.Context())
	if err != nil {
		writeRepositoryError(c.Writer, err)

		return
	}

	out := make([]model.ParseRule, 0, len(items))
	for _, item := range items {
		out = append(out, model.ParseRule{
			ID: item.ID, Name: item.Name, Spec: item.Spec, Version: item.Version,
			CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		})
	}

	result.Success(c.Writer, out)
}

// CreateRule 创建解析规则；编译不过时返回参数错误而不是落库。
func (a *Admin) CreateRule(c *gin.Context) {
	var request model.ParseRuleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		result.Fail(c.Writer, result.CodeInvalidRequest)

		return
	}

	created, err := a.Store.CreateRule(c.Request.Context(), repository.CreateRuleInput{
		Name: request.Name, Spec: request.Spec,
	})
	if err != nil {
		writeRepositoryError(c.Writer, err)

		return
	}

	result.Success(c.Writer, model.ParseRule{
		ID: created.ID, Name: created.Name, Spec: created.Spec, Version: created.Version,
		CreatedAt: created.CreatedAt, UpdatedAt: created.UpdatedAt,
	})
}

// UpdateRule 换规则内容并自增版本。
func (a *Admin) UpdateRule(c *gin.Context) {
	var request model.ParseRuleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		result.Fail(c.Writer, result.CodeInvalidRequest)

		return
	}

	updated, err := a.Store.UpdateRule(c.Request.Context(), c.Param("id"), repository.CreateRuleInput{
		Name: request.Name, Spec: request.Spec,
	})
	if err != nil {
		writeRepositoryError(c.Writer, err)

		return
	}

	result.Success(c.Writer, model.ParseRule{
		ID: updated.ID, Name: updated.Name, Spec: updated.Spec, Version: updated.Version,
		CreatedAt: updated.CreatedAt, UpdatedAt: updated.UpdatedAt,
	})
}

// DeleteRule 删除解析规则；仍被引用时返回 409。
func (a *Admin) DeleteRule(c *gin.Context) {
	if err := a.Store.DeleteRule(c.Request.Context(), c.Param("id")); err != nil {
		writeRepositoryError(c.Writer, err)

		return
	}

	c.Status(http.StatusNoContent)
}

// toModel 把仓储类型转成 HTTP 形状。
func toModel(item repository.Source) model.Source {
	return model.Source{
		ID: item.ID, Name: item.Name, Transport: item.Transport,
		ParseRuleID: item.ParseRuleID, SyslogPort: item.SyslogPort,
		KafkaBrokers: item.KafkaBrokers, KafkaTopic: item.KafkaTopic,
		Enabled: item.Enabled, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

// writeRepositoryError 把仓储错误映射成 HTTP 码。
// 映射写在边界上：仓储不认识 HTTP，handler 不认识 SQLSTATE。
func writeRepositoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		result.Fail(w, result.CodeNotFound)
	case errors.Is(err, repository.ErrDuplicateName), errors.Is(err, repository.ErrPortTaken),
		errors.Is(err, repository.ErrRuleReferenced):
		result.Fail(w, result.CodeConflict)
	case errors.Is(err, repository.ErrInvalidCombination), errors.Is(err, repository.ErrTransportImmutable):
		result.Fail(w, result.CodeInvalidRequest)
	default:
		// 规则编译失败也走 400：是调用方给的规则不合法，不是服务端故障。
		result.Fail(w, result.CodeInvalidRequest)
	}
}
