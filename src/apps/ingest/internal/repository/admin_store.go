package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository/sqlcgen"

	"github.com/EayonLee/SecPlexus/src/packages/go/iamclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// AdminStore 将管理变更与对应的待投递审计写入同一事务。
type AdminStore struct {
	pool *pgxpool.Pool
}

type writer = sqlcgen.DBTX

// ErrAuditWrite 表示管理变更未能与审计事实一同提交。
var ErrAuditWrite = errors.New("could not commit administrative audit")

// NewAdminStore 基于连接池创建管理面存储。
func NewAdminStore(pool *pgxpool.Pool) *AdminStore {
	return &AdminStore{pool: pool}
}

func (s *AdminStore) write(ctx context.Context, change func(pgx.Tx) (string, error)) error {
	event, ok := iamclient.AuditFromContext(ctx)
	if !ok {
		return ErrAuditWrite
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if event.TargetID, err = change(tx); err != nil {
		return err
	}
	event.EventID = uuid.NewString()
	payload, err := json.Marshal(event)
	if err != nil {
		return errors.Join(ErrAuditWrite, err)
	}
	if err = sqlcgen.New(tx).InsertAuditOutbox(ctx, sqlcgen.InsertAuditOutboxParams{ID: event.EventID, Event: payload}); err != nil {
		return errors.Join(ErrAuditWrite, err)
	}

	return tx.Commit(ctx)
}

// DeliverAudit 发送一批已提交的审计事实，收到确认后才删除待投递记录。
func (s *AdminStore) DeliverAudit(ctx context.Context, send func(context.Context, iamclient.AuditEvent) error) error {
	queries := sqlcgen.New(s.pool)
	rows, err := queries.ListAuditOutbox(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		var event iamclient.AuditEvent
		if err = json.Unmarshal(row.Event, &event); err == nil {
			occurredAt := row.QueuedAt.UTC()
			event.EventID = row.ID
			event.OccurredAt = &occurredAt
			err = send(ctx, event)
		} else {
			err = iamclient.ErrInvalidAuditEvent
		}
		if errors.Is(err, iamclient.ErrInvalidAuditEvent) {
			if err = queries.RejectAuditOutbox(ctx, row.ID); err != nil {
				return err
			}
			logging.From(ctx).Warn("Audit event needs review", "event", "audit.delivery_rejected", "audit_id", row.ID)
			continue
		}
		if err != nil {
			return err
		}
		// 确认后删除失败或多个进程同时发送时，IAM 仍按同一个事件标识入账一次。
		if err = queries.DeleteAuditOutbox(ctx, row.ID); err != nil {
			return err
		}
	}

	return nil
}

// AuditOutboxHealth 只读取索引前 10001 条，避免投递故障反过来拖慢监控。
func (s *AdminStore) AuditOutboxHealth(ctx context.Context) (depth, oldestSeconds int64, err error) {
	bounds, err := sqlcgen.New(s.pool).AuditOutboxHealth(ctx)
	return bounds.Depth, bounds.OldestSeconds, err
}
