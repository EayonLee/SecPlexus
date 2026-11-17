package publish

import (
	"testing"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
)

// 分流规则必须被钉住：失败事件去哪，是部署后无法回滚的行为。
func TestTopicForRoutesDeadLetters(t *testing.T) {
	producer := &Kafka{eventTopic: "spx.ingest.events", parseFailureTopic: "spx.ingest.deadletter"}

	tests := map[string]struct {
		event *pipeline.Event
		want  string
	}{
		"success to main":    {event: &pipeline.Event{}, want: "spx.ingest.events"},
		"dead letter routed": {event: &pipeline.Event{DeadLetter: true}, want: "spx.ingest.deadletter"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := producer.topicFor(test.event); got != test.want {
				t.Fatalf("topic = %q, want %q", got, test.want)
			}
		})
	}
}

// 未配置死信 topic 时，失败事件仍走主 topic（保持旧语义）。
func TestTopicForKeepsMainTopicWhenDeadLetterUnset(t *testing.T) {
	producer := &Kafka{eventTopic: "spx.ingest.events"}

	if got := producer.topicFor(&pipeline.Event{DeadLetter: true}); got != "spx.ingest.events" {
		t.Fatalf("topic = %q, want event topic", got)
	}
}
