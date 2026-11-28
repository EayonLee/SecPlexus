// Package syslogchannel 实现 UDP/TCP syslog 接收。
//
// 契约（spec）：一个端口绑定一个接入点，报文内不读任何身份字段；
// UDP 与 TCP 同端口；RFC5424/3164/6587 自动识别；单条 64 KiB 截断；
// 端口绑定关系来自注册表，支持周期刷新与 SIGHUP 立即重载。
package syslogchannel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

const (
	maxTCPConnections = 256
	tcpReadTimeout    = 2 * time.Minute
)

// Channel 管理所有 syslog 接入点的监听器。
type Channel struct {
	registry input.Lookup
	pipeline *pipeline.Pipeline
	tcpSlots chan struct{} // 所有 Syslog 端口共用的连接预算。

	mu        sync.Mutex
	listeners map[string]*binding // 接入点 ID → 监听器。

	reloadInterval time.Duration
	bindHost       string // 监听地址；空值表示所有网卡。
}

// binding 是一个接入点的 UDP 与 TCP 监听器。
type binding struct {
	entry  input.Entry
	udp    net.PacketConn
	tcp    net.Listener
	cancel context.CancelFunc
	done   sync.WaitGroup
	closed atomic.Bool
}

// New 创建 syslog 通道。reloadInterval 为零时用 10 秒。
func New(registry input.Lookup, line *pipeline.Pipeline, reloadInterval time.Duration, bindHost string) *Channel {
	if reloadInterval <= 0 {
		reloadInterval = 10 * time.Second
	}

	return &Channel{
		registry:       registry,
		pipeline:       line,
		tcpSlots:       make(chan struct{}, maxTCPConnections),
		listeners:      make(map[string]*binding),
		reloadInterval: reloadInterval,
		bindHost:       bindHost,
	}
}

// Ready 报告指定接入的实际 TCP、UDP 监听器是否已绑定。
func (c *Channel) Ready(entry input.Entry) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	active := c.listeners[entry.ID]
	return active != nil && active.entry.Port == entry.Port && !active.closed.Load()
}

// Start 首次绑定并进入周期刷新；SIGHUP 触发立即重载。
func (c *Channel) Start(ctx context.Context) error {
	if err := c.Reload(ctx); err != nil {
		// 首次绑定失败不阻断进程：其他通道仍可工作，错误进日志由运维处置。
		logging.From(ctx).Error("Initial syslog binding failed", "event", "syslog.bind_failed",
			"error", logging.Error(err))
	}

	hangup := make(chan os.Signal, 1)
	signal.Notify(hangup, syscall.SIGHUP)
	defer signal.Stop(hangup)

	ticker := time.NewTicker(c.reloadInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.Close()
			return nil
		case <-ticker.C:
			if err := c.Reload(ctx); err != nil {
				logging.From(ctx).Error("Periodic syslog reload failed", "event", "syslog.reload_failed",
					"error", logging.Error(err))
			}
		case <-hangup:
			logging.From(ctx).Info("Syslog reload requested", "event", "syslog.reload_requested")
			if err := c.Reload(ctx); err != nil {
				logging.From(ctx).Error("SIGHUP syslog reload failed", "event", "syslog.reload_failed",
					"error", logging.Error(err))
			}
		}
	}
}

// Reload 让监听器集合与注册表一致：新增的绑定、消失的关闭、变更的重绑。
// 端口变更必须靠重载完成，不重启进程。
func (c *Channel) Reload(ctx context.Context) error {
	desired := make(map[string]input.Entry)
	for _, entry := range c.registry.SyslogBindings() {
		desired[entry.ID] = entry
	}

	var failures []error
	var retired []*binding

	c.mu.Lock()

	// 关掉不再需要或端口已变的监听器。
	for id, active := range c.listeners {
		next, keep := desired[id]
		if keep && next.Port == active.entry.Port {
			continue
		}

		active.close()
		retired = append(retired, active)
		delete(c.listeners, id)
	}

	// 绑定新增的接入点。
	for id, entry := range desired {
		if _, exists := c.listeners[id]; exists {
			continue
		}

		created, err := c.bind(ctx, entry)
		if err != nil {
			failures = append(failures, fmt.Errorf("source %s: %w", id, err))
			continue
		}

		c.listeners[id] = created
	}
	c.mu.Unlock()

	for _, active := range retired {
		active.done.Wait()
	}

	return errors.Join(failures...)
}

