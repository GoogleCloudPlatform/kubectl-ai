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

import (
	"io"
	"slices"
	"sync"
)

type Document struct {
	mutex         sync.Mutex
	subscriptions []*subscription
	nextID        uint64

	blocks []Block
}

func (d *Document) Blocks() []Block {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	return d.blocks
}

func (d *Document) NumBlocks() int {
	return len(d.Blocks())
}

func (d *Document) IndexOf(find Block) int {
	blocks := d.Blocks()

	for i, b := range blocks {
		if b == find {
			return i
		}
	}
	return -1
}

func NewDocument() *Document {
	return &Document{
		nextID: 1,
	}
}

type Block interface {
	attached(doc *Document)

	Document() *Document
}

type Subscriber interface {
	DocumentChanged(doc *Document, block Block)
}

type SubscriberFunc func(doc *Document, block Block)

type funcSubscriber struct {
	fn SubscriberFunc
}

func (s *funcSubscriber) DocumentChanged(doc *Document, block Block) {
	s.fn(doc, block)
}

func SubscriberFromFunc(fn SubscriberFunc) Subscriber {
	return &funcSubscriber{fn: fn}
}

type subscription struct {
	doc        *Document
	id         uint64
	subscriber Subscriber
}

func (s *subscription) Close() error {
	s.doc.mutex.Lock()
	defer s.doc.mutex.Unlock()
	s.subscriber = nil
	return nil
}

func (d *Document) AddSubscription(subscriber Subscriber) io.Closer {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	id := d.nextID
	d.nextID++

	s := &subscription{
		doc:        d,
		id:         id,
		subscriber: subscriber,
	}

	// Copy on write so we don't need to lock the subscriber list
	newSubscriptions := make([]*subscription, 0, len(d.subscriptions)+1)
	for _, s := range d.subscriptions {
		if s == nil || s.subscriber == nil {
			continue
		}
		newSubscriptions = append(newSubscriptions, s)
	}
	newSubscriptions = append(newSubscriptions, s)
	d.subscriptions = newSubscriptions
	return s
}

func (d *Document) sendDocumentChanged(b Block) {
	d.mutex.Lock()
	subscriptions := d.subscriptions
	d.mutex.Unlock()

	for _, s := range subscriptions {
		if s == nil || s.subscriber == nil {
			continue
		}

		s.subscriber.DocumentChanged(d, b)
	}
}

func (d *Document) AddBlock(block Block) {
	d.mutex.Lock()

	// Copy-on-write to minimize locking
	newBlocks := slices.Clone(d.blocks)
	newBlocks = append(newBlocks, block)
	d.blocks = newBlocks

	block.attached(d)
	d.mutex.Unlock()

	d.sendDocumentChanged(block)
}

func (d *Document) blockChanged(block Block) {
	if d == nil {
		return
	}

	d.sendDocumentChanged(block)
}
