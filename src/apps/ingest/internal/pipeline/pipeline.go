// Package pipeline 把通道收到的原始报文变成已发布的 OCSF 事件。
//
// 它负责三件通道不该操心的事：固定 worker 池解析、按批流转、失败事件照常发布。
// 通道只调用 Submit；解析规则、并发、批大小与背压都在这里。
package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/normalizer"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/schema/events"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// Status 是事件的解析状态，随事件发布到 metadata.spx.parse.outcome。
type Status string

const (
	StatusPartial Status = "partial" // 部分字段通过验证，已生成 Event。
	StatusOK      Status = "ok"      // 规则解析成功。
	StatusError   Status = "error"   // 解析失败；事件仍发布，可查询、可补救。
)

// Stage 是失败发生的层次，决定排查方向：
// decode 看报文与解析模式，map 看字段映射与规则，limit 看上限与设备侧写法。
type Stage string

const (
	StageClassify Stage = "classify" // 未确定事件类别。
	StageDecode   Stage = "decode"   // 第 1 层：线格式解析失败。
	StageMap      Stage = "map"      // 第 2 层：规则映射失败。
)

// Reason 是稳定的机器可读原因码，用于聚合、告警与看板。
// 取值一旦发布不能改名：下游按它分组统计。
// 它与人类可读的描述刻意分开——原因码可枚举，描述不可枚举。
type Reason string

const (
	ReasonNoMatchingBranch  Reason = "no_matching_branch" // 没有符合条件的分支。
	ReasonNoRule            Reason = "no_rule"            // 接入点未挂解析规则，原文透传。
	ReasonFormatMismatch    Reason = "format_mismatch"    // 报文与规则的线格式不符。
	ReasonMultipleRecords   Reason = "multiple_records"   // 单条报文包含多条记录。
	ReasonTruncated         Reason = "truncated"          // 报文超上限被截断，解析结果不可信。
	ReasonContractViolation Reason = "contract_violation" // 映射结果不满足所选事件类的字段约束。
	ReasonEnginePanic       Reason = "engine_panic"       // 引擎 panic，正常不应出现。
)

// Outcome 是一次解析的结果。
// 四个字段回答四个不同的问题：Status 什么结果、Stage 哪一层失败、
// Reason 为什么（可聚合）。
type Outcome struct {
	Status Status // ok 或 error。
	Stage  Stage  // 失败层次；成功时为空。
	Reason Reason // 稳定原因码；成功时为空。
}

// OK 报告解析是否成功。
func (o Outcome) OK() bool { return o.Status == StatusOK }

// Product 描述产出方，填进 OCSF 必填的 metadata.product。
var Product = map[string]any{
	"name":        "SecPlexus ingest",
	"vendor_name": "SecPlexus",
	"version":     "0.1.0",
}

// ErrClosed 表示管道已进入关停，不再受理新报文。
// 调用方应把报文退回上游（HTTP 返回 503 让设备重推），而不是当作已受理。
var ErrClosed = errors.New("pipeline is closed")

// ErrBusy 表示队列在宽限期内仍无法接纳：调用方应回 503 + Retry-After 让上游重试。
var ErrBusy = errors.New("pipeline queue is full")

// Event 是一条待发布的事件。
type Event struct {
	BranchID        string
	CandidateClass  int
	TimeSource      string
	AppliedDefaults []string
	Issues          []normalizer.Issue
	SourceID        string  // 来源接入点。
	EventID         string  // 确定性标识，供下游去重。
	PartitionKey    string  // 分区键，保证同一实体的事件落在同一分区。
	Body            []byte  // OCSF 事件 JSON。
	Outcome         Outcome // 解析结果：状态、阶段、原因码与细节。

	// Truncated 与 OriginalBytes 是数据完整性事实，与"解析是否成功"分开：
	// 一条被截断的报文可能仍然解析成功（字段都在头部），
	// 下游需要能分别对"不完整"和"解析失败"告警。
	Truncated     bool
	OriginalBytes int
	observer      Observations
	confirmed     chan struct{} // HTTP 等待 Kafka 确认的单次回执。
}

// Publisher 是事件出口。管道只依赖这个接口，Kafka 是它的一个实现。
// Publish 返回 PartialPublishError 时仅重试指定记录，其他错误重试整批。
type Publisher interface {
	Publish(ctx context.Context, events []Event) error
}

