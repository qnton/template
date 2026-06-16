// Package events is a tiny synchronous, in-process event bus (stdlib only). Emit
// invokes each listener in registration order and collects their errors; a
// panicking listener is recovered into an error so it can't take down the caller.
//
// It is OPTIONAL and unwired. An event bus is only useful shared, so construct one
// where features can reach it (e.g. in internal/feature/registry, injected into
// the features that emit/listen). For async handling, have a listener enqueue a
// background job (internal/core/jobs) instead of doing slow work inline.
package events

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Listener handles an emitted payload. Returning an error is collected by Emit.
type Listener func(ctx context.Context, payload any) error

// Bus routes events (by name) to listeners. Safe for concurrent use.
type Bus struct {
	mu        sync.RWMutex
	listeners map[string][]Listener
}

// New returns an empty Bus.
func New() *Bus { return &Bus{listeners: make(map[string][]Listener)} }

// On registers a listener for an event name.
func (b *Bus) On(event string, l Listener) {
	b.mu.Lock()
	b.listeners[event] = append(b.listeners[event], l)
	b.mu.Unlock()
}

// Emit invokes every listener for event synchronously, in registration order,
// collecting their errors (a failing or panicking listener does not stop the
// others). Emitting an event with no listeners is a no-op.
func (b *Bus) Emit(ctx context.Context, event string, payload any) error {
	b.mu.RLock()
	ls := b.listeners[event]
	b.mu.RUnlock()

	var errs []error
	for _, l := range ls {
		if err := safe(ctx, l, payload); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func safe(ctx context.Context, l Listener, payload any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("events: listener panic: %v", r)
		}
	}()
	return l(ctx, payload)
}

// Listen registers a type-safe listener: a payload whose dynamic type is not T
// yields an error rather than a panic.
func Listen[T any](b *Bus, event string, fn func(ctx context.Context, payload T) error) {
	b.On(event, func(ctx context.Context, payload any) error {
		v, ok := payload.(T)
		if !ok {
			return fmt.Errorf("events: %q payload is %T, want %T", event, payload, *new(T))
		}
		return fn(ctx, v)
	})
}
