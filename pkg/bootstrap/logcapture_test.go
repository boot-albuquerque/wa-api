package bootstrap

import (
	"bytes"
	"os"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Log capture for this package's tests (F132).
//
// `log.Logger` is a plain global with no synchronisation. Production writes it
// EXACTLY ONCE, inside Main() at startup (main.go:170 and main.go:201), before
// any dispatch pool or webhook retry exists: one writer, no concurrent reader.
//
// The test binary was a different story. Ten call sites across eight files
// assigned `log.Logger` mid-run, while work started by EARLIER tests was still
// calling log.Warn(): dispatch-pool workers (dispatch.go:112) never stop, and
// pending webhook retry timers (dispatch_retry.go:161) fire tens of seconds
// after the test that armed them returned. Every assignment raced against those
// survivors.
//
// The fix removes the writes rather than chasing the survivors. `log.Logger` is
// set ONCE, by TestMain, to a logger whose sink is `testLogRouter`. Capturing
// output is now installing a buffer on that router under a mutex — the
// zerolog.Logger value the workers read never changes again, so there is
// nothing left to race with. It also holds for any FUTURE leaked producer,
// which a per-test drain would not.

// logCapture is a mutex-guarded buffer. The lock is not decoration: the whole
// point is that a leaked dispatch worker may write to it while the test reads.
type logCapture struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *logCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *logCapture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

func (c *logCapture) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Len()
}

// Bytes returns a COPY: the caller keeps reading it after the lock is dropped,
// and the underlying array can be reallocated by a concurrent Write.
func (c *logCapture) Bytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.buf.Bytes()...)
}

func (c *logCapture) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf.Reset()
}

// logRouter is the single sink installed in `log.Logger`. With no capture
// installed it discards, which is what a test that asserts nothing about logs
// wants anyway.
type logRouter struct {
	mu      sync.Mutex
	current *logCapture
}

func (r *logRouter) Write(p []byte) (int, error) {
	r.mu.Lock()
	current := r.current
	r.mu.Unlock()
	if current == nil {
		return len(p), nil
	}
	return current.Write(p)
}

func (r *logRouter) install(c *logCapture) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current = c
}

var testLogRouter = &logRouter{}

func TestMain(m *testing.M) {
	// The only assignment to log.Logger in the test binary, and it happens
	// before the first test starts. See the file comment.
	log.Logger = zerolog.New(testLogRouter)
	os.Exit(m.Run())
}

// captureLogInto routes the global logger into a fresh buffer for the duration
// of the test. It replaces the `orig := log.Logger; log.Logger = zerolog.New(&buf)`
// pattern that F132 measured as the racing side.
func captureLogInto(t *testing.T) *logCapture {
	t.Helper()
	c := &logCapture{}
	testLogRouter.install(c)
	t.Cleanup(func() { testLogRouter.install(nil) })
	return c
}

// TestF132_CapturaDeLogNaoCorreComDespachoVivo is the test for the defect.
//
// It reproduces the condition F132 measured, deterministically instead of by
// coincidence: a dispatch-pool worker inside log.Warn() — the surviving side of
// 35 of 35 measured race blocks — while the test installs log captures. Under
// the old pattern (`log.Logger = zerolog.New(&buf)`) this is an unsynchronised
// write against an unsynchronised read and -race flags it; with the router the
// global is never written after TestMain and there is nothing left to race.
//
// It runs the REAL dispatchGo, not a local pool: the local pool was never the
// racing side, and a test that used one would not exercise the path measured.
func TestF132_CapturaDeLogNaoCorreComDespachoVivo(t *testing.T) {
	parar := make(chan struct{})
	vivo := make(chan struct{})
	terminou := make(chan struct{})

	dispatchGo("f132-sobrevivente", 0, func() {
		defer close(terminou)
		close(vivo)
		for {
			select {
			case <-parar:
				return
			default:
			}
			// O mesmo log.Warn() que tentarWebhook emite quando o cliente
			// HTTP e' nil (dispatch_callhook.go:112).
			log.Warn().Str("origem", "f132").Msg("sobrevivente do despacho")
		}
	})
	<-vivo

	// Troca de captura em rajada, no lugar onde os dez pontos de teste
	// trocavam o log.Logger global.
	for i := 0; i < 500; i++ {
		testLogRouter.install(&logCapture{})
	}
	testLogRouter.install(nil)

	close(parar)
	<-terminou
}
