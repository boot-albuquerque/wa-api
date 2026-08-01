package bootstrap

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestBodyLimit_ContentLengthOverCap proves the plan's exact scenario:
// posting a body announced (via Content-Length) as larger than the cap
// gets 413, not 200 and not an attempt to read the whole thing into
// memory. curl --data-binary sends Content-Length for a file of known
// size, which is the common case this covers.
func TestBodyLimit_ContentLengthOverCap(t *testing.T) {
	d := minimalDeps()
	router := NewRouter(d)

	req := httptest.NewRequest(http.MethodPost, "/chat/send/text", bytes.NewReader([]byte("x")))
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

	req := httptest.NewRequest(http.MethodPost, "/chat/send/text", bytes.NewReader([]byte(`{"text":"hi"}`)))
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
