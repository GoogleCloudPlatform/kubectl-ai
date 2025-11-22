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

package sandbox

import (
	"context"
	"fmt"
)

// Executor defines the interface for executing commands.
type Executor interface {
	// Execute runs a command and returns the result.
	Execute(ctx context.Context, command string, env []string, workDir string) (*ExecResult, error)

	// Close cleans up any resources associated with the executor.
	Close(ctx context.Context) error
}

// ExecResult represents the result of a command execution.
type ExecResult struct {
	Command    string `json:"command,omitempty"`
	Error      string `json:"error,omitempty"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	ExitCode   int    `json:"exit_code,omitempty"`
	StreamType string `json:"stream_type,omitempty"`
}

func (e *ExecResult) String() string {
	return fmt.Sprintf("Command: %q\nError: %q\nStdout: %q\nStderr: %q\nExitCode: %d\nStreamType: %q}", e.Command, e.Error, e.Stdout, e.Stderr, e.ExitCode, e.StreamType)
}
