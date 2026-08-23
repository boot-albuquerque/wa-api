package bootstrap

import (
	"os"
	"runtime/debug"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// Event dispatch with a worker pool and a BYTE-bounded queue (F86).
//
// A domain event fans out to up to four deliveries — WebSocket, user webhook,
// global webhook and RabbitMQ — and each one does network I/O. Before this,
// every delivery was a loose goroutine: `SafeGo` is `go func()` with `recover`,
// which protects against panics and not against volume.
//
// The design is a degradation LADDER, measured rather than assumed. See F86 in
// HOUSEKEEP.md for the full tables:
//
//	burst <= pool     — indistinguishable from having no mechanism at all
//	burst  > pool     — queues: delivery is slower, and COMPLETE
//	burst  > budget   — the handler absorbs backpressure, still NO LOSS
//
// No rung ever drops a delivery. That is a choice, and it is why the saturated
// path blocks instead of discarding: dropping with a cap of 64 lost 93.6% of
// deliveries in the measurement, and 70.5% even with an 8MB queue.
//
// There is no burst detector and no mode switch. It would be a threshold to
// calibrate, hysteresis to get right and boundary flapping to debug — all to
// produce the behaviour the ladder above already produces on its own.

const (
	// envDispatchWorkers replaces WA_API_DISPATCH_MAX_CONCURRENCY, which was the
	// blocking cap of the first attempt. The name changed because the thing
	// changed: it is no longer a cap on the caller, it is the pool size.
	envDispatchWorkers    = "WA_API_DISPATCH_WORKERS"
	envDispatchQueueBytes = "WA_API_DISPATCH_QUEUE_BYTES"

	// dispatchDefaultWorkers comes from the burst measured in production: the
	// pairing peak was 23 events in 100ms, which is 92 deliveries. With 256
	// workers that peak passes straight through without queueing anything — the
	// mechanism only shows up from roughly 3 simultaneous pairings on.
	dispatchDefaultWorkers = 256

	// dispatchDefaultQueueBytes comes from the measured high-water mark: a burst
	// of 8,000 deliveries (~87 simultaneous pairings) held 26.8MB at peak. With
	// 32MB the event handler is never touched up to that scale.
	//
	// The unit is BYTES and not items on purpose: payloads measured on this
	// install range from 1.5KB to 120KB — the maximum is 40x the median, so a
	// queue of N items would have a memory ceiling that varies 40x with the luck
	// of the batch.
	dispatchDefaultQueueBytes = 32 * 1024 * 1024

	// dispatchQueueCapItems is a secondary guard, not the primary limit. It
	// exists for the degenerate case of tiny payloads, where the byte budget
	// alone would let the queue grow in COUNT without growing in bytes — each
	// item still costs a descriptor and a closure.
	dispatchQueueCapItems = 65536

	// dispatchWaitWarnThreshold is how much accumulated waiting in the handler
	// justifies a warning. Below it, waiting is the mechanism working as
	// designed and logging would be noise.
	dispatchWaitWarnThreshold = 500 * time.Millisecond
)

// dispatchJob is a queued delivery. `bytes` is the size of the payload the
// closure keeps alive, and it is what the budget accounting uses.
type dispatchJob struct {
	name  string
	bytes int
	fn    func()
}

// dispatchPool is the delivery pool with a byte-bounded queue.
type dispatchPool struct {
	jobs chan dispatchJob

	// mu protects bytesInFlight/peakBytes AND is cond's Locker. Byte accounting
	// cannot be a loose atomic: a waiting caller has to be woken when a worker
	// returns space, and that calls for a condition, not a counter.
	mu            sync.Mutex
	cond          *sync.Cond
	bytesInFlight int64
	bytesBudget   int64
	peakBytes     int64

	// Observability. Without it there is no way to answer "did the queue reach
	// its ceiling?" other than by speculation — which is precisely what this
	// whole implementation exists to avoid.
	inFlight           atomic.Int64
	peak               atomic.Int64
	waits              atomic.Int64
	waitedNanos        atomic.Int64
	warnedAboutWaiting atomic.Bool
}

func newDispatchPool(workers int, bytesBudget int64) *dispatchPool {
	if workers <= 0 {
		return &dispatchPool{} // jobs nil: mechanism disabled
	}
	p := &dispatchPool{
		jobs:        make(chan dispatchJob, dispatchQueueCapItems),
		bytesBudget: bytesBudget,
	}
	p.cond = sync.NewCond(&p.mu)
	for i := 0; i < workers; i++ {
		go p.worker()
	}
	return p
}

func (p *dispatchPool) worker() {
	for job := range p.jobs {
		p.runJob(job)
	}
}

// runJob runs one delivery and ALWAYS returns the space it occupied.
//
// The recover is PER JOB, not per goroutine as in SafeGo. The difference
// matters in a permanent pool: SafeGo recovers the panic and the goroutine dies
// right after, which here would shrink the pool on every panic until no worker
// was left.
func (p *dispatchPool) runJob(job dispatchJob) {
	defer func() {
		p.inFlight.Add(-1)
		p.mu.Lock()
		p.bytesInFlight -= int64(job.bytes)
		p.mu.Unlock()
		p.cond.Broadcast()
		if r := recover(); r != nil {
			log.Error().
				Str("goroutine", job.name).
				Interface("panic", r).
				Str("stack", string(debug.Stack())).
				Msg("panic recovered in dispatch worker")
		}
	}()

	current := p.inFlight.Add(1)
	for {
		peak := p.peak.Load()
		if current <= peak || p.peak.CompareAndSwap(peak, current) {
			break
		}
	}
	job.fn()
}

// Go queues a delivery. `bytes` is the size of the payload fn keeps alive.
//
// With the pool disabled it delegates to safeGo, and the behaviour is identical
// to what existed before F86 — that is what keeps the A/B possible and the
// rollback down to one environment variable.
func (p *dispatchPool) Go(name string, bytes int, fn func()) {
	if p == nil || p.jobs == nil {
		safeGo(name, fn)
		return
	}

	start := time.Now()
	p.mu.Lock()
	// An item larger than the entire budget goes through anyway. Without this
	// guard, the 120KB event measured on this install would be stuck forever
	// under a small budget, waiting for room that never arrives — the protective
	// mechanism would become a deadlock.
	if int64(bytes) <= p.bytesBudget {
		for p.bytesInFlight+int64(bytes) > p.bytesBudget {
			p.waits.Add(1)
			// sync.Cond, and not N tokens in a channel: with tokens and MORE
			// THAN ONE producer — in production there is one handler per
			// session — two callers can each hold half and neither complete.
			p.cond.Wait()
		}
	}
	p.bytesInFlight += int64(bytes)
	if p.bytesInFlight > p.peakBytes {
		p.peakBytes = p.bytesInFlight
	}
	p.mu.Unlock()

	p.jobs <- dispatchJob{name: name, bytes: bytes, fn: fn}

	if waited := time.Since(start); waited > 0 {
		total := p.waitedNanos.Add(int64(waited))
		// The warning fires ONCE: the caller is the SDK's event handler
		// goroutine, and logging per occurrence would turn saturation into a
		// second burst, this time of log lines. See F85.
		if total > int64(dispatchWaitWarnThreshold) && p.warnedAboutWaiting.CompareAndSwap(false, true) {
			log.Warn().
				Dur("accumulated_wait", time.Duration(total)).
				Int64("budget_bytes", p.bytesBudget).
				Msg("dispatch queue saturated: the event handler is being held")
		}
	}
}

// Metrics reports the observed state. Used by tests and by the measurements.
func (p *dispatchPool) Metrics() (inFlight, peak, waits, peakBytes int64) {
	if p == nil {
		return 0, 0, 0, 0
	}
	p.mu.Lock()
	observedPeakBytes := p.peakBytes
	p.mu.Unlock()
	return p.inFlight.Load(), p.peak.Load(), p.waits.Load(), observedPeakBytes
}

var (
	dispatchOnce sync.Once
	dispatch     *dispatchPool
)

// dispatchGo is the single point every one of the four deliveries goes through.
func dispatchGo(name string, bytes int, fn func()) {
	dispatchOnce.Do(func() {
		dispatch = newDispatchPool(configuredDispatchWorkers(), configuredDispatchQueueBytes())
	})
	dispatch.Go(name, bytes, fn)
}

// configuredDispatchWorkers reads the pool size from the environment. Zero
// DISABLES the mechanism — that is the rollback, and it is distinct from an
// invalid value.
func configuredDispatchWorkers() int {
	return readIntFromEnv(envDispatchWorkers, dispatchDefaultWorkers)
}

// configuredDispatchQueueBytes reads the queue budget from the environment.
func configuredDispatchQueueBytes() int64 {
	return int64(readIntFromEnv(envDispatchQueueBytes, dispatchDefaultQueueBytes))
}

// readIntFromEnv returns the default for an absent, invalid or negative value,
// always with a Warn in the last two cases: a typo must not change the
// protection silently, and with NON-ZERO defaults this path is the difference
// between protecting and not protecting.
func readIntFromEnv(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		log.Warn().Str("value", raw).Str("var", name).Int("using", fallback).
			Msg("invalid value; using the default")
		return fallback
	}
	if parsed < 0 {
		log.Warn().Int("value", parsed).Str("var", name).Int("using", fallback).
			Msg("negative value; using the default")
		return fallback
	}
	return parsed
}
