package iamclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
)

// AuditEvent 是一条要写入 IAM 审计的既有事实。
//
// 字段刻意最小：谁、对什么、做了什么、结果如何。
// 业务方不提交操作者身份——服务凭证证明的是"哪个服务在写"，
// 操作者由服务在 metadata 里注明并由 IAM 侧策略决定是否采信。
type AuditEvent struct {
	Action     string         `json:"action"`
	TargetType string         `json:"target_type"`
	TargetID   string         `json:"target_id,omitempty"`
	Outcome    string         `json:"outcome"`
	Reason     string         `json:"reason,omitempty"`
	RequestID  string         `json:"request_id"`
	DecisionID string         `json:"decision_id,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// RecordAudit 把一条审计事实交给 IAM。
//
// 审计写入失败不静默：返回错误，由调用方决定是降级记录还是让请求失败。
func (c *Client) RecordAudit(ctx context.Context, event AuditEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("could not encode audit event: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.origin+"/api/v1/iam/audit-events", bytes.NewReader(body))
	if err != nil {
		return errors.New("could not build audit request")
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.credential)

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("could not reach IAM for audit: %w", err)
	}

	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusOK {
		logging.From(ctx).Error("IAM rejected audit event", "event", "audit.rejected",
			"action", event.Action, "status", response.StatusCode)

		return fmt.Errorf("IAM rejected audit event with status %d", response.StatusCode)
	}

	return nil
}

// ErrCatalogDrift 表示代码里的动作键不在 IAM 的权限目录里：配置错误，必须停在启动期。
// 与"IAM 暂时不可达"区分开：后者是依赖故障，服务可以先起来并按请求返回 503。
var ErrCatalogDrift = errors.New("IAM permission catalog drifted from the compiled action set")

// ValidateActions 校验这些动作都还在 IAM 的权限目录里，并返回缺失的键。
//
// 用途是启动期漂移检测：业务代码里写的是生成常量，目录由 IAM 维护；
// 两边不一致时，请求会在运行期被静默拒绝（403），比启动失败难查得多。
// 因此这里宁可在启动时炸掉，也不要带着一个永远不通过的权限键上线。
func (c *Client) ValidateActions(ctx context.Context, actions ...string) error {
	if len(actions) == 0 {
		return nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.origin+"/api/v1/iam/permission-catalog", nil)
	if err != nil {
		return errors.New("could not build permission catalog request")
	}

	request.Header.Set("Authorization", "Bearer "+c.credential)

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("could not reach IAM for the permission catalog: %w", err)
	}

	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("IAM rejected the permission catalog request with status %d", response.StatusCode)
	}

	var payload struct {
		Data struct {
			Keys []string `json:"keys"`
		} `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return fmt.Errorf("could not decode permission catalog: %w", err)
	}

	known := make(map[string]bool, len(payload.Data.Keys))
	for _, key := range payload.Data.Keys {
		known[key] = true
	}

	missing := make([]string, 0, len(actions))
	for _, action := range actions {
		if !known[action] {
			missing = append(missing, action)
		}
	}

	if len(missing) > 0 {
		slices.Sort(missing)

		return fmt.Errorf("%w: missing actions %s", ErrCatalogDrift, strings.Join(missing, ", "))
	}

	return nil
}