// Meta 是通道提供的报文元信息。
// 截断必须随事件一起落库：分析师看到的是不完整数据，这一点不能只写在日志里。
type Meta struct {
	SourceRecordID string // Kafka 来源消息关联标识。
	Decoded        bool   // 通道已完成字符集转换。
	DecodeFailed   bool   // 通道的字符集转换失败。
	Truncated      bool   // 报文是否被截断。
	OriginalBytes  int    // 截断前的原始长度；未截断时为 0。
}

// Job 是一条待解析的报文。
type Job struct {
	Source   input.Entry
	Raw      []byte
	Received time.Time
	// Fields 非空表示第 1 层已由通道完成（如 syslog 剥信封），
	// 管道直接进第 2 层映射，不再调用解析器。
	Fields    map[string]any
	Meta      Meta
	confirmed chan struct{}
	eventID   string
}

// Stats 是可观测计数，供健康检查与运行指标使用。
type Stats struct {
	Accepted     uint64 // 受理的报文数。
	Parsed       uint64 // 解析成功的事件数。
	ParseFailed  uint64 // 解析失败的事件数（仍会发布）。
	Published    uint64 // 已发布的事件数。
	PublishRetry uint64 // 发布重试次数。
	Dropped      uint64 // 排空超时被丢弃的事件数；正常应恒为 0。
	// ByProtocol 是按接入协议分维度的计数，键是 http / syslog / kafka。
	// 看板需要"哪个通道出了问题"，全局计数回答不了。
	ByProtocol map[string]ProtocolStats
	// ByInput 是按接入点分维度的计数，键是接入点标识。
	// 基数受 maxInputSeries 限制：标签值来自用户创建的资源，必须设上限。
	ByInput       map[string]InputStats
	Panics        uint64 // 被 recover 兜住的引擎 panic 次数。
	QueueDepth    int    // 当前待解析队列长度。
	InFlightBytes int64  // 解析队列及 worker 持有的输入字节预算。
}

// Options 是管道容量参数。
// Observations 接收本地流水线阶段，不在受理或确认路径访问网络。
type Observations interface {
	Receive(string, time.Time)
	Normalize(string, time.Time, Status)
	Confirm(string, time.Time)
}

type Options struct {
	Observations     Observations
	Workers          int           // 解析 worker 数；默认等于 CPU 核数。
	QueueDepth       int           // 待解析队列长度；默认 10000（约 0.2 秒的 5w EPS）。
	MaxInFlightBytes int64         // 等待及解析中的输入字节上限；独立构造时零表示无上限。
	BatchSize        int           // 每批事件数；默认 500。
	FlushInterval    time.Duration // 批量最长等待；默认 2 秒。
	// OverflowGrace 是队列满时允许等待的上限；默认 250 毫秒。
	// 与其让设备等到 TCP 超时，不如尽快回 503 + Retry-After，让重试有明确节奏。
	OverflowGrace time.Duration
	// DrainTimeout 是关停时排空与最后一批发布的预算；默认 15 秒。
	// 有界是刻意的：无界排空会让进程永远退不出，强制杀掉又会丢数据。
	// 超时后丢弃并记 Error，让运维看得见，而不是静默卡住。
	DrainTimeout time.Duration
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
	if o.OverflowGrace <= 0 {
		o.OverflowGrace = 250 * time.Millisecond
	}
	if o.DrainTimeout <= 0 {
		o.DrainTimeout = 15 * time.Second
	}

	return o
}

