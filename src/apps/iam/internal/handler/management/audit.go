package management

import (
	"github.com/gin-gonic/gin"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/model"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/service"
	"github.com/EayonLee/SecPlexus/src/packages/go/httpx"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// ListAuditEvents 读取审计日志。可见范围由业务用例按授权结果裁剪，
// 客户端只能收窄结果（操作者、时间范围、结果），不能扩大范围。
func (s *Handler) ListAuditEvents(c *gin.Context) {
	var params model.AuditQuery
	if err := c.ShouldBindQuery(&params); err != nil {
		httpx.Reject(c, repository.ErrInvalid)
		return
	}

	page, err := s.Service.ListAuditEvents(c.Request.Context(), service.AuditFilter{
		Limit:   params.Limit,
		Cursor:  params.Cursor,
		Actor:   params.Actor,
		Outcome: params.Outcome,
		From:    params.From,
		To:      params.To,
	})
	if err != nil {
		httpx.Reject(c, err)
		return
	}

	items := make([]model.APIAuditEvent, 0, len(page.Items))
	for _, event := range page.Items {
		items = append(items, apiAuditEvent(event))
	}

	result.Success(c.Writer, model.APIAuditPage{
		Items:      items,
		Limit:      params.Limit,
		NextCursor: page.NextCursor,
	})
}

// apiAuditEvent 把审计投影映射为 HTTP DTO；空值保持 null 供前端区分。
func apiAuditEvent(event repository.AuditEvent) model.APIAuditEvent {
	var targetID *string
	if event.TargetID != nil {
		value := event.TargetID.String()
		targetID = &value
	}

	return model.APIAuditEvent{
		ID:         event.ID.String(),
		OccurredAt: event.OccurredAt,
		ActorType:  event.ActorType,
		ActorName:  event.ActorName,
		Action:     event.Action,
		TargetType: event.TargetType,
		TargetID:   targetID,
		Outcome:    event.Outcome,
		ReasonCode: event.ReasonCode,
		RequestID:  event.RequestID,
		Metadata:   event.Metadata,
	}
}
