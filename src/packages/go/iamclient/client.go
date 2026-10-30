// Package iamclient 通过 IAM 鉴权接口校验原始浏览器请求，不引用 IAM 私有实现或表。
package iamclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// Error 描述 IAM 接口返回的授权错误。
type Error struct {
	Status    int         // IAM 返回或本地映射的 HTTP 状态。
	Code      result.Code // 对外使用的统一结果码。
	Reason    string      // 受控失败阶段，不包含上游响应正文。
	RequestID string      // 上游生成的请求标识，用于跨服务关联。
	Cause     error       // 原始技术错误只供服务端诊断。
}

// Error 返回不包含凭据的授权失败说明。
func (e *Error) Error() string { return "IAM authorization failed: " + string(e.Code) }

// Unwrap 保留技术错误链，便于区分超时、TLS 与连接故障。
func (e *Error) Unwrap() error { return e.Cause }

// Client 通过 IAM 鉴权接口请求 IAM 授权。
type Client struct {
	origin     string
	credential string
	http       *http.Client
}

// New 创建使用 HTTP(S) 和服务凭证的 IAM 客户端。
func New(baseURL, credential string) (*Client, error) {
	if err := ValidateConfig(baseURL, credential, true); err != nil {
		return nil, err
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	httpClient := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Client{origin: strings.TrimSuffix(baseURL, "/"), credential: credential, http: httpClient}, nil
}

// Authorize 从原始请求获取会话；操作和组织由服务端路由确定，不能信任浏览器提交的值。
func (c *Client) Authorize(ctx context.Context, original *http.Request, action, orgID string, resource *Resource) (decision CapabilityDecision, err error) {
	ctx = logging.With(ctx, "operation", "iam.authorize_rpc", "action", action, "org_id", orgID)
	started := time.Now()
	var upstreamID string
	defer func() {
		log := logging.From(ctx)
		if err == nil && decision.Allowed {
			log.Debug("IAM 内部授权调用完成", "event", "rpc.completed", "upstream_request_id", upstreamID, "decision_id", decision.DecisionID, "duration_ms", time.Since(started).Milliseconds())
			return
		}
		var failure *Error
		if errors.As(err, &failure) {
			level := slog.LevelWarn
			if failure.Status >= 500 {
				level = slog.LevelError
			}
			log.Log(
				ctx, level,
				"IAM 内部授权调用未完成",
				"event", "rpc.failed",
				"upstream_request_id", upstreamID,
				"reason", failure.Reason,
				"status", failure.Status,
				"code", failure.Code,
				"duration_ms", time.Since(started).Milliseconds(),
				"error", logging.Error(err),
			)
			return
		}
		log.Error("IAM 内部请求构造失败", "event", "rpc.failed", "error", logging.Error(err))
	}()
	unavailable := func(reason string, cause error) error {
		return &Error{Status: 503, Code: result.CodeServiceUnavailable, Reason: reason, RequestID: upstreamID, Cause: cause}
	}

	input := AuthorizeRequest{Cookie: original.Header.Get("Cookie"), Method: original.Method, Path: original.URL.RequestURI(), Origin: original.Header.Get("Origin"), CSRFToken: original.Header.Get("X-CSRF-Token"), Action: action, OrgID: orgID, Resource: resource}
	// 单个授权 RPC 直接使用标准库和共享 DTO，不另建通用传输或解析框架。
	body, err := json.Marshal(input)
	if err != nil {
		return decision, unavailable("encode_request", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.origin+"/api/v1/iam/authorize", bytes.NewReader(body))
	if err != nil {
		return decision, unavailable("construct_request", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.credential)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return decision, unavailable("transport", err)
	}
	defer func() { _ = response.Body.Close() }()
	upstreamID = response.Header.Get("X-Request-ID")
	// 上游标识也是输入，仅接受本平台生成的 UUID，不能让任意响应头污染日志。
	if _, parseErr := uuid.Parse(upstreamID); parseErr != nil {
		upstreamID = ""
	}

	media, _, parseErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if parseErr != nil || media != "application/json" {
		return decision, unavailable("invalid_content_type", parseErr)
	}
	data, err := io.ReadAll(http.MaxBytesReader(nil, response.Body, 64*1024))
	if err != nil {
		return decision, unavailable("decode_result", err)
	}
	var decoded result.Result[json.RawMessage]
	if err := json.Unmarshal(data, &decoded); err != nil || decoded.RequestID == "" {
		return decision, unavailable("decode_result", err)
	}
	if response.StatusCode != http.StatusOK {
		if response.StatusCode != 401 && response.StatusCode != 403 && response.StatusCode != 429 {
			return decision, unavailable("upstream_unavailable", nil)
		}
		var rejected result.Result[any]
		if err := json.Unmarshal(data, &rejected); err != nil || string(decoded.Data) != "null" {
			return decision, unavailable("invalid_result_status", err)
		}
		definition, known := result.Lookup(result.Code(rejected.Code))
		if !known || definition.Status != response.StatusCode || rejected.RequestID == "" || rejected.Data != nil {
			return decision, unavailable("invalid_result_status", nil)
		}
		if id, err := uuid.Parse(rejected.RequestID); err == nil {
			upstreamID = id.String()
		}
		return decision, &Error{Status: response.StatusCode, Code: result.Code(rejected.Code), Reason: "authorization_rejected", RequestID: upstreamID}
	}
	if id, err := uuid.Parse(decoded.RequestID); err == nil {
		upstreamID = id.String()
	}
	if err := json.Unmarshal(decoded.Data, &decision); err != nil {
		return decision, unavailable("invalid_decision", err)
	}
	if result.Code(decoded.Code) != result.CodeOK || !decision.Allowed || decision.Principal.UserID == "" || decision.Principal.SessionID == "" || decision.DecisionID == "" {
		return CapabilityDecision{}, unavailable("invalid_decision", nil)
	}
	if orgID != "" && (decision.Scope.Kind != "organization" || decision.Scope.OrgID != orgID) {
		return CapabilityDecision{}, unavailable("organization_scope_mismatch", nil)
	}
	if orgID == "" && (decision.Scope.Kind != "platform" || decision.Scope.OrgID != "") {
		return CapabilityDecision{}, unavailable("platform_scope_mismatch", nil)
	}
	return decision, nil
}

// Close 释放不再使用的空闲 HTTP 连接。
func (c *Client) Close() { c.http.CloseIdleConnections() }

// ValidateConfig 只校验服务调用配置，不读取文件或建立网络连接。
func ValidateConfig(baseURL, credential string, required bool) error {
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return errors.New("IAM URL must be an HTTP(S) origin")
	}

	if (required || credential != "") && (len(credential) < 32 || len(credential) > 512) {
		return errors.New("service credential must contain 32–512 bytes")
	}

	return nil
}
