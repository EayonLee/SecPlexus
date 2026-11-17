package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// InputStore 是管理面需要的存取能力。
// 接口存在是为了让 handler 用假实现测试，不依赖数据库。
type InputStore interface {
	CreateInput(ctx context.Context, input repository.CreateInputParams) (repository.Input, string, error)
	ListInputs(ctx context.Context) ([]repository.Input, error)
	UpdateInput(ctx context.Context, id string, input repository.UpdateInputParams) (repository.Input, string, error)
	DeleteInput(ctx context.Context, id string) error

	CreateMapping(ctx context.Context, input repository.CreateMappingParams) (repository.Mapping, error)
	ListMappings(ctx context.Context) ([]repository.Mapping, error)
	UpdateMapping(ctx context.Context, id string, input repository.CreateMappingParams) (repository.Mapping, error)
	DeleteMapping(ctx context.Context, id string) error
}

// Admin 处理接入点与解析规则的管理请求。
type Admin struct {
	Store InputStore
}

// ListInputs 返回全部接入点。
func (a *Admin) ListInputs(c *gin.Context) {
	items, err := a.Store.ListInputs(c.Request.Context())
	if err != nil {
		writeRepositoryError(c.Writer, err)

		return
	}

	out := make([]model.Input, 0, len(items))
	for _, item := range items {
		out = append(out, toModel(item))
	}

	result.Success(c.Writer, out)
}

// CreateInput 创建接入点并返回一次性令牌。
func (a *Admin) CreateInput(c *gin.Context) {
	var request model.CreateInputRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		result.Fail(c.Writer, result.CodeInvalidRequest)

		return
	}

	created, token, err := a.Store.CreateInput(c.Request.Context(), repository.CreateInputParams{
		Name:         request.Name,
		Protocol:     request.Protocol,
		MappingID:    request.MappingID,
		SyslogPort:   request.SyslogPort,
		KafkaBrokers: request.KafkaBrokers,
		KafkaTopic:   request.KafkaTopic,
	})
	if err != nil {
		writeRepositoryError(c.Writer, err)

		return
	}

	result.Success(c.Writer, model.CreateInputResponse{Input: toModel(created), Token: token})
}

// UpdateInput 修改接入点；换令牌时同样只返回一次明文。
func (a *Admin) UpdateInput(c *gin.Context) {
	var request model.UpdateInputRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		result.Fail(c.Writer, result.CodeInvalidRequest)

		return
	}

	input := repository.UpdateInputParams{
		Name:        request.Name,
		Enabled:     request.Enabled,
		RotateToken: request.RotateToken,
	}

	// 解绑与换绑是两种意图，必须能区分：显式解绑才置空。
	if request.UnbindRule {
		empty := (*string)(nil)
		input.MappingID = &empty
	} else if request.MappingID != nil {
		bound := request.MappingID
		input.MappingID = &bound
	}

	updated, token, err := a.Store.UpdateInput(c.Request.Context(), c.Param("id"), input)
	if err != nil {
		writeRepositoryError(c.Writer, err)

		return
	}

	result.Success(c.Writer, model.CreateInputResponse{Input: toModel(updated), Token: token})
}

// DeleteInput 删除接入点。
func (a *Admin) DeleteInput(c *gin.Context) {
	if err := a.Store.DeleteInput(c.Request.Context(), c.Param("id")); err != nil {
		writeRepositoryError(c.Writer, err)

		return
	}

	c.Status(http.StatusNoContent)
}

// ListMappings 返回全部解析规则。
func (a *Admin) ListMappings(c *gin.Context) {
	items, err := a.Store.ListMappings(c.Request.Context())
	if err != nil {
		writeRepositoryError(c.Writer, err)

		return
	}

	out := make([]model.Mapping, 0, len(items))
	for _, item := range items {
		out = append(out, model.Mapping{
			ID: item.ID, Name: item.Name, Spec: item.Spec, Version: item.Version,
			CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		})
	}

	result.Success(c.Writer, out)
}

