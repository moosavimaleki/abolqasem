package login

import (
	"context"
	"sync"
)

// Registry makes login flows addressable by ID so closing a dialog can cancel
// the exact poller and never leave an orphan app-server process behind.
type Registry struct {
	mu      sync.Mutex
	entries map[string]context.CancelFunc
}

func (r *Registry) Register(id string, cancel context.CancelFunc) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entries == nil {
		r.entries = make(map[string]context.CancelFunc)
	}
	if _, exists := r.entries[id]; exists {
		return false
	}
	r.entries[id] = cancel
	return true
}

func (r *Registry) Cancel(id string) bool {
	r.mu.Lock()
	cancel, exists := r.entries[id]
	if exists {
		delete(r.entries, id)
	}
	r.mu.Unlock()
	if exists {
		cancel()
	}
	return exists
}

func (r *Registry) Complete(id string) { r.mu.Lock(); delete(r.entries, id); r.mu.Unlock() }

func (r *Registry) Len() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.entries) }
