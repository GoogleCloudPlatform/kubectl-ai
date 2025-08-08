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

import (
	"context"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
)

func TestRunHandlesExitMetaQuery(t *testing.T) {
	tests := []string{"exit", "quit"}
	for _, q := range tests {
		t.Run(q, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			a := &Agent{
				Input:   make(chan any, 1),
				Output:  make(chan any, 10),
				session: &api.Session{AgentState: api.AgentStateIdle},
			}

			if err := a.Run(ctx, ""); err != nil {
				t.Fatalf("Run returned error: %v", err)
			}

			// Wait for the agent to prompt for input
			timeout := time.After(time.Second)
		WaitForPrompt:
			for {
				select {
				case <-timeout:
					t.Fatalf("did not receive prompt for %q", q)
				case msg := <-a.Output:
					m := msg.(*api.Message)
					if m.Type == api.MessageTypeUserInputRequest && m.Payload.(string) == ">>>" {
						a.Input <- &api.UserInputResponse{Query: q}
						break WaitForPrompt
					}
				}
			}

			exitTimeout := time.After(time.Second)
			for {
				if a.AgentState() == api.AgentStateExited {
					return
				}
				select {
				case <-exitTimeout:
					t.Fatalf("agent did not exit for %q", q)
				case <-a.Output:
					// drain any remaining output messages
				default:
					time.Sleep(10 * time.Millisecond)
				}
			}
		})
	}
}
