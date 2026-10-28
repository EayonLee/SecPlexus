package repository

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// queryLogger 接入 GORM 的日志边界，保留错误和慢操作，禁止输出 SQL 与参数。
type queryLogger struct{ level logger.LogLevel }

// LogMode 返回指定级别的副本，避免在请求间修改共享日志器。
func (l queryLogger) LogMode(level logger.LogLevel) logger.Interface {
	l.level = level
	return l
}

// Info 不展开 GORM 的格式参数，库内提示只保留固定事件类别。
func (l queryLogger) Info(ctx context.Context, _ string, _ ...any) {
	if l.level >= logger.Info {
		logging.From(ctx).Debug("ORM 提示", "event", "db.info")
	}
}

// Warn 保留库级警告，不打印可能包含查询值的格式参数。
func (l queryLogger) Warn(ctx context.Context, _ string, _ ...any) {
	if l.level >= logger.Warn {
		logging.From(ctx).Warn("ORM 警告", "event", "db.warning")
	}
}

// Error 保留库级异常，具体查询诊断由 Trace 输出。
func (l queryLogger) Error(ctx context.Context, _ string, _ ...any) {
	if l.level >= logger.Error {
		logging.From(ctx).Error("ORM 异常", "event", "db.error")
	}
}

// Trace 只记录失败和慢操作，不调用 SQL 渲染函数；诊断包含 SQLSTATE、约束和代码位置。
func (l queryLogger) Trace(ctx context.Context, begin time.Time, _ func() (string, int64), err error) {
	if l.level == logger.Silent {
		return
	}
	if errors.Is(err, context.Canceled) && ctx.Err() != nil {
		// 调用方取消请求属于正常中断，保留 Debug 诊断而不触发故障告警。
		logging.From(ctx).Debug("数据库操作随请求取消", "event", "db.canceled", "duration_ms", time.Since(begin).Milliseconds())
		return
	}

	elapsed := time.Since(begin)
	if (err == nil || errors.Is(err, gorm.ErrRecordNotFound)) && elapsed < time.Second {
		return
	}
	level := slog.LevelWarn
	event := "db.slow"
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		level = slog.LevelError
		event = "db.failed"
		var failure *pgconn.PgError
		if errors.As(err, &failure) && (strings.HasPrefix(failure.Code, "23") || failure.Code == "22P02") {
			level = slog.LevelWarn
		}
	}
	if level == slog.LevelWarn && l.level < logger.Warn {
		return
	}
	log := logging.From(ctx)
	if !log.Enabled(ctx, level) {
		return
	}
	log.Log(ctx, level, "数据库操作结束", "event", event, "duration_ms", elapsed.Milliseconds(), "error", logging.Error(err), "source", logging.Stack())
}
