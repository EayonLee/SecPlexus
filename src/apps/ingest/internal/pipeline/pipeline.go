// Package pipeline 把通道收到的原始报文变成已发布的 OCSF 事件。
//
// 它负责三件通道不该操心的事：固定 worker 池解析、按批流转、失败事件照常发布。
// 通道只调用 Submit；解析规则、并发、批大小与背压都在这里。
package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connector"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// Status 是事件的解析状态，随事件发布到 metadata.parse_status。
type Status string

const (
	StatusOK    Status = "ok"    // 规则解析成功。
	StatusError Status = "error" // 解析失败；事件仍发布，可查询、可补救。
)

// 解析失败的原因码。下游按它做看板与告警，取值必须稳定。
const (
	ReasonFormatMismatch = "format_mismatch" // 报文与规则的线格式不符，或接入点尚未挂规则。
	ReasonEnginePanic    = "engine_panic"    // 引擎 panic，兜底记录，正常不应出现。
)

// ClassUIDGeneric 是未解析事件的兜底类。
// 6003 由 spec 指定为"通用接入（原文透传 message）"的类，
// 目的是让未挂规则的报文仍然可查询、可统计，而不是被丢弃。
const ClassUIDGeneric = 6003

// Event 是一条待发布的事件。
type Event struct {
	ConnectorID  string   // 来源接入点。
	EventID      string   // 确定性标识，供下游去重。
	PartitionKey string   // 分区键，保证同一实体的事件落在同一分区。
	Body         []byte   // OCSF 事件 JSON。
	Status       Status   // 解析状态。
	Reasons      []string // 解析失败原因码；成功时为空。
}

// Publisher 是事件出口。管道只依赖这个接口，Kafka 是它的一个实现。
// Publish 返回错误表示这一批没有送达，管道会重试而不是丢弃。
type Publisher interface {
	Publish(ctx context.Context, events []Event) error
}

// Job 是一条待解析的报文。
type Job struct {
	Snapshot connector.Snapshot
	Raw      []byte
	Received time.Time
	// Fields 非空表示第 1 层已由通道完成（如 syslog 剥信封），
	// 管道直接进第 2 层映射，不再调用解析器。
	Fields map[string]any
}

// Stats 是可观测计数，供健康检查与运行指标使用。
type Stats struct {
	Accepted     uint64 // 受理的报文数。
	Parsed       uint64 // 解析成功的事件数。
	ParseFailed  uint64 // 解析失败的事件数（仍会发布）。
	Published    uint64 // 已发布的事件数。
	PublishRetry uint64 // 发布重试次数。
	Panics       uint64 // 被 recover 兜住的引擎 panic 次数。
	QueueDepth   int    // 当前待解析队列长度。
}

// Options 是管道容量参数。
type Options struct {
	Workers       int           // 解析 worker 数；默认等于 CPU 核数。
	QueueDepth    int           // 待解析队列长度；默认 10000（约 0.2 秒的 5w EPS）。
	BatchSize     int           // 每批事件数；默认 500。
	FlushInterval time.Duration // 批量最长等待；默认 2 秒。
}

// defaults 填齐零值参数，让调用方只关心想改的那几个。
func (o Options) defaults() Options {
	if o.Workers <= 0 {
		o.Workers = runtime.NumCPU()
	}
	if o.QueueDepth <= 0 {
		o.QueueDepth = 10000
	}
	if o.BatchSize <= 0 {
		o.BatchSize = 500
	}
	if o.FlushInterval <= 0 {
		o.FlushInterval = 2 * time.Second
	}

	return o
}

// Pipeline 是接收与发布之间的解析流水线。
// 启动后由固定数量的 worker 消费队列，无 per-event goroutine。
type Pipeline struct {
	options   Options
	publisher Publisher

	jobs   chan Job
	events chan Event
	flush  chan chan struct{}

	sequence atomic.Uint64

	// inflight 统计已提交但尚未进入批处理的报文数。
	// Flush 与优雅停机都靠它判断"在途是否排空"，只看批处理缓冲区会漏掉解析队列里的事件。
	inflight sync.WaitGroup

	accepted     atomic.Uint64
	parsed       atomic.Uint64
	parseFailed  atomic.Uint64
	published    atomic.Uint64
	publishRetry atomic.Uint64
	panics       atomic.Uint64

	closeOnce sync.Once
}

