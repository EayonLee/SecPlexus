package normalizer_test

import (
	"testing"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
)

// referenceTime 是基准测试的固定接收时刻，消除时钟调用对度量的干扰。
var referenceTime = time.Unix(1762986541, 0).UTC()

// benchMessage 是一条贴近真实 FortiGate 攻击日志的 CEF 报文。
var benchMessage = []byte("CEF:0|Fortinet|FortiGate|7.4|100|Web Attack|5|" +
	"src=10.20.3.44 dst=10.20.3.10 spt=51324 dpt=443 act=deny msg=SQL injection blocked")

// BenchmarkApplyCEF 度量单核下"解析 + 映射"一条事件的成本。
//
// 容量目标 5 万 EPS：这个基准给出单核吞吐，乘以 worker 数即为单机上限。
// 运行方式：go test -bench=Apply -benchmem ./src/apps/ingest/internal/normalizer/
func BenchmarkApplyCEF(b *testing.B) {
	engine, err := normalizer.Compile(cefRule())
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(benchMessage)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err = engine.Apply(benchMessage, referenceTime); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkApplyJSON 度量结构化 JSON 的同一路径成本。
func BenchmarkApplyJSON(b *testing.B) {
	rule := normalizer.Rule{
		ID:    "json-bench",
		Parse: parser.Spec{Mode: parser.ModeJSON},
		FieldMap: map[string]string{
			"src_ip": "src_endpoint.ip",
			"dst_ip": "dst_endpoint.ip",
			"level":  "severity",
		},
		Constants: map[string]any{"class_uid": 2004},
	}

	engine, err := normalizer.Compile(rule)
	if err != nil {
		b.Fatal(err)
	}

	message := []byte(`{"src_ip":"10.0.0.1","dst_ip":"10.0.0.2","level":"high","msg":"hello"}`)

	b.ReportAllocs()
	b.SetBytes(int64(len(message)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err = engine.Apply(message, referenceTime); err != nil {
			b.Fatal(err)
		}
	}
}
