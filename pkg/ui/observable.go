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
	"sync"
)

type Observable[T any] struct {
	mutex         sync.Mutex
	condition     *sync.Cond
	value         T
	err           error
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

	for i, e := range o.subscriptions {
		if e == nil || e.subscriber == nil {
			o.subscriptions[i] = s
			return s
		}
	}

	o.subscriptions = append(o.subscriptions, s)
	return s
}

func (o *Observable[T]) Set(t T, err error) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	o.value = t
	o.err = err
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

func (o *Observable[T]) Wait() (T, error) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if o.generation != 0 {
		return o.value, o.err
	}

	if o.condition == nil {
		o.condition = sync.NewCond(&o.mutex)
	}

	for o.generation == 0 {
		o.condition.Wait()
	}

	return o.value, o.err
}
