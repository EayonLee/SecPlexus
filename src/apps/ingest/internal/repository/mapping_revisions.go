package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository/sqlcgen"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/validation"
)

// MappingRevision 是解析规则的不可变修订和发布证据。
type MappingRevision struct {
	ParsingRuleID string
	Version       int32
	Name          string
	Vendor        string
	Product       string
	Definition    json.RawMessage
	Verification  *validation.RuleVerification
	AuthorID      *string
	RecordedAt    time.Time
}

// ListMappingRevisions 按修订倒序读取摘要，使用规则 ID 和上一页的最早修订继续读取。
func (s *AdminStore) ListMappingRevisions(ctx context.Context, id string, before *int32, limit int32) ([]MappingRevision, error) {
	rows, err := sqlcgen.New(s.pool).ListMappingRevisions(ctx, sqlcgen.ListMappingRevisionsParams{
		ParsingRuleID: id, BeforeVersion: before, PageLimit: limit,
	})
	if err != nil {
		return nil, classify(err)
	}

	revisions := make([]MappingRevision, 0, len(rows))
	for _, row := range rows {
		var verification *validation.RuleVerification
		if len(row.Verification) > 0 {
			if err := json.Unmarshal(row.Verification, &verification); err != nil {
				return nil, err
			}
		}
		revisions = append(revisions, MappingRevision{
			ParsingRuleID: row.ParsingRuleID, Version: row.Version, Name: row.Name, Vendor: row.Vendor,
			Product: row.Product, RecordedAt: row.RecordedAt, Verification: verification, AuthorID: row.AuthorID,
		})
	}
	return revisions, nil
}

// ReadMappingRevision 获取可用于比较或恢复的定义快照。
func (s *AdminStore) ReadMappingRevision(ctx context.Context, id string, version int32) (MappingRevision, error) {
	row, err := sqlcgen.New(s.pool).ReadMappingRevision(ctx, sqlcgen.ReadMappingRevisionParams{ParsingRuleID: id, Version: version})
	if err != nil {
		return MappingRevision{}, classify(err)
	}
	var verification *validation.RuleVerification
	if len(row.Verification) > 0 {
		if err := json.Unmarshal(row.Verification, &verification); err != nil {
			return MappingRevision{}, err
		}
	}

	return MappingRevision{
		ParsingRuleID: row.ParsingRuleID, Version: row.Version, Name: row.Name, Vendor: row.Vendor,
		Product: row.Product, Definition: row.Definition, RecordedAt: row.RecordedAt, Verification: verification, AuthorID: row.AuthorID,
	}, nil
}
