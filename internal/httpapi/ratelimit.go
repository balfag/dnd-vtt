package httpapi

import (
	"sync"
	"time"
)

const (
	failWindow = time.Minute
	// maxLimiterKeys bounds memory: past it, fail drops keys without fresh failures.
	maxLimiterKeys = 10_000

	limitPerIP    = 20
	limitPerLogin = 5
)

// limiter counts failed attempts per key in a sliding window.
type limiter struct {
	mu     sync.Mutex
	window time.Duration
	now    func() time.Time
	fails  map[string][]time.Time
}

func newLimiter(window time.Duration, now func() time.Time) *limiter {
	return &limiter{window: window, now: now, fails: make(map[string][]time.Time)}
}

// fresh returns the failures of key that are still inside the window.
// The caller holds l.mu.
func (l *limiter) fresh(key string, now time.Time) []time.Time {
	stamps := l.fails[key]
	i := 0
	for i < len(stamps) && now.Sub(stamps[i]) >= l.window {
		i++
	}
	return stamps[i:]
}

// blocked reports whether key has at least limit failures inside the window
// and how long until the oldest of them leaves it.
func (l *limiter) blocked(key string, limit int) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	stamps := l.fresh(key, now)
	if len(stamps) < limit {
		return false, 0
	}
	return true, stamps[0].Add(l.window).Sub(now)
}

// fail records a failure for key.
func (l *limiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.fails[key] = append(l.fresh(key, now), now)

	if len(l.fails) > maxLimiterKeys {
		for k := range l.fails {
			if len(l.fresh(k, now)) == 0 {
				delete(l.fails, k)
			}
		}
	}
}
