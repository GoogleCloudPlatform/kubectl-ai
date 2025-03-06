package journal

import (
	"context"

	"k8s.io/klog/v2"
)

type LogRecorder struct {
}

func (r *LogRecorder) Write(ctx context.Context, event *Event) error {
	log := klog.FromContext(ctx)

	log.V(2).Info("Tracing event", "event", event)
	return nil
}

func (r *LogRecorder) Close() error {
	return nil
}
