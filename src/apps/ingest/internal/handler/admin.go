package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/observation"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/service"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/validation"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// InputStore 是管理面需要的存取能力。
// 接口存在是为了让 handler 用假实现测试，不依赖数据库。
type InputStore interface {
	CreateInput(ctx context.Context, input repository.CreateInputParams) (repository.Input, string, error)
	ListInputPage(context.Context, repository.InputListFilter) (repository.Page[repository.Input], error)
	ReadInput(context.Context, string) (repository.Input, error)
	MappingInputCounts(context.Context, []string) (map[string]int64, error)
	UpdateInput(ctx context.Context, id string, input repository.UpdateInputParams) (repository.Input, string, error)
	DeleteInput(ctx context.Context, id string) error

	CreateMapping(ctx context.Context, input repository.CreateMappingParams) (repository.Mapping, error)
	ListMappingPage(context.Context, repository.MappingListFilter) (repository.Page[repository.Mapping], error)
	ReadMapping(context.Context, string) (repository.Mapping, error)
	ListMappingRevisions(context.Context, string, *int32, int32) ([]repository.MappingRevision, error)
	ReadMappingRevision(context.Context, string, int32) (repository.MappingRevision, error)
	UpdateMapping(ctx context.Context, id string, input repository.CreateMappingParams) (repository.Mapping, error)
	DeleteMapping(ctx context.Context, id string) error
}

// Admin 处理接入点与解析规则的管理请求。
type Admin struct {
	KafkaCredentialAllowed func(string, []string) bool
	Store                  InputStore
	Observations           *observation.Store
}

// CreateInput 创建接入点并返回一次性令牌。
func (a *Admin) CreateInput(c *gin.Context) {
	var request model.CreateInputRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		result.Fail(c.Writer, result.CodeInvalidRequest)

		return
	}

	if request.KafkaCredential != "" &&
		(a.KafkaCredentialAllowed == nil || !a.KafkaCredentialAllowed(request.KafkaCredential, request.KafkaBrokers)) {
		result.FailFields(c.Writer, result.CodeInvalidRequest, result.FieldError{Field: "kafka_credential", Code: "unknown_credential"})
		return
	}

	created, token, err := a.Store.CreateInput(c.Request.Context(), repository.CreateInputParams{
		Name: request.Name, Vendor: request.Vendor, Product: request.Product,
		Protocol:            request.Protocol,
		ParsingRuleID:       request.ParsingRuleID,
		ExpectedRuleVersion: request.ExpectedRuleVersion,
		SyslogPort:          request.SyslogPort,
		KafkaBrokers:        request.KafkaBrokers,
		KafkaCredential:     request.KafkaCredential,
		KafkaTopic:          request.KafkaTopic,
		KafkaStartPosition:  request.KafkaStartPosition,
	})
	if err != nil {
		writeRepositoryError(c.Writer, err)

		return
	}

	result.Created(c.Writer, model.CreateInputResponse{Input: toModel(created), Token: token})
}

// UpdateInput 修改接入点；换令牌时同样只返回一次明文。
func (a *Admin) UpdateInput(c *gin.Context) {
	var request model.UpdateInputRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		result.Fail(c.Writer, result.CodeInvalidRequest)

		return
	}

	input := repository.UpdateInputParams{
		Name: request.Name, Vendor: request.Vendor, Product: request.Product,
		Enabled:             request.Enabled,
		RotateToken:         request.RotateToken,
		ExpectedRuleVersion: request.ExpectedRuleVersion,
	}

	// 解绑与换绑是两种意图，必须能区分：显式解绑才置空。
	if request.UnbindRule {
		empty := (*string)(nil)
		input.ParsingRuleID = &empty
	} else if request.ParsingRuleID != nil {
		bound := request.ParsingRuleID
		input.ParsingRuleID = &bound
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

// CreateMapping 绑定解析规则请求并返回已保存的配置。
func (a *Admin) CreateMapping(c *gin.Context) {
	a.saveRule(c, "", http.StatusCreated)
}

// UpdateMapping 绑定规则修订请求，使用路径中的规则 ID。
func (a *Admin) UpdateMapping(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		result.Fail(c.Writer, result.CodeNotFound)
		return
	}
	a.saveRule(c, id, http.StatusOK)
}

