package repository

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository/sqlcgen"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/validation"
)

// Mapping 是管理面看到的解析规则。
type Mapping struct {
	Verification *validation.RuleVerification
	AuthorID     *string
	ClassUIDs    []int32
	Description  string
	ParseType    string
	Vendor       string
	Product      string
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Spec         json.RawMessage `json:"definition"`
	Version      int             `json:"version"`
	Builtin      bool            `json:"builtin"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// CreateMappingParams 保存用例已校验、归一化的规则及版本信息。
type CreateMappingParams struct {
	Verification    *validation.RuleVerification
	AuthorID        *string
	ExpectedVersion int32
	Vendor          string
	Product         string
	Name            string          `json:"name"`
	Definition      json.RawMessage `json:"definition"`
}

// CreateMapping 持久化已通过用例校验的规则和首个修订。
func CreateMapping(ctx context.Context, pool writer, input CreateMappingParams) (Mapping, error) {

	verification, err := encodeVerification(input.Verification)
	if err != nil {
		return Mapping{}, err
	}
	row, err := sqlcgen.New(pool).CreateMapping(ctx, sqlcgen.CreateMappingParams{
		Name: input.Name, Vendor: input.Vendor, Product: input.Product, Definition: input.Definition,
		Verification: verification, AuthorID: input.AuthorID,
	})
	if err != nil {
		return Mapping{}, classify(err)
	}
	return mappingFromRow(row), nil
}

func encodeVerification(value *validation.RuleVerification) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return json.Marshal(value)
}

func mappingFromRow(row sqlcgen.IngestParsingRule) Mapping {
	var definition struct {
		Branches []struct {
			Class int32 `json:"class_uid"`
		}
		Constants struct {
			Class int32 `json:"class_uid"`
		}
		Parse       struct{ Mode string }
		Description string
	}
	_ = json.Unmarshal(row.Definition, &definition)
	classes := make([]int32, 0, len(definition.Branches))
	for _, branch := range definition.Branches {
		if branch.Class > 0 && !slices.Contains(classes, branch.Class) {
			classes = append(classes, branch.Class)
		}
	}
	if len(classes) == 0 {
		classes = append(classes, 0)
	}
	slices.Sort(classes)
	var verification *validation.RuleVerification
	_ = json.Unmarshal(row.Verification, &verification)
	return Mapping{Verification: verification, AuthorID: row.AuthorID, ID: row.ID, Name: row.Name, Spec: row.Definition, Vendor: row.Vendor, Product: row.Product,
		Version: int(row.Version), Builtin: row.Builtin, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		ClassUIDs: classes, ParseType: definition.Parse.Mode, Description: definition.Description}
}

// UpdateMapping 按预期版本更新规则并记录新修订。
func UpdateMapping(ctx context.Context, pool writer, id string, input CreateMappingParams) (Mapping, error) {
	if input.ExpectedVersion <= 0 {
		return Mapping{}, ErrParsingRuleVersion
	}
	verification, err := encodeVerification(input.Verification)
	if err != nil {
		return Mapping{}, err
	}
	queries := sqlcgen.New(pool)
	row, err := queries.UpdateMapping(ctx, sqlcgen.UpdateMappingParams{
		ID: id, Name: input.Name, Vendor: input.Vendor, Product: input.Product, Definition: input.Definition,
		ExpectedVersion: input.ExpectedVersion, Verification: verification, AuthorID: input.AuthorID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		current, readErr := queries.ReadMapping(ctx, id)
		if readErr == nil && !current.Builtin {
			return Mapping{}, ErrParsingRuleVersion
		}
		if readErr != nil && !errors.Is(readErr, pgx.ErrNoRows) {
			return Mapping{}, classify(readErr)
		}
	}
	if err != nil {
		return Mapping{}, classify(err)
	}
	return mappingFromRow(row), nil
}

// ErrParsingRuleVersion 表示规则修订缺失或已被其他请求更新。
var ErrParsingRuleVersion = errors.New("mapping revision conflict")

// DeleteMapping 删除规则；仍被接入点引用时返回 ErrMappingInUse，上层转 409。
func DeleteMapping(ctx context.Context, pool writer, id string) error {
	tag, err := pool.Exec(ctx, `DELETE FROM ingest.parsing_rules WHERE id = $1 AND NOT builtin`, id)
	if err != nil {
		return classify(err)
	}

	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// ReadMapping 读取单条映射定义；拉模式连接器每次拉取前取一次规则。
//
// 不缓存：拉取间隔最小 5 秒，编译成本相对可忽略，
// 而缓存会引入"规则改了但拉取还在用旧版本"的隐蔽偏差。
func ReadMapping(ctx context.Context, pool *pgxpool.Pool, id string) (Mapping, error) {
	row, err := sqlcgen.New(pool).ReadMapping(ctx, id)
	if err != nil {
		return Mapping{}, classify(err)
	}
	return mappingFromRow(row), nil
}

// CreateMapping 创建解析映射。
func (s *AdminStore) CreateMapping(ctx context.Context, params CreateMappingParams) (mapping Mapping, err error) {
	err = s.write(ctx, func(tx pgx.Tx) (string, error) {
		var writeErr error
		mapping, writeErr = CreateMapping(ctx, tx, params)
		return mapping.ID, writeErr
	})
	return
}

// UpdateMapping 修改解析映射。
func (s *AdminStore) UpdateMapping(ctx context.Context, id string, params CreateMappingParams) (mapping Mapping, err error) {
	err = s.write(ctx, func(tx pgx.Tx) (string, error) {
		var writeErr error
		mapping, writeErr = UpdateMapping(ctx, tx, id, params)
		return mapping.ID, writeErr
	})
	return
}

// DeleteMapping 删除解析映射。
func (s *AdminStore) DeleteMapping(ctx context.Context, id string) error {
	return s.write(ctx, func(tx pgx.Tx) (string, error) { return id, DeleteMapping(ctx, tx, id) })
}