// bind 绑定一个接入点的 UDP 与 TCP 监听器并启动读取循环。
func (c *Channel) bind(ctx context.Context, entry input.Entry) (*binding, error) {
	// 监听地址跟随配置，不再写死所有网卡：
	// 部署在反向代理后面时，syslog 也不该绕过代理直接暴露。
	address := net.JoinHostPort(c.bindHost, strconv.Itoa(entry.Port))

	// ListenConfig 带 context：取消时绑定立刻中断，不会卡在慢启动的地址解析上。
	var listenConfig net.ListenConfig

	udp, err := listenConfig.ListenPacket(ctx, "udp", address)
	if err != nil {
		return nil, fmt.Errorf("udp bind: %w", err)
	}

	tcp, err := listenConfig.Listen(ctx, "tcp", address)
	if err != nil {
		_ = udp.Close()

		return nil, fmt.Errorf("tcp bind: %w", err)
	}

	bindingCtx, cancel := context.WithCancel(ctx)
	created := &binding{entry: entry, udp: udp, tcp: tcp, cancel: cancel}

	logging.From(ctx).Info("Syslog listener bound", "event", "syslog.bound",
		"input_id", entry.ID, "port", entry.Port)

	created.done.Go(func() { c.readUDP(bindingCtx, created) })
	created.done.Go(func() { c.readTCP(bindingCtx, created) })

	return created, nil
}

// readUDP 逐报文读取；UDP 无连接语义，永不向发送方回错误。
func (c *Channel) readUDP(ctx context.Context, active *binding) {
	buffer := make([]byte, MaxMessageBytes+1)

	for {
		size, _, err := active.udp.ReadFrom(buffer)
		if err != nil {
			if active.isClosed() || ctx.Err() != nil {
				return
			}

			logging.From(ctx).Warn("UDP read failed", "event", "syslog.read_failed",
				"input_id", active.entry.ID, "error", logging.Error(err))

			continue
		}

		// 必须复制：deliver 只是把 slice 交给管道入队，解析发生在 worker，
		// 下一次 ReadFrom 会覆盖同一块内存，导致事件内容与 event_id 错乱。
		frame := make([]byte, size)
		copy(frame, buffer[:size])

		c.deliver(ctx, active.entry, frame)
	}
}

// readTCP 按 RFC6587 分帧读取；解析失败只计数，不断开连接。
func (c *Channel) readTCP(ctx context.Context, active *binding) {
	var connections sync.WaitGroup
	defer connections.Wait()

	for {
		connection, err := active.tcp.Accept()
		if err != nil {
			if active.isClosed() || ctx.Err() != nil {
				return
			}

			logging.From(ctx).Warn("TCP accept failed", "event", "syslog.accept_failed",
				"input_id", active.entry.ID, "error", logging.Error(err))

			continue
		}

		select {
		case c.tcpSlots <- struct{}{}:
			connections.Go(func() {
				defer func() { <-c.tcpSlots }()
				c.serveTCPConnection(ctx, active.entry, connection)
			})
		default:
			_ = connection.Close()
		}
	}
}

