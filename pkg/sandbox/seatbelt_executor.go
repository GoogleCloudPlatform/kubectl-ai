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

//go:build darwin

package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// Seatbelt executes commands in a seatbelt sandbox.
type Seatbelt struct {
	local *Local
}

// NewSeatbeltExecutor creates a new SeatbeltExecutor.
func NewSeatbeltExecutor() *Seatbelt {
	return &Seatbelt{
		local: NewLocalExecutor(),
	}
}

// Execute executes the command in the seatbelt sandbox.
func (e *Seatbelt) Execute(ctx context.Context, command string, env []string, workDir string) (*ExecResult, error) {
	// Use the provided context directly
	cmdCtx := ctx

	// This profile allows reading/writing to the working directory and /tmp,
	// but denies writing to other system locations by default (implicitly, though 'allow default' is permissive).

	// Use a basic profile for now.
	wrappedCommand := fmt.Sprintf("sandbox-exec -p %q /bin/bash -c %q", "(version 1) (allow default)", command)
	cmd := exec.CommandContext(cmdCtx, "/bin/bash", "-c", wrappedCommand)
	cmd.Dir = workDir
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := &ExecResult{
		Command:  command,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}

	if err != nil {
		result.Error = err.Error()
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
		}
	}

	return result, nil
}

// Close is a no-op for Seatbelt executor.
func (e *Seatbelt) Close(ctx context.Context) error {
	return nil
}
