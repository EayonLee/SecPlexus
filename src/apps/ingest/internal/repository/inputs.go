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

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository/sqlcgen"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/enum"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
)

// 仓储层的错误分类：上层据此映射 HTTP 状态，而不是从数据库错误里猜字符串。
var (
	ErrNotFound           = errors.New("not found")
	ErrDuplicateName      = errors.New("name already exists")
	ErrPortTaken          = errors.New("syslog port already in use")
	ErrMappingInUse       = errors.New("a data input uses this parsing rule")
	ErrInvalidCombination = errors.New("fields do not match the protocol")
	ErrTransportImmutable = errors.New("protocol cannot be changed")
)

// Input 是管理面看到的数据接入点。
//
// 与运行期的 input.Entry 分开：管理面需要名称、启用状态、时间戳，
// 运行期只关心路由与凭据；两者混用会让 Entry 承担展示职责。
type Input struct {
	Revision             int64
	CredentialGeneration int64
	ParsingRuleVersion   int
	Vendor               string
	Product              string
	ID                   string                  `json:"id"`
	Name                 string                  `json:"name"`
	Protocol             string                  `json:"protocol"`
	ParsingRuleID        *string                 `json:"parsing_rule_id"`
	SyslogPort           *int32                  `json:"syslog_port"`
	KafkaBrokers         []string                `json:"kafka_brokers"`
	KafkaCredential      string                  `json:"kafka_credential,omitempty"`
	KafkaTopic           *string                 `json:"kafka_topic"`
	KafkaStartPosition   enum.KafkaStartPosition `json:"kafka_start_position,omitempty"`
	Enabled              bool                    `json:"enabled"`
	CreatedAt            time.Time               `json:"created_at"`
	UpdatedAt            time.Time               `json:"updated_at"`
}

// protocolConfiguration 是 inputs.configuration 的 JSON 形态。
//
// 为什么不是固定列：加一种协议就要改表、改约束、改索引。
// 协议专属设置放在这里，形状由 Go 侧按 protocol 校验。
type protocolConfiguration struct {
	Credential    string                  `json:"credential,omitempty"`
	TokenSHA256   string                  `json:"token_sha256,omitempty"`
	Port          *int32                  `json:"port,omitempty"`
	Brokers       []string                `json:"brokers,omitempty"`
	Topic         string                  `json:"topic,omitempty"`
	StartPosition enum.KafkaStartPosition `json:"start_position,omitempty"`
}

