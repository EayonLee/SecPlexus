package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/observation"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// ListInputs 返回数据接入列表的一页及筛选结果总数。
func (a *Admin) ListInputs(c *gin.Context) {
	ctx := c.Request.Context()
	var query model.InputListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		result.Fail(c.Writer, result.CodeInvalidRequest)
		return
	}
	filter := repository.InputListFilter{
		Limit: query.Limit, Offset: query.Offset, Query: strings.TrimSpace(query.Q), Protocol: query.Protocol,
		ParsingRuleID: query.ParsingRuleID, ExactName: query.ExactName,
	}
	if query.Status != "" {
		enabled := query.Status == "enabled"
		filter.Enabled = &enabled
	}
	if query.SyslogPort != nil {
		port := strconv.Itoa(int(*query.SyslogPort))
		filter.SyslogPort = &port
	}
	page, err := a.Store.ListInputPage(ctx, filter)
	if err != nil {
		writeRepositoryError(c.Writer, err)
		return
	}
	result.Success(c.Writer, model.Page[model.Input]{
		Items: a.inputModels(ctx, page.Items), Total: page.Total, Limit: query.Limit, Offset: page.Offset,
	})
}

// ReadInput 返回单个接入点及本实例的诊断快照。
func (a *Admin) ReadInput(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		result.Fail(c.Writer, result.CodeNotFound)
		return
	}
	item, err := a.Store.ReadInput(c.Request.Context(), id)
	if err != nil {
		writeRepositoryError(c.Writer, err)
		return
	}
	result.Success(c.Writer, a.inputModels(c.Request.Context(), []repository.Input{item})[0])
}

// MappingInputCounts 返回指定解析规则绑定的数据接入数量，使用数据接入读取权限。
func (a *Admin) MappingInputCounts(c *gin.Context) {
	var query model.MappingInputCountsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		result.Fail(c.Writer, result.CodeInvalidRequest)
		return
	}
	counts, err := a.Store.MappingInputCounts(c.Request.Context(), query.IDs)
	if err != nil {
		writeRepositoryError(c.Writer, err)
		return
	}
	result.Success(c.Writer, counts)
}

// ListMappings 返回解析规则摘要的一页及筛选结果总数。
func (a *Admin) ListMappings(c *gin.Context) {
	ctx := c.Request.Context()
	var query model.MappingListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		result.Fail(c.Writer, result.CodeInvalidRequest)
		return
	}
	filter := repository.MappingListFilter{
		Limit: query.Limit, Offset: query.Offset, Query: strings.TrimSpace(query.Q),
		ParseType: query.ParseType, IDs: query.IDs, ClassUID: query.ClassUID,
	}
	if query.Origin != "" {
		builtin := query.Origin == "builtin"
		filter.Builtin = &builtin
	}
	page, err := a.Store.ListMappingPage(ctx, filter)
	if err != nil {
		writeRepositoryError(c.Writer, err)
		return
	}
	result.Success(c.Writer, model.Page[model.Mapping]{
		Items: mappingModels(page.Items), Total: page.Total, Limit: query.Limit, Offset: page.Offset,
	})
}

// ReadMapping 返回编辑和试算使用的完整规则。
func (a *Admin) ReadMapping(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		result.Fail(c.Writer, result.CodeNotFound)
		return
	}
	item, err := a.Store.ReadMapping(c.Request.Context(), id)
	if err != nil {
		writeRepositoryError(c.Writer, err)
		return
	}
	result.Success(c.Writer, toMapping(item))
}

func (a *Admin) inputModels(ctx context.Context, items []repository.Input) []model.Input {
	out := make([]model.Input, 0, len(items))
	var stats map[string]observation.Counts
	var activations map[string]observation.Activation
	var ready bool
	now := time.Now()
	if a.Observations != nil && len(items) != 0 {
		ids := make([]string, 0, len(items))
		desired := make(map[string]input.SourceVersion, len(items))
		for _, item := range items {
			ids = append(ids, item.ID)
			desired[item.ID] = input.SourceVersion{Revision: item.Revision, CredentialGeneration: item.CredentialGeneration,
				ParsingRuleVersion: item.ParsingRuleVersion, Enabled: item.Enabled}
		}
		stats, ready = a.Observations.Read(ctx, ids, now)
		activations = a.Observations.Activations(ctx, ids, desired)
	}

	for _, item := range items {
		entry := toModel(item)
		entry.ObservationsReady = ready
		activation, known := activations[item.ID]
		if !known {
			activation.State = "unknown"
		}
		entry.Activation = &activation
		entry.TrafficStatus = observation.TrafficStatus(ready, item.Enabled, stats[item.ID].LastConfirmed, now, item.CreatedAt)
		if observed, ok := stats[item.ID]; ok && observed.LastReceived != 0 {
			lastReceived := time.UnixMilli(observed.LastReceived).UTC()
			entry.Stats = &model.InputStats{
				Received: observed.Received, Parsed: observed.Parsed, Partial: observed.Partial, Unclassified: observed.Unclassified,
				ParseFailed: observed.Partial + observed.Unclassified, LastReceived: &lastReceived,
				Confirmed: observed.Confirmed,
			}
			if observed.LastConfirmed > 0 {
				lastConfirmed := time.UnixMilli(observed.LastConfirmed).UTC()
				entry.Stats.LastConfirmed = &lastConfirmed
			}
		}
		out = append(out, entry)
	}
	return out
}

func mappingModels(items []repository.Mapping) []model.Mapping {
	out := make([]model.Mapping, 0, len(items))
	for _, item := range items {
		out = append(out, toMapping(item))
	}
	return out
}

func toMapping(item repository.Mapping) model.Mapping {
	return model.Mapping{
		ID: item.ID, Name: item.Name, Vendor: item.Vendor, Product: item.Product, Definition: item.Spec,
		Description: item.Description, ParseType: item.ParseType, Verification: item.Verification, Version: item.Version, Builtin: item.Builtin, ClassUIDs: item.ClassUIDs,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}
