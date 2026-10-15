package httpapi

import (
	"fmt"
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newLimiter(time.Minute, func() time.Time { return clock })

	for range 4 {
		l.fail("k")
	}
	if blocked, _ := l.blocked("k", 5); blocked {
		t.Fatal("blocked after 4 failures with limit 5")
	}

	clock = clock.Add(10 * time.Second)
	l.fail("k")
	// The first failure was 10 seconds ago, so 50 seconds remain.
	blocked, retry := l.blocked("k", 5)
	if !blocked || retry != 50*time.Second {
		t.Fatalf("blocked = %v, retry = %v; want true, 50s", blocked, retry)
	}

	if blocked, _ := l.blocked("other", 5); blocked {
		t.Error("an unrelated key is blocked")
	}

	// The first four failures leave the window, leaving one.
	clock = clock.Add(50 * time.Second)
	if blocked, _ := l.blocked("k", 5); blocked {
		t.Error("still blocked a minute after the first failure")
	}
}

func TestLimiterDropsStaleKeys(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newLimiter(time.Minute, func() time.Time { return clock })

	for i := range maxLimiterKeys + 1 {
		l.fail(fmt.Sprintf("k%d", i))
	}
	clock = clock.Add(2 * time.Minute)
	l.fail("fresh") // exceeds the key bound and triggers the sweep

	if n := len(l.fails); n != 1 {
		t.Errorf("keys after sweep = %d, want 1", n)
	}
}
