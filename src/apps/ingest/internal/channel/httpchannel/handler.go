// Package httpchannel 实现设备主动推送通道。
//
// 契约（spec）：POST /api/v1/ingest/events/{connector_id}，Bearer 令牌按接入点校验，
// 请求体 10 MiB、数组 1000 条上限，成功 202 返回受理计数；
// 解析与发布在响应之后异步完成，失败不回传设备。
package httpchannel

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/connector"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/parser"
	"github.com/EayonLee/SecPlexus/src/apps/ingest/internal/pipeline"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

const (
	// DefaultMaxBodyBytes 是请求体上限的默认值，与 spec 一致；超出返回 413。
	DefaultMaxBodyBytes int64 = 10 << 20
	// MaxBatch 是一次请求允许的事件条数上限；超出返回 413。
	MaxBatch = 1000
)

// acceptedContentTypes 是允许的请求体类型。
var acceptedContentTypes = map[string]struct{}{
	"application/json": {},
	"text/plain":       {},
}

// Channel 是 HTTP 通道。
type Channel struct {
	registry connector.Registry // 接入点与令牌查询。
	limiter  Limiter            // 每接入点频率限制。
	pipeline *pipeline.Pipeline // 受理后的解析与发布。

	// MaxBodyBytes 是请求体上限；零值使用 DefaultMaxBodyBytes。
	// 由配置注入而不是读全局：同一进程里不同部署可以给不同上限。
	MaxBodyBytes int64
}

// New 创建 HTTP 通道。
func New(registry connector.Registry, limiter Limiter, line *pipeline.Pipeline) *Channel {
	return &Channel{registry: registry, limiter: limiter, pipeline: line}
}

// ServeHTTP 让通道能独立挂到标准 mux（测试、直连反向代理）。',
// 路由层自己解析路径参数时请直接调用 Handle。',
func (c *Channel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.Handle(w, r, r.PathValue("connector_id"))
}

// Handle 处理一次设备推送。connectorID 由路由层解析后传入——
// 通道不假设自己被哪种路由器挂载（gin 的 :param 与标准库的 {param} 语法不同）。
// 每个失败分支都必须有明确状态码：设备侧靠状态码决定重试策略。
func (c *Channel) Handle(w http.ResponseWriter, r *http.Request, connectorID string) {
	if connectorID == "" {
		writeError(w, http.StatusNotFound, "unknown connector")
		return
	}

	snapshot, ok := c.authorize(w, r, connectorID)
	if !ok {
		return
	}

	if !contentTypeAllowed(r.Header.Get("Content-Type")) {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported content type")
		return
	}

	if err := c.limiter.Allow(r.Context(), snapshot.ID); err != nil {
		if errors.Is(err, ErrRateLimited) {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusTooManyRequests, "rate limited")

			return
		}

		// 限流后端不可用时拒绝：放行等于关掉防护。
		writeError(w, http.StatusServiceUnavailable, "rate limiter unavailable")

		return
	}

	body, ok := c.readBody(w, r)
	if !ok {
		return
	}

	// 拆分模式取自解析规则，而不是通道类型：
	// 通道说"怎么进来"，模式说"字节长什么样"，两者不同。
	messages, err := parser.Split(splitMode(snapshot, r.Header.Get("Content-Type")), body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "malformed payload")
		return
	}
	if len(messages) > MaxBatch {
		writeError(w, http.StatusRequestEntityTooLarge, "batch exceeds limit")
		return
	}

	received := time.Now()
	for _, message := range messages {
		// 队列满时此处阻塞，背压直接传到设备：不丢数据。
		if err = c.pipeline.Submit(r.Context(), snapshot, message, received, pipeline.Meta{}); err != nil {
			logging.From(r.Context()).Warn("Submit aborted", "event", "ingest.submit_aborted",
				"connector_id", snapshot.ID, "error", logging.Error(err))

			// 已受理的部分仍然有效；客户端可重推，去重层按 event_id 兜底。
			break
		}
	}

	writeAccepted(w, len(messages))
}

// splitMode 决定用什么线格式拆分请求体。
// 挂了规则就用规则声明的模式；未挂规则时按请求体类型推断：
// text/plain 一行一条，其余按 JSON（数组与 NDJSON 都能拆）。
func splitMode(snapshot connector.Snapshot, contentType string) string {
	if snapshot.Engine != nil {
		return snapshot.Engine.Rule().Parse.Mode
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil && mediaType == "text/plain" {
		// 行模式：Split 对非 JSON 模式按行拆分。
		return ""
	}

	return parser.ModeJSON
}

// authorize 校验 Bearer 令牌与路径上的接入点是否配对。
// 令牌属于 A 而路径写 B 必须拒绝：否则一个令牌能往任意接入点写事件。
func (c *Channel) authorize(w http.ResponseWriter, r *http.Request, connectorID string) (connector.Snapshot, bool) {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing token")
		return connector.Snapshot{}, false
	}

	snapshot, ok := c.registry.ByTokenDigest(connector.DigestToken(token))
	if !ok || !sameID(snapshot.ID, connectorID) {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return connector.Snapshot{}, false
	}

	return snapshot, true
}

// bearerToken 取出 Authorization 头里的令牌。
func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}

	return strings.TrimSpace(header[len(prefix):])
}

// sameID 做常数时间比较：令牌与标识都是秘密，比较时间不应泄露前缀匹配长度。
func sameID(left, right string) bool {
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

// contentTypeAllowed 判断请求体类型；缺省按 JSON 处理（部分设备不发该头）。
func contentTypeAllowed(header string) bool {
	if header == "" {
		return true
	}

	mediaType, _, err := mime.ParseMediaType(header)
	if err != nil {
		return false
	}

	_, ok := acceptedContentTypes[mediaType]
	return ok
}

// readBody 读取请求体并强制大小上限。
func (c *Channel) readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	limit := c.MaxBodyBytes
	if limit <= 0 {
		limit = DefaultMaxBodyBytes
	}

	reader := http.MaxBytesReader(w, r.Body, limit)

	body, err := io.ReadAll(reader)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "payload exceeds limit")

			return nil, false
		}

		writeError(w, http.StatusBadRequest, "cannot read payload")
		return nil, false
	}

	return body, true
}

// response 是设备侧看到的响应体。
type response struct {
	Accepted int `json:"accepted"`
}

// writeAccepted 返回 202 与受理计数。
func writeAccepted(w http.ResponseWriter, accepted int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(response{Accepted: accepted})
}

// writeError 返回统一的错误体；不泄露内部细节。
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
