package iamclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

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
