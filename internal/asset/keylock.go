package asset

import "sync"

// keyLock serializes work per key; keys nobody holds take no memory.
// The zero value is ready to use.
type keyLock struct {
	mu   sync.Mutex
	held map[string]*keyEntry
}

type keyEntry struct {
	mu   sync.Mutex
	refs int // holders and waiters; guarded by keyLock.mu
}

// lock blocks until key is free and returns the function that frees it.
func (l *keyLock) lock(key string) (unlock func()) {
	l.mu.Lock()
	if l.held == nil {
		l.held = make(map[string]*keyEntry)
	}
	e, ok := l.held[key]
	if !ok {
		e = &keyEntry{}
		l.held[key] = e
	}
	e.refs++
	l.mu.Unlock()

	e.mu.Lock()
	return func() {
		e.mu.Unlock()
		l.mu.Lock()
		e.refs--
		if e.refs == 0 {
			delete(l.held, key)
		}
		l.mu.Unlock()
	}
}
