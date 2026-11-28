package bootstrap

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/observation"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// sourceRefresher 让运行期的接入点集合跟随数据库变化。
//
// 数据库不可访问时，上一份快照只在租期内可用。单条配置损坏时只隔离对应接入。
type sourceRefresher struct {
	pool         *pgxpool.Pool
	registry     *input.Memory
	interval     time.Duration
	observations *observation.Store
	activation   func(input.Entry) input.ChannelActivation

	mu            sync.RWMutex
	degraded      string    // 非空表示最近一次刷新失败的原因。
	lastConfirmed time.Time // 最近一次完成配置及运行确认的本地时间。
}

// newSourceRefresher 创建刷新器。
func newSourceRefresher(pool *pgxpool.Pool, registry *input.Memory, observations *observation.Store, activation func(input.Entry) input.ChannelActivation, interval time.Duration) *sourceRefresher {
	if interval <= 0 {
		interval = repository.ReloadInterval
	}

	return &sourceRefresher{pool: pool, registry: registry, interval: interval, observations: observations, activation: activation}
}

const sourceLease = 15 * time.Second

// Load 读取一次权威快照，失败时不会续期。
func (r *sourceRefresher) Load(ctx context.Context) error {
	expires := time.Now().Add(sourceLease)
	loadContext, cancel := context.WithTimeout(ctx, r.interval)
	defer cancel()
	snapshot, err := repository.LoadInputs(loadContext, r.pool)
	if err == nil {
		err = r.observations.Participate(loadContext, expires)
	}

	if err != nil {
		r.mu.Lock()
		r.degraded = err.Error()
		r.mu.Unlock()

		// 上一份配置继续服务：数据库抖动不该让接入点集体消失。
		logging.From(ctx).Error("Keeping previous sources", "event", "input.reload_kept_previous",
			"error", logging.Error(err))

		return err
	}

	for id, target := range snapshot.Targets {
		if target.Problem != "" {
			logging.From(ctx).Warn("Source configuration rejected", "event", "input.configuration.rejected",
				"input_id", id, "reason", target.Problem)
		}
	}
	r.registry.ReplaceUntil(snapshot.Entries, expires)
	for _, entry := range snapshot.Entries {
		target := snapshot.Targets[entry.ID]
		state := r.activation(entry)
		target.Online, target.Draining, target.Problem = state.Online, state.Draining, state.Problem
		snapshot.Targets[entry.ID] = target
	}
	if err := r.observations.Acknowledge(loadContext, snapshot.Targets, expires); err != nil {
		r.registry.ReplaceUntil(nil, time.Now())
		r.mu.Lock()
		r.degraded = err.Error()
		r.mu.Unlock()
		logging.From(ctx).Error("Could not confirm source configuration", "event", "input.configuration.confirmation_failed",
			"error", logging.Error(err))
		return err
	}

	r.mu.Lock()
	r.degraded = ""
	r.lastConfirmed = time.Now()
	r.mu.Unlock()

	logging.From(ctx).Info("Sources loaded", "event", "input.loaded", "count", len(snapshot.Entries))

	return nil
}

// SnapshotAge 返回距上次确认的时间，用于在快照租期内发现故障。
func (r *sourceRefresher) SnapshotAge() time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.lastConfirmed.IsZero() {
		return 0
	}
	return time.Since(r.lastConfirmed)
}

// Degraded 返回最近一次刷新失败的原因；空串表示健康。
//
// 首次加载失败必须让启动失败（调用方负责），因此这里的降级只覆盖"曾经成功过"
// 之后的刷新失败：那种情况下旧配置仍在服务，报 503 会让负载均衡把实例摘掉，
// 反而中断还在正常工作的接入点。
func (r *sourceRefresher) Degraded() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.degraded
}

// Watch 周期刷新，并在 SIGHUP 时立即刷新；ctx 取消后退出。
func (r *sourceRefresher) Watch(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	hangup := make(chan os.Signal, 1)
	signal.Notify(hangup, syscall.SIGHUP)
	defer signal.Stop(hangup)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = r.Load(ctx)
		case <-hangup:
			logging.From(ctx).Info("SIGHUP requested source reload", "event", "input.reload_requested")
			_ = r.Load(ctx)
		}
	}
}
