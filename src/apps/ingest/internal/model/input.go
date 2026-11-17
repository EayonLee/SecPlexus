// Package model 定义 Ingest 管理面的 HTTP 数据形状。
//
// 与仓储层类型分开：HTTP 契约一旦发布就不能随内部重构改动。
package model

import (
	"encoding/json"
	"time"
)

// Input 是管理面返回的数据接入点。
// 令牌永不出现：创建时只返回一次明文，之后无从取回。
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

// CreateInputRequest 是创建接入点的请求体。
type CreateInputRequest struct {
	Name         string   `json:"name" binding:"required,min=1,max=100"`
	Protocol     string   `json:"protocol" binding:"required,oneof=http syslog kafka"`
	MappingID    *string  `json:"mapping_id"`
	SyslogPort   *int32   `json:"syslog_port"`
	KafkaBrokers []string `json:"kafka_brokers"`
	KafkaTopic   *string  `json:"kafka_topic"`
}

// CreateInputResponse 创建成功后返回一次明文令牌。
type CreateInputResponse struct {
	Input Input `json:"source"`
	// Token 只在本次响应里出现；服务端只保存摘要。
	Token string `json:"token,omitempty"`
}

// UpdateInputRequest 是修改接入点的请求体。
// 没有 protocol：改协议意味着换一条接入点。
type UpdateInputRequest struct {
	Name        *string `json:"name" binding:"omitempty,min=1,max=100"`
	MappingID   *string `json:"mapping_id"`
	UnbindRule  bool    `json:"unbind_rule"`
	Enabled     *bool   `json:"enabled"`
	RotateToken bool    `json:"rotate_token"`
}

// Mapping 是管理面返回的解析规则。
type Mapping struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Spec      json.RawMessage `json:"spec"`
	Version   int             `json:"version"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// MappingRequest 是创建或修改规则的请求体。
type MappingRequest struct {
	Name string          `json:"name" binding:"required,min=1,max=100"`
	Spec json.RawMessage `json:"spec" binding:"required"`
}
