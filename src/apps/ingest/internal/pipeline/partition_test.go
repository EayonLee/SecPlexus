package pipeline

import "testing"

// 分区键决定同一实体的事件是否落在同一分区。
// 一旦优先级或字段名变了，同一台设备的事件会被拆到不同分区，
// Kafka 只保证分区内有序——乱序是静默发生的，没有任何报警。
func TestPartitionKeyPrefersEntityIdentity(t *testing.T) {
	cases := []struct {
		name   string
		fields map[string]any
		want   string
	}{
		{
			name:   "源地址优先",
			fields: map[string]any{"src_endpoint": map[string]any{"ip": "10.0.0.7"}, "device": map[string]any{"hostname": "web-01"}},
			want:   "10.0.0.7",
		},
		{
			name:   "没有源地址用主机名",
			fields: map[string]any{"device": map[string]any{"hostname": "web-01"}},
			want:   "web-01",
		},
		{
			name:   "都没有用账号名",
			fields: map[string]any{"user": map[string]any{"name": "alice"}},
			want:   "alice",
		},
		{
			name:   "源地址为空串时继续回退",
			fields: map[string]any{"src_endpoint": map[string]any{"ip": ""}, "device": map[string]any{"hostname": "web-02"}},
			want:   "web-02",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := partitionKey(testCase.fields, "event-1"); got != testCase.want {
				t.Fatalf("分区键 = %q, 期望 %q", got, testCase.want)
			}
		})
	}
}

// 没有任何实体标识时必须回退到事件标识，而不是固定常量：
// 固定常量会把所有无序事件压进同一个分区，削掉 Kafka 的并行度。
func TestPartitionKeyFallsBackToEventID(t *testing.T) {
	if got := partitionKey(map[string]any{}, "event-abc"); got != "event-abc" {
		t.Fatalf("分区键 = %q, 期望事件标识", got)
	}

	if got := partitionKey(nil, "event-xyz"); got != "event-xyz" {
		t.Fatalf("空字段的分区键 = %q, 期望事件标识", got)
	}

	first := partitionKey(map[string]any{}, "event-1")
	second := partitionKey(map[string]any{}, "event-2")
	if first == second {
		t.Fatalf("不同事件落到了同一分区键 %q", first)
	}
}
