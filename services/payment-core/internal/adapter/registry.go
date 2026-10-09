package adapter

import (
	"errors"
	"fmt"
	"sync"
)

var ErrAdapterNotFound = errors.New("provider adapter not found")

type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]Adapter),
	}
}

func (r *Registry) Register(name string, a Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[name] = a
}

func (r *Registry) Get(name string) (Adapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrAdapterNotFound, name)
	}
	return a, nil
}