// validateConfiguration 按协议校验必填设置。
//
// 改成 configuration jsonb 后，数据库无法再用 CHECK 表达"http 必须有令牌、
// syslog 必须有端口、kafka 必须有 brokers 与 topic"；校验因此上移到用例层，
// 校验随协议走：每种协议各自声明必填项。
func validateConfiguration(params CreateInputParams) error {
	if params.Protocol != string(input.ProtocolKafka) && (params.KafkaStartPosition != "" || params.KafkaCredential != "") {
		return ErrInvalidCombination
	}
	switch params.Protocol {
	case string(input.ProtocolHTTP):
		return nil
	case string(input.ProtocolSyslog):
		if params.SyslogPort == nil {
			return ErrInvalidCombination
		}

		return nil
	case string(input.ProtocolKafka):
		if len(params.KafkaBrokers) == 0 || params.KafkaTopic == nil || *params.KafkaTopic == "" || !params.KafkaStartPosition.Valid() ||
			(params.KafkaCredential != "" && !input.ValidKafkaCredential(params.KafkaCredential)) {
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
		configuration.Credential = params.KafkaCredential
		configuration.StartPosition = params.KafkaStartPosition
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

// decodeInput 展开协议连接配置，数据库字段不会向管理面暴露令牌摘要。
func decodeInput(row sqlcgen.IngestInput) (Input, error) {
	item := Input{ID: row.ID, Name: row.Name, Vendor: row.Vendor, Product: row.Product, Protocol: row.Protocol, ParsingRuleID: row.ParsingRuleID,
		Revision: row.Revision, CredentialGeneration: row.CredentialGeneration, Enabled: row.Enabled, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	var cfg protocolConfiguration
	if err := json.Unmarshal(row.Configuration, &cfg); err != nil {
		return item, fmt.Errorf("could not decode input configuration: %w", err)
	}
	item.SyslogPort, item.KafkaBrokers = cfg.Port, cfg.Brokers
	item.KafkaStartPosition = cfg.StartPosition
	item.KafkaCredential = cfg.Credential
	if cfg.Topic != "" {
		item.KafkaTopic = &cfg.Topic
	}
	return item, nil
}

// CreateInputParams 是创建接入点的入参。
type CreateInputParams struct {
	ExpectedRuleVersion int32
	Vendor              string
	Product             string
	Name                string                  `json:"name"`
	Protocol            string                  `json:"protocol"`
	ParsingRuleID       *string                 `json:"parsing_rule_id"`
	SyslogPort          *int32                  `json:"syslog_port"`
	KafkaBrokers        []string                `json:"kafka_brokers"`
	KafkaCredential     string                  `json:"kafka_credential"`
	KafkaTopic          *string                 `json:"kafka_topic"`
	KafkaStartPosition  enum.KafkaStartPosition `json:"kafka_start_position"`
}

// CreateInput 创建接入点；http 传输会同时生成令牌，明文只在本函数返回时出现一次。
func CreateInput(ctx context.Context, pool writer, params CreateInputParams) (Input, string, error) {
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

	queries := sqlcgen.New(pool)
	if err := checkInputRuleVersion(ctx, queries, params.ParsingRuleID, params.ExpectedRuleVersion); err != nil {
		return Input{}, "", err
	}
	row, err := queries.CreateInput(ctx, sqlcgen.CreateInputParams{
		Name: params.Name, Vendor: params.Vendor, Product: params.Product, Protocol: params.Protocol, ParsingRuleID: params.ParsingRuleID, Configuration: configuration})
	if err != nil {
		return Input{}, "", classify(err)
	}
	created, err := decodeInput(row)
	return created, plaintext, err
}

// UpdateInputParams 是修改接入点的入参；nil 表示不改。
//
// Transport 不在入参里：改协议意味着换一条接入点，这是刻意留不出的能力。
type UpdateInputParams struct {
	ExpectedRuleVersion int32
	Vendor              *string
	Product             *string
	Name                *string  `json:"name"`
	ParsingRuleID       **string `json:"parsing_rule_id"`
	Enabled             *bool    `json:"enabled"`
	RotateToken         bool     `json:"rotate_token"`
}

// UpdateInput 修改接入点；rotate_token 为真时换发新令牌并返回明文。
func UpdateInput(ctx context.Context, pool writer, id string, params UpdateInputParams) (Input, string, error) {
	var token string
	var digest *string
	if params.RotateToken {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return Input{}, "", errors.New("could not generate token")
		}

		token = "spx_" + base64.RawURLEncoding.EncodeToString(raw)
		sum := sha256.Sum256([]byte(token))
		encoded := hex.EncodeToString(sum[:])
		digest = &encoded
	}

	queries := sqlcgen.New(pool)
	if err := checkInputRuleVersion(ctx, queries, deref(params.ParsingRuleID), params.ExpectedRuleVersion); err != nil {
		return Input{}, "", err
	}
	row, err := queries.UpdateInput(ctx, sqlcgen.UpdateInputParams{
		ID: id, Name: params.Name, Vendor: params.Vendor, Product: params.Product,
		Enabled: params.Enabled, ChangeMapping: params.ParsingRuleID != nil,
		ParsingRuleID: deref(params.ParsingRuleID), Digest: digest,
	})
	if err != nil {
		return Input{}, "", classify(err)
	}

	updated, err := decodeInput(row)
	if err != nil {
		return Input{}, "", err
	}
	return updated, token, nil
}

// checkInputRuleVersion 将试算所用修订锁定到接入变更提交。
func checkInputRuleVersion(ctx context.Context, queries *sqlcgen.Queries, id *string, expected int32) error {
	if expected == 0 {
		return nil
	}
	if id == nil {
		return ErrInvalidCombination
	}
	version, err := queries.LockInputRuleVersion(ctx, *id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && version != expected) {
		return ErrParsingRuleVersion
	}
	if err != nil {
		return classify(err)
	}
	return nil
}

// DeleteInput 物理删除接入点；运行期下一次刷新即释放端口。
func DeleteInput(ctx context.Context, pool writer, id string) error {
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
		switch pgErr.ConstraintName {
		case "inputs_syslog_port_key":
			return ErrPortTaken
		case "inputs_name_normalized_key", "parsing_rules_name_normalized_key":
			return ErrDuplicateName
		}
	case "23503":
		return ErrMappingInUse
	case "23514":
		return ErrInvalidCombination
	}

	return fmt.Errorf("database error: %w", err)
}

// CreateInput 创建接入点。
func (s *AdminStore) CreateInput(ctx context.Context, params CreateInputParams) (input Input, token string, err error) {
	err = s.write(ctx, func(tx pgx.Tx) (string, error) {
		var writeErr error
		input, token, writeErr = CreateInput(ctx, tx, params)
		return input.ID, writeErr
	})
	return
}

// UpdateInput 修改接入点。
func (s *AdminStore) UpdateInput(ctx context.Context, id string, params UpdateInputParams) (input Input, token string, err error) {
	err = s.write(ctx, func(tx pgx.Tx) (string, error) {
		var writeErr error
		input, token, writeErr = UpdateInput(ctx, tx, id, params)
		return input.ID, writeErr
	})
	return
}

// DeleteInput 删除接入点。
func (s *AdminStore) DeleteInput(ctx context.Context, id string) error {
	return s.write(ctx, func(tx pgx.Tx) (string, error) { return id, DeleteInput(ctx, tx, id) })
}
