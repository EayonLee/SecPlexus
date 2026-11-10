package repository

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// queryStartedAt 是查询开始时间的上下文键，由 TraceQueryStart 写入。
type queryStartedAt struct{}

// QueryTracer 只记录失败和慢查询。
// pgx 的诊断回调不提供 SQL 文本与参数，因此这里结构上就不可能输出它们；
// 保留的只有 SQLSTATE、约束名和代码位置。
type QueryTracer struct{}

// TraceQueryStart 记录开始时间，随调用方上下文向内传递。
func (QueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, queryStartedAt{}, time.Now())
}

// TraceQueryEnd 只在失败或超过一秒时输出，正常快查询保持静默。
func (QueryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	started, ok := ctx.Value(queryStartedAt{}).(time.Time)
	if !ok {
		return
	}

	if errors.Is(data.Err, context.Canceled) && ctx.Err() != nil {
		// 调用方取消请求属于正常中断，保留 Debug 诊断而不触发故障告警。
		logging.From(ctx).Debug("Canceled database operation", "event", "db.canceled", "duration_ms", time.Since(started).Milliseconds())
		return
	}

	elapsed := time.Since(started)
	if data.Err == nil && elapsed < time.Second {
		return
	}

	level := slog.LevelWarn
	event := "db.slow"
	if data.Err != nil {
		level = slog.LevelError
		event = "db.failed"
		var failure *pgconn.PgError
		if errors.As(data.Err, &failure) && (strings.HasPrefix(failure.Code, "23") || failure.Code == "22P02") {
			level = slog.LevelWarn
		}
	}

	log := logging.From(ctx)
	if !log.Enabled(ctx, level) {
		return
	}

	log.Log(ctx, level, "Database operation finished", "event", event, "duration_ms", elapsed.Milliseconds(), "error", logging.Error(data.Err), "source", logging.Stack())
}
