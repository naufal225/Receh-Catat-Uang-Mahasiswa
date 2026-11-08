// Package keymutex provides one sync.Mutex per key, so two payments from the
// same wallet run one at a time while payments from different wallets still
// run in parallel.
package keymutex

import (
	"slices"
	"sync"
)

type entry struct {
	mu   sync.Mutex
	refs int
}

// Map hands out per-key locks and frees them once nobody holds or waits on them,
// so memory does not grow with the number of wallets ever touched.
type Map struct {
	mu      sync.Mutex
	entries map[int64]*entry
}

func New() *Map { return &Map{entries: make(map[int64]*entry)} }

func (m *Map) acquire(key int64) {
	m.mu.Lock()
	e, ok := m.entries[key]
	if !ok {
		e = &entry{}
		m.entries[key] = e
	}
	e.refs++
	m.mu.Unlock()
	e.mu.Lock()
}

func (m *Map) release(key int64) {
	m.mu.Lock()
	e := m.entries[key]
	e.refs--
	if e.refs == 0 {
		delete(m.entries, key)
	}
	m.mu.Unlock()
	e.mu.Unlock()
}

// Lock acquires every key and returns the function that releases them. Keys are
// taken in ascending order, so two callers locking the same pair of wallets in
// opposite order cannot deadlock.
func (m *Map) Lock(keys ...int64) (unlock func()) {
	sorted := slices.Clone(keys)
	slices.Sort(sorted)
	sorted = slices.Compact(sorted)
	for _, k := range sorted {
		m.acquire(k)
	}
	return func() {
		for i := len(sorted) - 1; i >= 0; i-- {
			m.release(sorted[i])
		}
	}
}

// Len reports how many keys are currently tracked.
func (m *Map) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries)
}