func (a *Admin) saveRule(c *gin.Context, id string, status int) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, validation.MaxSampleRequestBytes)
	var request model.MappingRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeMappingError(c.Writer, err)
		return
	}

	saved, err := service.SaveRule(c.Request.Context(), a.Store, service.SaveRuleInput{
		ID: id, Name: request.Name, Vendor: request.Vendor, Product: request.Product, Definition: request.Definition,
		Samples: request.Samples, ExpectedVersion: request.ExpectedVersion, AuthorID: ruleAuthor(c),
	})
	if err != nil {
		writeMappingError(c.Writer, err)
		return
	}
	if status == http.StatusCreated {
		result.Created(c.Writer, toMapping(saved))
		return
	}
	result.Success(c.Writer, toMapping(saved))
}

func ruleAuthor(c *gin.Context) *string {
	value, found := c.Get(iamclient.PrincipalKey)
	if !found {
		return nil
	}
	decision, ok := value.(iamclient.CapabilityDecision)
	if !ok {
		return nil
	}
	if _, err := uuid.Parse(decision.Principal.UserID); err != nil {
		return nil
	}
	return &decision.Principal.UserID
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
	Vendor          string          `json:"vendor"`
	Product         string          `json:"product"`
	ParsingRuleID   string          `json:"parsing_rule_id" binding:"required_with=ExpectedVersion,omitempty,uuid"`
	ExpectedVersion int32           `json:"expected_version" binding:"required_with=ParsingRuleID,omitempty,gt=0"`
	Definition      json.RawMessage `json:"definition"`
	Samples         []string        `json:"samples"`
}

// sampleFailure 描述单条样本为何没通过。',
type sampleFailure struct {
	Index  int    `json:"index"`
	Reason string `json:"reason"`
}

type samplePreview struct {
	AppliedDefaults []string           `json:"applied_defaults,omitempty"`
	BranchID        string             `json:"branch_id,omitempty"`
	CandidateClass  int                `json:"candidate_class_uid,omitempty"`
	Status          pipeline.Status    `json:"status,omitempty"`
	Issues          []normalizer.Issue `json:"issues,omitempty"`
	Fields          map[string]any     `json:"fields"`
	Index           int                `json:"index"`
	Event           json.RawMessage    `json:"event"`
	PartitionKey    string             `json:"partition_key"`
}

// sampleResponse 是样本验证结果：总数、命中数与失败明细。',
type sampleResponse struct {
	CompileIssue     *normalizer.Issue `json:"compile_issue,omitempty"`
	Compiled         bool              `json:"compiled"`
	Total            int               `json:"total"`
	Matched          int               `json:"matched"`
	CompileErr       string            `json:"compile_error,omitempty"`
	Failures         []sampleFailure   `json:"failures,omitempty"`
	Previews         []samplePreview   `json:"previews,omitempty"`
	RequiredBranches []string          `json:"required_branches"`
}

// ValidateMapping 用真实样本试跑规则，返回命中率与失败原因。',
//
// 存在的意义：规则编译通过不代表能解析真实报文。让用户在保存前看到',
// "这条样本解析不出来"，比保存后发现整条接入点都是 parse.outcome=error 好得多。',
func (a *Admin) ValidateMapping(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, validation.MaxSampleRequestBytes)
	var request sampleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeMappingError(c.Writer, err)

		return
	}

	if err := validation.CheckSamples(request.Samples); err != nil {
		writeMappingError(c.Writer, err)

		return
	}

	for _, item := range []struct {
		field validation.Field
		value *string
	}{{validation.Vendor, &request.Vendor}, {validation.Product, &request.Product}} {
		if *item.value == "" {
			continue
		}
		value, err := validation.RuleText(item.field, *item.value)
		if err != nil {
			writeMappingError(c.Writer, err)
			return
		}
		*item.value = value
	}

	engine, err := normalizer.CompileDraft(request.Definition)
	if err != nil {
		response := sampleResponse{Total: len(request.Samples), CompileErr: err.Error(), RequiredBranches: []string{}}
		var issue *validation.Issue
		if errors.As(err, &issue) {
			response.CompileErr = ""
			response.CompileIssue = &normalizer.Issue{Path: string(issue.Field), Code: issue.Code}
		}
		result.Success(c.Writer, response)

		return
	}

	ctx := c.Request.Context()
	var current repository.Mapping
	if request.ParsingRuleID != "" {
		current, err = a.Store.ReadMapping(ctx, request.ParsingRuleID)
		if err != nil {
			writeRepositoryError(c.Writer, err)
			return
		}
		if int(request.ExpectedVersion) != current.Version {
			writeMappingError(c.Writer, repository.ErrParsingRuleVersion)
			return
		}
	}
	required, err := service.RuleBranchesToVerify(current, request.Definition, request.Vendor, request.Product)
	if err != nil {
		writeMappingError(c.Writer, err)
		return
	}
	source := input.Entry{Mapping: engine, Vendor: request.Vendor, Product: request.Product}
	preview := previewSamples(ctx, source, request.Samples)
	preview.RequiredBranches = append(preview.RequiredBranches, required...)
	result.Success(c.Writer, preview)
}