// Pipeline 是接收与发布之间的解析流水线。
// 启动后由固定数量的 worker 消费队列，无 per-event goroutine。
type Pipeline struct {
	options   Options
	publisher Publisher

	// 停机先关闭受理入口，再等待提交者离开后关闭 jobs。
	jobs   chan Job
	events chan Event
	flush  chan chan struct{}

	done           chan struct{}
	closeOnce      sync.Once
	submitMu       sync.Mutex
	submitters     sync.WaitGroup
	inputBytes     atomic.Int64
	workers        sync.WaitGroup
	batcher        chan struct{}
	publishContext context.Context
	cancelPublish  context.CancelFunc
	drainTimer     *time.Timer

	sequence atomic.Uint64
	// epoch 标识本次进程启动；参与 event_id，避免重启后与上一批事件撞 ID。
	epoch [16]byte

	// inflight 统计已提交但尚未进入批处理的报文数。
	// Flush 与优雅停机都靠它判断"在途是否排空"，只看批处理缓冲区会漏掉解析队列里的事件。
	inflight sync.WaitGroup

	accepted     atomic.Uint64
	parsed       atomic.Uint64
	parseFailed  atomic.Uint64
	published    atomic.Uint64
	publishRetry atomic.Uint64
	panics       atomic.Uint64
	dropped      atomic.Uint64
	// 分协议计数：固定三组原子量，避免热路径上出现 map 锁。
	httpAccepted   atomic.Uint64
	httpParsed     atomic.Uint64
	syslogAccepted atomic.Uint64
	syslogParsed   atomic.Uint64
	pullAccepted   atomic.Uint64
	// byInput 按接入点计数；sync.Map 让热路径无锁，容量由 maxInputSeries 兜底。
	byInput       sync.Map
	inputSeries   atomic.Int64
	pullParsed    atomic.Uint64
	kafkaAccepted atomic.Uint64
	kafkaParsed   atomic.Uint64
}

// New 创建管道；Start 才会启动 worker。
func New(options Options, publisher Publisher) *Pipeline {
	options = options.defaults()
	publishContext, cancelPublish := context.WithCancel(context.Background())

	instance := &Pipeline{
		options:        options,
		publisher:      publisher,
		jobs:           make(chan Job, options.QueueDepth),
		events:         make(chan Event, options.BatchSize*2),
		flush:          make(chan chan struct{}),
		done:           make(chan struct{}),
		batcher:        make(chan struct{}),
		publishContext: publishContext,
		cancelPublish:  cancelPublish,
	}

	// 启动期标识：取不到随机源时退化为时间戳，仍能区分进程。
	if _, err := rand.Read(instance.epoch[:]); err != nil {
		binary.BigEndian.PutUint64(instance.epoch[:8], uint64(time.Now().UnixNano()))
	}

	return instance
}

// SubmitFields 受理一条已由通道完成线格式解析的报文。
// syslog 通道用 go-syslog 剥完信封后走这里：字段已就绪，只差规则映射。
func (p *Pipeline) SubmitFields(ctx context.Context, entry input.Entry, fields map[string]any, received time.Time, meta Meta) error {
	return p.submit(ctx, Job{Source: entry, Fields: fields, Received: received, Meta: meta})
}

// Submit 受理一条报文。条数及在途字节预算用尽时返回 ErrBusy。
func (p *Pipeline) Submit(ctx context.Context, entry input.Entry, raw []byte, received time.Time, meta Meta) error {
	return p.submit(ctx, Job{Source: entry, Raw: raw, Received: received, Meta: meta})
}

// SubmitTracked 受理一条需确认的报文。回执只在 Kafka 已确认后关闭。
func (p *Pipeline) SubmitTracked(ctx context.Context, entry input.Entry, raw []byte, received time.Time) (<-chan struct{}, error) {
	confirmed := make(chan struct{})
	err := p.submit(ctx, Job{Source: entry, Raw: raw, Received: received, confirmed: confirmed})
	return confirmed, err
}

// Done 在发布排空期限结束或流水线退出时关闭。
func (p *Pipeline) Done() <-chan struct{} { return p.publishContext.Done() }

func (p *Pipeline) submit(ctx context.Context, job Job) error {
	p.submitMu.Lock()
	if err := ctx.Err(); err != nil {
		p.submitMu.Unlock()
		return err
	}
	select {
	case <-p.done:
		p.submitMu.Unlock()
		return ErrClosed
	default:
	}
	p.submitters.Add(1)
	p.submitMu.Unlock()
	defer p.submitters.Done()

	size := job.bytes()
	for {
		used := p.inputBytes.Load()
		if p.options.MaxInFlightBytes > 0 && size > p.options.MaxInFlightBytes-used {
			return ErrBusy
		}
		if p.inputBytes.CompareAndSwap(used, used+size) {
			break
		}
	}

	// Flush 必须能看到正在提交的报文，即使此时仍在等队列空位。
	p.inflight.Add(1)
	if err := p.enqueue(ctx, job); err != nil {
		p.inflight.Done()
		p.inputBytes.Add(-size)
		return err
	}
	p.accepted.Add(1)
	if p.options.Observations != nil {
		p.options.Observations.Receive(job.Source.ID, time.Now())
	}
	return nil
}

