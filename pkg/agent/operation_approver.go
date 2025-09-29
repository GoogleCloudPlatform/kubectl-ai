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

package agent

import "strings"

// OperationApprover classifies the type of work performed by a tool call so the
// agent can make informed decisions when responding to cancellation requests.
//
// Implementations should return OperationKindWrite for operations that modify
// cluster state (or when they are unsure) so the agent can cancel them
// gracefully.
type OperationApprover interface {
	Classify(call ToolCallAnalysis) OperationKind
}

// DefaultOperationApprover infers the operation kind from the tool's
// CheckModifiesResource response.
type DefaultOperationApprover struct{}

// Classify returns OperationKindWrite for any tool call that is known (or
// suspected) to modify cluster state. "Unknown" responses are treated as writes
// to prefer safety when cancelling requests.
func (DefaultOperationApprover) Classify(call ToolCallAnalysis) OperationKind {
	switch strings.ToLower(call.ModifiesResourceStr) {
	case "no":
		return OperationKindRead
	case "yes":
		return OperationKindWrite
	default:
		return OperationKindWrite
	}
}
