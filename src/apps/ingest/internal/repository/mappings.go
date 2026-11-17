package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
)

// Mapping 是管理面看到的解析规则。
type Mapping struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Spec      json.RawMessage `json:"definition"`
	Version   int             `json:"version"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// CreateMappingParams 是创建规则的入参。
type CreateMappingParams struct {
	Name string          `json:"name"`
	Spec json.RawMessage `json:"definition"`
}

// CreateMapping 先编译再落库：编译不过的规则不进表，避免运行期才发现。
// 这是唯一一处"校验与写入同处"的地方，因为校验规则本身属于本模块。
func CreateMapping(ctx context.Context, pool *pgxpool.Pool, input CreateMappingParams) (Mapping, error) {
	if _, err := compileMapping(input.Spec); err != nil {
		return Mapping{}, err
	}

	var created Mapping
	err := pool.QueryRow(ctx, `
		INSERT INTO ingest.mappings (id, name, definition) VALUES (gen_random_uuid(), $1, $2::jsonb)
		RETURNING id, name, definition, version, created_at, updated_at`,
		input.Name, string(input.Spec),
	).Scan(&created.ID, &created.Name, &created.Spec, &created.Version, &created.CreatedAt, &created.UpdatedAt)
	if err != nil {
		return Mapping{}, classify(err)
	}

	return created, nil
}

// ListMappings 按名称排序列出规则。
func ListMappings(ctx context.Context, pool *pgxpool.Pool) ([]Mapping, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, name, definition, version, created_at, updated_at FROM ingest.mappings ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("could not list rules: %w", err)
	}

	defer rows.Close()

	items := make([]Mapping, 0, 8)
	for rows.Next() {
		var item Mapping
		if err = rows.Scan(&item.ID, &item.Name, &item.Spec, &item.Version, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("could not read rule row: %w", err)
		}

		items = append(items, item)
	}

	return items, rows.Err()
}

// UpdateMapping 换规则内容并自增版本；事件按 version 归因解析质量。
func UpdateMapping(ctx context.Context, pool *pgxpool.Pool, id string, input CreateMappingParams) (Mapping, error) {
	if _, err := compileMapping(input.Spec); err != nil {
		return Mapping{}, err
	}

	var updated Mapping
	err := pool.QueryRow(ctx, `
		UPDATE ingest.mappings SET definition = $2::jsonb, version = version + 1, updated_at = now()
		 WHERE id = $1 RETURNING id, name, definition, version, created_at, updated_at`,
		id, string(input.Spec),
	).Scan(&updated.ID, &updated.Name, &updated.Spec, &updated.Version, &updated.CreatedAt, &updated.UpdatedAt)
	if err != nil {
		return Mapping{}, classify(err)
	}

	return updated, nil
}

// DeleteMapping 删除规则；仍被接入点引用时返回 ErrMappingReferenced，上层转 409。
func DeleteMapping(ctx context.Context, pool *pgxpool.Pool, id string) error {
	tag, err := pool.Exec(ctx, `DELETE FROM ingest.mappings WHERE id = $1`, id)
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
	var mapping Mapping

	err := pool.QueryRow(ctx, `
		SELECT id, name, version, definition, created_at, updated_at
		  FROM ingest.mappings WHERE id = $1`, id).
		Scan(&mapping.ID, &mapping.Name, &mapping.Version, &mapping.Spec, &mapping.CreatedAt, &mapping.UpdatedAt)
	if err != nil {
		return Mapping{}, classify(err)
	}

	return mapping, nil
}

// CompileMapping 编译规则定义，供样本验证复用管理面同一套校验。
//
// 与写入路径共用 compileMapping：验证通过而写入失败（或反之）会让用户困惑。
func CompileMapping(spec []byte) (*normalizer.Compiled, error) {
	return compileMapping(spec)
}

// ValidateMappingSpec 供管理面的样本验证接口复用同一套编译校验。
func ValidateMappingSpec(definition []byte) error {
	_, err := compileMapping(definition)

	return err
}
