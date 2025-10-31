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

package model

import "fmt"

type TaskResult struct {
	Task      string    `json:"name"`
	LLMConfig LLMConfig `json:"llmConfig"`
	Result    string    `json:"result"`

	// Failure contains a list of test failures, if there were unmet expectations.
	// These do not indicate an infrastructure failure, rather they are the details of a test failure.
	Failures []Failure `json:"failures,omitempty"`

	// Error contains the error message, if there was an unexpected error during the execution of the test.
	// This normally indicates an infrastructure failure, rather than a test failure.
	Error string `json:"error"`
}

type Failure struct {
	Message string `json:"message"`
}

type LLMConfig struct {
	// ID is a short identifier for this configuration set, useful for writing logs etc
	ID string `json:"id"`

	ProviderID string `json:"provider"`
	ModelID    string `json:"model"`

	EnableToolUseShim bool `json:"enableToolUseShim"`

	Quiet bool `json:"quiet"`

	McpClient bool `json:"mcpClient"`

	// TODO: Maybe different styles of invocation, or different temperatures etc?
}

// AddFailure is a helper for adding a formatted failure message; it also marks the test as failed
func (r *TaskResult) AddFailure(msg string, args ...any) {
	failure := Failure{
		Message: fmt.Sprintf(msg, args...),
	}
	r.Result = "fail"
	r.Failures = append(r.Failures, failure)
}
