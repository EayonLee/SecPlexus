package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/EayonLee/SecPlexus/src/packages/go/logging"
	"github.com/EayonLee/SecPlexus/src/packages/go/result"
)

// EventReader 以事件 ID 和发生时间范围查询标准事件。
type EventReader interface {
	ReadEvent(context.Context, uuid.UUID, time.Time, time.Time) (json.RawMessage, error)
}

// ReadEvent 校验查询范围，再读取已持久化的完整事件。
func ReadEvent(reader EventReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.Param("event_id"))
		start, startErr := time.Parse(time.RFC3339Nano, c.Query("from"))
		end, endErr := time.Parse(time.RFC3339Nano, c.Query("to"))
		if err != nil || id == uuid.Nil || startErr != nil || endErr != nil ||
			!end.After(start) || end.Sub(start) > 31*24*time.Hour {
			result.Fail(c.Writer, result.CodeInvalidRequest)
			return
		}

		ctx := c.Request.Context()
		value, err := reader.ReadEvent(ctx, id, start.UTC(), end.UTC())
		if err != nil {
			logging.From(ctx).Error("Could not read event", "event", "ingest.event.read_failed", "error", logging.Error(err))
			result.Fail(c.Writer, result.CodeServiceUnavailable)
			return
		}
		if len(value) == 0 {
			result.Fail(c.Writer, result.CodeNotFound)
			return
		}

		c.Header("Cache-Control", "no-store")
		result.Success(c.Writer, value)
	}
}
