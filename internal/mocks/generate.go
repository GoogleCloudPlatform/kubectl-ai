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

// Package mocks holds go:generate directives for gomock.
package mocks

// Generate gomock types for external interfaces we depend on.
// NOTE: run `go generate ./...` from repo root to (re)create mocks.
// Requires: go install go.uber.org/mock/mockgen@latest

// gollm interfaces
//   - Client, Chat
// tools interface
//   - Tool

//go:generate go run go.uber.org/mock/mockgen -destination=gollm_mock.go -package=mocks github.com/GoogleCloudPlatform/kubectl-ai/gollm Client,Chat
//go:generate go run go.uber.org/mock/mockgen -destination=tools_mock.go -package=mocks github.com/GoogleCloudPlatform/kubectl-ai/pkg/tools Tool
//go:generate go run go.uber.org/mock/mockgen -destination=agent_mock.go -package=mocks github.com/GoogleCloudPlatform/kubectl-ai/pkg/api ChatMessageStore
