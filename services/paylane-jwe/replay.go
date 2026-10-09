package jwe

import (
	"sync"
	"time"
)

// ReplayProtector checks and stores seen JTI values within their validity window.
type ReplayProtector interface {
	// CheckAndRecord returns true if the JTI is fresh and successfully recorded,
	// or false if the JTI has already been seen (replayed).
	CheckAndRecord(jti string, expiresAt time.Time) bool
}

type memoryReplayProtector struct {
	mu    sync.Mutex
	items map[string]time.Time
}

// NewMemoryReplayProtector creates an in-memory replay cache with background cleanup.
func NewMemoryReplayProtector() ReplayProtector {
	p := &memoryReplayProtector{
		items: make(map[string]time.Time),
	}
	go p.cleanupLoop()
	return p
}

func (p *memoryReplayProtector) CheckAndRecord(jti string, expiresAt time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now().UTC()
	if !expiresAt.After(now) {
		// Already expired
		return false
	}

	if exp, exists := p.items[jti]; exists && exp.After(now) {
		return false // Seen and not expired yet -> replay
	}

	p.items[jti] = expiresAt
	return true
}

func (p *memoryReplayProtector) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		p.mu.Lock()
		now := time.Now().UTC()
		for id, exp := range p.items {
			if !exp.After(now) {
				delete(p.items, id)
			}
		}
		p.mu.Unlock()
	}
}
