// Package iamclient 通过内部接口校验原始浏览器请求，不引用 IAM 私有实现或表。
package iamclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/contracts/common"
	contract "github.com/EayonLee/SecPlexus/src/contracts/iam"
	"github.com/EayonLee/SecPlexus/src/contracts/iam/internalclient"
	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// Error 描述 IAM 接口返回的授权错误。
type Error struct {
	Status    int         // IAM 返回或本地映射的 HTTP 状态。
	Code      common.Code // 对外使用的统一结果码。
	Reason    string      // 受控失败阶段，不包含上游响应正文。
	RequestID string      // 上游生成的请求标识，用于跨服务关联。
	Cause     error       // 原始技术错误只供服务端诊断。
}

// Error 返回不包含凭据的授权失败说明。
func (e *Error) Error() string { return "IAM authorization failed: " + string(e.Code) }

// Unwrap 保留技术错误链，便于区分超时、TLS 与连接故障。
func (e *Error) Unwrap() error { return e.Cause }

// Client 通过内部接口请求 IAM 授权。
type Client struct {
	sdk  *internalclient.Client
	http *http.Client
}

// New 创建使用 TLS 和服务凭证的 IAM 客户端。
func New(baseURL, credential, caFile string) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, errors.New("IAM internal URL must be an HTTPS origin")
	}

	if len(credential) < 32 || len(credential) > 512 {
		return nil, errors.New("internal credential must contain 32–512 bytes")
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile != "" {
		data, err := os.ReadFile(caFile)
		if err != nil || len(data) > 1024*1024 {
			return nil, errors.New("cannot read IAM CA certificate")
		}

		roots, err := x509.SystemCertPool()
		if err != nil {
			roots = x509.NewCertPool()
		}

		if !roots.AppendCertsFromPEM(data) {
			return nil, errors.New("invalid IAM CA certificate")
		}

		tlsConfig.RootCAs = roots
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	httpClient := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	sdk, err := internalclient.NewClient(strings.TrimSuffix(baseURL, "/"), internalclient.WithHTTPClient(httpClient), internalclient.WithRequestEditorFn(func(_ context.Context, request *http.Request) error {
		request.Header.Set("Authorization", "Bearer "+credential)
		return nil
	}))
	if err != nil {
		transport.CloseIdleConnections()
		return nil, errors.New("invalid IAM internal client configuration")
	}
	return &Client{sdk: sdk, http: httpClient}, nil
}

// Authorize 从原始请求获取会话；操作和组织由服务端路由确定，不能信任浏览器提交的值。
func (c *Client) Authorize(ctx context.Context, original *http.Request, action, orgID string, resource *contract.Resource) (decision contract.CapabilityDecision, err error) {
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
		return &Error{Status: 503, Code: common.CodeServiceUnavailable, Reason: reason, RequestID: upstreamID, Cause: cause}
	}

	input := contract.AuthorizeRequest{Cookie: original.Header.Get("Cookie"), Method: original.Method, Path: original.URL.RequestURI(), Origin: original.Header.Get("Origin"), CSRFToken: original.Header.Get("X-CSRF-Token"), Action: action, OrgID: orgID, Resource: resource}
	// 请求构造与JSON编解码来自内部契约生成器；这里只维护TLS/凭证/超时及结果策略。
	response, err := c.sdk.AuthorizeInternal(ctx, input)
	if err != nil {
		return decision, unavailable("transport", err)
	}
	upstreamID = response.Header.Get("X-Request-ID")
	// 上游标识也是输入，仅接受本平台生成的 UUID，不能让任意响应头污染日志。
	if _, parseErr := uuid.Parse(upstreamID); parseErr != nil {
		upstreamID = ""
	}

	// 对生成解析器的标准响应体设置上限，避免读取无界上游数据。
	response.Body = http.MaxBytesReader(nil, response.Body, 64*1024)
	parsed, err := internalclient.ParseAuthorizeInternalResponse(response)
	if err != nil {
		return decision, unavailable("decode_result", err)
	}
	if response.StatusCode != http.StatusOK {
		var rejected *contract.APIError
		switch response.StatusCode {
		case 401:
			rejected = parsed.JSON401
		case 403:
			rejected = parsed.JSON403
		case 429:
			rejected = parsed.JSON429
		default:
			return decision, unavailable("upstream_unavailable", nil)
		}
		if rejected == nil {
			return decision, unavailable("invalid_result_status", nil)
		}
		definition, known := common.Lookup(common.Code(rejected.Code))
		if !known || definition.Status != response.StatusCode || rejected.RequestID == "" || rejected.Data != nil {
			return decision, unavailable("invalid_result_status", nil)
		}
		if id, err := uuid.Parse(rejected.RequestID); err == nil {
			upstreamID = id.String()
		}
		return decision, &Error{Status: response.StatusCode, Code: common.Code(rejected.Code), Reason: "authorization_rejected", RequestID: upstreamID}
	}
	decoded := parsed.JSON200
	if decoded == nil || decoded.RequestID == "" {
		return decision, unavailable("invalid_result_status", nil)
	}
	if id, err := uuid.Parse(decoded.RequestID); err == nil {
		upstreamID = id.String()
	}
	decision = decoded.Data
	if common.Code(decoded.Code) != common.CodeOK || !decision.Allowed || decision.Principal.UserID == "" || decision.Principal.SessionID == "" || decision.DecisionID == "" {
		return contract.CapabilityDecision{}, unavailable("invalid_decision", nil)
	}
	if orgID != "" && (decision.Scope.Kind != "organization" || decision.Scope.OrgID != orgID) {
		return contract.CapabilityDecision{}, unavailable("organization_scope_mismatch", nil)
	}
	if orgID == "" && (decision.Scope.Kind != "platform" || decision.Scope.OrgID != "") {
		return contract.CapabilityDecision{}, unavailable("platform_scope_mismatch", nil)
	}
	return decision, nil
}

// Close 释放不再使用的空闲 HTTP 连接。
func (c *Client) Close() { c.http.CloseIdleConnections() }
