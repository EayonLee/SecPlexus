package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/source"
)

// Problem 描述某条接入点为何没能加载。
//
// 加载采用"全量或全不"：只要有一条失败，整次加载被拒绝，运行期继续用上一份
// 成功配置。这是 nginx fail-safe reload 的语义——新配置解析失败时旧 worker
// 继续服务，整份新配置不生效。逐条跳过反而更危险：某个接入点会静默消失，
// 它的端口停止监听，而运维看不到任何接入点级告警。
type Problem struct {
	SourceID string // 出问题的接入点。
	Reason   string // 人类可读原因，写进日志与 readiness 说明。
}

// LoadSources 从数据库编译出运行期的接入点集合。
//
// 只取 enabled 的行：停用的接入点不该占用端口或消费组。
// 任何一条规则编译失败都会让整次加载失败（Problems 非空即视为失败），
// 调用方据此保留上一份成功配置。
func LoadSources(ctx context.Context, pool *pgxpool.Pool) ([]source.Entry, []Problem, error) {
	rows, err := pool.Query(ctx, `
		SELECT s.id, s.name, s.transport, s.token_digest, s.syslog_port,
		       s.kafka_brokers, s.kafka_topic, r.spec
		  FROM ingest.sources s
		  LEFT JOIN ingest.parse_rules r ON r.id = s.parse_rule_id
		 WHERE s.enabled
		 ORDER BY s.name`)
	if err != nil {
		return nil, nil, fmt.Errorf("could not read sources: %w", err)
	}

	defer rows.Close()

	var (
		entries  []source.Entry
		problems []Problem
	)

	for rows.Next() {
		var (
			sourceID  string
			name      string
			transport string
			token     []byte
			port      *int32
			brokers   []string
			topic     *string
			ruleRaw   []byte
		)

		if err = rows.Scan(&sourceID, &name, &transport, &token, &port, &brokers, &topic, &ruleRaw); err != nil {
			return nil, nil, fmt.Errorf("could not read source row: %w", err)
		}

		entry := source.Entry{
			ID:        sourceID,
			Name:      name,
			Transport: source.Transport(transport),
			Kafka:     source.KafkaBinding{Brokers: brokers},
		}

		if topic != nil {
			entry.Kafka.Topic = *topic
		}

		if port != nil {
			entry.Port = int(*port)
		}

		if len(token) == len(entry.Token) {
			copy(entry.Token[:], token)
		}

		if ruleRaw != nil {
			engine, compileErr := compileRule(ruleRaw)
			if compileErr != nil {
				// 不中断遍历：把所有问题一次报全，运维一轮就能修完。
				problems = append(problems, Problem{SourceID: sourceID, Reason: compileErr.Error()})

				continue
			}

			entry.Rule = engine
		}

		entries = append(entries, entry)
	}

	if err = rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("could not read sources: %w", err)
	}

	return entries, problems, nil
}

// compileRule 解析规则 JSON 并编译；失败时返回可读原因。
func compileRule(raw []byte) (*normalizer.Engine, error) {
	var spec ruleSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, errors.New("rule spec is not valid JSON")
	}

	engine, err := normalizer.Compile(spec.rule())
	if err != nil {
		return nil, fmt.Errorf("rule %q did not compile: %w", spec.Name, err)
	}

	return engine, nil
}

// ReloadInterval 是接入点刷新间隔；SIGHUP 会立即触发一次。
const ReloadInterval = 10 * time.Second