// serveTCPConnection 处理一条长连接；分帧残留在连接内累积。
func (c *Channel) serveTCPConnection(ctx context.Context, entry input.Entry, connection net.Conn) {
	defer func() { _ = connection.Close() }()
	// 取消 binding 时唤醒阻塞的 Read，包括刚从 Accept 返回的连接。
	stop := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stop()

	chunk := make([]byte, 8<<10)
	var pending []byte

	for ctx.Err() == nil {
		if err := connection.SetReadDeadline(time.Now().Add(tcpReadTimeout)); err != nil {
			return
		}
		size, err := connection.Read(chunk)
		if size > 0 {
			pending = append(pending, chunk[:size]...)

			frames, trailing := splitFrames(pending)
			pending = trailing
			if len(pending) > MaxMessageBytes*2 {
				// 单条报文超长：丢弃残留，避免无界增长。
				pending = nil
			}

			for _, frame := range frames {
				c.deliver(ctx, entry, frame)
			}
		}

		if err != nil {
			return
		}
	}
}

// deliver 把一条报文交给流水线；失败只记日志，不影响连接。
func (c *Channel) deliver(ctx context.Context, binding input.Entry, frame []byte) {
	// 已建立的 TCP 连接和 UDP 监听器复用端口，每条报文取当前内存配置。
	entry, ok := c.registry.ByID(binding.ID)
	if !ok {
		return
	}

	original := len(frame)
	trimmed, truncated := truncate(frame)
	if truncated {
		logging.From(ctx).Warn("Syslog message truncated", "event", "syslog.truncated",
			"input_id", entry.ID, "limit", MaxMessageBytes, "original_bytes", original)
	}

	// 截断信息随事件落库：分析师必须能看出这条数据不完整。
	meta := pipeline.Meta{Truncated: truncated, OriginalBytes: original}

	if len(trimmed) == 0 {
		return
	}

	// 先按映射声明的字符集转成 UTF-8：syslog 信封里的主机名与消息
	// 若按错误字符集解析，转出来的就是乱码，之后再无补救机会。
	if entry.Mapping != nil {
		decoded, decodeErr := entry.Mapping.Decode(trimmed)
		if decodeErr != nil {
			meta.DecodeFailed = true
			logging.From(ctx).Warn("Syslog message could not be decoded", "event", "syslog.decode_failed", "input_id", entry.ID)

			// 解码失败仍然受理：按原文透传，事件带 parse.outcome=error 可查。
		} else {
			trimmed = decoded
			meta.Decoded = true
		}
	}

	fields, err := flatten(trimmed)
	if err != nil {
		logging.From(ctx).Debug("Syslog envelope unparsed", "event", "syslog.unparsed",
			"input_id", entry.ID, "bytes", len(trimmed))

		// 信封解析不了仍然受理：按原文透传，事件带 parse.outcome=error 可查。
		if submitErr := c.pipeline.Submit(ctx, entry, trimmed, time.Now(), meta); submitErr != nil {
			logging.From(ctx).Warn("Submit aborted", "event", "syslog.submit_aborted",
				"input_id", entry.ID, "error", logging.Error(submitErr))
		}

		return
	}

	if err = c.pipeline.SubmitFields(ctx, entry, fields, time.Now(), meta); err != nil {
		logging.From(ctx).Warn("Submit aborted", "event", "syslog.submit_aborted",
			"input_id", entry.ID, "error", logging.Error(err))
	}
}

// Close 关闭全部监听器。
func (c *Channel) Close() {
	c.mu.Lock()
	retired := c.listeners
	c.listeners = make(map[string]*binding)
	for _, active := range retired {
		active.close()
	}
	c.mu.Unlock()

	for _, active := range retired {
		active.done.Wait()
	}
}

// close 取消连接处理并释放两个监听器。调用方在锁外等待退出。
func (b *binding) close() {
	if b.closed.Swap(true) {
		return
	}

	b.cancel()
	_ = b.udp.Close()
	_ = b.tcp.Close()
}

// isClosed 报告监听器是否已关闭。
func (b *binding) isClosed() bool {
	return b.closed.Load()
}
