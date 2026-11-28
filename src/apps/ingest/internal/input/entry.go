// Package input 定义通道所需的最小接入点视图与它的查询端口。
//
// 通道不认识数据库：它只拿到一份不可变的 Entry（接入点 + 已编译的解析规则），
// 仓储层从数据库加载快照，通道通过 Lookup 查询。
package input

import (
	"crypto/sha256"
	"regexp"
	"sync"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/enum"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
)

// Protocol 是接入方式。取值与 inputs 表的 protocol 字段一致。
type Protocol string

const (
	ProtocolHTTP Protocol = "http" // 设备主动推送。
	// ProtocolPull 是拉模式连接器：字节由我们主动取回，而不是设备送来。
	// 它不出现在 ingest.inputs 的协议闭集里——拉取任务有自己的表。
	ProtocolPull   Protocol = "pull"
	ProtocolSyslog Protocol = "syslog" // UDP/TCP syslog 监听。
	ProtocolKafka  Protocol = "kafka"  // 消费客户已有总线。
)

// Entry 是通道运行期需要知道的全部接入点信息。
// 通道不改它，也不持有数据库连接。
type Entry struct {
	Revision             int64 // 数据接入配置修订。
	CredentialGeneration int64 // 设备凭据代次。

	ParsingRuleID      string               // 实际编译使用的解析规则 ID。
	ParsingRuleVersion int                  // 实际编译使用的规则修订。
	Vendor             string               // 来源厂商，由接入配置提供。
	Product            string               // 来源产品，可独立于厂商配置。
	ID                 string               // 接入点标识，事件里回填。
	Name               string               // 展示名称，仅用于日志与诊断。
	Protocol           Protocol             // 接入方式。
	Mapping            *normalizer.Compiled // 已编译的解析规则；nil 表示未绑定。
	Token              [sha256.Size]byte    // HTTP 通道的令牌摘要；空值表示该接入点不走 HTTP。
	Port               int                  // syslog 通道的监听端口；0 表示未绑定。
	Kafka              KafkaBinding         // Kafka 消费绑定；仅 ProtocolKafka 使用。
}

// KafkaBinding 描述要消费的外部总线。
type KafkaBinding struct {
	Credential    string                  // 部署侧配置中的凭据名，空值表示明文连接。
	Brokers       []string                // 客户已有总线的 broker 列表。
	Topic         string                  // 外部 topic 名。
	StartPosition enum.KafkaStartPosition // 首次消费位置，有提交时沿用已提交位移。
	CreatedAt     time.Time               // 接入创建时间，用作固定的 Kafka 消息时间下界。
}

var kafkaCredentialName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// ValidKafkaCredential 验证数据接入使用的部署侧凭据名。
func ValidKafkaCredential(name string) bool { return kafkaCredentialName.MatchString(name) }

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

// Memory 是通道共享的不可变来源快照索引。
// 加载责任在调用方，Memory 只提供并发安全的读。
type Memory struct {
	mu      sync.RWMutex
	byID    map[string]Entry
	byToken map[[sha256.Size]byte]string
	loaded  bool
	expires time.Time
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
	m.ReplaceUntil(entries, time.Time{})
}

// ReplaceUntil 切换快照。有效期届满时，所有通道停止受理旧配置。
func (m *Memory) ReplaceUntil(entries []Entry, expires time.Time) {
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
	m.expires = expires
}

// Loaded 报告当前快照是否完成加载且仍有效，空集合也是有效快照。
func (m *Memory) Loaded() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.available()
}

// available 需要持有注册表的读锁或写锁。
func (m *Memory) available() bool {
	return m.loaded && (m.expires.IsZero() || time.Now().Before(m.expires))
}

// ByID 按标识查找接入点。
func (m *Memory) ByID(id string) (Entry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.available() {
		return Entry{}, false
	}

	entry, ok := m.byID[id]
	return entry, ok
}

// ByTokenDigest 按令牌摘要查找接入点。
func (m *Memory) ByTokenDigest(digest [sha256.Size]byte) (Entry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.available() {
		return Entry{}, false
	}

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
	if !m.available() {
		return nil
	}

	bindings := make([]Entry, 0, len(m.byID))
	for _, entry := range m.byID {
		if entry.Protocol == ProtocolSyslog && entry.Port > 0 {
			bindings = append(bindings, entry)
		}
	}

	return bindings
}

// KafkaSources 返回绑定完整的 Kafka 接入点。
func (m *Memory) KafkaSources() []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.available() {
		return nil
	}

	sources := make([]Entry, 0, len(m.byID))
	for _, entry := range m.byID {
		if entry.Protocol == ProtocolKafka && entry.Kafka.Usable() {
			sources = append(sources, entry)
		}
	}

	return sources
}
