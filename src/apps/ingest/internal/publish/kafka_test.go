package publish

import (
	"context"
	"strings"
	"testing"
)

// 分流规则必须被钉住：失败事件去哪，是部署后无法回滚的行为。
// 诊断流缺失时应在连接 broker 前拒绝配置。
func TestNewKafkaRequiresEventTopic(t *testing.T) {
	_, err := NewKafka(context.Background(), Options{Brokers: []string{"127.0.0.1:9092"}})
	if err == nil || !strings.Contains(err.Error(), "kafka event topic") {
		t.Fatalf("expected required topic error, got %v", err)
	}
}
