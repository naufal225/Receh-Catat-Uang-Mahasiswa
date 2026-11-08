// Package ratelimit is a small fixed-window limiter guarded by a sync.Mutex.
package ratelimit

import (
	"sync"
	"time"
)

type window struct {
	count   int
	resetAt time.Time
}

type Limiter struct {
	mu     sync.Mutex
	limit  int
	period time.Duration
	hits   map[string]*window
	now    func() time.Time
}

func New(limit int, period time.Duration) *Limiter {
	return &Limiter{limit: limit, period: period, hits: make(map[string]*window), now: time.Now}
}

// Blocked reports whether key has used up its allowance, without counting a hit.
func (l *Limiter) Blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	w, ok := l.hits[key]
	if !ok {
		return false
	}
	if l.now().After(w.resetAt) {
		delete(l.hits, key)
		return false
	}
	return w.count >= l.limit
}

// Hit records one attempt against key.
func (l *Limiter) Hit(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	w, ok := l.hits[key]
	if !ok || l.now().After(w.resetAt) {
		l.hits[key] = &window{count: 1, resetAt: l.now().Add(l.period)}
		return
	}
	w.count++
}

// Reset forgets key, used after a successful login.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}

// Sweep drops expired windows. The app calls it from a background goroutine.
func (l *Limiter) Sweep() {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for k, w := range l.hits {
		if now.After(w.resetAt) {
			delete(l.hits, k)
		}
	}
}