// CreateMapping 创建解析规则；编译不过时返回参数错误而不是落库。
func (a *Admin) CreateMapping(c *gin.Context) {
	var request model.MappingRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		result.Fail(c.Writer, result.CodeInvalidRequest)

		return
	}

	created, err := a.Store.CreateMapping(c.Request.Context(), repository.CreateMappingParams{
		Name: request.Name, Spec: request.Spec,
	})
	if err != nil {
		writeRepositoryError(c.Writer, err)

		return
	}

	result.Success(c.Writer, model.Mapping{
		ID: created.ID, Name: created.Name, Spec: created.Spec, Version: created.Version,
		CreatedAt: created.CreatedAt, UpdatedAt: created.UpdatedAt,
	})
}

// UpdateMapping 换规则内容并自增版本。
func (a *Admin) UpdateMapping(c *gin.Context) {
	var request model.MappingRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		result.Fail(c.Writer, result.CodeInvalidRequest)

		return
	}

	updated, err := a.Store.UpdateMapping(c.Request.Context(), c.Param("id"), repository.CreateMappingParams{
		Name: request.Name, Spec: request.Spec,
	})
	if err != nil {
		writeRepositoryError(c.Writer, err)

		return
	}

	result.Success(c.Writer, model.Mapping{
		ID: updated.ID, Name: updated.Name, Spec: updated.Spec, Version: updated.Version,
		CreatedAt: updated.CreatedAt, UpdatedAt: updated.UpdatedAt,
	})
}

// DeleteMapping 删除解析规则；仍被引用时返回 409。
func (a *Admin) DeleteMapping(c *gin.Context) {
	if err := a.Store.DeleteMapping(c.Request.Context(), c.Param("id")); err != nil {
		writeRepositoryError(c.Writer, err)

		return
	}

	c.Status(http.StatusNoContent)
}

// sampleRequest 是样本验证的请求体。',
type sampleRequest struct {
	Definition json.RawMessage `json:"definition"`
	Samples    []string        `json:"samples"`
}

// sampleFailure 描述单条样本为何没通过。',
type sampleFailure struct {
	Index  int    `json:"index"`
	Sample string `json:"sample"`
	Reason string `json:"reason"`
}

// sampleResponse 是样本验证结果：总数、命中数与失败明细。',
type sampleResponse struct {
	Compiled   bool            `json:"compiled"`
	Total      int             `json:"total"`
	Matched    int             `json:"matched"`
	CompileErr string          `json:"compile_error,omitempty"`
	Failures   []sampleFailure `json:"failures,omitempty"`
}

// ValidateMapping 用真实样本试跑规则，返回命中率与失败原因。',
//
// 存在的意义：规则编译通过不代表能解析真实报文。让用户在保存前看到',
// "这条样本解析不出来"，比保存后发现整条接入点都是 parse.outcome=error 好得多。',
func (a *Admin) ValidateMapping(c *gin.Context) {
	var request sampleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		result.Fail(c.Writer, result.CodeInvalidRequest)

		return
	}

	if len(request.Samples) == 0 || len(request.Samples) > maxSamples {
		result.Fail(c.Writer, result.CodeInvalidRequest)

		return
	}

	engine, err := repository.CompileMapping(request.Definition)
	if err != nil {
		result.Success(c.Writer, sampleResponse{Total: len(request.Samples), CompileErr: err.Error()})

		return
	}

	response := sampleResponse{Compiled: true, Total: len(request.Samples)}
	for index, sample := range request.Samples {
		if _, applyErr := engine.Apply([]byte(sample), time.Unix(0, 0).UTC()); applyErr != nil {
			response.Failures = append(response.Failures, sampleFailure{
				Index: index, Sample: sample, Reason: applyErr.Error(),
			})

			continue
		}

		response.Matched++
	}

	result.Success(c.Writer, response)
}

// maxSamples 限制单次验证的样本数：验证是同步的，不设上限会变成 CPU 放大器。',
const maxSamples = 50

// toModel 把仓储类型转成 HTTP 形状。
func toModel(item repository.Input) model.Input {
	return model.Input{
		ID: item.ID, Name: item.Name, Protocol: item.Protocol,
		MappingID: item.MappingID, SyslogPort: item.SyslogPort,
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
		errors.Is(err, repository.ErrMappingReferenced):
		result.Fail(w, result.CodeConflict)
	case errors.Is(err, repository.ErrInvalidCombination), errors.Is(err, repository.ErrTransportImmutable):
		result.Fail(w, result.CodeInvalidRequest)
	default:
		// 规则编译失败也走 400：是调用方给的规则不合法，不是服务端故障。
		result.Fail(w, result.CodeInvalidRequest)
	}
}
