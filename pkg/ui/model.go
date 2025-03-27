package ui

import (
	"io"
	"sync"
)

type Document struct {
	mutex         sync.Mutex
	subscriptions []*subscription
	nextID        uint64

	Blocks []Block
}

func (d *Document) NumBlocks() int {
	return len(d.Blocks)
}

func (d *Document) IndexOf(find Block) int {
	for i, b := range d.Blocks {
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
	Document() *Document
}

// AgentTextBlock is used to render agent textual responses
type AgentTextBlock struct {
	doc *Document

	// text is populated if this is agent text output
	text string
}

func (b *AgentTextBlock) Document() *Document {
	return b.doc
}

func (b *AgentTextBlock) Text() string {
	return b.text
}

func (b *AgentTextBlock) SetText(agentText string) {
	b.text = agentText
	b.doc.blockChanged(b)
}

func (b *AgentTextBlock) AppendText(text string) {
	b.text = b.text + text
	b.doc.blockChanged(b)
}

func (d *Document) AddAgentTextBlock() *AgentTextBlock {
	block := &AgentTextBlock{doc: d}
	d.addBlock(block)
	return block
}

// FunctionCallRequestBlock is used to render the LLM's request to invoke a function
type FunctionCallRequestBlock struct {
	doc *Document

	// text is populated if this is agent text output
	text string
}

func (b *FunctionCallRequestBlock) Document() *Document {
	return b.doc
}

func (b *FunctionCallRequestBlock) Text() string {
	return b.text
}

func (b *FunctionCallRequestBlock) SetText(agentText string) {
	b.text = agentText
	b.doc.blockChanged(b)
}

func (d *Document) AddFunctionCallRequestBlock() *FunctionCallRequestBlock {
	block := &FunctionCallRequestBlock{doc: d}
	d.addBlock(block)
	return block
}

// ErrorBlock is used to render an error condition
type ErrorBlock struct {
	doc *Document

	// text is populated if this is agent text output
	text string
}

func (b *ErrorBlock) Document() *Document {
	return b.doc
}

func (b *ErrorBlock) Text() string {
	return b.text
}

func (b *ErrorBlock) SetText(agentText string) {
	b.text = agentText
	b.doc.blockChanged(b)
}

func (d *Document) AddErrorBlock() *ErrorBlock {
	block := &ErrorBlock{doc: d}
	d.addBlock(block)
	return block
}

// InputTextBlock is used to prompt for user input
type InputTextBlock struct {
	doc *Document

	// text is populated when we have input from the user
	text Observable[string]
}

func (b *InputTextBlock) Document() *Document {
	return b.doc
}

func (b *InputTextBlock) Observable() *Observable[string] {
	return &b.text
}

func (d *Document) AddInputTextBlock() *InputTextBlock {
	block := &InputTextBlock{doc: d}
	d.addBlock(block)
	return block
}

// reader := bufio.NewReader(os.Stdin)
// query, err := reader.ReadString('\n')

type Subscriber interface {
	DocumentChanged(doc *Document, block Block)
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
	for i, s := range d.subscriptions {
		if s == nil {
			d.subscriptions[i] = s
			return s
		}
	}
	d.subscriptions = append(d.subscriptions, s)
	return s
}

func (d *Document) sendDocumentChangedHoldingLock(b Block) {
	for i, s := range d.subscriptions {
		if s == nil {
			continue
		}
		if s.subscriber == nil {
			d.subscriptions[i] = nil
			continue
		}

		s.subscriber.DocumentChanged(d, b)
	}
}

func (d *Document) addBlock(block Block) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.Blocks = append(d.Blocks, block)
	d.sendDocumentChangedHoldingLock(block)
}

func (d *Document) blockChanged(block Block) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.sendDocumentChangedHoldingLock(block)
}
