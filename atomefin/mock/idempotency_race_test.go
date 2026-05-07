package mock

import (
	"net/http"
	"strconv"
	"sync"
	"testing"
)

// TestIdempotencyCache_ConcurrentGetPut exercises the LRU cache
// from N goroutines doing interleaved Get / Put / Reset. Run
// with `go test -race ./atomefin/mock/` to surface any
// unsynchronised access in container/list traversal or the
// items map.
//
// The cache is also exercised end-to-end via Server's
// concurrent request handling; this test is the focused stress
// path for the data-structure invariants.
func TestIdempotencyCache_ConcurrentGetPut(t *testing.T) {
	const (
		numWriters = 8
		numReaders = 8
		numEntries = 200
		numIters   = 50
	)
	cache := newIdempotencyCache(64) // smaller than numEntries → exercises eviction

	var wg sync.WaitGroup
	wg.Add(numWriters + numReaders + 1)

	// Writers: continuously Put random keys.
	for w := 0; w < numWriters; w++ {
		w := w
		go func() {
			defer wg.Done()
			for i := 0; i < numIters; i++ {
				for j := 0; j < numEntries; j++ {
					key := "writer-" + strconv.Itoa(w) + "-key-" + strconv.Itoa(j)
					cache.Put(key, &cacheEntry{
						key:     key,
						status:  200,
						body:    []byte(`{"code":"SUCCESS"}`),
						headers: http.Header{},
					})
				}
			}
		}()
	}

	// Readers: continuously Get keys (mostly miss; some hit).
	for r := 0; r < numReaders; r++ {
		r := r
		go func() {
			defer wg.Done()
			for i := 0; i < numIters; i++ {
				for j := 0; j < numEntries; j++ {
					// Mix: sometimes look up writer-0's keys (likely
					// hit early on), sometimes our own (always miss).
					key := "writer-0-key-" + strconv.Itoa(j)
					if r%2 == 0 {
						key = "reader-" + strconv.Itoa(r) + "-key-" + strconv.Itoa(j)
					}
					_ = cache.Get(key)
				}
			}
		}()
	}

	// One goroutine intermittently Resets — simulates Server.Reset
	// during a sub-test break.
	go func() {
		defer wg.Done()
		for i := 0; i < numIters; i++ {
			cache.Reset()
		}
	}()

	wg.Wait()

	// Sanity: cache stays bounded after all the activity.
	cache.mu.Lock()
	got := cache.order.Len()
	cache.mu.Unlock()
	if got > cache.cap {
		t.Errorf("cache.order.Len() = %d, exceeds cap %d", got, cache.cap)
	}
}

// TestIdempotencyCache_EvictionUnderPressure pins that
// repeatedly Putting > capacity entries leaves exactly the
// most-recent `cap` items in the cache. Single-threaded; the
// race test above covers concurrency.
func TestIdempotencyCache_EvictionUnderPressure(t *testing.T) {
	const cap = 16
	cache := newIdempotencyCache(cap)

	for i := 0; i < cap*4; i++ {
		key := "k-" + strconv.Itoa(i)
		cache.Put(key, &cacheEntry{key: key, status: 200, body: []byte("ok")})
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.order.Len() != cap {
		t.Errorf("cache.order.Len() = %d, want %d", cache.order.Len(), cap)
	}
	if len(cache.items) != cap {
		t.Errorf("len(cache.items) = %d, want %d", len(cache.items), cap)
	}
	// The most-recent `cap` entries must be present; the older
	// ones evicted.
	for i := cap * 3; i < cap*4; i++ {
		key := "k-" + strconv.Itoa(i)
		if _, ok := cache.items[key]; !ok {
			t.Errorf("expected recent key %q in cache", key)
		}
	}
	for i := 0; i < cap; i++ {
		key := "k-" + strconv.Itoa(i)
		if _, ok := cache.items[key]; ok {
			t.Errorf("stale key %q should have been evicted", key)
		}
	}
}
