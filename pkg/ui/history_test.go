// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package ui

import (
	"testing"
	"time"
)

// MockSubscriber is a mock implementation of the Subscriber interface for testing.
type MockSubscriber struct {
	notifications []Block
}

func (ms *MockSubscriber) DocumentChanged(doc *History, block Block) {
	ms.notifications = append(ms.notifications, block)
}

func (ms *MockSubscriber) GetNotifications() []Block {
	// Return a copy to avoid race conditions if the caller modifies the slice
	// while MockSubscriber is appending to it.
	n := make([]Block, len(ms.notifications))
	copy(n, ms.notifications)
	return n
}

func TestHistoryNotification(t *testing.T) {
	h := NewHistory()
	b1 := NewAgentTextBlock()
	b2 := NewErrorBlock()

	subscriber := &MockSubscriber{}
	closer := h.AddSubscription(subscriber)
	defer closer.Close()

	h.AddBlock(b1)
	if len(h.blocks) != 1 {
		t.Errorf("After AddBlock(b1), len(blocks) = %d; want 1", len(h.blocks))
	}
	if h.Blocks()[0] != b1 {
		t.Errorf("After AddBlock(b1), blocks[0] = %v; want %v", h.blocks[0], b1)
	}
	if b1.History() != h {
		t.Errorf("After AddBlock(b1), b1.History() = %v; want %v", b1.history, h)
	}

	// Allow time for notification
	time.Sleep(10 * time.Millisecond)
	notifications := subscriber.GetNotifications()
	if len(notifications) != 1 {
		t.Fatalf("After AddBlock(b1), subscriber received %d notifications; want 1", len(notifications))
	}
	if notifications[0] != b1 {
		t.Errorf("After AddBlock(b1), subscriber received notification for %v; want %v", notifications[0], b1)
	}

	h.AddBlock(b2)
	if len(h.Blocks()) != 2 {
		t.Errorf("After AddBlock(b2), len(blocks) = %d; want 2", len(h.blocks))
	}
	if h.Blocks()[1] != b2 {
		t.Errorf("After AddBlock(b2), blocks[1] = %v; want %v", h.blocks[1], b2)
	}
	if b2.History() != h {
		t.Errorf("After AddBlock(b2), b2.doc = %v; want %v", b2.doc, h)
	}

	// Allow time for notification
	time.Sleep(10 * time.Millisecond)
	notifications = subscriber.GetNotifications()
	if len(notifications) != 2 {
		t.Fatalf("After AddBlock(b2), subscriber received %d notifications; want 2", len(notifications))
	}
	if notifications[1] != b2 {
		t.Errorf("After AddBlock(b2), subscriber received notification for %v; want %v", notifications[1], b2)
	}
}
