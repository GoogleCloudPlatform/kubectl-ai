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

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
)

type Tool interface {
	Name() string
	Description() string
	FunctionDefinition() *gollm.FunctionDefinition
	Run(ctx context.Context, args map[string]any) (any, error)
}

type Tools map[string]Tool

func (t Tools) Lookup(name string) Tool {
	return t[name]
}
