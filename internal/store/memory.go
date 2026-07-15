package store

import (
	"context"
	"sync"
	"time"

	"github.com/AbubakarMahmood/go-rate-limiter/pkg/limiter"
)

// janitorInterval controls how often expired entries are evicted.
const janitorInterval = 30 * time.Second

// MemoryStore is an in-process limiter.Store for single-instance deployments
// and tests. Atomicity is provided by a mutex per key, so unrelated keys
// never contend with each other.
type MemoryStore struct {
	mu      sync.RWMutex
	windows map[string]*windowEntry
	buckets map[string]*bucketEntry

	stopOnce sync.Once
	stop     chan struct{}
}

// windowEntry holds the two windows a decision can depend on. Older windows
// carry no information, so state is O(1) per key regardless of uptime.
type windowEntry struct {
	mu        sync.Mutex
	deleted   bool  // set by the janitor; holders must re-fetch from the map
	curStart  int64 // unix microseconds
	cur, prev int64
	expiresAt int64 // unix microseconds
}

type bucketEntry struct {
	mu        sync.Mutex
	deleted   bool
	tokens    float64
	ts        float64 // unix seconds of the last refill, fractional
	expiresAt int64   // unix microseconds
}

// NewMemoryStore creates an in-memory store and starts its eviction loop.
// Call Close to stop the loop.
func NewMemoryStore() *MemoryStore {
	ms := &MemoryStore{
		windows: make(map[string]*windowEntry),
		buckets: make(map[string]*bucketEntry),
		stop:    make(chan struct{}),
	}
	go ms.janitor()
	return ms
}

// IncrWindow implements limiter.Store.
func (ms *MemoryStore) IncrWindow(_ context.Context, key string, window time.Duration, n, limit int64, weightPrev bool, ttl time.Duration) (*limiter.WindowResult, error) {
	for {
		e := ms.windowEntry(key)
		e.mu.Lock()
		if e.deleted {
			e.mu.Unlock()
			continue // evicted between lookup and lock; fetch a fresh entry
		}

		now := time.Now()
		nowUS := now.UnixMicro()
		w := window.Microseconds()
		start := nowUS - nowUS%w

		switch {
		case e.curStart == start:
			// still in the same window
		case e.curStart == start-w:
			e.prev, e.cur, e.curStart = e.cur, 0, start
		default:
			e.prev, e.cur, e.curStart = 0, 0, start
		}

		weighted := float64(e.cur)
		if weightPrev && e.prev > 0 {
			weighted += float64(e.prev) * (1 - float64(nowUS-start)/float64(w))
		}

		allowed := weighted+float64(n) <= float64(limit)
		if allowed && n > 0 {
			e.cur += n
		}
		// Every touch counts as activity, including peeks and denials;
		// otherwise an entry created by a peek would never expire.
		e.expiresAt = nowUS + ttl.Microseconds()

		res := &limiter.WindowResult{
			Allowed:     allowed,
			Current:     e.cur,
			Previous:    e.prev,
			WindowStart: time.UnixMicro(start),
			Now:         now,
		}
		e.mu.Unlock()
		return res, nil
	}
}

// TakeTokens implements limiter.Store.
func (ms *MemoryStore) TakeTokens(_ context.Context, key string, capacity, refillPerSec, n float64, ttl time.Duration) (bool, float64, error) {
	for {
		e := ms.bucketEntry(key, capacity)
		e.mu.Lock()
		if e.deleted {
			e.mu.Unlock()
			continue
		}

		now := time.Now()
		nowSec := float64(now.UnixMicro()) / 1e6

		// Commit the refill unconditionally: tokens and ts must advance
		// together or the same elapsed time would be credited twice.
		if elapsed := nowSec - e.ts; elapsed > 0 {
			e.tokens += elapsed * refillPerSec
			if e.tokens > capacity {
				e.tokens = capacity
			}
		}
		e.ts = nowSec

		allowed := n <= e.tokens
		if allowed && n > 0 {
			e.tokens -= n
		}
		e.expiresAt = now.UnixMicro() + ttl.Microseconds()

		tokens := e.tokens
		e.mu.Unlock()
		return allowed, tokens, nil
	}
}

// Delete implements limiter.Store.
func (ms *MemoryStore) Delete(_ context.Context, key string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if e, ok := ms.windows[key]; ok {
		e.mu.Lock()
		e.deleted = true
		e.mu.Unlock()
		delete(ms.windows, key)
	}
	if e, ok := ms.buckets[key]; ok {
		e.mu.Lock()
		e.deleted = true
		e.mu.Unlock()
		delete(ms.buckets, key)
	}
	return nil
}

// Ping implements limiter.Store.
func (ms *MemoryStore) Ping(context.Context) error { return nil }

// Close stops the eviction loop.
func (ms *MemoryStore) Close() error {
	ms.stopOnce.Do(func() { close(ms.stop) })
	return nil
}

func (ms *MemoryStore) windowEntry(key string) *windowEntry {
	ms.mu.RLock()
	e, ok := ms.windows[key]
	ms.mu.RUnlock()
	if ok {
		return e
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()
	if e, ok := ms.windows[key]; ok {
		return e
	}
	e = &windowEntry{}
	ms.windows[key] = e
	return e
}

func (ms *MemoryStore) bucketEntry(key string, capacity float64) *bucketEntry {
	ms.mu.RLock()
	e, ok := ms.buckets[key]
	ms.mu.RUnlock()
	if ok {
		return e
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()
	if e, ok := ms.buckets[key]; ok {
		return e
	}
	// A key that has never been seen starts with a full bucket, refilled now.
	e = &bucketEntry{tokens: capacity, ts: float64(time.Now().UnixMicro()) / 1e6}
	ms.buckets[key] = e
	return e
}

// janitor evicts entries whose TTL has passed. Expiry is equivalent to a
// rolled-over window (counters) or a fully refilled bucket (tokens), so
// eviction never changes observable behaviour.
func (ms *MemoryStore) janitor() {
	ticker := time.NewTicker(janitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ms.stop:
			return
		case <-ticker.C:
			ms.sweep(time.Now().UnixMicro())
		}
	}
}

// sweep evicts every entry that expired before cutoff (unix microseconds).
func (ms *MemoryStore) sweep(cutoff int64) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	for key, e := range ms.windows {
		e.mu.Lock()
		if e.expiresAt != 0 && e.expiresAt < cutoff {
			e.deleted = true
			delete(ms.windows, key)
		}
		e.mu.Unlock()
	}
	for key, e := range ms.buckets {
		e.mu.Lock()
		if e.expiresAt != 0 && e.expiresAt < cutoff {
			e.deleted = true
			delete(ms.buckets, key)
		}
		e.mu.Unlock()
	}
}