func previewSamples(ctx context.Context, source input.Entry, samples []string) sampleResponse {
	response := sampleResponse{Compiled: true, Total: len(samples), RequiredBranches: []string{}}
	received := time.Now().UTC()
	for index, sample := range samples {
		if ctx.Err() != nil {
			break
		}
		preview, fields, applyErr := pipeline.PreviewSource(source, sample, received)
		if applyErr != nil {
			response.Failures = append(response.Failures, sampleFailure{
				Index: index, Reason: "Could not construct event preview",
			})

			continue
		}

		if preview.BranchID != "" || (!source.Mapping.Classifies() && preview.Outcome.Status != pipeline.StatusError) {
			response.Matched++
		}
		item := samplePreview{Index: index, Fields: fields, Event: preview.Body, PartitionKey: preview.PartitionKey}
		item.AppliedDefaults = preview.AppliedDefaults
		item.BranchID, item.CandidateClass, item.Status, item.Issues = preview.BranchID, preview.CandidateClass, preview.Outcome.Status, preview.Issues
		response.Previews = append(response.Previews, item)
	}

	return response
}

// toModel 把仓储类型转成 HTTP 形状。
func toModel(item repository.Input) model.Input {
	return model.Input{
		ID: item.ID, Name: item.Name, Vendor: item.Vendor, Product: item.Product, Revision: item.Revision, CredentialGeneration: item.CredentialGeneration, Protocol: item.Protocol,
		ParsingRuleID: item.ParsingRuleID, SyslogPort: item.SyslogPort,
		KafkaBrokers: item.KafkaBrokers, KafkaTopic: item.KafkaTopic, KafkaStartPosition: item.KafkaStartPosition, KafkaCredential: item.KafkaCredential,
		Enabled: item.Enabled, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func writeMappingError(w http.ResponseWriter, err error) {
	var issue *validation.Issue
	var wrongType *json.UnmarshalTypeError
	switch {
	case errors.As(err, &issue):
		result.FailFields(w, result.CodeInvalidRequest, result.FieldError{
			Field: string(issue.Field), Code: issue.Code, Limit: issue.Limit,
		})
	case errors.Is(err, repository.ErrDuplicateName):
		result.FailFields(w, result.CodeConflict, result.FieldError{Field: string(validation.Name), Code: "duplicate_name"})
	case errors.Is(err, repository.ErrParsingRuleVersion):
		result.FailFields(w, result.CodeConflict, result.FieldError{Field: "expected_version", Code: "revision_conflict"})
	case errors.As(err, &wrongType) && (wrongType.Field == "name" || wrongType.Field == "vendor" || wrongType.Field == "product"):
		result.FailFields(w, result.CodeInvalidRequest, result.FieldError{Field: wrongType.Field, Code: "invalid_type"})
	default:
		writeRepositoryError(w, err)
	}
}

// writeRepositoryError 把仓储错误映射成 HTTP 码。
// 映射写在边界上：仓储不认识 HTTP，handler 不认识 SQLSTATE。
func writeRepositoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrAuditWrite):
		result.Fail(w, result.CodeServiceUnavailable)
	case errors.Is(err, repository.ErrNotFound):
		result.Fail(w, result.CodeNotFound)
	case errors.Is(err, repository.ErrDuplicateName), errors.Is(err, repository.ErrPortTaken),
		errors.Is(err, repository.ErrMappingInUse), errors.Is(err, repository.ErrParsingRuleVersion):
		result.Fail(w, result.CodeConflict)
	case errors.Is(err, repository.ErrInvalidCombination), errors.Is(err, repository.ErrTransportImmutable):
		result.Fail(w, result.CodeInvalidRequest)
	default:
		// 规则编译失败也走 400：是调用方给的规则不合法，不是服务端故障。
		result.Fail(w, result.CodeInvalidRequest)
	}
}
