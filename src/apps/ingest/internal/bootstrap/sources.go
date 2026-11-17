package bootstrap

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/input"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/repository"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// sourceRefresher 让运行期的接入点集合跟随数据库变化。
//
// 语义与 nginx 的 fail-safe reload 一致：新配置整体校验通过才切换，
// 任何一条失败就保留上一份成功配置并在 readiness 上报告降级——
// 绝不出现"一部分生效、一部分静默消失"的中间状态。
type sourceRefresher struct {
	pool     *pgxpool.Pool
	registry *input.Memory
	interval time.Duration

	mu       sync.RWMutex
	degraded string // 非空表示最近一次刷新失败的原因。
}

// newSourceRefresher 创建刷新器。
func newSourceRefresher(pool *pgxpool.Pool, registry *input.Memory, interval time.Duration) *sourceRefresher {
	if interval <= 0 {
		interval = repository.ReloadInterval
	}

	return &sourceRefresher{pool: pool, registry: registry, interval: interval}
}

// Load 加载一次；失败时保留旧集合并记下降级原因。
func (r *sourceRefresher) Load(ctx context.Context) error {
	entries, problems, err := repository.LoadInputs(ctx, r.pool)
	if err == nil && len(problems) > 0 {
		for _, problem := range problems {
			logging.From(ctx).Error("Source did not compile", "event", "input.compile_failed",
				"input_id", problem.SourceID, "reason", problem.Reason)
		}

		err = errors.New("some sources did not compile")
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

	r.registry.Replace(entries)

	r.mu.Lock()
	r.degraded = ""
	r.mu.Unlock()

	logging.From(ctx).Info("Sources loaded", "event", "input.loaded", "count", len(entries))

	return nil
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
