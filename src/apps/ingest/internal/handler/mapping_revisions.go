package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/service"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/validation"
	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// ListMappingRevisions 分页返回同一个解析规则的修订，不返回规则定义和样本原文。
func (a *Admin) ListMappingRevisions(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		result.Fail(c.Writer, result.CodeNotFound)
		return
	}
	var query struct {
		Before *int32 `form:"before_version" binding:"omitempty,gt=0"`
	}
	if err := c.ShouldBindQuery(&query); err != nil {
		result.Fail(c.Writer, result.CodeInvalidRequest)
		return
	}
	const pageSize = 20
	rows, err := a.Store.ListMappingRevisions(c.Request.Context(), id, query.Before, pageSize+1)
	if err != nil {
		writeRepositoryError(c.Writer, err)
		return
	}
	var next *int32
	if len(rows) > pageSize {
		rows = rows[:pageSize]
		next = &rows[pageSize-1].Version
	}
	items := make([]model.MappingRevision, 0, len(rows))
	for _, row := range rows {
		items = append(items, mappingRevisionModel(row))
	}
	result.Success(c.Writer, struct {
		Items  []model.MappingRevision `json:"items"`
		Before *int32                  `json:"next_before_version,omitempty"`
	}{Items: items, Before: next})
}

// ReadMappingRevision 返回可比较的指定修订。
func (a *Admin) ReadMappingRevision(c *gin.Context) {
	item, ok := a.readRevision(c)
	if !ok {
		return
	}
	result.Success(c.Writer, mappingRevisionModel(item))
}

// RestoreMappingRevision 将可信历史定义采用为新修订，并以当前版本防止覆盖并发编辑。
func (a *Admin) RestoreMappingRevision(c *gin.Context) {
	var request struct {
		ExpectedVersion int32 `json:"expected_version" binding:"required,gt=0"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		writeMappingError(c.Writer, err)
		return
	}
	item, ok := a.readRevision(c)
	if !ok {
		return
	}
	digest, err := normalizer.ExecutionDigest(item.Definition, item.Vendor, item.Product)
	if err != nil || item.Verification == nil || digest != item.Verification.Digest {
		writeMappingError(c.Writer, &validation.Issue{Field: "version", Code: "revision_requires_verification"})
		return
	}

	ctx := c.Request.Context()
	if audit, ok := iamclient.AuditFromContext(ctx); ok {
		audit.Metadata = map[string]any{"restored_from_version": item.Version}
		ctx = iamclient.ContextWithAudit(ctx, audit)
	}
	params, _, err := service.PrepareRuleWrite(service.SaveRuleInput{
		Name: item.Name, Vendor: item.Vendor, Product: item.Product, Definition: item.Definition,
		AuthorID: ruleAuthor(c), ExpectedVersion: request.ExpectedVersion,
	})
	if err != nil {
		writeMappingError(c.Writer, err)
		return
	}
	params.Verification = item.Verification
	restored, err := a.Store.UpdateMapping(ctx, item.ParsingRuleID, params)
	if err != nil {
		writeMappingError(c.Writer, err)
		return
	}
	result.Success(c.Writer, toMapping(restored))
}

func (a *Admin) readRevision(c *gin.Context) (repository.MappingRevision, bool) {
	id := c.Param("id")
	version, err := strconv.ParseInt(c.Param("version"), 10, 32)
	if err != nil || version <= 0 {
		result.Fail(c.Writer, result.CodeNotFound)
		return repository.MappingRevision{}, false
	}
	if _, err := uuid.Parse(id); err != nil {
		result.Fail(c.Writer, result.CodeNotFound)
		return repository.MappingRevision{}, false
	}
	item, err := a.Store.ReadMappingRevision(c.Request.Context(), id, int32(version))
	if err != nil {
		writeRepositoryError(c.Writer, err)
		return repository.MappingRevision{}, false
	}
	return item, true
}

func mappingRevisionModel(row repository.MappingRevision) model.MappingRevision {
	return model.MappingRevision{
		ParsingRuleID: row.ParsingRuleID, Version: row.Version, Name: row.Name, Vendor: row.Vendor, Product: row.Product,
		RecordedAt: row.RecordedAt, Verification: row.Verification, AuthorID: row.AuthorID, Definition: row.Definition,
	}
}
