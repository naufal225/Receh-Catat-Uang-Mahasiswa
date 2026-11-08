package keymutex

import (
	"sync"
	"testing"
)

func TestLockSerializesSameKeyAndFreesMemory(t *testing.T) {
	m := New()
	counter := 0 // Deliberately unsynchronised: -race fails here if the lock leaks.
	var wg sync.WaitGroup
	for range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer m.Lock(7)()
			counter++
		}()
	}
	wg.Wait()
	if counter != 200 {
		t.Fatalf("counter %d", counter)
	}
	if m.Len() != 0 {
		t.Fatalf("%d entries leaked", m.Len())
	}
}

// Opposite lock order is the classic deadlock; this test hangs if ordering breaks.
func TestLockOrderingPreventsDeadlock(t *testing.T) {
	m := New()
	var wg sync.WaitGroup
	for i := range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, b := int64(1), int64(2)
			if i%2 == 0 {
				a, b = b, a
			}
			defer m.Lock(a, b)()
		}()
	}
	wg.Wait()
}

func TestDuplicateKeysDoNotSelfDeadlock(t *testing.T) {
	m := New()
	m.Lock(5, 5)()
	if m.Len() != 0 {
		t.Fatal("entry leaked")
	}
}