func (j Job) bytes() int64 {
	return int64(max(cap(j.Raw), j.Meta.OriginalBytes))
}

// Start 启动解析 worker 与批处理器，阻塞到 ctx 取消或 Close 被调用，然后按顺序关停：
// 拒绝新提交 → 排空在途解析 → 用独立预算发布最后一批。
func (p *Pipeline) Start(ctx context.Context) error {
	for worker := 0; worker < p.options.Workers; worker++ {
		p.workers.Add(1)

		go func() {
			defer p.workers.Done()
			p.parseLoop()
		}()
	}

	go func() {
		p.batchLoop()
		close(p.batcher)
	}()

	select {
	case <-ctx.Done():
	case <-p.done:
	}

	return p.shutdown()
}

// Close 让管道进入关停：不再受理新报文，已在途的继续处理。
// 幂等，可在信号处理器里直接调用。
func (p *Pipeline) Close() {
	p.closeOnce.Do(func() {
		p.submitMu.Lock()
		p.drainTimer = time.AfterFunc(p.options.DrainTimeout, p.cancelPublish)
		close(p.done)
		p.submitMu.Unlock()
	})
}

// shutdown 执行有序关停，返回最后一批的发布错误。
func (p *Pipeline) shutdown() error {
	p.Close()
	defer p.drainTimer.Stop()
	defer p.cancelPublish()

	p.submitters.Wait()
	close(p.jobs)
	p.workers.Wait()
	close(p.events)
	<-p.batcher

	if p.publishContext.Err() != nil {
		logging.From(p.publishContext).Error("Pipeline drain deadline reached",
			"event", "publish.drain_timeout", "dropped_total", p.dropped.Load())
	}

	return nil
}

