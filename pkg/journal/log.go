// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
