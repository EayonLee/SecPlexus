package logging

import (
	"context"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"
)

// fieldsKey 只保存请求诊断字段，不通过 context 查找业务依赖。
type fieldsKey struct{}

// With 派生带诊断字段的上下文；不修改父请求的字段切片。
func With(ctx context.Context, fields ...any) context.Context {
	previous, _ := ctx.Value(fieldsKey{}).([]any)
	combined := append(append([]any{}, previous...), fields...)
	return context.WithValue(ctx, fieldsKey{}, combined)
}

// From 返回携带当前请求或业务阶段字段的标准 slog 日志器。
func From(ctx context.Context) *slog.Logger {
	fields, _ := ctx.Value(fieldsKey{}).([]any)
	return slog.Default().With(fields...)
}

// Error 只提取受控诊断字段；原始错误可能包含 SQL 参数、密码和地址，不能直接打印。
func Error(err error) slog.Value {
	if err == nil {
		return slog.GroupValue(slog.String("kind", "none"))
	}
	fields := []slog.Attr{slog.String("kind", fmt.Sprintf("%T", err))}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		fields = append(fields, slog.String("reason", "deadline_exceeded"))
	case errors.Is(err, context.Canceled):
		fields = append(fields, slog.String("reason", "canceled"))
	case errors.Is(err, sql.ErrConnDone), errors.Is(err, net.ErrClosed), errors.Is(err, redis.ErrClosed):
		fields = append(fields, slog.String("reason", "connection_closed"))
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		fields = append(fields, slog.String("reason", "unexpected_eof"))
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		fields = append(fields, slog.String("sqlstate", pg.Code), slog.String("constraint", pg.ConstraintName), slog.String("table", pg.TableName))
	}
	var network *net.OpError
	if errors.As(err, &network) {
		fields = append(fields, slog.String("network_operation", network.Op), slog.Bool("timeout", network.Timeout()))
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		fields = append(fields, slog.Int("os_errno", int(errno)), slog.String("os_error", errno.Error()))
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		fields = append(fields, slog.Bool("dns_not_found", dns.IsNotFound), slog.Bool("dns_timeout", dns.IsTimeout))
	}
	var authority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	if errors.As(err, &authority) {
		fields = append(fields, slog.String("tls_reason", "unknown_authority"))
	}
	if errors.As(err, &hostname) {
		fields = append(fields, slog.String("tls_reason", "hostname_mismatch"))
	}

	var reply redis.Error
	if errors.As(err, &reply) {
		// Redis 标准错误类别可公开用于排障，错误全文和命令参数仍不输出。
		code, _, _ := strings.Cut(reply.Error(), " ")
		switch code {
		case "NOAUTH", "WRONGPASS", "OOM", "READONLY", "LOADING", "NOPERM", "NOSCRIPT", "ERR":
			fields = append(fields, slog.String("redis_code", code))
		}
	}
	// 错误链类型有助于区分 TLS、网络和数据库连接错误，不读取各层 Error 文本。
	if cause := errors.Unwrap(err); cause != nil {
		fields = append(fields, slog.String("cause_kind", fmt.Sprintf("%T", cause)))
	}
	return slog.GroupValue(fields...)
}

// Stack 返回本项目的代码位置，不包含 panic 内容、请求值或机器绝对路径。
func Stack() []string {
	pcs := make([]uintptr, 40)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs)])
	locations := []string{}
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, "SecPlexus/src/") && !strings.Contains(frame.Function, "/logging.") {
			locations = append(locations, fmt.Sprintf("%s (%s:%d)", frame.Function, filepath.Base(frame.File), frame.Line))
		}
		if !more || len(locations) >= 12 {
			break
		}
	}
	return locations
}
