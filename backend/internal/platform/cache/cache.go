// Package cache is a small in-process cache: a TTL map guarded by a
// sync.RWMutex (many readers, one writer) and a per-user version counter used
// to invalidate derived views without tracking every key they produced.
//
// It is deliberately local to one process. Receh runs as a single server; a
// second instance would need a shared cache (or no cache) instead.
package cache

import (
	"context"
	"sync"
	"time"
)

type item[V any] struct {
	value   V
	expires time.Time
}

// TTL maps keys to values that expire. A zero TTL disables it: Get always
// misses and Set stores nothing, so callers need no special case.
type TTL[K comparable, V any] struct {
	mu    sync.RWMutex
	items map[K]item[V]
	ttl   time.Duration
	max   int
	now   func() time.Time
}

// New makes a cache holding at most max entries for ttl each.
func New[K comparable, V any](ttl time.Duration, max int) *TTL[K, V] {
	return &TTL[K, V]{items: make(map[K]item[V]), ttl: ttl, max: max, now: time.Now}
}

func (c *TTL[K, V]) Enabled() bool { return c.ttl > 0 }

func (c *TTL[K, V]) Get(key K) (V, bool) {
	var zero V
	if !c.Enabled() {
		return zero, false
	}
	c.mu.RLock()
	it, ok := c.items[key]
	c.mu.RUnlock()
	if !ok || !c.now().Before(it.expires) {
		return zero, false
	}
	return it.value, true
}

func (c *TTL[K, V]) Set(key K, value V) {
	if !c.Enabled() {
		return
	}
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.items[key]; !exists && c.max > 0 && len(c.items) >= c.max {
		c.evictLocked(now)
	}
	c.items[key] = item[V]{value: value, expires: now.Add(c.ttl)}
}

// evictLocked drops expired entries and, if the cache is still full, an
// arbitrary one. Simple beats clever here: the bound only exists so memory
// cannot grow without limit.
func (c *TTL[K, V]) evictLocked(now time.Time) {
	for k, it := range c.items {
		if !now.Before(it.expires) {
			delete(c.items, k)
		}
	}
	if len(c.items) >= c.max {
		for k := range c.items {
			delete(c.items, k)
			break
		}
	}
}

// Take returns the value and removes it in one step under the write lock, so a
// one-time value (an OAuth state) cannot be used by two callers racing.
func (c *TTL[K, V]) Take(key K) (V, bool) {
	var zero V
	c.mu.Lock()
	it, ok := c.items[key]
	delete(c.items, key)
	c.mu.Unlock()
	if !ok || !c.now().Before(it.expires) {
		return zero, false
	}
	return it.value, true
}

func (c *TTL[K, V]) Delete(key K) {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

// DeleteFunc removes every entry the predicate matches, for invalidations that
// are known by value (all sessions of one user) rather than by key.
func (c *TTL[K, V]) DeleteFunc(match func(K, V) bool) {
	c.mu.Lock()
	for k, it := range c.items {
		if match(k, it.value) {
			delete(c.items, k)
		}
	}
	c.mu.Unlock()
}

func (c *TTL[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Sweep removes expired entries.
func (c *TTL[K, V]) Sweep() {
	now := c.now()
	c.mu.Lock()
	for k, it := range c.items {
		if !now.Before(it.expires) {
			delete(c.items, k)
		}
	}
	c.mu.Unlock()
}

// RunJanitor sweeps on an interval until ctx ends. Run it in its own goroutine.
func (c *TTL[K, V]) RunJanitor(ctx context.Context, every time.Duration) {
	if !c.Enabled() {
		return
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.Sweep()
		}
	}
}

// Versions counts changes per user. A cache key that includes the current
// version can never be hit again once the version moves on, so one Changed
// call invalidates every cached view of that user at once.
type Versions struct {
	mu sync.Mutex
	v  map[int64]uint64
}

func NewVersions() *Versions { return &Versions{v: make(map[int64]uint64)} }

func (s *Versions) Get(userID int64) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.v[userID]
}

// Changed implements domain.ChangeNotifier.
func (s *Versions) Changed(userID int64) {
	s.mu.Lock()
	s.v[userID]++
	s.mu.Unlock()
}
