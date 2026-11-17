package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AdminStore 把仓储函数适配成管理面需要的接口。
//
// handler 只依赖接口，不认识连接池；测试用假实现即可替换。
type AdminStore struct {
	pool *pgxpool.Pool
}

// NewAdminStore 基于连接池创建管理面存储。
func NewAdminStore(pool *pgxpool.Pool) *AdminStore {
	return &AdminStore{pool: pool}
}

// CreateInput 创建接入点。
func (s *AdminStore) CreateInput(ctx context.Context, params CreateInputParams) (Input, string, error) {
	return CreateInput(ctx, s.pool, params)
}

// ListInputs 列出接入点。
func (s *AdminStore) ListInputs(ctx context.Context) ([]Input, error) {
	return ListInputs(ctx, s.pool)
}

// UpdateInput 修改接入点。
func (s *AdminStore) UpdateInput(ctx context.Context, id string, params UpdateInputParams) (Input, string, error) {
	return UpdateInput(ctx, s.pool, id, params)
}

// DeleteInput 删除接入点。
func (s *AdminStore) DeleteInput(ctx context.Context, id string) error {
	return DeleteInput(ctx, s.pool, id)
}

// CreateMapping 创建解析映射。
func (s *AdminStore) CreateMapping(ctx context.Context, params CreateMappingParams) (Mapping, error) {
	return CreateMapping(ctx, s.pool, params)
}

// ListMappings 列出解析映射。
func (s *AdminStore) ListMappings(ctx context.Context) ([]Mapping, error) {
	return ListMappings(ctx, s.pool)
}

// UpdateMapping 修改解析映射。
func (s *AdminStore) UpdateMapping(ctx context.Context, id string, params CreateMappingParams) (Mapping, error) {
	return UpdateMapping(ctx, s.pool, id, params)
}

// DeleteMapping 删除解析映射。
func (s *AdminStore) DeleteMapping(ctx context.Context, id string) error {
	return DeleteMapping(ctx, s.pool, id)
}
