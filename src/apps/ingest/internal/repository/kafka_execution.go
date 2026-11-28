package repository

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository/sqlcgen"
)

// KafkaExecutionStore 固定分区发布配置，数据库操作按批次而非按记录执行。
type KafkaExecutionStore struct {
	pool     *pgxpool.Pool
	mu       sync.Mutex
	compiled map[[sha256.Size]byte]*normalizer.Compiled
}

// NewKafkaExecutionStore 复用 Ingest 业务连接池，缓存少量已编译的历史规则。
func NewKafkaExecutionStore(pool *pgxpool.Pool) *KafkaExecutionStore {
	return &KafkaExecutionStore{pool: pool, compiled: make(map[[sha256.Size]byte]*normalizer.Compiled)}
}

// Bind 固定本批次并返回覆盖本批次的历史边界。
// 所在分区第一次接收、历史重放和配置切换使用同一个事务锁。
func (s *KafkaExecutionStore) Bind(ctx context.Context, wanted input.Entry, topicID string, window input.KafkaWindow) ([]input.KafkaBoundary, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	q := sqlcgen.New(tx)
	if err := q.CreateKafkaPartitionHead(ctx, sqlcgen.CreateKafkaPartitionHeadParams{
		InputID: wanted.ID, Partition: window.Partition, TopicID: topicID, FirstOffset: window.First,
	}); err != nil {
		return nil, err
	}
	head, err := q.LockKafkaPartitionHead(ctx, sqlcgen.LockKafkaPartitionHeadParams{InputID: wanted.ID, Partition: window.Partition})
	if err != nil {
		return nil, err
	}
	if head.TopicID != topicID {
		return nil, input.ErrKafkaPosition
	}
	if window.First < head.FirstOffset || (window.Committed < 0 && window.First > head.FirstOffset) ||
		(window.First > head.PreparedThrough && window.Committed <= head.PreparedThrough) {
		return nil, input.ErrKafkaPosition
	}

	request := sqlcgen.KafkaExecutionBoundariesParams{
		InputID: wanted.ID, Partition: window.Partition, FromOffset: window.First, ToOffset: window.Last,
	}
	rows, err := q.KafkaExecutionBoundaries(ctx, request)
	if err != nil {
		return nil, err
	}
	activate := len(rows) == 0
	if !activate && window.First > head.PreparedThrough && window.Committed >= window.First {
		active := rows[len(rows)-1].IngestKafkaExecutionBoundary
		// 旧副本遇到更晚修订时继续用已生效配置，不能再把分区切回去。
		activate = wanted.Revision > active.SourceRevision ||
			(wanted.Revision == active.SourceRevision && wanted.ParsingRuleVersion > int(active.ParsingRuleVersion))
	}
	if activate {
		var mappingID *string
		if wanted.ParsingRuleID != "" {
			mappingID = &wanted.ParsingRuleID
		}
		if err := q.CreateKafkaExecutionBoundary(ctx, sqlcgen.CreateKafkaExecutionBoundaryParams{
			InputID: wanted.ID, Partition: window.Partition, FirstOffset: window.First,
			SourceRevision: wanted.Revision, SourceName: wanted.Name, Vendor: wanted.Vendor, Product: wanted.Product,
			ParsingRuleID: mappingID, ParsingRuleVersion: int32(wanted.ParsingRuleVersion),
		}); err != nil {
			return nil, err
		}
	}
	if err := q.AdvanceKafkaPartitionHead(ctx, sqlcgen.AdvanceKafkaPartitionHeadParams{
		InputID: wanted.ID, Partition: window.Partition, PreparedThrough: window.Last,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	if activate {
		return []input.KafkaBoundary{{Offset: window.First, Source: wanted}}, nil
	}
	boundaries := make([]input.KafkaBoundary, 0, len(rows))
	for _, row := range rows {
		state := row.IngestKafkaExecutionBoundary
		source := wanted
		source.Name, source.Vendor, source.Product = state.SourceName, state.Vendor, state.Product
		source.Revision, source.ParsingRuleVersion = state.SourceRevision, int(state.ParsingRuleVersion)
		source.ParsingRuleID = ""
		source.Mapping = nil
		if state.ParsingRuleID != nil {
			source.ParsingRuleID = *state.ParsingRuleID
			if len(row.Definition) == 0 {
				return nil, errors.New("missing rule revision for Kafka execution")
			}
			source.Mapping, err = s.compile(row.Definition)
			if err != nil {
				return nil, err
			}
		}
		boundaries = append(boundaries, input.KafkaBoundary{Offset: state.FirstOffset, Source: source})
	}
	return boundaries, nil
}

func (s *KafkaExecutionStore) compile(definition []byte) (*normalizer.Compiled, error) {
	digest := sha256.Sum256(definition)
	s.mu.Lock()
	defer s.mu.Unlock()
	if engine := s.compiled[digest]; engine != nil {
		return engine, nil
	}
	engine, err := normalizer.CompileDefinition(definition)
	if err == nil {
		if len(s.compiled) >= 128 {
			clear(s.compiled)
		}
		s.compiled[digest] = engine
	}
	return engine, err
}
