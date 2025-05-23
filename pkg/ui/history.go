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

package ui

// History allows us to have a history of interaction between Agents and Users.
// It's a collection of Bloocks and has the ability to subscribe to changes.

import (
	"io"
	"slices"
	"sync"
)

type History struct {
	mutex         sync.Mutex
	subscriptions []*subscription
	nextID        uint64

	blocks []Block
}

func (h *History) Blocks() []Block {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	return h.blocks
}

func (h *History) NumBlocks() int {
	return len(h.Blocks())
}

func (h *History) IndexOf(find Block) int {
	blocks := h.Blocks()

	for i, b := range blocks {
		if b == find {
			return i
		}
	}
	return -1
}

func NewHistory() *History {
	return &History{
		nextID: 1,
	}
}

type Block interface {
	attached(history *History)

	History() *History
}

type Subscriber interface {
	DocumentChanged(doc *History, block Block)
}

type SubscriberFunc func(doc *History, block Block)

type funcSubscriber struct {
	fn SubscriberFunc
}

func (s *funcSubscriber) DocumentChanged(doc *History, block Block) {
	s.fn(doc, block)
}

func SubscriberFromFunc(fn SubscriberFunc) Subscriber {
	return &funcSubscriber{fn: fn}
}

type subscription struct {
	history    *History
	id         uint64
	subscriber Subscriber
}

func (s *subscription) Close() error {
	s.history.mutex.Lock()
	defer s.history.mutex.Unlock()
	s.subscriber = nil
	return nil
}

func (h *History) AddSubscription(subscriber Subscriber) io.Closer {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	id := h.nextID
	h.nextID++

	s := &subscription{
		history:    h,
		id:         id,
		subscriber: subscriber,
	}

	// Copy on write so we don't need to lock the subscriber list
	newSubscriptions := make([]*subscription, 0, len(h.subscriptions)+1)
	for _, s := range h.subscriptions {
		if s == nil || s.subscriber == nil {
			continue
		}
		newSubscriptions = append(newSubscriptions, s)
	}
	newSubscriptions = append(newSubscriptions, s)
	h.subscriptions = newSubscriptions
	return s
}

func (h *History) sendDocumentChanged(b Block) {
	h.mutex.Lock()
	subscriptions := h.subscriptions
	h.mutex.Unlock()

	for _, s := range subscriptions {
		if s == nil || s.subscriber == nil {
			continue
		}

		s.subscriber.DocumentChanged(h, b)
	}
}

func (h *History) AddBlock(block Block) {
	h.mutex.Lock()

	// Copy-on-write to minimize locking
	newBlocks := slices.Clone(h.blocks)
	newBlocks = append(newBlocks, block)
	h.blocks = newBlocks

	block.attached(h)
	h.mutex.Unlock()

	h.sendDocumentChanged(block)
}

func (h *History) blockChanged(block Block) {
	if h == nil {
		return
	}

	h.sendDocumentChanged(block)
}
