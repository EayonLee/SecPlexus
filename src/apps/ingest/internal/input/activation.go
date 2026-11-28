package input

// KafkaPositionUnavailable 表示 Kafka 位移或主题身份需要人工核对。
const KafkaPositionUnavailable = "kafka_position_unavailable"

// SourceVersion 是数据接入的协议配置、设备凭据和解析规则的期望状态。
type SourceVersion struct {
	Revision             int64 `json:"revision"`
	CredentialGeneration int64 `json:"credential_generation"`
	ParsingRuleVersion   int   `json:"parsing_rule_version"`
	Enabled              bool  `json:"enabled"`
}

// SourceActivation 是一个实例对指定配置的确认和执行阶段。
type SourceActivation struct {
	SourceVersion
	Online   bool   `json:"online"`
	Draining bool   `json:"draining,omitempty"`
	Problem  string `json:"problem,omitempty"`
}

// ChannelActivation 是当前实例的协议运行状态。排空旧配置时尚未完成应用。
type ChannelActivation struct {
	Online   bool
	Draining bool
	Problem  string
}

// Snapshot 是仓储加载的一次权威配置，包括运行条目和各接入的版本。
type Snapshot struct {
	Entries []Entry
	Targets map[string]SourceActivation
}