// New 创建管道；Start 才会启动 worker。
func New(options Options, publisher Publisher) *Pipeline {
	options = options.defaults()

	return &Pipeline{
		options:   options,
		publisher: publisher,
		jobs:      make(chan Job, options.QueueDepth),
		events:    make(chan Event, options.BatchSize*2),
		flush:     make(chan chan struct{}),
	}
}

// SubmitFields 受理一条已由通道完成线格式解析的报文。
// syslog 通道用 go-syslog 剥完信封后走这里：字段已就绪，只差规则映射。
func (p *Pipeline) SubmitFields(ctx context.Context, snapshot connector.Snapshot, fields map[string]any, received time.Time) error {
	job := Job{Snapshot: snapshot, Fields: fields, Received: received}

	p.inflight.Add(1)

	select {
	case p.jobs <- job:
		p.accepted.Add(1)
		return nil
	case <-ctx.Done():
		p.inflight.Done()

		return ctx.Err()
	}
}

// Submit 受理一条报文。
// 队列满时阻塞而不是丢弃：背压必须传到通道层，由它阻塞发送方——
// 丢数据比让设备重发更难补救。
func (p *Pipeline) Submit(ctx context.Context, snapshot connector.Snapshot, raw []byte, received time.Time) error {
	job := Job{Snapshot: snapshot, Raw: raw, Received: received}

	// 先登记再入队：Flush 看到计数为零才是真的全空。
	p.inflight.Add(1)

	select {
	case p.jobs <- job:
		p.accepted.Add(1)
		return nil
	case <-ctx.Done():
		p.inflight.Done()

		return ctx.Err()
	}
}

// Start 启动解析 worker 与批处理器，直到 ctx 取消后按停机顺序退出：
// 停接收 → 排空在途解析 → 发布剩余批次。
func (p *Pipeline) Start(ctx context.Context) error {
	var workers sync.WaitGroup
	for worker := 0; worker < p.options.Workers; worker++ {
		workers.Add(1)

		go func() {
			defer workers.Done()
			p.parseLoop(ctx)
		}()
	}

	batcherDone := make(chan error, 1)
	go func() { batcherDone <- p.batchLoop(ctx) }()

	<-ctx.Done()

	// 停接收：关掉队列让 worker 把剩余任务跑完后退出。
	p.closeOnce.Do(func() { close(p.jobs) })
	workers.Wait()
	close(p.events)

	return <-batcherDone
}

// Flush 等待在途报文全部解析完，再让批处理器立刻发送当前批次。
// 只冲刷批处理缓冲区是不够的：解析队列里的事件还没变成事件，
// 调用方会看到"零事件"而误判为丢数据。
func (p *Pipeline) Flush(ctx context.Context) {
	drained := make(chan struct{})
	go func() {
		p.inflight.Wait()
		close(drained)
	}()

	select {
	case <-drained:
	case <-ctx.Done():
		return
	}

	done := make(chan struct{})
	select {
	case p.flush <- done:
		<-done
	case <-ctx.Done():
	}
}

// Stats 返回当前计数快照。
func (p *Pipeline) Stats() Stats {
	return Stats{
		Accepted:     p.accepted.Load(),
		Parsed:       p.parsed.Load(),
		ParseFailed:  p.parseFailed.Load(),
		Published:    p.published.Load(),
		PublishRetry: p.publishRetry.Load(),
		Panics:       p.panics.Load(),
		QueueDepth:   len(p.jobs),
	}
}

// parseLoop 消费队列直到队列关闭。
func (p *Pipeline) parseLoop(ctx context.Context) {
	for job := range p.jobs {
		event := p.parse(job)

		select {
		case p.events <- event:
			p.inflight.Done()
		case <-ctx.Done():
			p.inflight.Done()

			return
		}
	}
}

