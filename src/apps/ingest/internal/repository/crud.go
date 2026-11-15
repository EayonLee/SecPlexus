package repository

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/source"
)

// 仓储层的错误分类：上层据此映射 HTTP 状态，而不是从数据库错误里猜字符串。
var (
	ErrNotFound           = errors.New("not found")
	ErrDuplicateName      = errors.New("name already exists")
	ErrPortTaken          = errors.New("syslog port already in use")
	ErrRuleReferenced     = errors.New("parse rule is still referenced by a source")
	ErrInvalidCombination = errors.New("fields do not match the transport")
	ErrTransportImmutable = errors.New("transport cannot be changed")
)

// Source 是管理面看到的接入点。
//
// 与运行期的 source.Entry 分开：管理面需要名称、启用状态、时间戳，
// 运行期只关心路由与凭据；两者混用会让 Entry 承担展示职责。
type Source struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Transport    string    `json:"transport"`
	ParseRuleID  *string   `json:"parse_rule_id"`
	SyslogPort   *int32    `json:"syslog_port"`
	KafkaBrokers []string  `json:"kafka_brokers"`
	KafkaTopic   *string   `json:"kafka_topic"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreateSourceInput 是创建接入点的入参。
type CreateSourceInput struct {
	Name         string   `json:"name"`
	Transport    string   `json:"transport"`
	ParseRuleID  *string  `json:"parse_rule_id"`
	SyslogPort   *int32   `json:"syslog_port"`
	KafkaBrokers []string `json:"kafka_brokers"`
	KafkaTopic   *string  `json:"kafka_topic"`
}

// CreateSource 创建接入点；http 传输会同时生成令牌，明文只在本函数返回时出现一次。
func CreateSource(ctx context.Context, pool *pgxpool.Pool, input CreateSourceInput) (Source, string, error) {
	var (
		digest    []byte
		plaintext string
	)

	if input.Transport == string(source.TransportHTTP) {
		// 32 字节随机量，base64url 后约 43 字符；库里只存摘要。
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return Source{}, "", errors.New("could not generate token")
		}

		plaintext = "spx_" + base64.RawURLEncoding.EncodeToString(raw)
		sum := sha256.Sum256([]byte(plaintext))
		digest = sum[:]
	}

	var created Source
	err := pool.QueryRow(ctx, `
		INSERT INTO ingest.sources (id, name, transport, parse_rule_id, token_digest, syslog_port, kafka_brokers, kafka_topic)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7)
		RETURNING id, name, transport, parse_rule_id, syslog_port, kafka_brokers, kafka_topic, enabled, created_at, updated_at`,
		input.Name, input.Transport, input.ParseRuleID, digest, input.SyslogPort, input.KafkaBrokers, input.KafkaTopic,
	).Scan(&created.ID, &created.Name, &created.Transport, &created.ParseRuleID, &created.SyslogPort,
		&created.KafkaBrokers, &created.KafkaTopic, &created.Enabled, &created.CreatedAt, &created.UpdatedAt)
	if err != nil {
		return Source{}, "", classify(err)
	}

	return created, plaintext, nil
}

// ListSources 按名称排序列出全部接入点。
func ListSources(ctx context.Context, pool *pgxpool.Pool) ([]Source, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, name, transport, parse_rule_id, syslog_port, kafka_brokers, kafka_topic, enabled, created_at, updated_at
		  FROM ingest.sources ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("could not list sources: %w", err)
	}

	defer rows.Close()

	items := make([]Source, 0, 8)
	for rows.Next() {
		var item Source
		if err = rows.Scan(&item.ID, &item.Name, &item.Transport, &item.ParseRuleID, &item.SyslogPort,
			&item.KafkaBrokers, &item.KafkaTopic, &item.Enabled, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("could not read source row: %w", err)
		}

		items = append(items, item)
	}

	return items, rows.Err()
}

// UpdateSourceInput 是修改接入点的入参；nil 表示不改。
//
// Transport 不在入参里：改协议意味着换一条接入点，这是刻意留不出的能力。
type UpdateSourceInput struct {
	Name        *string  `json:"name"`
	ParseRuleID **string `json:"parse_rule_id"`
	Enabled     *bool    `json:"enabled"`
	RotateToken bool     `json:"rotate_token"`
}

// UpdateSource 修改接入点；rotate_token 为真时换发新令牌并返回明文。
func UpdateSource(ctx context.Context, pool *pgxpool.Pool, id string, input UpdateSourceInput) (Source, string, error) {
	var rotatedDigest []byte
	if input.RotateToken {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return Source{}, "", errors.New("could not generate token")
		}

		generated := "spx_" + base64.RawURLEncoding.EncodeToString(raw)
		sum := sha256.Sum256([]byte(generated))
		rotatedDigest = sum[:]

		var updated Source
		err := pool.QueryRow(ctx, `
			UPDATE ingest.sources SET token_digest = $2, updated_at = now()
			 WHERE id = $1 AND transport = 'http'
			RETURNING id, name, transport, parse_rule_id, syslog_port, kafka_brokers, kafka_topic, enabled, created_at, updated_at`,
			id, rotatedDigest).Scan(&updated.ID, &updated.Name, &updated.Transport, &updated.ParseRuleID,
			&updated.SyslogPort, &updated.KafkaBrokers, &updated.KafkaTopic, &updated.Enabled,
			&updated.CreatedAt, &updated.UpdatedAt)
		if err != nil {
			return Source{}, "", classify(err)
		}

		return updated, generated, nil
	}

	var updated Source
	err := pool.QueryRow(ctx, `
		UPDATE ingest.sources SET
		    name = COALESCE($2, name),
		    parse_rule_id = CASE WHEN $3 THEN $4::uuid ELSE parse_rule_id END,
		    enabled = COALESCE($5, enabled),
		    updated_at = now()
		 WHERE id = $1
		RETURNING id, name, transport, parse_rule_id, syslog_port, kafka_brokers, kafka_topic, enabled, created_at, updated_at`,
		id, input.Name, input.ParseRuleID != nil, deref(input.ParseRuleID), input.Enabled,
	).Scan(&updated.ID, &updated.Name, &updated.Transport, &updated.ParseRuleID, &updated.SyslogPort,
		&updated.KafkaBrokers, &updated.KafkaTopic, &updated.Enabled, &updated.CreatedAt, &updated.UpdatedAt)
	if err != nil {
		return Source{}, "", classify(err)
	}

	return updated, "", nil
}

// DeleteSource 物理删除接入点；运行期下一次刷新即释放端口。
func DeleteSource(ctx context.Context, pool *pgxpool.Pool, id string) error {
	tag, err := pool.Exec(ctx, `DELETE FROM ingest.sources WHERE id = $1`, id)
	if err != nil {
		return classify(err)
	}

	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// deref 把二级指针摊平成可空参数。
func deref(value **string) *string {
	if value == nil {
		return nil
	}

	return *value
}

// classify 把数据库错误翻成仓储层错误。
// SQLSTATE 是稳定契约，比匹配错误文本可靠。
func classify(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return fmt.Errorf("database error: %w", err)
	}

	switch pgErr.Code {
	case "23505":
		if pgErr.ConstraintName == "sources_syslog_port_key" {
			return ErrPortTaken
		}

		return ErrDuplicateName
	case "23503":
		return ErrRuleReferenced
	case "23514":
		return ErrInvalidCombination
	}

	return fmt.Errorf("database error: %w", err)
}
