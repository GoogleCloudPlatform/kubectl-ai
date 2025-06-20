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
	"fmt"
	"io"
	"slices"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

type DocumentOptions struct {
	MarkdownTerminalRenderer *glamour.TermRenderer
	MarkdownHTMLRenderer     goldmark.Markdown
}

func NewDocumentOptions(useMarkdown bool) (*DocumentOptions, error) {
	options := &DocumentOptions{}
	if useMarkdown {
		mdRenderer, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithPreservedNewLines(),
			glamour.WithEmoji(),
		)
		if err != nil {
			return nil, fmt.Errorf("initializing the markdown renderer: %w", err)
		}
		options.MarkdownTerminalRenderer = mdRenderer

		// Initialize Goldmark for HTML rendering
		md := goldmark.New(
			// goldmark.WithExtensions(
			// 	extension.GFM,
			// 	extension.DefinitionList,
			// ),
			goldmark.WithParserOptions(
				parser.WithAutoHeadingID(),
			),
			goldmark.WithRendererOptions(
				html.WithHardWraps(),
				html.WithXHTML(),
			),
		)
		options.MarkdownHTMLRenderer = md
	}
	return options, nil
}

type Document struct {
	options DocumentOptions

	mutex              sync.Mutex
	subscriptions      []*subscription
	nextSubscriptionID uint64

	blocks      []Block
	nextBlockID uint64
}

func (d *Document) MarkdownTerminalRenderer() *glamour.TermRenderer {
	return d.options.MarkdownTerminalRenderer
}

func (d *Document) MarkdownHTMLRenderer() goldmark.Markdown {
	return d.options.MarkdownHTMLRenderer
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

func NewDocument(options *DocumentOptions) *Document {
	return &Document{
		nextSubscriptionID: 1,
		nextBlockID:        1,
		options:            *options,
	}
}

type Block interface {
	attached(doc *Document, id uint64)

	Document() *Document

	ID() uint64
}

type blockBase struct {
	doc *Document
	id  uint64
}

func (b *blockBase) attached(doc *Document, id uint64) {
	b.doc = doc
	b.id = id
}

func (b *blockBase) ID() uint64 {
	return b.id
}

func (b *blockBase) Document() *Document {
	return b.doc
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

	id := d.nextSubscriptionID
	d.nextSubscriptionID++

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

	id := d.nextBlockID
	d.nextBlockID++

	block.attached(d, id)
	d.mutex.Unlock()

	d.sendDocumentChanged(block)
}

func (d *Document) blockChanged(block Block) {
	if d == nil {
		return
	}

	d.sendDocumentChanged(block)
}
