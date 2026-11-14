// Package connector 定义通道所需的最小接入点视图与它的查询端口。
//
// 通道不认识数据库：它只拿到一份不可变的 Snapshot（接入点 + 已编译的解析规则），
// 因此通道实现可以完全用内存注册表测试，注册表从数据库加载是第五组的事。
package connector

import (
	"crypto/sha256"
	"sync"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
)

// Channel 是接入方式。取值与 connectors 表的 channel 字段一致。
type Channel string

const (
	ChannelHTTP   Channel = "http"   // 设备主动推送。
	ChannelSyslog Channel = "syslog" // UDP/TCP syslog 监听。
	ChannelKafka  Channel = "kafka"  // 消费客户已有总线。
)

// Snapshot 是通道运行期需要知道的全部接入点信息。
// 通道不改它，也不持有数据库连接。
type Snapshot struct {
	ID      string             // 接入点标识，事件里回填。
	Name    string             // 展示名称，仅用于日志与诊断。
	Channel Channel            // 接入方式。
	Engine  *normalizer.Engine // 已编译的解析规则；nil 表示"通用接入"，只做原文透传。
	Token   [sha256.Size]byte  // HTTP 通道的令牌摘要；空值表示该接入点不走 HTTP。
	Port    int                // syslog 通道的监听端口；0 表示未绑定。
	Kafka   KafkaBinding       // Kafka 消费绑定；仅 ChannelKafka 使用。
}

// KafkaBinding 描述要消费的外部总线。
type KafkaBinding struct {
	Brokers []string // 客户已有总线的 broker 列表。
	Topic   string   // 外部 topic 名。
}

// Usable 报告绑定是否完整。不完整的接入点不启动消费循环。
func (k KafkaBinding) Usable() bool {
	return len(k.Brokers) > 0 && k.Topic != ""
}

// HasToken 报告接入点是否配置了 HTTP 令牌。
func (s Snapshot) HasToken() bool {
	return s.Token != [sha256.Size]byte{}
}

// DigestToken 计算令牌摘要。
// 存储与比较都用摘要：数据库泄露不直接等于令牌泄露，比较也不受长度影响。
func DigestToken(token string) [sha256.Size]byte {
	return sha256.Sum256([]byte(token))
}

// Registry 是通道查询接入点的端口。
// 实现必须并发安全，且返回的 Snapshot 不得被调用方修改。
type Registry interface {
	// ByID 按标识查找接入点。
	ByID(id string) (Snapshot, bool)
	// ByTokenDigest 按令牌摘要查找接入点，供 HTTP 通道鉴权使用。
	ByTokenDigest(digest [sha256.Size]byte) (Snapshot, bool)
	// SyslogBindings 返回所有已绑定端口的 syslog 接入点，供热重载与监听器启动使用。
	SyslogBindings() []Snapshot
	// KafkaSources 返回所有绑定完整的 Kafka 消费接入点。
	KafkaSources() []Snapshot
}

// Memory 是内存注册表：测试与第五组数据库加载之前的占位实现。
// 加载责任在调用方，Memory 只提供并发安全的读。
type Memory struct {
	mu      sync.RWMutex
	byID    map[string]Snapshot
	byToken map[[sha256.Size]byte]string
}

// NewMemory 创建空注册表。
func NewMemory() *Memory {
	return &Memory{
		byID:    make(map[string]Snapshot),
		byToken: make(map[[sha256.Size]byte]string),
	}
}

// Replace 用给定集合整体替换注册表内容，供热重载使用。
// 整体替换而不是增量修改：一条接入点被删除时，旧令牌必须立刻失效。
func (m *Memory) Replace(snapshots []Snapshot) {
	byID := make(map[string]Snapshot, len(snapshots))
	byToken := make(map[[sha256.Size]byte]string, len(snapshots))

	for _, snapshot := range snapshots {
		byID[snapshot.ID] = snapshot

		if snapshot.HasToken() {
			byToken[snapshot.Token] = snapshot.ID
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.byID = byID
	m.byToken = byToken
}

// ByID 按标识查找接入点。
func (m *Memory) ByID(id string) (Snapshot, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot, ok := m.byID[id]
	return snapshot, ok
}

// ByTokenDigest 按令牌摘要查找接入点。
func (m *Memory) ByTokenDigest(digest [sha256.Size]byte) (Snapshot, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.byToken[digest]
	if !ok {
		return Snapshot{}, false
	}

	return m.byID[id], true
}

// SyslogBindings 返回已绑定端口的 syslog 接入点。
func (m *Memory) SyslogBindings() []Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bindings := make([]Snapshot, 0, len(m.byID))
	for _, snapshot := range m.byID {
		if snapshot.Channel == ChannelSyslog && snapshot.Port > 0 {
			bindings = append(bindings, snapshot)
		}
	}

	return bindings
}

// KafkaSources 返回绑定完整的 Kafka 接入点。
func (m *Memory) KafkaSources() []Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sources := make([]Snapshot, 0, len(m.byID))
	for _, snapshot := range m.byID {
		if snapshot.Channel == ChannelKafka && snapshot.Kafka.Usable() {
			sources = append(sources, snapshot)
		}
	}

	return sources
}
