package repository

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
)

// 仓储层的错误分类：上层据此映射 HTTP 状态，而不是从数据库错误里猜字符串。
var (
	ErrNotFound           = errors.New("not found")
	ErrDuplicateName      = errors.New("name already exists")
	ErrPortTaken          = errors.New("syslog port already in use")
	ErrMappingReferenced  = errors.New("parse mapping is still referenced by an input")
	ErrInvalidCombination = errors.New("fields do not match the protocol")
	ErrTransportImmutable = errors.New("protocol cannot be changed")
)

// Input 是管理面看到的数据接入点。
//
// 与运行期的 input.Entry 分开：管理面需要名称、启用状态、时间戳，
// 运行期只关心路由与凭据；两者混用会让 Entry 承担展示职责。
type Input struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Protocol     string    `json:"protocol"`
	MappingID    *string   `json:"mapping_id"`
	SyslogPort   *int32    `json:"syslog_port"`
	KafkaBrokers []string  `json:"kafka_brokers"`
	KafkaTopic   *string   `json:"kafka_topic"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// protocolConfiguration 是 inputs.configuration 的 JSON 形态。
//
// 为什么不是固定列：加一种协议就要改表、改约束、改索引。
// 协议专属设置放在这里，形状由 Go 侧按 protocol 校验。
type protocolConfiguration struct {
	TokenSHA256 string   `json:"token_sha256,omitempty"`
	Port        *int32   `json:"port,omitempty"`
	Brokers     []string `json:"brokers,omitempty"`
	Topic       string   `json:"topic,omitempty"`
}

// validateConfiguration 按协议校验必填设置。
//
// 改成 configuration jsonb 后，数据库无法再用 CHECK 表达"http 必须有令牌、
// syslog 必须有端口、kafka 必须有 brokers 与 topic"；校验因此上移到用例层，
// 校验随协议走：每种协议各自声明必填项。
func validateConfiguration(params CreateInputParams) error {
	switch params.Protocol {
	case string(input.ProtocolHTTP):
		return nil
	case string(input.ProtocolSyslog):
		if params.SyslogPort == nil {
			return ErrInvalidCombination
		}

		return nil
	case string(input.ProtocolKafka):
		if len(params.KafkaBrokers) == 0 || params.KafkaTopic == nil || *params.KafkaTopic == "" {
			return ErrInvalidCombination
		}

		return nil
	default:
		return ErrInvalidCombination
	}
}

// encodeConfiguration 把入参摊成 JSON 字节；nil 表示空配置。
func encodeConfiguration(params CreateInputParams, digest []byte) ([]byte, error) {
	configuration := protocolConfiguration{}

	if len(digest) > 0 {
		configuration.TokenSHA256 = hex.EncodeToString(digest)
	}

	switch params.Protocol {
	case string(input.ProtocolSyslog):
		configuration.Port = params.SyslogPort
	case string(input.ProtocolKafka):
		configuration.Brokers = params.KafkaBrokers
		if params.KafkaTopic != nil {
			configuration.Topic = *params.KafkaTopic
		}
	}

	encoded, err := json.Marshal(configuration)
	if err != nil {
		return nil, fmt.Errorf("could not encode input configuration: %w", err)
	}

	return encoded, nil
}

// decodeConfiguration 把 JSON 摊回管理面字段；解码失败视为数据损坏。
func decodeConfiguration(raw []byte, item *Input) error {
	var configuration protocolConfiguration
	if err := json.Unmarshal(raw, &configuration); err != nil {
		return fmt.Errorf("could not decode input configuration: %w", err)
	}

	item.SyslogPort = configuration.Port
	item.KafkaBrokers = configuration.Brokers
	if configuration.Topic != "" {
		topic := configuration.Topic
		item.KafkaTopic = &topic
	}

	return nil
}

// CreateInputParams 是创建接入点的入参。
type CreateInputParams struct {
	Name         string   `json:"name"`
	Protocol     string   `json:"protocol"`
	MappingID    *string  `json:"mapping_id"`
	SyslogPort   *int32   `json:"syslog_port"`
	KafkaBrokers []string `json:"kafka_brokers"`
	KafkaTopic   *string  `json:"kafka_topic"`
}

// CreateInput 创建接入点；http 传输会同时生成令牌，明文只在本函数返回时出现一次。
func CreateInput(ctx context.Context, pool *pgxpool.Pool, params CreateInputParams) (Input, string, error) {
	var (
		digest    []byte
		plaintext string
	)

	if params.Protocol == string(input.ProtocolHTTP) {
		// 32 字节随机量，base64url 后约 43 字符；库里只存摘要。
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return Input{}, "", errors.New("could not generate token")
		}

		plaintext = "spx_" + base64.RawURLEncoding.EncodeToString(raw)
		sum := sha256.Sum256([]byte(plaintext))
		digest = sum[:]
	}

	if err := validateConfiguration(params); err != nil {
		return Input{}, "", err
	}

	configuration, err := encodeConfiguration(params, digest)
	if err != nil {
		return Input{}, "", err
	}

	var created Input
	var raw []byte

	err = pool.QueryRow(ctx, `
		INSERT INTO ingest.inputs (id, name, protocol, mapping_id, configuration)
		VALUES (gen_random_uuid(), $1, $2, $3, $4::jsonb)
		RETURNING id, name, protocol, mapping_id, configuration, enabled, created_at, updated_at`,
		params.Name, params.Protocol, params.MappingID, configuration,
	).Scan(&created.ID, &created.Name, &created.Protocol, &created.MappingID, &raw,
		&created.Enabled, &created.CreatedAt, &created.UpdatedAt)
	if err != nil {
		return Input{}, "", classify(err)
	}

	if err = decodeConfiguration(raw, &created); err != nil {
		return Input{}, "", err
	}

	return created, plaintext, nil
}

// ListInputs 按名称排序列出全部接入点。
func ListInputs(ctx context.Context, pool *pgxpool.Pool) ([]Input, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, name, protocol, mapping_id, configuration, enabled, created_at, updated_at
		  FROM ingest.inputs ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("could not list inputs: %w", err)
	}

	defer rows.Close()

	items := make([]Input, 0, 8)
	for rows.Next() {
		var item Input
		var raw []byte

		if err = rows.Scan(&item.ID, &item.Name, &item.Protocol, &item.MappingID, &raw,
			&item.Enabled, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("could not read input row: %w", err)
		}

		if err = decodeConfiguration(raw, &item); err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return items, rows.Err()
}

// UpdateInputParams 是修改接入点的入参；nil 表示不改。
//
// Transport 不在入参里：改协议意味着换一条接入点，这是刻意留不出的能力。
type UpdateInputParams struct {
	Name        *string  `json:"name"`
	MappingID   **string `json:"mapping_id"`
	Enabled     *bool    `json:"enabled"`
	RotateToken bool     `json:"rotate_token"`
}

// UpdateInput 修改接入点；rotate_token 为真时换发新令牌并返回明文。
func UpdateInput(ctx context.Context, pool *pgxpool.Pool, id string, params UpdateInputParams) (Input, string, error) {
	var rotatedDigest []byte
	if params.RotateToken {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return Input{}, "", errors.New("could not generate token")
		}

		generated := "spx_" + base64.RawURLEncoding.EncodeToString(raw)
		sum := sha256.Sum256([]byte(generated))
		rotatedDigest = sum[:]

		var updated Input

		err := pool.QueryRow(ctx, `
			UPDATE ingest.inputs
			   SET configuration = jsonb_set(configuration, '{token_sha256}', to_jsonb($2::text)),
			       updated_at = now()
			 WHERE id = $1 AND protocol = 'http'
			RETURNING id, name, protocol, mapping_id, configuration, enabled, created_at, updated_at`,
			id, hex.EncodeToString(rotatedDigest)).Scan(&updated.ID, &updated.Name, &updated.Protocol,
			&updated.MappingID, &raw, &updated.Enabled, &updated.CreatedAt, &updated.UpdatedAt)
		if err != nil {
			return Input{}, "", classify(err)
		}

		if err = decodeConfiguration(raw, &updated); err != nil {
			return Input{}, "", err
		}

		return updated, generated, nil
	}

	var updated Input
	var raw []byte

	err := pool.QueryRow(ctx, `
		UPDATE ingest.inputs SET
		    name = COALESCE($2, name),
		    mapping_id = CASE WHEN $3 THEN $4::uuid ELSE mapping_id END,
		    enabled = COALESCE($5, enabled),
		    updated_at = now()
		 WHERE id = $1
		RETURNING id, name, protocol, mapping_id, configuration, enabled, created_at, updated_at`,
		id, params.Name, params.MappingID != nil, deref(params.MappingID), params.Enabled,
	).Scan(&updated.ID, &updated.Name, &updated.Protocol, &updated.MappingID, &raw,
		&updated.Enabled, &updated.CreatedAt, &updated.UpdatedAt)
	if err != nil {
		return Input{}, "", classify(err)
	}

	if err = decodeConfiguration(raw, &updated); err != nil {
		return Input{}, "", err
	}

	return updated, "", nil
}

// DeleteInput 物理删除接入点；运行期下一次刷新即释放端口。
func DeleteInput(ctx context.Context, pool *pgxpool.Pool, id string) error {
	tag, err := pool.Exec(ctx, `DELETE FROM ingest.inputs WHERE id = $1`, id)
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
		if pgErr.ConstraintName == "inputs_syslog_port_key" {
			return ErrPortTaken
		}

		return ErrDuplicateName
	case "23503":
		return ErrMappingReferenced
	case "23514":
		return ErrInvalidCombination
	}

	return fmt.Errorf("database error: %w", err)
}