// Parse 只做解析与信封构造，不发布。
// Kafka 消费需要"确认发布成功后再提交位移"，因此由它自己掌控发布与提交的顺序。
func (p *Pipeline) Parse(snapshot connector.Snapshot, raw []byte, received time.Time) Event {
	return p.parse(Job{Snapshot: snapshot, Raw: raw, Received: received})
}

// parse 把一条报文解析成事件。
// 引擎 panic 只影响这一条：记原因码、计数，事件照常发布——
// 单条坏消息不能拖垮 worker，也不能让整批消失。
func (p *Pipeline) parse(job Job) Event {
	event := Event{
		ConnectorID: job.Snapshot.ID,
		EventID:     normalizer.EventID(job.Snapshot.ID, p.sequence.Add(1), job.Raw),
	}

	fields, reasons := p.apply(job)
	if len(reasons) > 0 {
		event.Status = StatusError
		event.Reasons = reasons
		p.parseFailed.Add(1)
	} else {
		event.Status = StatusOK
		p.parsed.Add(1)
	}

	event.PartitionKey = partitionKey(fields, event.EventID)
	event.Body = buildBody(job, fields, event)

	return event
}

// apply 调用解析规则；未挂规则的接入点走原文透传。
func (p *Pipeline) apply(job Job) (fields map[string]any, reasons []string) {
	// 通道已完成第 1 层：直接用它的字段。
	if job.Fields != nil {
		return job.Fields, nil
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			p.panics.Add(1)
			logging.From(context.Background()).Error("Parse engine panicked",
				"event", "parse.panic", "connector_id", job.Snapshot.ID,
				"panic_kind", fmt.Sprintf("%T", recovered), "stack", logging.Stack())

			fields, reasons = nil, []string{ReasonEnginePanic}
		}
	}()

	// 未挂规则：原文透传。spec 规定这类事件的 parse_status 仍为 error，
	// 因为它还没有被真正解析过，后续可换正式规则重放。
	if job.Snapshot.Engine == nil {
		return nil, []string{ReasonFormatMismatch}
	}

	event, err := job.Snapshot.Engine.Apply(job.Raw, job.Received)
	if err != nil {
		if errors.Is(err, parser.ErrMalformed) {
			return nil, []string{ReasonFormatMismatch}
		}

		return nil, []string{ReasonFormatMismatch}
	}

	for _, warning := range event.Warnings {
		logging.From(context.Background()).Debug("Rule warning", "event", "parse.warning",
			"connector_id", job.Snapshot.ID, "warning", warning)
	}

	return event.Fields, nil
}

// partitionKey 按 spec 的实体优先级取分区键：
// src_endpoint.ip → device.hostname → user.name → event_id。
// 同一实体的事件必须落在同一分区，否则下游无法按实体保序。
func partitionKey(fields map[string]any, eventID string) string {
	for _, path := range []string{"src_endpoint.ip", "device.hostname"} {
		if value := nestedString(fields, path); value != "" {
			return value
		}
	}

	if user, ok := fields["user"].(map[string]any); ok {
		if name, ok := user["name"].(string); ok && name != "" {
			return name
		}
	}

	return eventID
}

// nestedString 读取 dot-path 上的字符串值。
func nestedString(fields map[string]any, path string) string {
	current := any(fields)

	for _, segment := range splitPath(path) {
		nested, ok := current.(map[string]any)
		if !ok {
			return ""
		}

		current, ok = nested[segment]
		if !ok {
			return ""
		}
	}

	value, _ := current.(string)
	return value
}

