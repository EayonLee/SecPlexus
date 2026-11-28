package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/enum"
	"github.com/EayonLee/SecPlexus/src/apps/iam/internal/repository"
)

// RecordServiceAudit 将平台服务上报的事实按系统身份登记。
func (s *Service) RecordServiceAudit(ctx context.Context, event ServiceAuditInput) error {
	if event.Action == "" || event.TargetType == "" || event.RequestID == "" ||
		(event.Outcome != string(enum.AuditOutcomeSuccess) &&
			event.Outcome != string(enum.AuditOutcomeDenied) &&
			event.Outcome != string(enum.AuditOutcomeFailure)) {
		return repository.ErrInvalid
	}

	audit := repository.Audit{
		OccurredAt: event.OccurredAt,
		ActorType:  enum.ActorTypeSystem,
		Action:     event.Action,
		TargetType: event.TargetType,
		Outcome:    enum.AuditOutcome(event.Outcome),
		Reason:     event.Reason,
		RequestID:  event.RequestID,
		Metadata:   event.Metadata,
	}

	if event.EventID != "" {
		id, err := uuid.Parse(event.EventID)
		if err != nil || id == uuid.Nil {
			return repository.ErrInvalid
		}
		audit.EventID = id
	}

	if event.TargetID != "" {
		if parsed, err := uuid.Parse(event.TargetID); err == nil {
			audit.TargetID = &parsed
		}
	}

	if event.DecisionID != "" {
		if parsed, err := uuid.Parse(event.DecisionID); err == nil {
			audit.DecisionID = &parsed
		}
	}

	return s.Store.RecordAudit(ctx, audit)
}
