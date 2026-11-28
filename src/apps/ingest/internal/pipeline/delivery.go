package pipeline

import (
	"context"
	"errors"
	"time"
)

// PartialPublishError 标记未确认的批内索引，索引按提交顺序排列。
type PartialPublishError struct {
	Failed []int
	Cause  error
}

func (e *PartialPublishError) Error() string { return e.Cause.Error() }
func (e *PartialPublishError) Unwrap() error { return e.Cause }

// PublishPending 确认成功项，并在原切片中保留未确认的记录。调用方拥有该切片。
func PublishPending(ctx context.Context, publisher Publisher, events []Event) ([]Event, error) {
	err := publisher.Publish(ctx, events)
	var partial *PartialPublishError
	if err != nil && !errors.As(err, &partial) {
		return events, err
	}
	remaining := events[:0]
	for i, event := range events {
		if partial != nil && len(partial.Failed) > len(remaining) && partial.Failed[len(remaining)] == i {
			remaining = append(remaining, event)
			continue
		}
		if event.observer != nil {
			event.observer.Confirm(event.SourceID, time.Now())
		}
		if event.confirmed != nil {
			close(event.confirmed)
		}
	}
	clear(events[len(remaining):])
	return remaining, err
}
