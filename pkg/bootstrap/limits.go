package bootstrap

import (
	"net"
	"net/http"
	"sync"
	"time"

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
// rateLimitEntryTTL and rateLimitMaxEntries bound the per-IP map (F290):
// the map's key is net.RemoteAddr, which is untrusted client input — every
// distinct source IP that ever connects gets an entry, and nothing removed
// one before this fix. Two independent bounds:
//
//   - rateLimitEntryTTL: an entry idle longer than this is assumed to be a
//     client that isn't coming back soon; it's evicted for free (a
//     rate.Limiter with a full bucket is indistinguishable from a
//     freshly-created one, so evicting an idle entry loses no state that
//     matters).
//   - rateLimitMaxEntries: a hard cap on cardinality. If eviction of
//     expired entries doesn't bring the map back under the cap (e.g. an
//     attacker holding many distinct source IPs simultaneously active),
//     the single least-recently-seen entry is evicted instead (LRU
//     fallback) so the map can never exceed this size regardless of
//     traffic shape.
//
// Regra 1 do CLAUDE.md (inventário de detentores): o único detentor de uma
// entrada neste mapa é um IP de origem distinto que já fez pelo menos um
// pedido. Pior caso de ocupação por detentor: um pedido único (o `Allow()`
// já cria a entrada). Não há goroutine nem timer por pedido — a limpeza só
// corre inline, dentro do mesmo lock que já protege o mapa, disparada pelo
// PRÓPRIO pedido que encontraria o mapa cheio. Isso evita introduzir um
// novo mecanismo de fundo (goroutine com ticker) só para isto.
const (
	rateLimitEntryTTL   = 10 * time.Minute
	rateLimitMaxEntries = 10000
)

// rateLimitEntry pairs a limiter with the last time it was touched, so
// idle entries can be told apart from active ones without a background
// sweep — the check happens lazily, on the request that would grow the
// map past its cap.
type rateLimitEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type rateLimitObserver struct {
	mu       sync.Mutex
	limiters map[string]*rateLimitEntry
	// perIPRate/perIPBurst are the values a future active limiter would
	// enforce — recorded here so the log line names the number now.
	perIPRate  rate.Limit
	perIPBurst int
}

func newRateLimitObserver() *rateLimitObserver {
	return &rateLimitObserver{
		limiters:   make(map[string]*rateLimitEntry),
		perIPRate:  rate.Limit(10), // 10 req/s per IP
		perIPBurst: 20,
	}
}

func (o *rateLimitObserver) limiterFor(ip string) *rate.Limiter {
	o.mu.Lock()
	defer o.mu.Unlock()

	now := time.Now()

	if e, ok := o.limiters[ip]; ok {
		// Caminho de sucesso: um cliente ativo (visto de novo antes do
		// TTL) só atualiza lastSeen — nunca é candidato a despejo LRU
		// enquanto continuar a aparecer aqui, mesmo que o mapa esteja no
		// teto quando OUTRO IP novo chegar.
		e.lastSeen = now
		return e.limiter
	}

	if len(o.limiters) >= rateLimitMaxEntries {
		o.evictLocked(now)
	}

	e := &rateLimitEntry{
		limiter:  rate.NewLimiter(o.perIPRate, o.perIPBurst),
		lastSeen: now,
	}
	o.limiters[ip] = e
	return e.limiter
}

// evictLocked frees room in the map for a new entry. Called with o.mu
// already held. First pass: remove every entry idle past the TTL — free
// for anyone still active, since nothing they'd notice is lost. If that
// isn't enough to get back under the cap (every entry is still fresh),
// fall back to evicting the single least-recently-seen entry, which bounds
// the map at rateLimitMaxEntries unconditionally.
func (o *rateLimitObserver) evictLocked(now time.Time) {
	for ip, e := range o.limiters {
		if now.Sub(e.lastSeen) > rateLimitEntryTTL {
			delete(o.limiters, ip)
		}
	}

	if len(o.limiters) < rateLimitMaxEntries {
		return
	}

	log.Warn().
		Int("cardinality", len(o.limiters)).
		Int("cap", rateLimitMaxEntries).
		Msg("rate limit observer map at cap, evicting least-recently-seen entry")

	var oldestIP string
	var oldestSeen time.Time
	first := true
	for ip, e := range o.limiters {
		if first || e.lastSeen.Before(oldestSeen) {
			oldestIP, oldestSeen = ip, e.lastSeen
			first = false
		}
	}
	if oldestIP != "" {
		delete(o.limiters, oldestIP)
	}
}

// middleware wraps next, observing but never blocking.
func (o *rateLimitObserver) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !o.limiterFor(ip).Allow() {
			log.Warn().
				Str("ip", ip).
				Str("method", r.Method).
				Str("url", redactURL(r.URL)).
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
