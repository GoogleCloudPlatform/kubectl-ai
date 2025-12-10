package journal

import (
	"context"
	"time"
)

// RecordSessionMetadata annotates the current recorder with session level context.
func RecordSessionMetadata(ctx context.Context, recorder Recorder, metadata SessionMetadata) {
	if recorder == nil {
		return
	}

	if metadata.SessionID == "" && metadata.ProjectHash == "" {
		return
	}

	_ = recorder.Write(ctx, &Event{
		Timestamp: time.Now(),
		Action:    ActionSessionMetadata,
		Payload:   metadata,
	})
}
