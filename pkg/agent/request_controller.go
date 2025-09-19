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
	"sync"

	"github.com/google/uuid"
)

// RequestController manages cancellation for a single user request.
type RequestController struct {
	id       string
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	children []context.CancelFunc
}

// NewRequestController creates a new RequestController with its own context.
func NewRequestController(parent context.Context) *RequestController {
	ctx, cancel := context.WithCancel(parent)
	return &RequestController{
		id:     uuid.New().String(),
		ctx:    ctx,
		cancel: cancel,
	}
}

// ID returns the request identifier.
func (r *RequestController) ID() string { return r.id }

// Context returns the request-scoped context.
func (r *RequestController) Context() context.Context { return r.ctx }

// RegisterChild registers a cancel function for a child operation.
func (r *RequestController) RegisterChild(cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.children = append(r.children, cancel)
}

// Cancel cancels the request and all registered child operations.
func (r *RequestController) Cancel() {
	r.mu.Lock()
	children := r.children
	r.children = nil
	r.mu.Unlock()
	for _, c := range children {
		c()
	}
	r.cancel()
}

// Close releases resources associated with the request.
func (r *RequestController) Close() { r.Cancel() }
