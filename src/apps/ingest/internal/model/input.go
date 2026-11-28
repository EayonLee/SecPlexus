// Package model 定义 Ingest 管理面的 HTTP 数据形状。
//
// 与仓储层类型分开：HTTP 契约一旦发布就不能随内部重构改动。
package model

import (
	"encoding/json"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/enum"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/observation"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/validation"
)

// Input 是管理面返回的数据接入点。
// 令牌永不出现：创建时只返回一次明文，之后无从取回。
type Input struct {
	Vendor               string                  `json:"vendor"`
	Product              string                  `json:"product"`
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
	ObservationsReady    bool                    `json:"observations_ready"`
	Activation           *observation.Activation `json:"activation,omitempty"`
	Revision             int64                   `json:"revision"`
	CredentialGeneration int64                   `json:"credential_generation"`
	TrafficStatus        string                  `json:"traffic_status,omitempty"`
	Stats                *InputStats             `json:"stats,omitempty"`
}

// InputStats 是多个 Ingest 实例最近五个分钟桶的接收、解析和发布统计。
type InputStats struct {
	Parsed        uint64     `json:"parsed"`
	Partial       uint64     `json:"partial"`
	Unclassified  uint64     `json:"unclassified"`
	Received      uint64     `json:"received"`
	ParseFailed   uint64     `json:"parse_failed"`
	Confirmed     uint64     `json:"confirmed"`
	LastReceived  *time.Time `json:"last_received,omitempty"`
	LastConfirmed *time.Time `json:"last_confirmed,omitempty"`
}

// CreateInputRequest 是创建接入点的请求体。
type CreateInputRequest struct {
	ExpectedRuleVersion int32                   `json:"expected_rule_version" binding:"required_with=ParsingRuleID,omitempty,gt=0"`
	Vendor              string                  `json:"vendor" binding:"omitempty,max=120"`
	Product             string                  `json:"product" binding:"omitempty,max=120"`
	Name                string                  `json:"name" binding:"required,min=1,max=100"`
	Protocol            string                  `json:"protocol" binding:"required,oneof=http syslog kafka"`
	ParsingRuleID       *string                 `json:"parsing_rule_id"`
	SyslogPort          *int32                  `json:"syslog_port"`
	KafkaBrokers        []string                `json:"kafka_brokers"`
	KafkaCredential     string                  `json:"kafka_credential" binding:"omitempty,max=64"`
	KafkaTopic          *string                 `json:"kafka_topic"`
	KafkaStartPosition  enum.KafkaStartPosition `json:"kafka_start_position" binding:"omitempty,oneof=earliest since_created"`
}

// CreateInputResponse 创建成功后返回一次明文令牌。
type CreateInputResponse struct {
	Input Input `json:"input"`
	// Token 只在本次响应里出现；服务端只保存摘要。
	Token string `json:"token,omitempty"`
}

// UpdateInputRequest 是修改接入点的请求体。
// 没有 protocol：改协议意味着换一条接入点。
type UpdateInputRequest struct {
	ExpectedRuleVersion int32   `json:"expected_rule_version" binding:"required_with=ParsingRuleID,omitempty,gt=0"`
	Vendor              *string `json:"vendor" binding:"omitempty,max=120"`
	Product             *string `json:"product" binding:"omitempty,max=120"`
	Name                *string `json:"name" binding:"omitempty,min=1,max=100"`
	ParsingRuleID       *string `json:"parsing_rule_id"`
	UnbindRule          bool    `json:"unbind_rule"`
	Enabled             *bool   `json:"enabled"`
	RotateToken         bool    `json:"rotate_token"`
}

// Mapping 是管理面返回的解析规则。
type Mapping struct {
	Verification *validation.RuleVerification `json:"verification,omitempty"`
	ClassUIDs    []int32                      `json:"class_uids,omitempty"`
	Description  string                       `json:"description,omitempty"`
	ParseType    string                       `json:"parse_type,omitempty"`
	Vendor       string                       `json:"vendor"`
	Product      string                       `json:"product"`
	ID           string                       `json:"id"`
	Name         string                       `json:"name"`
	Definition   json.RawMessage              `json:"definition,omitempty"`
	Version      int                          `json:"version"`
	Builtin      bool                         `json:"builtin"`
	CreatedAt    time.Time                    `json:"created_at"`
	UpdatedAt    time.Time                    `json:"updated_at"`
}

// MappingRequest 是创建或修改规则的请求体。
type MappingRequest struct {
	ExpectedVersion int32           `json:"expected_version" binding:"omitempty,gt=0"`
	Vendor          string          `json:"vendor"`
	Product         string          `json:"product"`
	Name            string          `json:"name"`
	Definition      json.RawMessage `json:"definition" binding:"required"`
	Samples         []string        `json:"samples"`
}

// MappingRevision 是发布历史的只读投影。列表省略定义。
type MappingRevision struct {
	ParsingRuleID string                       `json:"parsing_rule_id"`
	Version       int32                        `json:"version"`
	Name          string                       `json:"name"`
	Vendor        string                       `json:"vendor"`
	Product       string                       `json:"product"`
	AuthorID      *string                      `json:"author_id,omitempty"`
	RecordedAt    time.Time                    `json:"recorded_at"`
	Verification  *validation.RuleVerification `json:"verification,omitempty"`
	Definition    json.RawMessage              `json:"definition,omitempty"`
}
