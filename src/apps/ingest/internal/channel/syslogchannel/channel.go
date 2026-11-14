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

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connector"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// Channel 管理所有 syslog 接入点的监听器。
type Channel struct {
	registry connector.Registry
	pipeline *pipeline.Pipeline

	mu        sync.Mutex
	listeners map[string]*binding // 接入点 ID → 监听器。

	reloadInterval time.Duration
}

// binding 是一个接入点的 UDP 与 TCP 监听器。
type binding struct {
	snapshot connector.Snapshot
	udp      net.PacketConn
	tcp      net.Listener
	closed   atomic.Bool
}

// New 创建 syslog 通道。reloadInterval 为零时用 10 秒。
func New(registry connector.Registry, line *pipeline.Pipeline, reloadInterval time.Duration) *Channel {
	if reloadInterval <= 0 {
		reloadInterval = 10 * time.Second
	}

	return &Channel{
		registry:       registry,
		pipeline:       line,
		listeners:      make(map[string]*binding),
		reloadInterval: reloadInterval,
	}
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
	desired := make(map[string]connector.Snapshot)
	for _, snapshot := range c.registry.SyslogBindings() {
		desired[snapshot.ID] = snapshot
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	var failures []error

	// 关掉不再需要或端口已变的监听器。
	for id, active := range c.listeners {
		next, keep := desired[id]
		if keep && next.Port == active.snapshot.Port {
			continue
		}

		active.close()
		delete(c.listeners, id)
	}

	// 绑定新增的接入点。
	for id, snapshot := range desired {
		if _, exists := c.listeners[id]; exists {
			continue
		}

		created, err := c.bind(ctx, snapshot)
		if err != nil {
			failures = append(failures, fmt.Errorf("connector %s: %w", id, err))
			continue
		}

		c.listeners[id] = created
	}

	return errors.Join(failures...)
}

// bind 绑定一个接入点的 UDP 与 TCP 监听器并启动读取循环。
func (c *Channel) bind(ctx context.Context, snapshot connector.Snapshot) (*binding, error) {
	address := ":" + strconv.Itoa(snapshot.Port)

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

	created := &binding{snapshot: snapshot, udp: udp, tcp: tcp}

	logging.From(ctx).Info("Syslog listener bound", "event", "syslog.bound",
		"connector_id", snapshot.ID, "port", snapshot.Port)

	go c.readUDP(ctx, created)
	go c.readTCP(ctx, created)

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
				"connector_id", active.snapshot.ID, "error", logging.Error(err))

			continue
		}

		c.deliver(ctx, active.snapshot, buffer[:size])
	}
}

// readTCP 按 RFC6587 分帧读取；解析失败只计数，不断开连接。
func (c *Channel) readTCP(ctx context.Context, active *binding) {
	for {
		connection, err := active.tcp.Accept()
		if err != nil {
			if active.isClosed() || ctx.Err() != nil {
				return
			}

			logging.From(ctx).Warn("TCP accept failed", "event", "syslog.accept_failed",
				"connector_id", active.snapshot.ID, "error", logging.Error(err))

			continue
		}

		go c.serveTCPConnection(ctx, active.snapshot, connection)
	}
}

// serveTCPConnection 处理一条长连接；分帧残留在连接内累积。
func (c *Channel) serveTCPConnection(ctx context.Context, snapshot connector.Snapshot, connection net.Conn) {
	defer func() { _ = connection.Close() }()

	chunk := make([]byte, 8<<10)
	var pending []byte

	for {
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
				c.deliver(ctx, snapshot, frame)
			}
		}

		if err != nil {
			return
		}
	}
}

// deliver 把一条报文交给流水线；失败只记日志，不影响连接。
func (c *Channel) deliver(ctx context.Context, snapshot connector.Snapshot, frame []byte) {
	original := len(frame)
	trimmed, truncated := truncate(frame)
	if truncated {
		logging.From(ctx).Warn("Syslog message truncated", "event", "syslog.truncated",
			"connector_id", snapshot.ID, "limit", MaxMessageBytes, "original_bytes", original)
	}

	// 截断信息随事件落库：分析师必须能看出这条数据不完整。
	meta := pipeline.Meta{Truncated: truncated, OriginalBytes: original, DroppedBytes: original - len(trimmed)}

	if len(trimmed) == 0 {
		return
	}

	fields, err := flatten(trimmed)
	if err != nil {
		logging.From(ctx).Debug("Syslog envelope unparsed", "event", "syslog.unparsed",
			"connector_id", snapshot.ID, "bytes", len(trimmed))

		// 信封解析不了仍然受理：按原文透传，事件带 parse_status=error 可查。
		if submitErr := c.pipeline.Submit(ctx, snapshot, trimmed, time.Now(), meta); submitErr != nil {
			logging.From(ctx).Warn("Submit aborted", "event", "syslog.submit_aborted",
				"connector_id", snapshot.ID, "error", logging.Error(submitErr))
		}

		return
	}

	if err = c.pipeline.SubmitFields(ctx, snapshot, fields, time.Now(), meta); err != nil {
		logging.From(ctx).Warn("Submit aborted", "event", "syslog.submit_aborted",
			"connector_id", snapshot.ID, "error", logging.Error(err))
	}
}

// Close 关闭全部监听器。
func (c *Channel) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for id, active := range c.listeners {
		active.close()
		delete(c.listeners, id)
	}
}

// close 幂等关闭两个监听器。
func (b *binding) close() {
	if b.closed.Swap(true) {
		return
	}

	_ = b.udp.Close()
	_ = b.tcp.Close()
}

// isClosed 报告监听器是否已关闭。
func (b *binding) isClosed() bool {
	return b.closed.Load()
}
