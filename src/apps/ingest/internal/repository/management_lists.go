package repository

import (
	"context"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository/sqlcgen"
)

// Page 是有界管理列表，返回同一查询快照中的条目及总量。
type Page[T any] struct {
	Items  []T
	Total  int64
	Offset int32
}

// InputListFilter 是接入点列表的已校验筛选条件。
type InputListFilter struct {
	Limit, Offset              int32
	Query, Protocol, ExactName string
	Enabled                    *bool
	ParsingRuleID, SyslogPort      *string
}

// MappingListFilter 是解析规则列表的已校验筛选条件。
type MappingListFilter struct {
	Limit, Offset    int32
	Query, ParseType string
	Builtin          *bool
	ClassUID         *int32
	IDs              []string
}

// ReadInput 按 ID 获取接入点配置。
func (s *AdminStore) ReadInput(ctx context.Context, id string) (Input, error) {
	row, err := sqlcgen.New(s.pool).ReadInput(ctx, id)
	if err != nil {
		return Input{}, classify(err)
	}
	item, _ := decodeInput(row.IngestInput)
	item.ParsingRuleVersion = int(row.ParsingRuleVersion)
	return item, nil
}

// ListInputPage 查询接入点的一页。删除最后一页时返回第一页，以及实际偏移量。
func (s *AdminStore) ListInputPage(ctx context.Context, filter InputListFilter) (Page[Input], error) {
	q := sqlcgen.New(s.pool)
	params := sqlcgen.ListInputPageParams{
		Q: filter.Query, Protocol: filter.Protocol, ExactName: filter.ExactName, Enabled: filter.Enabled,
		ParsingRuleID: filter.ParsingRuleID, SyslogPort: filter.SyslogPort, PageLimit: filter.Limit, PageOffset: filter.Offset,
	}
	rows, err := q.ListInputPage(ctx, params)
	if err == nil && len(rows) == 0 && params.PageOffset > 0 {
		params.PageOffset = 0
		rows, err = q.ListInputPage(ctx, params)
	}
	if err != nil {
		return Page[Input]{}, err
	}
	page := Page[Input]{Items: make([]Input, 0, len(rows)), Offset: params.PageOffset}
	for _, row := range rows {
		// 无效的协议配置留给运行状态报告，管理面仍能定位该接入。
		item, _ := decodeInput(row.IngestInput)
		item.ParsingRuleVersion = int(row.ParsingRuleVersion)
		page.Items = append(page.Items, item)
		page.Total = row.Total
	}
	return page, nil
}

// ReadMapping 按 ID 获取规则和定义，供详情或编辑使用。
func (s *AdminStore) ReadMapping(ctx context.Context, id string) (Mapping, error) {
	return ReadMapping(ctx, s.pool, id)
}

// ListMappingPage 读取一页规则摘要，不传输完整定义。
func (s *AdminStore) ListMappingPage(ctx context.Context, filter MappingListFilter) (Page[Mapping], error) {
	q := sqlcgen.New(s.pool)
	params := sqlcgen.ListMappingPageParams{
		Q: filter.Query, ParseType: filter.ParseType, Builtin: filter.Builtin, Ids: filter.IDs, ClassUid: filter.ClassUID,
		PageLimit: filter.Limit, PageOffset: filter.Offset,
	}
	rows, err := q.ListMappingPage(ctx, params)
	if err == nil && len(rows) == 0 && params.PageOffset > 0 {
		params.PageOffset = 0
		rows, err = q.ListMappingPage(ctx, params)
	}
	if err != nil {
		return Page[Mapping]{}, err
	}
	page := Page[Mapping]{Items: make([]Mapping, 0, len(rows)), Offset: params.PageOffset}
	for _, row := range rows {
		page.Items = append(page.Items, Mapping{
			ID: row.ID, Name: row.Name, Vendor: row.Vendor, Product: row.Product, Version: int(row.Version),
			Builtin: row.Builtin, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			Description: row.Description, ParseType: row.ParseType, ClassUIDs: row.ClassUids,
		})
		page.Total = row.Total
	}
	return page, nil
}

// MappingInputCounts 按解析规则统计数据接入，只返回有绑定接入的规则。
func (s *AdminStore) MappingInputCounts(ctx context.Context, ids []string) (map[string]int64, error) {
	rows, err := sqlcgen.New(s.pool).MappingInputCounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[*row.ParsingRuleID] = row.InputCount
	}
	return counts, nil
}
