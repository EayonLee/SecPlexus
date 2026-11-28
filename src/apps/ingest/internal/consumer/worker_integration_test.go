//go:build integration

package consumer_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connection/kafka"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/consumer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository/event"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/migrations/clickhouse"
	"github.com/EayonLee/SecPlexus/tests/integration/kafkafixture"
)

func TestEventStorageWithKafkaAndClickHouse(t *testing.T) {
	address := os.Getenv("SPX_TEST_CLICKHOUSE_URL")
	brokers := os.Getenv("SPX_TEST_KAFKA_BROKERS")
	if address == "" || brokers == "" {
		if os.Getenv("SPX_TEST_REQUIRE_CLICKHOUSE") == "1" {
			t.Fatal("SPX_TEST_CLICKHOUSE_URL and SPX_TEST_KAFKA_BROKERS are required")
		}
		t.Skip("SPX_TEST_CLICKHOUSE_URL and SPX_TEST_KAFKA_BROKERS are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	httpClient := &http.Client{Timeout: 5 * time.Second}
	sql := func(statement string) string {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, address, strings.NewReader(statement))
		if err != nil {
			t.Fatal(err)
		}
		response, err := httpClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("ClickHouse test query failed: HTTP %d, %s, %v", response.StatusCode, data, err)
		}
		return string(data)
	}

	const retentionMS = int64(7 * 24 * time.Hour / time.Millisecond)
	// CREATE without IF NOT EXISTS prevents taking ownership of any existing data.
	sql("CREATE DATABASE spx")
	t.Cleanup(func() { sql("DROP DATABASE spx SYNC") })

	client := func(opts ...kgo.Opt) *kgo.Client {
		t.Helper()
		kafka, err := kgo.NewClient(append([]kgo.Opt{kgo.SeedBrokers(strings.Split(brokers, ",")...)}, opts...)...)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(kafka.CloseAllowingRebalance)
		return kafka
	}
	admin := client(kgo.RecordPartitioner(kgo.ManualPartitioner()))
	suffix := uuid.NewString()
	eventsTopic := "spx-test-event-store-events-" + suffix
	failuresTopic := "spx-test-event-store-failures-" + suffix
	for topic, partitions := range map[string]int32{eventsTopic: 2, failuresTopic: 1} {
		if err := kafkafixture.CreateTopic(admin, topic, partitions); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := kafkafixture.DeleteTopic(admin, topic); err != nil {
				t.Error(err)
			}
		})
	}
	if err := kafka.CheckEvents(ctx, strings.Split(brokers, ","), kafka.Security{}, eventsTopic, failuresTopic,
		"spx-test-new-event-store-"+suffix, retentionMS); err != nil {
		t.Fatalf("a new event store group on the broker should pass preflight: %v", err)
	}
	store, err := event.NewStore(address, "spx", "", "", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckPermissions(ctx); err != nil {
		t.Fatal(err)
	}
	projector, err := event.NewProjector()
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	inputID := uuid.NewString()
	raw := fmt.Sprintf(`{"time":%d,"class_uid":4001,"category_uid":4,"activity_id":6,"type_uid":400106,"severity_id":4,
		"src_endpoint":{"ip":"192.0.2.1","port":0},"dst_endpoint":{"ip":"2001:db8::2"},
		"metadata":{"uid":%q,"version":"1.10.0","spx":{"contract_version":"v1","parse.outcome":"ok","received_at":%d,
			"input_id":%q,"vendor":"Example","product":""}}}`, now.UnixMilli(), suffix, now.UnixMilli(), inputID)
	invalid := strings.Replace(raw, `"contract_version":"v1","parse.outcome":"ok"`, `"contract_version":"2.0.0"`, 1)
	otherID := uuid.NewString()
	late := strings.NewReplacer(
		fmt.Sprintf(`"time":%d`, now.UnixMilli()), fmt.Sprintf(`"time":%d`, now.Add(-24*time.Hour).UnixMilli()),
		suffix, otherID, inputID, uuid.NewString(), `"vendor":"Example"`, `"vendor":"Example Two"`,
		`"severity_id":4`, `"severity_id":99,"severity":"vendor-priority"`,
	).Replace(raw)
	fallbackID := uuid.NewString()
	fallback := fmt.Sprintf(`{"time":%d,"class_uid":0,"category_uid":0,"activity_id":0,"type_uid":0,"severity_id":0,
		"metadata":{"uid":%q,"version":"1.10.0","spx":{"contract_version":"v1","received_at":%d,
			"input_id":%q,"vendor":"Example","product":"","parse.outcome":"partial","candidate_class_uid":3002}}}`,
		now.UnixMilli(), fallbackID, now.UnixMilli(), inputID)
	if err := admin.ProduceSync(ctx, &kgo.Record{Topic: eventsTopic, Value: []byte(raw)},
		&kgo.Record{Topic: eventsTopic, Value: []byte(invalid)},
		&kgo.Record{Topic: eventsTopic, Value: []byte(late)},
		&kgo.Record{Topic: eventsTopic, Value: []byte(fallback)}).FirstErr(); err != nil {
		t.Fatal(err)
	}

	newWorker := func(group string, destination *event.Store) consumer.Worker {
		return consumer.Worker{
			Kafka: client(kgo.ConsumerGroup(group), kgo.ConsumeTopics(eventsTopic), kgo.ConsumeStartOffset(kgo.NewOffset().AtStart()),
				kgo.ConsumeResetOffset(kgo.NoResetOffset()), kgo.DisableAutoCommit(), kgo.BlockRebalanceOnPoll()),
			Store: destination, Projector: projector, FailureTopic: failuresTopic,
			MaxRecords: 10, MaxBytes: 64 << 10, BatchTimeout: 5 * time.Second, CheckInterval: time.Second,
		}
	}
	group := "spx-test-event-store-" + suffix
	replay := group + "-replay"
	for _, name := range []string{group, replay} {
		t.Cleanup(func() {
			if err := kafkafixture.DeleteGroup(admin, name); err != nil {
				t.Error(err)
			}
		})
	}
	partitionOffset := func(group string, partition int32) int64 {
		t.Helper()
		req := kmsg.NewPtrOffsetFetchRequest()
		req.Group = group
		req.Topics = []kmsg.OffsetFetchRequestTopic{{Topic: eventsTopic, Partitions: []int32{partition}}}
		res, err := req.RequestWith(ctx, admin)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Groups) == 1 && len(res.Groups[0].Topics) == 1 && len(res.Groups[0].Topics[0].Partitions) == 1 {
			return res.Groups[0].Topics[0].Partitions[0].Offset
		}
		if len(res.Topics) == 1 && len(res.Topics[0].Partitions) == 1 {
			return res.Topics[0].Partitions[0].Offset
		}
		return -1
	}
	offset := func(group string) int64 { return partitionOffset(group, 0) }

	readOnlyUser := "spx_test_" + strings.ReplaceAll(suffix, "-", "_")
	sql("CREATE USER " + readOnlyUser + " IDENTIFIED WITH no_password")
	t.Cleanup(func() { sql("DROP USER " + readOnlyUser) })
	sql("GRANT SELECT ON system.columns TO " + readOnlyUser)
	sql("GRANT SELECT ON system.tables TO " + readOnlyUser)
	sql("GRANT SELECT ON spx.events TO " + readOnlyUser)
	readOnly, err := event.NewStore(address, "spx", readOnlyUser, "", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer readOnly.Close()
	if err := readOnly.CheckSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if err := readOnly.CheckPermissions(ctx); err == nil {
		t.Fatal("a read-only account passed the event writer permission check")
	}
	failed := newWorker(group, readOnly)
	err = failed.Run(ctx)
	failed.Kafka.CloseAllowingRebalance()
	if err == nil || !strings.Contains(err.Error(), "store event batch") || offset(group) != -1 {
		t.Fatalf("write failure must leave the partition uncommitted: %v", err)
	}

	runToCommit := func(name string, target *event.Store, partition int32, expected int64) {
		t.Helper()
		worker := newWorker(name, target)
		run, stop := context.WithCancel(ctx)
		finished := make(chan error, 1)
		go func() { finished <- worker.Run(run) }()
		poll := time.NewTicker(100 * time.Millisecond)
		defer poll.Stop()
		defer func() {
			stop()
			worker.Kafka.CloseAllowingRebalance()
		}()
		for partitionOffset(name, partition) != expected {
			select {
			case err := <-finished:
				t.Fatalf("event writer stopped before commit: %v", err)
			case <-ctx.Done():
				t.Fatal("event writer did not commit the expected next offset")
			case <-poll.C:
			}
		}
		stop()
		if err := <-finished; err != nil {
			t.Fatal(err)
		}
	}

	runToCommit(group, store, 0, 4)
	runToCommit(replay, store, 0, 4)
	rows := sql(fmt.Sprintf(`SELECT event_json, toUnixTimestamp64Milli(event_time) AS time, toString(src_ip) AS source,
		toString(dst_ip) AS destination, src_port, dst_port, product FROM spx.events FINAL
		WHERE class_uid=4001 AND event_time=fromUnixTimestamp64Milli(%d) AND event_id='%s'
		FORMAT JSONEachRow`, now.UnixMilli(), suffix))
	var got struct {
		EventJSON   string `json:"event_json"`
		Time        int64  `json:"time"`
		Source      string `json:"source"`
		Destination string `json:"destination"`
		SrcPort     *int   `json:"src_port"`
		DstPort     *int   `json:"dst_port"`
		Product     *string
	}
	if err := json.Unmarshal([]byte(rows), &got); err != nil {
		t.Fatal(err)
	}
	if got.EventJSON != raw || got.Time != now.UnixMilli() ||
		(got.Source != "::ffff:192.0.2.1" && got.Source != "::ffff:c000:201") || got.Destination != "2001:db8::2" ||
		got.SrcPort == nil || *got.SrcPort != 0 || got.DstPort != nil || got.Product != nil {
		t.Fatalf("ClickHouse row differs from the published event: %+v", got)
	}
	if got := strings.TrimSpace(sql("SELECT class_uid, parse_outcome FROM spx.events FINAL WHERE event_id='" + fallbackID + "'")); got != "0\tpartial" {
		t.Fatalf("partial Base Event missing from event store: %s", got)
	}
	count := sql(`SELECT count() FROM spx.events FINAL WHERE src_ip=toIPv6('::ffff:192.0.2.1')`)
	if strings.TrimSpace(count) != "2" {
		t.Fatalf("one logical event per source after replay: %s", count)
	}
	other := sql("SELECT severity_id, JSONExtractString(event_json, 'severity') FROM spx.events FINAL WHERE event_id='" + otherID + "'")
	if strings.TrimSpace(other) != "99\tvendor-priority" {
		t.Fatalf("Other severity and its vendor label differ: %s", other)
	}
	failures := client(kgo.ConsumeTopics(failuresTopic)).PollRecords(ctx, 1)
	if err := failures.Err(); err != nil {
		t.Fatal(err)
	}
	var rejected struct {
		Topic     string
		Partition int32
		Offset    int64
		Reason    string
		Event     []byte
		Bytes     int
	}
	if err := json.Unmarshal(failures.Records()[0].Value, &rejected); err != nil {
		t.Fatal(err)
	}
	if rejected.Topic != eventsTopic || rejected.Partition != 0 || rejected.Offset != 1 ||
		rejected.Event != nil || rejected.Bytes != len(invalid) || rejected.Reason != "invalid_event_projection" {
		t.Fatalf("unexpected quarantined record: %+v", rejected)
	}

	historicalID := uuid.NewString()
	historical := strings.NewReplacer(
		fmt.Sprintf(`"time":%d`, now.UnixMilli()), fmt.Sprintf(`"time":%d`, now.AddDate(-2, 0, 0).UnixMilli()),
		suffix, historicalID,
	).Replace(raw)
	row, err := projector.Project([]byte(historical))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Insert(ctx, encoded); err != nil {
		t.Fatal(err)
	}
	if rows := strings.TrimSpace(sql("SELECT count() FROM spx.events FINAL")); rows != "4" {
		t.Fatalf("historical events are retained: %s", rows)
	}
	historicalTime := now.AddDate(-2, 0, 0)
	payload, err := readOnly.ReadEvent(ctx, uuid.MustParse(historicalID), historicalTime, historicalTime.Add(time.Millisecond))
	if err != nil || !strings.Contains(string(payload), historicalID) {
		t.Fatalf("read-only lookup of retained event: bytes=%d, error=%v", len(payload), err)
	}
	payload, err = readOnly.ReadEvent(ctx, uuid.MustParse(historicalID), historicalTime.Add(-time.Second), historicalTime)
	if err != nil || len(payload) != 0 {
		t.Fatalf("lookup includes the exclusive end: bytes=%d, error=%v", len(payload), err)
	}
	if partitions := strings.TrimSpace(sql("SELECT uniqExact(_partition_id) FROM spx.events")); partitions == "1" {
		t.Fatal("historical events must use the partition for their occurrence month")
	}
	sql("ALTER TABLE spx.events MODIFY TTL event_time + INTERVAL 100 YEAR DELETE")
	if err := store.CheckSchema(ctx); err == nil || !strings.Contains(err.Error(), "TTL") {
		t.Fatalf("TTL drift was not identified: %v", err)
	}
	sql("ALTER TABLE spx.events REMOVE TTL")
	sql("ALTER TABLE spx.events COMMENT COLUMN src_ip 'drift'")
	if err := store.CheckSchema(ctx); err == nil || !strings.Contains(err.Error(), "src_ip") {
		t.Fatalf("column drift was not identified: %v", err)
	}
	sql("ALTER TABLE spx.events COMMENT COLUMN src_ip 'src_endpoint.ip | 源 IP'")
	if err := store.CheckSchema(ctx); err != nil {
		t.Fatalf("correcting drift should restore schema validity: %v", err)
	}
	// 模拟同一次初始化在建表后、写迁移账本前中断。已有事件必须完整保留。
	sql("DROP TABLE spx.event_schema_migrations SYNC")
	for range 2 {
		if err := store.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.TrimSpace(sql("SELECT count() FROM spx.event_schema_migrations FINAL")); got != "1" {
		t.Fatalf("each migration must be recorded once: %s", got)
	}
	if got := strings.TrimSpace(sql("SELECT count() FROM spx.events FINAL")); got != "4" {
		t.Fatalf("adopting the existing table changed its contents: %s", got)
	}
	if os.Getenv("SPX_TEST_CLICKHOUSE_BACKUPS") == "1" {
		backup := "File('event-recovery-" + suffix + "')"
		sql("BACKUP TABLE spx.events, TABLE spx.event_schema_migrations TO " + backup)
		sql("CREATE DATABASE spx_restored")
		t.Cleanup(func() { sql("DROP DATABASE spx_restored SYNC") })
		sql("RESTORE TABLE spx.events AS spx_restored.events, TABLE spx.event_schema_migrations AS spx_restored.event_schema_migrations FROM " + backup)
		compare := "SELECT event_id, event_json FROM %s.events FINAL ORDER BY event_id FORMAT JSONEachRow"
		if got := sql(fmt.Sprintf(compare, "spx_restored")); got != sql(fmt.Sprintf(compare, "spx")) {
			t.Fatal("restored Event bodies differ from the backup source")
		}
		readLedger := "SELECT version, checksum FROM %s.event_schema_migrations FINAL ORDER BY version"
		if got := sql(fmt.Sprintf(readLedger, "spx_restored")); got != sql(fmt.Sprintf(readLedger, "spx")) {
			t.Fatal("restored migration ledger differs from the backup source")
		}

		// 备份前写入已停止。使用已记录同一主题位移的消费组，把备份之后的事件写进恢复库。
		checkpoint := partitionOffset(replay, 0)
		if checkpoint != 4 {
			t.Fatalf("unexpected backup checkpoint: %d", checkpoint)
		}
		restored, err := event.NewStore(address, "spx_restored", "", "", 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer restored.Close()
		resumedID := uuid.NewString()
		resumed := strings.Replace(raw, suffix, resumedID, 1)
		if err := admin.ProduceSync(ctx, &kgo.Record{Topic: eventsTopic, Partition: 1, Value: []byte(resumed)}).FirstErr(); err != nil {
			t.Fatal(err)
		}
		runToCommit(replay, restored, 1, 1)
		if partitionOffset(replay, 0) != checkpoint {
			t.Fatal("recovery changed the saved partition checkpoint")
		}
		if count := strings.TrimSpace(sql("SELECT count() FROM spx_restored.events FINAL")); count != "5" {
			t.Fatalf("wrong logical count after resuming recovery: %s", count)
		}
		if row, err := restored.ReadEvent(ctx, uuid.MustParse(resumedID), now, now.Add(time.Second)); err != nil || !strings.Contains(string(row), resumedID) {
			t.Fatalf("recovered table did not receive the new event: bytes=%d error=%v", len(row), err)
		}
	}
	healthyPartitionCount := int64(1)
	if os.Getenv("SPX_TEST_CLICKHOUSE_BACKUPS") == "1" {
		healthyPartitionCount++
	}
	t.Run("retention gap preserves the committed offset", func(t *testing.T) {
		if err := admin.ProduceSync(ctx, &kgo.Record{Topic: eventsTopic, Value: []byte(raw)},
			&kgo.Record{Topic: eventsTopic, Value: []byte(raw)},
			&kgo.Record{Topic: eventsTopic, Partition: 1, Value: []byte(raw)}).FirstErr(); err != nil {
			t.Fatal(err)
		}
		request := kmsg.NewPtrDeleteRecordsRequest()
		request.Topics = []kmsg.DeleteRecordsRequestTopic{{Topic: eventsTopic, Partitions: []kmsg.DeleteRecordsRequestTopicPartition{{Partition: 0, Offset: 5}}}}
		response, err := request.RequestWith(ctx, admin)
		if err != nil || len(response.Topics) != 1 || len(response.Topics[0].Partitions) != 1 || response.Topics[0].Partitions[0].LowWatermark != 5 {
			t.Fatalf("move the test partition log start: %+v, %v", response, err)
		}

		logs := make(chan map[string]any, 1)
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(archiveLogWriter(func(line []byte) {
			var record map[string]any
			if json.Unmarshal(line, &record) == nil && record["event"] == "ingest.events.offset_unavailable" {
				logs <- record
			}
		}), nil)))
		defer slog.SetDefault(previous)

		worker := newWorker(group, store)
		run, stop := context.WithCancel(ctx)
		finished := make(chan error, 1)
		go func() { finished <- worker.Run(run) }()
		defer func() {
			stop()
			worker.Kafka.CloseAllowingRebalance()
		}()
		select {
		case record := <-logs:
			if record["committed_offset"] != float64(4) || record["earliest_offset"] != float64(5) || record["end_offset"] != float64(6) {
				t.Fatalf("unexpected retention range: %v", record)
			}
		case err := <-finished:
			t.Fatalf("event writer exited instead of pausing the affected partition: %v", err)
		case <-ctx.Done():
			t.Fatal("event writer did not report the retention gap")
		}
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for partitionOffset(group, 1) != healthyPartitionCount {
			select {
			case err := <-finished:
				t.Fatalf("event writer stopped while a healthy partition was still pending: %v", err)
			case <-ctx.Done():
				t.Fatal("the healthy partition did not commit")
			case <-ticker.C:
			}
		}
		stop()
		if err := <-finished; err != nil {
			t.Fatal(err)
		}
		if got := offset(group); got != 4 {
			t.Fatalf("event writer advanced the offset across the gap to %d", got)
		}
	})

	t.Run("first migration checksum covers columns", func(t *testing.T) {
		columns := clickhouse.EventTableColumnsV1
		t.Cleanup(func() { clickhouse.EventTableColumnsV1 = columns })
		clickhouse.EventTableColumnsV1 = strings.Replace(columns, "事件类别 ID", "不同的事件类别注释", 1)
		if err := store.Migrate(ctx); err == nil || !strings.Contains(err.Error(), "00001 checksum") {
			t.Fatalf("change to the first migration columns was not rejected: %v", err)
		}
	})
	sql("INSERT INTO spx.event_schema_migrations (version, checksum) VALUES (1, 'unexpected')")
	if err := store.Migrate(ctx); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("expected migration checksum mismatch, got %v", err)
	}

}

type archiveLogWriter func([]byte)

func (write archiveLogWriter) Write(line []byte) (int, error) {
	write(line)
	return len(line), nil
}
