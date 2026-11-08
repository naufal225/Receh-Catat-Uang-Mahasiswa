package cache

import (
	"sync"
	"testing"
	"time"
)

func TestTTLExpiresAndInvalidates(t *testing.T) {
	now := time.Unix(1000, 0)
	c := New[string, int](time.Minute, 0)
	c.now = func() time.Time { return now }

	c.Set("a", 1)
	c.Set("b", 2)
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("get a = %v, %v", v, ok)
	}
	c.Delete("a")
	if _, ok := c.Get("a"); ok {
		t.Fatal("deleted key still served")
	}
	c.DeleteFunc(func(_ string, v int) bool { return v == 2 })
	if _, ok := c.Get("b"); ok {
		t.Fatal("DeleteFunc did not remove b")
	}

	c.Set("c", 3)
	now = now.Add(time.Minute)
	if _, ok := c.Get("c"); ok {
		t.Fatal("expired key still served")
	}
	c.Sweep()
	if c.Len() != 0 {
		t.Fatalf("sweep left %d entries", c.Len())
	}
}

func TestTakeIsOneTime(t *testing.T) {
	c := New[string, int](time.Minute, 0)
	c.Set("state", 7)
	var wg sync.WaitGroup
	var mu sync.Mutex
	won := 0
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := c.Take("state"); ok {
				mu.Lock()
				won++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if won != 1 {
		t.Fatalf("%d callers took the value, want exactly 1", won)
	}
}

func TestTTLDisabled(t *testing.T) {
	c := New[string, int](0, 0)
	c.Set("a", 1)
	if _, ok := c.Get("a"); ok || c.Len() != 0 {
		t.Fatal("a zero TTL must not cache")
	}
}

func TestTTLBounded(t *testing.T) {
	c := New[int, int](time.Minute, 10)
	for i := 0; i < 100; i++ {
		c.Set(i, i)
	}
	if c.Len() > 10 {
		t.Fatalf("cache grew to %d entries, max 10", c.Len())
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := New[int, int](time.Minute, 50)
	v := NewVersions()
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				c.Set(i%60, i)
				c.Get(i % 60)
				if i%50 == 0 {
					c.DeleteFunc(func(k, _ int) bool { return k%2 == 0 })
				}
				v.Changed(int64(g % 4))
			}
		}(g)
	}
	wg.Wait()
	var total uint64
	for u := int64(0); u < 4; u++ {
		total += v.Get(u)
	}
	if total != 16*500 {
		t.Fatalf("versions lost updates: %d, want %d", total, 16*500)
	}
}
