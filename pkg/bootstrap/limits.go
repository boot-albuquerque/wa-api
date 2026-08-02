package bootstrap

import (
	"net"
	"net/http"
	"sync"

	customhttp "wa-api/pkg/presentation/http"

	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"
)

// maxRequestBodyBytes caps request bodies at 10 MiB. Before this middleware,
// nothing in the repo capped body size at all (verified: zero occurrences
// of MaxBytesReader) — an unbounded body can run a handler out of memory
// well before any per-field validation gets a chance to reject it.
const maxRequestBodyBytes = 10 << 20 // 10 MiB

// bodyLimitMiddleware rejects a request whose announced Content-Length
// already exceeds the limit (the common case: curl/most HTTP clients send
// Content-Length for a known-size body) and, as defense in depth for
// chunked transfers or a lying Content-Length, wraps the body in
// http.MaxBytesReader so no handler can read past the cap regardless of
// what the client claimed upfront.
func bodyLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > maxRequestBodyBytes {
			customhttp.RespondJSON(w, http.StatusRequestEntityTooLarge, nil, errBodyTooLarge)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		next.ServeHTTP(w, r)
	})
}

var errBodyTooLarge = &simpleBootstrapError{msg: "request body too large"}

type simpleBootstrapError struct{ msg string }

func (e *simpleBootstrapError) Error() string { return e.msg }

// rateLimitObserver logs which requests WOULD have been rejected by a
// per-IP rate limit, without actually rejecting any of them. The plan
// calls for this deliberately: nobody has ever measured real traffic
// against a limit here, so shipping an active limiter now risks derived
// numbers dropping legitimate traffic. One release of observation is what
// turns "limit BLOCKER, no data" into a limit backed by what was actually
// seen — see plano-correcao-wa-api.md Fase 5c.
type rateLimitObserver struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	// perIPRate/perIPBurst are the values a future active limiter would
	// enforce — recorded here so the log line names the number now.
	perIPRate  rate.Limit
	perIPBurst int
}

func newRateLimitObserver() *rateLimitObserver {
	return &rateLimitObserver{
		limiters:   make(map[string]*rate.Limiter),
		perIPRate:  rate.Limit(10), // 10 req/s per IP
		perIPBurst: 20,
	}
}

func (o *rateLimitObserver) limiterFor(ip string) *rate.Limiter {
	o.mu.Lock()
	defer o.mu.Unlock()
	l, ok := o.limiters[ip]
	if !ok {
		l = rate.NewLimiter(o.perIPRate, o.perIPBurst)
		o.limiters[ip] = l
	}
	return l
}

// middleware wraps next, observing but never blocking.
func (o *rateLimitObserver) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !o.limiterFor(ip).Allow() {
			log.Warn().
				Str("ip", ip).
				Str("method", r.Method).
				Stringer("url", r.URL).
				Float64("limit_per_sec", float64(o.perIPRate)).
				Int("burst", o.perIPBurst).
				Msg("rate limit observe-only: this request would have been rejected")
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP extracts the request's remote IP, stripping the port. This
// intentionally does not trust X-Forwarded-For: without a configured,
// trusted proxy list, that header is client-controlled and would let a
// single client evade the per-IP bucket by sending a different claimed IP
// on every request.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
