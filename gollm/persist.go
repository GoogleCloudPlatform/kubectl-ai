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

package gollm

// We define some standard structs to allow for persistence of the LLM requests and responses.
// This lets us store the history of the conversation for later analysis.

type RecordCompletionResponse struct {
	Text string `json:"text"`
	Raw  any    `json:"raw"`
}

type RecordChatResponse struct {
	// TODO: Structured data?
	Raw any `json:"raw"`
}
