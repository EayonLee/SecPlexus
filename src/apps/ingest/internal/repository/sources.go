package repository

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
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

// LoadInputs 从数据库编译出运行期的接入点集合。
//
// 只取 enabled 的行：停用的接入点不该占用端口或消费组。
// 任何一条规则编译失败都会让整次加载失败（Problems 非空即视为失败），
// 调用方据此保留上一份成功配置。
func LoadInputs(ctx context.Context, pool *pgxpool.Pool) ([]input.Entry, []Problem, error) {
	rows, err := pool.Query(ctx, `
		SELECT s.id, s.name, s.protocol, s.configuration, r.definition
		  FROM ingest.inputs s
		  LEFT JOIN ingest.mappings r ON r.id = s.mapping_id
		 WHERE s.enabled
		 ORDER BY s.name`)
	if err != nil {
		return nil, nil, fmt.Errorf("could not read sources: %w", err)
	}

	defer rows.Close()

	var (
		entries  []input.Entry
		problems []Problem
	)

	for rows.Next() {
		var (
			sourceID      string
			name          string
			protocol      string
			configuration []byte
			ruleRaw       []byte
		)

		if err = rows.Scan(&sourceID, &name, &protocol, &configuration, &ruleRaw); err != nil {
			return nil, nil, fmt.Errorf("could not read input row: %w", err)
		}

		// configuration 里是协议专属设置；解码失败视为数据损坏而整次加载失败。
		var decoded protocolConfiguration
		if err = json.Unmarshal(configuration, &decoded); err != nil {
			return nil, nil, fmt.Errorf("could not decode input configuration: %w", err)
		}

		var token []byte
		if decoded.TokenSHA256 != "" {
			if token, err = hex.DecodeString(decoded.TokenSHA256); err != nil {
				return nil, nil, fmt.Errorf("could not decode input token digest: %w", err)
			}
		}

		port := decoded.Port
		brokers := decoded.Brokers
		var topic *string
		if decoded.Topic != "" {
			value := decoded.Topic
			topic = &value
		}

		entry := input.Entry{
			ID:       sourceID,
			Name:     name,
			Protocol: input.Protocol(protocol),
			Kafka:    input.KafkaBinding{Brokers: brokers},
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
			engine, compileErr := compileMapping(ruleRaw)
			if compileErr != nil {
				// 不中断遍历：把所有问题一次报全，运维一轮就能修完。
				problems = append(problems, Problem{SourceID: sourceID, Reason: compileErr.Error()})

				continue
			}

			entry.Mapping = engine
		}

		entries = append(entries, entry)
	}

	if err = rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("could not read sources: %w", err)
	}

	return entries, problems, nil
}

// compileMapping 解析规则 JSON 并编译；失败时返回可读原因。
func compileMapping(raw []byte) (*normalizer.Compiled, error) {
	var spec mappingSpec
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