// Flush 等待在途报文全部解析完，再让批处理器立刻发送当前批次。
// 只冲刷批处理缓冲区是不够的：解析队列里的事件还没变成事件，
// 调用方会看到"零事件"而误判为丢数据。
func (p *Pipeline) Flush(ctx context.Context) error {
	drained := make(chan struct{})
	go func() {
		p.inflight.Wait()
		close(drained)
	}()

	select {
	case <-drained:
	case <-ctx.Done():
		return ctx.Err()
	}

	done := make(chan struct{})
	select {
	case p.flush <- done:
		<-done
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stats 返回当前计数快照。
func (p *Pipeline) Stats() Stats {
	return Stats{
		Accepted:      p.accepted.Load(),
		Parsed:        p.parsed.Load(),
		ParseFailed:   p.parseFailed.Load(),
		Published:     p.published.Load(),
		PublishRetry:  p.publishRetry.Load(),
		Panics:        p.panics.Load(),
		Dropped:       p.dropped.Load(),
		ByProtocol:    p.protocolSnapshot(),
		ByInput:       p.inputSnapshot(),
		QueueDepth:    len(p.jobs),
		InFlightBytes: p.inputBytes.Load(),
	}
}

// parseLoop 消费队列，直到停止接收并结清队列内的报文。
func (p *Pipeline) parseLoop() {
	for job := range p.jobs {
		p.handle(job)
	}
}

// handle 解析一条报文并投给批处理器。
func (p *Pipeline) handle(job Job) {
	defer p.inflight.Done()
	defer p.inputBytes.Add(-job.bytes())
	if p.publishContext.Err() != nil {
		p.dropped.Add(1)
		return
	}

	event := p.parse(job)
	select {
	case p.events <- event:
	case <-p.publishContext.Done():
		p.dropped.Add(1)
	}
}

// Parse 只做解析与信封构造，不发布。
// Kafka 消费需要"确认发布成功后再提交位移"，因此由它自己掌控发布与提交的顺序。
func (p *Pipeline) Parse(entry input.Entry, raw []byte, received time.Time, eventID string) Event {
	return p.parse(Job{Source: entry, Raw: raw, Received: received, eventID: eventID})
}

// ParseKafka 解析已由分区边界固定配置的记录，并保留独立的来源身份。
func (p *Pipeline) ParseKafka(entry input.Entry, raw []byte, received time.Time, eventID, sourceRecordID string) Event {
	if p.options.Observations != nil {
		p.options.Observations.Receive(entry.ID, time.Now())
	}
	return p.parse(Job{Source: entry, Raw: raw, Received: received, eventID: eventID, Meta: Meta{SourceRecordID: sourceRecordID}})
}

// parse 把一条报文解析成事件。
// 引擎 panic 只影响这一条：记原因码、计数，事件照常发布——
// 单条坏消息不能拖垮 worker，也不能让整批消失。
func (p *Pipeline) parse(job Job) Event {
	event := Event{
		SourceID:  job.Source.ID,
		EventID:   job.eventID,
		confirmed: job.confirmed,
		observer:  p.options.Observations,
	}
	if event.EventID == "" {
		event.EventID = normalizer.EventID(job.Source.ID, p.epoch, p.sequence.Add(1), job.Raw)
	}

	event, _, _ = job.mixedEvent(event)
	if event.Outcome.Reason == ReasonEnginePanic {
		p.panics.Add(1)
		logging.From(context.Background()).Error("Parse engine panicked",
			"event", "parse.engine_panic", "input_id", job.Source.ID)
	}
	if p.options.Observations != nil {
		p.options.Observations.Normalize(job.Source.ID, time.Now(), event.Outcome.Status)
	}
	p.countOutcome(job.Source, event.Outcome.Status)
	return event
}

func (p *Pipeline) countOutcome(source input.Entry, status Status) {
	parsed := status == StatusOK
	p.countProtocol(source.Protocol, parsed)
	p.countInput(source.ID, status)
	if parsed {
		p.parsed.Add(1)
	} else {
		p.parseFailed.Add(1)
	}
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

// buildBody 组装发布正文。无分类的诊断记录保留接收时间和来源。
func buildBody(job Job, fields, unmapped map[string]any, event Event) ([]byte, error) {
	envelope := make(map[string]any, len(fields)+2)
	for key, value := range fields {
		envelope[key] = value
	}

	// OCSF base_event 的 required 属性：category_uid、type_uid、activity_id、metadata。
	// 缺了它们，任何 OCSF 消费者都无法分类或建表，所以在这里补齐而不是留给下游猜。
	if classUID, ok := numeric(envelope["class_uid"]); ok {
		activityID := activityIDOf(envelope)
		envelope["activity_id"] = activityID
		envelope["category_uid"] = classUID / 1000
		envelope["type_uid"] = classUID*100 + activityID
	}

	// OCSF 的 unmapped 属性就是给"规则未使用的源字段"准备的；
	// 丢掉它们会让排查时看不到原始上下文，也会让脱敏逻辑变成死代码。
	if len(unmapped) > 0 {
		envelope["unmapped"] = unmapped
	}

	// 我方簿记放进 metadata.spx：metadata 是 OCSF 预留对象，
	// 直接塞 input_id/parse.outcome 会占用标准语义并挡掉必填的 version/product。
	bookkeeping := map[string]any{
		"input_id":      job.Source.ID,
		"vendor":        job.Source.Vendor,
		"product":       job.Source.Product,
		"parse.outcome": string(event.Outcome.Status),
	}

	if job.Meta.SourceRecordID != "" {
		bookkeeping["source_record_id"] = job.Meta.SourceRecordID
		bookkeeping["source_revision"] = job.Source.Revision
	}
	if event.BranchID != "" {
		bookkeeping["branch_id"] = event.BranchID
	}
	bookkeeping["time_source"] = event.TimeSource
	if len(event.Issues) > 0 {
		bookkeeping["parse.issues"] = event.Issues
	}
	if event.CandidateClass > 0 {
		bookkeeping["candidate_class_uid"] = event.CandidateClass
	}
	if job.Source.ParsingRuleID != "" {
		bookkeeping["parsing_rule_id"] = job.Source.ParsingRuleID
		bookkeeping["parsing_rule_version"] = job.Source.ParsingRuleVersion
	}

	if !event.Outcome.OK() {
		bookkeeping["parse.error"] = map[string]any{
			"stage":   string(event.Outcome.Stage),
			"reason":  string(event.Outcome.Reason),
			"message": "Could not parse event",
		}
	}

	if event.Truncated {
		bookkeeping["truncated"] = true
		bookkeeping["original_bytes"] = event.OriginalBytes
	}

	// 规则若已写 metadata（如 product 常量），合并而不是丢弃。
	metadata := map[string]any{}
	if existing, ok := envelope["metadata"].(map[string]any); ok {
		for key, value := range existing {
			metadata[key] = value
		}
	}

	metadata["version"] = events.CurrentOCSFVersion
	bookkeeping["contract_version"] = events.CurrentVersion
	bookkeeping["received_at"] = job.Received.UTC().UnixMilli()
	product, _ := metadata["product"].(map[string]any)
	if product == nil {
		product = make(map[string]any, 2)
	}
	defaults := Product
	if job.Source.Product != "" {
		defaults = map[string]any{"name": job.Source.Product, "vendor_name": job.Source.Vendor}
	}
	for _, key := range []string{"name", "vendor_name"} {
		if _, exists := product[key]; !exists && defaults[key] != "" {
			product[key] = defaults[key]
		}
	}
	metadata["product"] = product

	metadata["uid"] = event.EventID

	metadata["spx"] = bookkeeping
	envelope["metadata"] = metadata

	return json.Marshal(envelope)
}

// activityIDOf 取规则声明的活动标识；未声明时用 OCSF 的 0（Unknown）。
func activityIDOf(envelope map[string]any) int64 {
	if value, ok := numeric(envelope["activity_id"]); ok {
		return value
	}

	return 0
}

// numeric 把 JSON 数字或整型统一成 int64。
// 规则常量可能是 int、int64 或 float64，这里一处收口，避免三处类型断言。
func numeric(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	}

	return 0, false
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

// batchLoop 累积事件并在满批或超时时发布；events 关闭后发布最后一批再退出。
func (p *Pipeline) batchLoop() {
	batch := make([]Event, 0, p.options.BatchSize)
	ticker := time.NewTicker(p.options.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case event, ok := <-p.events:
			if !ok {
				p.publish(batch)
				return
			}

			batch = append(batch, event)
			if len(batch) >= p.options.BatchSize {
				p.publish(batch)
				clear(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				p.publish(batch)
				clear(batch)
				batch = batch[:0]
			}
		case done := <-p.flush:
			// Flush 保证在途任务都已投递到 events，但可能还在缓冲里；
			// select 可能先选中 flush，所以先非阻塞排空再发布。
			batch = drain(p.events, batch)
			p.publish(batch)
			clear(batch)
			batch = batch[:0]

			close(done)
		}
	}
}

// drain 非阻塞地把事件通道里已就绪的事件并入当前批次。
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

// publish 发送一批事件。
//
// 稳态下无限重试：发布失败意味着下游缺数据，背压继续往上游传。
// 关停时改为有界——从进入关停起 DrainTimeout 内还发不出去就丢弃并记 Error，
// 因为无界等待会让进程永远退不出，而编排层强杀只会丢数据。
func (p *Pipeline) publish(batch []Event) {
	if len(batch) == 0 {
		return
	}

	backoff := 100 * time.Millisecond

	ctx := p.publishContext
	for ctx.Err() == nil {
		size := len(batch)
		pending, err := PublishPending(ctx, p.publisher, batch)
		p.published.Add(uint64(size - len(pending)))
		batch = pending
		if err == nil {
			return
		}

		if ctx.Err() != nil {
			break
		}

		p.publishRetry.Add(1)
		logging.From(context.Background()).Warn("Publish failed; retrying", "event", "publish.retry",
			"events", len(batch), "backoff_ms", backoff.Milliseconds(), "error", logging.Error(err))

		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
		case <-ctx.Done():
		}
		timer.Stop()
		backoff = min(backoff*2, 5*time.Second)
	}

	p.dropped.Add(uint64(len(batch)))
}

// ProtocolStats 是单个协议的计数。
type ProtocolStats struct {
	Accepted uint64
	Parsed   uint64
}

// countProtocol 累加分协议计数；解析成功与否分别记账。
func (p *Pipeline) countProtocol(protocol input.Protocol, parsed bool) {
	switch protocol {
	case input.ProtocolHTTP:
		p.httpAccepted.Add(1)
		if parsed {
			p.httpParsed.Add(1)
		}
	case input.ProtocolSyslog:
		p.syslogAccepted.Add(1)
		if parsed {
			p.syslogParsed.Add(1)
		}
	case input.ProtocolPull:
		p.pullAccepted.Add(1)
		if parsed {
			p.pullParsed.Add(1)
		}
	case input.ProtocolKafka:
		p.kafkaAccepted.Add(1)
		if parsed {
			p.kafkaParsed.Add(1)
		}
	}
}

// protocolSnapshot 读取分协议计数。
func (p *Pipeline) protocolSnapshot() map[string]ProtocolStats {
	return map[string]ProtocolStats{
		string(input.ProtocolHTTP):   {Accepted: p.httpAccepted.Load(), Parsed: p.httpParsed.Load()},
		string(input.ProtocolSyslog): {Accepted: p.syslogAccepted.Load(), Parsed: p.syslogParsed.Load()},
		string(input.ProtocolKafka):  {Accepted: p.kafkaAccepted.Load(), Parsed: p.kafkaParsed.Load()},
		string(input.ProtocolPull):   {Accepted: p.pullAccepted.Load(), Parsed: p.pullParsed.Load()},
	}
}

// maxInputSeries 限制按接入点的序列数。
//
// 指标标签的高基数是采集侧的真实风险；接入点是用户创建的资源，
// 必须设上限，否则一次误操作（批量建接入点）就能拖垮抓取端。
const maxInputSeries = 512

// inputCounters 是单个接入点的计数。
type inputCounters struct {
	partial      atomic.Uint64
	unclassified atomic.Uint64
	accepted     atomic.Uint64
	parsed       atomic.Uint64
	lastReceived atomic.Int64
}

// InputStats 是本进程观察到的接入点累计收包和解析结果。
type InputStats struct {
	ProtocolStats
	Partial      uint64
	Unclassified uint64
	LastReceived time.Time
}

// countInput 累加按接入点的计数。
//
// 超过上限后不再新增序列：宁可在指标里"看不到那个接入点"，
// 也不能让内存与抓取端基数无限增长。
func (p *Pipeline) countInput(inputID string, status Status) {
	if inputID == "" {
		return
	}

	value, ok := p.byInput.Load(inputID)
	if !ok {
		if p.inputSeries.Load() >= maxInputSeries {
			return
		}

		counters := &inputCounters{}
		actual, loaded := p.byInput.LoadOrStore(inputID, counters)
		if !loaded {
			p.inputSeries.Add(1)
		}

		value = actual
	}

	counters, ok := value.(*inputCounters)
	if !ok {
		return
	}

	counters.accepted.Add(1)
	counters.lastReceived.Store(time.Now().UnixMilli())
	switch status {
	case StatusOK:
		counters.parsed.Add(1)
	case StatusPartial:
		counters.partial.Add(1)
	default:
		counters.unclassified.Add(1)
	}
}

// inputSnapshot 读取按接入点的计数。
func (p *Pipeline) inputSnapshot() map[string]InputStats {
	snapshot := make(map[string]InputStats, 8)
	p.byInput.Range(func(key, value any) bool {
		inputID, keyOK := key.(string)
		counters, valueOK := value.(*inputCounters)
		if keyOK && valueOK {
			parsed := counters.parsed.Load()
			if last := counters.lastReceived.Load(); last > 0 {
				snapshot[inputID] = InputStats{
					ProtocolStats: ProtocolStats{Accepted: counters.accepted.Load(), Parsed: parsed},
					LastReceived:  time.UnixMilli(last).UTC(),
					Partial:       counters.partial.Load(), Unclassified: counters.unclassified.Load(),
				}
			}
		}

		return true
	})

	return snapshot
}

// enqueue 先快速尝试入队，队列满时最多等待 OverflowGrace。
//
// 有界等待是刻意的：设备端的重试节奏应来自我们的 503 + Retry-After，
// 而不是让它一直挂到 TCP 超时才发现没人收。
func (p *Pipeline) enqueue(ctx context.Context, job Job) error {
	select {
	case p.jobs <- job:
		return nil
	default:
	}

	timer := time.NewTimer(p.options.OverflowGrace)
	defer timer.Stop()

	select {
	case p.jobs <- job:
		return nil
	case <-p.done:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ErrBusy
	}
}
