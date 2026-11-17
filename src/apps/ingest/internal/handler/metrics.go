package handler

import (
	"fmt"
	"net/http"
	"sort"

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

	// 按协议分维度：看板需要"哪个通道出了问题"，全局计数回答不了。
	// 标签顺序固定，避免抓取端出现同一序列的不同文本形式。
	protocols := make([]string, 0, len(stats.ByProtocol))
	for protocol := range stats.ByProtocol {
		protocols = append(protocols, protocol)
	}

	sort.Strings(protocols)

	for _, protocol := range protocols {
		byProtocol := stats.ByProtocol[protocol]
		writeCounterLabel(w, "spx_ingest_protocol_accepted_total", "按协议的受理总数", "protocol", protocol, byProtocol.Accepted)
		writeCounterLabel(w, "spx_ingest_protocol_parsed_total", "按协议的解析成功总数", "protocol", protocol, byProtocol.Parsed)
	}

	// 按接入点分维度：定位到"是哪台设备在发坏数据"。
	// 基数由流水线的 maxInputSeries 限制，抓取端不会被拖垮。
	inputs := make([]string, 0, len(stats.ByInput))
	for inputID := range stats.ByInput {
		inputs = append(inputs, inputID)
	}

	sort.Strings(inputs)

	for _, inputID := range inputs {
		byInput := stats.ByInput[inputID]
		writeCounterLabel(w, "spx_ingest_input_accepted_total", "按接入点的受理总数", "input_id", inputID, byInput.Accepted)
		writeCounterLabel(w, "spx_ingest_input_parsed_total", "按接入点的解析成功总数", "input_id", inputID, byInput.Parsed)
	}
}

// writeCounter 输出一个 counter（单调递增）。
func writeCounter(w http.ResponseWriter, name, help string, value uint64) {
	_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n%s %d\n", name, help, name, name, value)
}

// writeCounterLabel 输出带单个标签的 counter。
//
// 标签名与值都由调用方给定：协议来自受控枚举，接入点标识是 UUID，
// 二者都不含引号或换行，因此无需转义。
func writeCounterLabel(w http.ResponseWriter, name, help, label, value string, count uint64) {
	_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n%s{%s=%q} %d\n", name, help, name, name, label, value, count)
}

// writeGauge 输出一个 gauge（可增可减）。
func writeGauge(w http.ResponseWriter, name, help string, value int64) {
	_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %d\n", name, help, name, name, value)
}

// 编译期断言：使用 pipeline.Stats 的字段，避免改名后指标静默变空。
var _ = pipeline.Stats{}
