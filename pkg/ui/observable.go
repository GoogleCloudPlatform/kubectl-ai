package ui

import (
	"io"
	"sync"
)

type Observable[T any] struct {
	mutex         sync.Mutex
	condition     *sync.Cond
	value         T
	generation    int64
	subscriptions []*observableSubscription[T]
}

type observableSubscription[T any] struct {
	subscriber ObservableSubscriber[T]
}

func (o *observableSubscription[T]) Close() error {
	o.subscriber = nil
	return nil
}

type ObservableSubscriber[T any] interface {
	ValueChanged(v *Observable[T])
}

func (o *Observable[T]) AddSubscription(subscriber ObservableSubscriber[T]) io.Closer {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	s := &observableSubscription[T]{
		subscriber: subscriber,
	}

	for i, s := range o.subscriptions {
		if s == nil {
			o.subscriptions[i] = s
			return s
		}
		if s.subscriber == nil {
			o.subscriptions[i] = s
			return s
		}
	}

	o.subscriptions = append(o.subscriptions, s)
	return s
}

func (o *Observable[T]) Set(t T) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	o.value = t
	o.generation++

	if o.condition != nil {
		o.condition.Broadcast()
	}

	o.sendChangeHoldingLock()
}

func (o *Observable[T]) sendChangeHoldingLock() {
	for i, s := range o.subscriptions {
		if s == nil {
			continue
		}
		if s.subscriber == nil {
			o.subscriptions[i] = nil
			continue
		}

		s.subscriber.ValueChanged(o)
	}
}

func (o *Observable[T]) Wait() T {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if o.generation != 0 {
		return o.value
	}

	if o.condition == nil {
		o.condition = sync.NewCond(&o.mutex)
	}

	for o.generation == 0 {
		o.condition.Wait()
	}
	o.mutex.Unlock()

	return o.value
}
