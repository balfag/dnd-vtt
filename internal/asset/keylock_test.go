package asset

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestKeyLockSerializesSameKey(t *testing.T) {
	var (
		l       keyLock
		inside  atomic.Int32
		maxSeen atomic.Int32
		wg      sync.WaitGroup
	)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				unlock := l.lock("k")
				n := inside.Add(1)
				if n > maxSeen.Load() {
					maxSeen.Store(n)
				}
				inside.Add(-1)
				unlock()
			}
		}()
	}
	wg.Wait()
	if got := maxSeen.Load(); got != 1 {
		t.Fatalf("holders at once = %d, want 1", got)
	}
}

func TestKeyLockDifferentKeysRunTogether(t *testing.T) {
	var (
		l       keyLock
		entered sync.WaitGroup
		wg      sync.WaitGroup
	)
	entered.Add(2)
	for _, key := range []string{"a", "b"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := l.lock(key)
			defer unlock()
			entered.Done()
			entered.Wait() // returns only when both goroutines are inside
		}()
	}
	wg.Wait()
}

func TestKeyLockForgetsFreeKeys(t *testing.T) {
	var (
		l  keyLock
		wg sync.WaitGroup
	)
	for i := range 20 {
		key := string(rune('a' + i%3))
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.lock(key)()
		}()
	}
	wg.Wait()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.held) != 0 {
		t.Fatalf("held = %d entries, want 0", len(l.held))
	}
}
