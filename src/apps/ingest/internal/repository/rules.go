package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ParseRule 是管理面看到的解析规则。
type ParseRule struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Spec      json.RawMessage `json:"spec"`
	Version   int             `json:"version"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// CreateRuleInput 是创建规则的入参。
type CreateRuleInput struct {
	Name string          `json:"name"`
	Spec json.RawMessage `json:"spec"`
}

// CreateRule 先编译再落库：编译不过的规则不进表，避免运行期才发现。
// 这是唯一一处"校验与写入同处"的地方，因为校验规则本身属于本模块。
func CreateRule(ctx context.Context, pool *pgxpool.Pool, input CreateRuleInput) (ParseRule, error) {
	if _, err := compileRule(input.Spec); err != nil {
		return ParseRule{}, err
	}

	var created ParseRule
	err := pool.QueryRow(ctx, `
		INSERT INTO ingest.parse_rules (id, name, spec) VALUES (gen_random_uuid(), $1, $2::jsonb)
		RETURNING id, name, spec, version, created_at, updated_at`,
		input.Name, string(input.Spec),
	).Scan(&created.ID, &created.Name, &created.Spec, &created.Version, &created.CreatedAt, &created.UpdatedAt)
	if err != nil {
		return ParseRule{}, classify(err)
	}

	return created, nil
}

// ListRules 按名称排序列出规则。
func ListRules(ctx context.Context, pool *pgxpool.Pool) ([]ParseRule, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, name, spec, version, created_at, updated_at FROM ingest.parse_rules ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("could not list rules: %w", err)
	}

	defer rows.Close()

	items := make([]ParseRule, 0, 8)
	for rows.Next() {
		var item ParseRule
		if err = rows.Scan(&item.ID, &item.Name, &item.Spec, &item.Version, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("could not read rule row: %w", err)
		}

		items = append(items, item)
	}

	return items, rows.Err()
}

// UpdateRule 换规则内容并自增版本；事件按 version 归因解析质量。
func UpdateRule(ctx context.Context, pool *pgxpool.Pool, id string, input CreateRuleInput) (ParseRule, error) {
	if _, err := compileRule(input.Spec); err != nil {
		return ParseRule{}, err
	}

	var updated ParseRule
	err := pool.QueryRow(ctx, `
		UPDATE ingest.parse_rules SET spec = $2::jsonb, version = version + 1, updated_at = now()
		 WHERE id = $1 RETURNING id, name, spec, version, created_at, updated_at`,
		id, string(input.Spec),
	).Scan(&updated.ID, &updated.Name, &updated.Spec, &updated.Version, &updated.CreatedAt, &updated.UpdatedAt)
	if err != nil {
		return ParseRule{}, classify(err)
	}

	return updated, nil
}

// DeleteRule 删除规则；仍被接入点引用时返回 ErrRuleReferenced，上层转 409。
func DeleteRule(ctx context.Context, pool *pgxpool.Pool, id string) error {
	tag, err := pool.Exec(ctx, `DELETE FROM ingest.parse_rules WHERE id = $1`, id)
	if err != nil {
		return classify(err)
	}

	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// ValidateRuleSpec 供管理面的样本验证接口复用同一套编译校验。
func ValidateRuleSpec(spec []byte) error {
	_, err := compileRule(spec)

	return err
}
