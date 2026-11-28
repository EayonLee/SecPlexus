package handler

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

// Metrics 以指标采集通用文本格式暴露流水线计数器。
//
// 手写 exposition 格式而不是引入客户端库：指标集很小，
// 而多一个依赖就多一份版本与初始化顺序的负担。
// 该端点应仅在内网可达；对外暴露由反向代理拦截。
func (s *Server) Metrics(c *gin.Context) {
	if s.Stats == nil {
		c.Status(http.StatusNotFound)

		return
	}

	stats := s.Stats()

	c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	w := c.Writer
	writeCounter(w, "spx_ingest_accepted_total", "事件受理总数", stats.Accepted)
	writeCounter(w, "spx_ingest_parsed_total", "解析成功总数", stats.Parsed)
	writeCounter(w, "spx_ingest_parse_failed_total", "解析失败总数", stats.ParseFailed)
	writeCounter(w, "spx_ingest_published_total", "发布成功的事件总数", stats.Published)
	writeCounter(w, "spx_ingest_publish_retry_total", "发布重试次数", stats.PublishRetry)
	writeCounter(w, "spx_ingest_panic_total", "解析引擎 panic 次数", stats.Panics)
	writeCounter(w, "spx_ingest_dropped_total", "排空超时被丢弃的事件数", stats.Dropped)
	writeGauge(w, "spx_ingest_queue_depth", "当前待解析队列长度", int64(stats.QueueDepth))
	writeGauge(w, "spx_ingest_in_flight_input_bytes", "待解析及正在解析的输入字节", stats.InFlightBytes)
	if s.SourceSnapshotAge != nil {
		writeGauge(w, "spx_ingest_source_snapshot_age_seconds", "距上次确认接入配置的秒数", int64(s.SourceSnapshotAge()/time.Second))
	}
	if s.KafkaOffsetsRejected != nil {
		writeCounter(w, "spx_ingest_kafka_offsets_rejected_total", "Kafka 因位移或主题身份异常暂停分区的次数", s.KafkaOffsetsRejected())
	}
	if s.AuditOutboxHealth != nil {
		check, cancel := context.WithTimeout(c.Request.Context(), 250*time.Millisecond)
		depth, oldest, err := s.AuditOutboxHealth(check)
		cancel()
		if err == nil {
			writeGauge(w, "spx_ingest_audit_outbox_available", "可读取审计投递队列", 1)
			writeGauge(w, "spx_ingest_audit_outbox_depth", "待投递审计记录数，最多采样10001条", depth)
			writeGauge(w, "spx_ingest_audit_outbox_oldest_seconds", "最老待投递审计记录等待时间", oldest)
		} else {
			writeGauge(w, "spx_ingest_audit_outbox_available", "可读取审计投递队列", 0)
		}
	}

	// 按协议分维度：看板需要"哪个通道出了问题"，全局计数回答不了。
	// 标签顺序固定，避免抓取端出现同一序列的不同文本形式。
	protocols := make([]string, 0, len(stats.ByProtocol))
	for protocol := range stats.ByProtocol {
		protocols = append(protocols, protocol)
	}

	sort.Strings(protocols)

	writeLabeledCounter(w, "spx_ingest_protocol_accepted_total", "按协议的受理总数", "protocol", protocols, func(key string) uint64 { return stats.ByProtocol[key].Accepted })
	writeLabeledCounter(w, "spx_ingest_protocol_parsed_total", "按协议的解析成功总数", "protocol", protocols, func(key string) uint64 { return stats.ByProtocol[key].Parsed })

	// 按接入点分维度：定位到"是哪台设备在发坏数据"。
	// 基数由流水线的 maxInputSeries 限制，抓取端不会被拖垮。
	inputs := make([]string, 0, len(stats.ByInput))
	for inputID := range stats.ByInput {
		inputs = append(inputs, inputID)
	}

	sort.Strings(inputs)

	writeLabeledCounter(w, "spx_ingest_input_accepted_total", "按数据接入的受理总数", "input_id", inputs, func(key string) uint64 { return stats.ByInput[key].Accepted })
	writeLabeledCounter(w, "spx_ingest_input_parsed_total", "按数据接入的解析成功总数", "input_id", inputs, func(key string) uint64 { return stats.ByInput[key].Parsed })
}

// writeCounter 输出一个 counter（单调递增）。
func writeCounter(w http.ResponseWriter, name, help string, value uint64) {
	_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n%s %d\n", name, help, name, name, value)
}

// writeLabeledCounter 一次输出计数器的全部样本，元数据只写一次。
//
// 标签名与值都由调用方给定：协议来自受控枚举，接入点标识是 UUID，
// 二者都不含引号或换行，因此无需转义。
func writeLabeledCounter(w http.ResponseWriter, name, help, label string, values []string, count func(string) uint64) {
	if len(values) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n", name, help, name)
	for _, value := range values {
		_, _ = fmt.Fprintf(w, "%s{%s=%q} %d\n", name, label, value, count(value))
	}
}

// writeGauge 输出一个 gauge（可增可减）。
func writeGauge(w http.ResponseWriter, name, help string, value int64) {
	_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %d\n", name, help, name, name, value)
}

// 编译期断言：使用 pipeline.Stats 的字段，避免改名后指标静默变空。
var _ = pipeline.Stats{}
