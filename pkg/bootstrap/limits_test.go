package bootstrap

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestBodyLimit_ContentLengthOverCap proves the plan's exact scenario:
// posting a body announced (via Content-Length) as larger than the cap
// gets 413, not 200 and not an attempt to read the whole thing into
// memory. curl --data-binary sends Content-Length for a file of known
// size, which is the common case this covers.
func TestBodyLimit_ContentLengthOverCap(t *testing.T) {
	d := minimalDeps()
	router := NewRouter(d)

	req := httptest.NewRequest(http.MethodPost, "/chats/send/text", bytes.NewReader([]byte("x")))
	req.ContentLength = maxRequestBodyBytes + 1
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

// TestBodyLimit_UnderCapPassesThrough proves the middleware doesn't
// interfere with ordinary requests — a body under the cap reaches auth
// (and gets 401 for lack of credentials, same as every other route in
// the golden baseline), not 413.
func TestBodyLimit_UnderCapPassesThrough(t *testing.T) {
	d := minimalDeps()
	router := NewRouter(d)

	req := httptest.NewRequest(http.MethodPost, "/chats/send/text", bytes.NewReader([]byte(`{"text":"hi"}`)))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, small body should not be rejected as too large", rec.Code)
	}
}

// TestRateLimitObserver_NeverBlocks proves the "observe-only" contract:
// even after exceeding the configured burst, requests keep flowing
// through — nothing gets a 429 from this middleware, by design, for this
// release (plano-correcao-wa-api.md Fase 5c).
func TestRateLimitObserver_NeverBlocks(t *testing.T) {
	d := minimalDeps()
	router := NewRouter(d)

	const attempts = 50 // well past the 20-request burst
	for i := 0; i < attempts; i++ {
		req := httptest.NewRequest(http.MethodGet, "/livez", nil)
		req.RemoteAddr = "203.0.113.5:12345"
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d got 429 — rate limiter is blocking, but this release is observe-only", i)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want %d (/livez should always succeed)", i, rec.Code, http.StatusOK)
		}
	}
}

// TestRateLimitObserver_MapBoundedByCardinalityCap proves F290's fix: the
// per-IP map used to grow without bound, one entry per distinct source IP
// forever. Feeding it rateLimitMaxEntries+1000 distinct IPs — none of them
// idle, all inserted in the same instant — must still leave the map at or
// under the cap, because the LRU fallback in evictLocked kicks in once the
// TTL pass finds nothing expired to reclaim.
func TestRateLimitObserver_MapBoundedByCardinalityCap(t *testing.T) {
	o := newRateLimitObserver()

	const extra = 1000
	for i := 0; i < rateLimitMaxEntries+extra; i++ {
		o.limiterFor(fmt.Sprintf("10.0.%d.%d", i/256, i%256))
	}

	o.mu.Lock()
	got := len(o.limiters)
	o.mu.Unlock()

	if got > rateLimitMaxEntries {
		t.Fatalf("map cardinality = %d, want <= %d (rateLimitMaxEntries) — the cap is not being enforced", got, rateLimitMaxEntries)
	}
}

// TestRateLimitObserver_ExpiredEntriesEvictedBeforeLRU proves the TTL pass
// runs first and reclaims idle entries for free: fill the map to the cap,
// backdate every entry's lastSeen past the TTL, then insert one more IP.
// The new entry must fit without the map growing past the cap, and the
// stale entries (not just one, via LRU) must be gone — proving eviction
// used the TTL path, not a single LRU eviction that happened to make room
// for exactly one more entry.
func TestRateLimitObserver_ExpiredEntriesEvictedBeforeLRU(t *testing.T) {
	o := newRateLimitObserver()

	for i := 0; i < rateLimitMaxEntries; i++ {
		o.limiterFor(fmt.Sprintf("10.1.%d.%d", i/256, i%256))
	}

	o.mu.Lock()
	if len(o.limiters) != rateLimitMaxEntries {
		o.mu.Unlock()
		t.Fatalf("setup: map = %d entries, want exactly %d", len(o.limiters), rateLimitMaxEntries)
	}
	stale := time.Now().Add(-rateLimitEntryTTL - time.Minute)
	for _, e := range o.limiters {
		e.lastSeen = stale
	}
	o.mu.Unlock()

	o.limiterFor("10.2.0.1") // one new IP: should trigger the TTL sweep

	o.mu.Lock()
	got := len(o.limiters)
	o.mu.Unlock()

	// TTL sweep removes ALL stale entries, not just enough for one slot,
	// so the map should have shrunk drastically — nowhere near the cap.
	if got >= rateLimitMaxEntries {
		t.Fatalf("map = %d entries after TTL sweep, want well under %d — stale entries were not purged in bulk", got, rateLimitMaxEntries)
	}
	if got < 1 {
		t.Fatalf("map = %d entries, want at least the 1 freshly-inserted IP", got)
	}
}

// TestRateLimitObserver_ActiveClientSurvivesEviction is the success-path
// half the anti-regression note demands: an entry that keeps being touched
// (lastSeen refreshed on every request, exactly like an active client
// still being rate-limited) must never be the one picked by the LRU
// fallback, even while the map sits at its cap and a flood of brand-new
// IPs keeps arriving. A cap that evicts by insertion order instead of
// recency would fail this.
func TestRateLimitObserver_ActiveClientSurvivesEviction(t *testing.T) {
	o := newRateLimitObserver()

	const activeIP = "203.0.113.9"
	o.limiterFor(activeIP)

	for i := 0; i < rateLimitMaxEntries+500; i++ {
		// The active client makes a request between every few newcomers,
		// so its lastSeen is always fresher than the flood's.
		if i%7 == 0 {
			o.limiterFor(activeIP)
		}
		o.limiterFor(fmt.Sprintf("198.51.%d.%d", i/256, i%256))
	}
	o.limiterFor(activeIP)

	o.mu.Lock()
	_, stillPresent := o.limiters[activeIP]
	o.mu.Unlock()

	if !stillPresent {
		t.Fatal("active client was evicted while a flood of new IPs kept the map at cap — LRU picked the wrong entry")
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		remoteAddr string
		want       string
	}{
		{"203.0.113.5:12345", "203.0.113.5"},
		{"[2001:db8::1]:12345", "2001:db8::1"},
		{"not-a-valid-addr", "not-a-valid-addr"},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = tt.remoteAddr
		if got := clientIP(req); got != tt.want {
			t.Errorf("clientIP(%q) = %q, want %q", tt.remoteAddr, got, tt.want)
		}
	}
}
