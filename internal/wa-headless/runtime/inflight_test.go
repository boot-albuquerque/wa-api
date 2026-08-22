package runtime

import (
	"context"
	"errors"
	"runtime"
	"syscall"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

// TestAnInFlightCallSurvivesTheBrowserDying measures the Fase 2 item the suite
// did not cover: what an operation ALREADY RUNNING gets when the browser dies
// under it.
//
// The Holder's contract for a dead session is documented and tested —
// ErrSessionDied, and it refuses to re-boot rather than hiding a browser that
// keeps dying. What nothing measured is the call that was already in the page
// when the process went away. Two failures are possible and only one is
// acceptable: returning a classified error within the deadline, or hanging.
//
// IT USES A TEMPORARY PROFILE AND A LOCAL PAGE, NOT THE LAB ONE, and that is not
// convenience. SIGKILL against a PAIRED profile risks corrupting it, and
// re-pairing needs a human with the phone — a cost this measurement has no right
// to spend. The mechanism under test (Runner.Do, the CDP transport, the deadline)
// is the same either way; only the credential at risk differs.
func TestAnInFlightCallSurvivesTheBrowserDying(t *testing.T) {
	cfg := holderConfig(t, t.TempDir())
	h := NewHolder(cfg)
	defer h.Stop(context.Background())

	sess, err := h.Session(context.Background())
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	pid, err := h.BrowserPID()
	if err != nil || pid <= 0 {
		t.Fatalf("no browser pid to kill (%d, %v)", pid, err)
	}

	before := runtime.NumGoroutine()

	// UMA CHAMADA QUE FICA NA PAGINA. O laco ocupa o alvo por bem mais tempo que
	// a morte que vem a seguir, para que ela aconteca COM a chamada em voo — que
	// e' o unico instante que este teste mede.
	type outcome struct {
		err   error
		after time.Duration
	}
	done := make(chan outcome, 1)
	go func() {
		start := time.Now()
		var out string
		e := cfg.Runner.Do(context.Background(), engine.OpStateProbe, "inflight/kill", func(ctx context.Context) error {
			return sess.Tab().Evaluate(ctx, `(() => { const t = Date.now(); while (Date.now() - t < 120000) {} return "never"; })()`, &out)
		})
		done <- outcome{e, time.Since(start)}
	}()

	time.Sleep(2 * time.Second)
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatalf("killing the browser: %v", err)
	}

	select {
	case got := <-done:
		t.Logf("a chamada em voo voltou em %s com err=%v", got.after.Round(time.Millisecond), got.err)
		if got.err == nil {
			t.Fatal("the call reported SUCCESS after the browser was killed under " +
				"it; a caller would act on an answer that never came")
		}
		// O QUE IMPORTA E' TER VOLTADO CLASSIFICADO, nao qual classe. Um erro de
		// transporte e um estouro de prazo sao ambos respostas honestas; o que
		// nao pode e' pendurar.
		var to *engine.TimeoutError
		t.Logf("classificado como timeout: %t", errors.As(got.err, &to))
	case <-time.After(90 * time.Second):
		t.Fatal("the in-flight call never returned after the browser died; a dead " +
			"browser must not be able to hold a caller forever")
	}

	// E NAO PODE DEIXAR GOROUTINE PARA TRAS. Uma chamada que volta com erro e
	// deixa a sua goroutine de transporte viva troca um travamento visivel por um
	// vazamento silencioso.
	for i := 0; i < 3; i++ {
		runtime.GC()
		time.Sleep(1500 * time.Millisecond)
	}
	after := runtime.NumGoroutine()
	t.Logf("goroutines: %d antes, %d depois", before, after)
	if after > before+4 {
		t.Errorf("the dead browser left %d goroutine(s) behind (%d -> %d)",
			after-before, before, after)
	}

	// E A SESSAO SEGUINTE TEM DE DIZER QUE MORREU, nao rebootar em silencio.
	if _, err := h.Session(context.Background()); !errors.Is(err, ErrSessionDied) {
		t.Fatalf("after the kill, Session() = %v; want ErrSessionDied", err)
	}
}