// buildBody 生成要发布的事件 JSON：OCSF 字段 + metadata。
func buildBody(job Job, fields map[string]any, event Event) []byte {
	envelope := make(map[string]any, len(fields)+2)
	for key, value := range fields {
		envelope[key] = value
	}

	if event.Status == StatusError {
		// 兜底类 + 原文：未解析的报文仍然可查询、可统计、可换规则重放。
		envelope["class_uid"] = ClassUIDGeneric
		envelope["severity_id"] = normalizer.SeverityUnknown
		envelope["time"] = job.Received.UTC().UnixMilli()
		envelope["message"] = string(job.Raw)
	}

	metadata := map[string]any{
		"connector_id": job.Snapshot.ID,
		"event_id":     event.EventID,
		"parse_status": string(event.Status),
	}
	if len(event.Reasons) > 0 {
		metadata["parse_reasons"] = event.Reasons
	}
	envelope["metadata"] = metadata

	body, err := json.Marshal(envelope)
	if err != nil {
		// 字段里出现无法序列化的值只可能是内部类型错误；
		// 退回一个带原因的事件，不让它消失。
		fallback, _ := json.Marshal(map[string]any{
			"class_uid":   ClassUIDGeneric,
			"severity_id": normalizer.SeverityUnknown,
			"time":        job.Received.UTC().UnixMilli(),
			"message":     string(job.Raw),
			"metadata": map[string]any{
				"connector_id":  job.Snapshot.ID,
				"event_id":      event.EventID,
				"parse_status":  string(StatusError),
				"parse_reasons": []string{ReasonFormatMismatch},
			},
		})

		return fallback
	}

	return body
}

// splitPath 按点号切分路径。
func splitPath(path string) []string {
	segments := make([]string, 0, 4)
	start := 0

	for index := 0; index < len(path); index++ {
		if path[index] == '.' {
			segments = append(segments, path[start:index])
			start = index + 1
		}
	}

	return append(segments, path[start:])
}

// batchLoop 累积事件并在满批或超时时发布；只在 events 关闭后退出。
func (p *Pipeline) batchLoop(ctx context.Context) error {
	batch := make([]Event, 0, p.options.BatchSize)
	ticker := time.NewTicker(p.options.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case event, ok := <-p.events:
			if !ok {
				p.publish(ctx, batch)
				return nil
			}

			batch = append(batch, event)
			if len(batch) >= p.options.BatchSize {
				p.publish(ctx, batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				p.publish(ctx, batch)
				batch = batch[:0]
			}
		case done := <-p.flush:
			// Flush 保证在途任务都已投递到 events，但这些事件可能还在通道缓冲里，
			// 而 select 可能先选中 flush 分支。先非阻塞排空缓冲，再发布，
			// 否则最后一个事件会留到下一批，调用方看到的计数偏少。
			batch = drain(p.events, batch)
			p.publish(ctx, batch)
			batch = batch[:0]

			close(done)
		}
	}
}

// drain 非阻塞地把事件通道里已就绪的事件并入当前批次。
// 通道关闭时返回已合并的批次，由调用方按停机路径处理。
func drain(events <-chan Event, batch []Event) []Event {
	for {
		select {
		case pending, open := <-events:
			if !open {
				return batch
			}

			batch = append(batch, pending)
		default:
			return batch
		}
	}
}

// publish 发送一批事件；失败重试直到成功或进程退出。
// 不丢弃：发布失败意味着下游会缺数据，而重试的成本远低于丢事件。
func (p *Pipeline) publish(ctx context.Context, batch []Event) {
	if len(batch) == 0 {
		return
	}

	backoff := 100 * time.Millisecond
	for {
		err := p.publisher.Publish(ctx, batch)
		if err == nil {
			p.published.Add(uint64(len(batch)))
			return
		}

		if ctx.Err() != nil {
			logging.From(ctx).Warn("Dropping batch at shutdown", "event", "publish.aborted",
				"events", len(batch), "error", logging.Error(err))
			return
		}

		p.publishRetry.Add(1)
		logging.From(ctx).Warn("Publish failed; retrying", "event", "publish.retry",
			"events", len(batch), "backoff_ms", backoff.Milliseconds(), "error", logging.Error(err))

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
		}

		if backoff < 5*time.Second {
			backoff *= 2
		}
	}
}
