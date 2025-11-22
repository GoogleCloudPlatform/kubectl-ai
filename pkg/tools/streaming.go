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

package tools

import (
	"context"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/sandbox"
)

// StreamDetector determines if a command is a streaming command and returns the stream type.
// It returns (true, streamType) if it is a streaming command, and (false, "") otherwise.
type StreamDetector func(command string) (isStreaming bool, streamType string)

// ExecuteWithStreamingHandling executes a command using the provided executor,
// handling streaming commands (watch, logs -f, attach) by applying a timeout
// and capturing partial output.
func ExecuteWithStreamingHandling(ctx context.Context, executor sandbox.Executor, command string, workDir string, env []string, detector StreamDetector) (*sandbox.ExecResult, error) {
	isStreaming, streamType := false, ""
	if detector != nil {
		isStreaming, streamType = detector(command)
	}

	var cmdCtx context.Context
	var cancel context.CancelFunc

	if isStreaming {
		// Create a context with timeout for streaming commands
		cmdCtx, cancel = context.WithTimeout(ctx, 7*time.Second)
		defer cancel()
	} else {
		// Use the provided context directly
		cmdCtx = ctx
		cancel = func() {} // No-op cancel
	}

	result, err := executor.Execute(cmdCtx, command, env, workDir)

	// If executor returns nil result on error (it shouldn't, but let's be safe), create one
	if result == nil {
		result = &sandbox.ExecResult{Command: command}
	}

	if isStreaming {
		if cmdCtx.Err() == context.DeadlineExceeded {
			// Timeout is expected for streaming commands
			result.StreamType = "timeout"
			result.Error = "Timeout reached after 7 seconds"
			// Clear the error if it was just the timeout
			err = nil
			// Set the detected stream type
			result.StreamType = streamType
			return result, nil
		}
	}

	return result, err
}
