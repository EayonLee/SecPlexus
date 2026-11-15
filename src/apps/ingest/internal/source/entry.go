// Package source 定义通道所需的最小接入点视图与它的查询端口。
//
// 通道不认识数据库：它只拿到一份不可变的 Entry（接入点 + 已编译的解析规则），
// 因此通道实现可以完全用内存注册表测试，注册表从数据库加载是第五组的事。
package source

import (
	"crypto/sha256"
	"sync"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
)

// Transport 是接入方式。取值与 connectors 表的 channel 字段一致。
type Transport string

const (
	TransportHTTP   Transport = "http"   // 设备主动推送。
	TransportSyslog Transport = "syslog" // UDP/TCP syslog 监听。
	TransportKafka  Transport = "kafka"  // 消费客户已有总线。
)

// Entry 是通道运行期需要知道的全部接入点信息。
// 通道不改它，也不持有数据库连接。
type Entry struct {
	ID        string             // 接入点标识，事件里回填。
	Name      string             // 展示名称，仅用于日志与诊断。
	Transport Transport          // 接入方式。
	Engine    *normalizer.Engine // 已编译的解析规则；nil 表示"通用接入"，只做原文透传。
	Token     [sha256.Size]byte  // HTTP 通道的令牌摘要；空值表示该接入点不走 HTTP。
	Port      int                // syslog 通道的监听端口；0 表示未绑定。
	Kafka     KafkaBinding       // Kafka 消费绑定；仅 TransportKafka 使用。
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
func (s Entry) HasToken() bool {
	return s.Token != [sha256.Size]byte{}
}

// DigestToken 计算令牌摘要。
// 存储与比较都用摘要：数据库泄露不直接等于令牌泄露，比较也不受长度影响。
func DigestToken(token string) [sha256.Size]byte {
	return sha256.Sum256([]byte(token))
}

// Lookup 是通道查询接入点的端口。
// 实现必须并发安全，且返回的 Entry 不得被调用方修改。
type Lookup interface {
	// ByID 按标识查找接入点。
	ByID(id string) (Entry, bool)
	// ByTokenDigest 按令牌摘要查找接入点，供 HTTP 通道鉴权使用。
	ByTokenDigest(digest [sha256.Size]byte) (Entry, bool)
	// SyslogBindings 返回所有已绑定端口的 syslog 接入点，供热重载与监听器启动使用。
	SyslogBindings() []Entry
	// KafkaSources 返回所有绑定完整的 Kafka 消费接入点。
	KafkaSources() []Entry
}

// Memory 是内存注册表：测试与第五组数据库加载之前的占位实现。
// 加载责任在调用方，Memory 只提供并发安全的读。
type Memory struct {
	mu      sync.RWMutex
	byID    map[string]Entry
	byToken map[[sha256.Size]byte]string
	loaded  bool
}

// NewMemory 创建空注册表。
func NewMemory() *Memory {
	return &Memory{
		byID:    make(map[string]Entry),
		byToken: make(map[[sha256.Size]byte]string),
	}
}

// Replace 用给定集合整体替换注册表内容，供热重载使用。
// 整体替换而不是增量修改：一条接入点被删除时，旧令牌必须立刻失效。
func (m *Memory) Replace(entries []Entry) {
	byID := make(map[string]Entry, len(entries))
	byToken := make(map[[sha256.Size]byte]string, len(entries))

	for _, entry := range entries {
		byID[entry.ID] = entry

		if entry.HasToken() {
			byToken[entry.Token] = entry.ID
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.byID = byID
	m.byToken = byToken
	m.loaded = true
}

// Loaded 报告注册表是否至少被加载过一次。
// 与"当前是否为空"不同：加载完成但确实没有接入点，也算已加载。
func (m *Memory) Loaded() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.loaded
}

// ByID 按标识查找接入点。
func (m *Memory) ByID(id string) (Entry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.byID[id]
	return entry, ok
}

// ByTokenDigest 按令牌摘要查找接入点。
func (m *Memory) ByTokenDigest(digest [sha256.Size]byte) (Entry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, ok := m.byToken[digest]
	if !ok {
		return Entry{}, false
	}

	return m.byID[id], true
}

// SyslogBindings 返回已绑定端口的 syslog 接入点。
func (m *Memory) SyslogBindings() []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bindings := make([]Entry, 0, len(m.byID))
	for _, entry := range m.byID {
		if entry.Transport == TransportSyslog && entry.Port > 0 {
			bindings = append(bindings, entry)
		}
	}

	return bindings
}

// KafkaSources 返回绑定完整的 Kafka 接入点。
func (m *Memory) KafkaSources() []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sources := make([]Entry, 0, len(m.byID))
	for _, entry := range m.byID {
		if entry.Transport == TransportKafka && entry.Kafka.Usable() {
			sources = append(sources, entry)
		}
	}

	return sources
}
