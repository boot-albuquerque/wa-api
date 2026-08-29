package headless

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeTeardownLeaks is the opening measurement of Fase 2, and it exists
// because nothing measured this before: `NumGoroutine` appears in ZERO tests in
// this module. The shutdown suite proves the protocol PATH — that CleanStop
// signals, waits, and labels a refusal — and says nothing about what is left
// behind afterwards.
//
// THE PHASE CHANGES WHAT COUNTS AS EVIDENCE. Fase 1 measured against a
// reference: does the upstream do this? Fase 2 measures against failure and
// load, and the project's own rule from the F86 dispatch pool governs it —
// measure the scenario where the mechanism CHARGES the price. For a teardown,
// that price is what survives it.
//
// THREE CYCLES, NOT ONE. A single boot/stop cannot distinguish a leak from the
// runtime's own warm-up: goroutines settle after the first use of a package. A
// growing floor across cycles is the signal; a raised-then-flat floor is not.
func TestProbeTeardownLeaks(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_TEARDOWN") == "" {
		t.Skip("set WA_PROBE_TEARDOWN=1 (boots and stops a session three times)")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}

	// PROCESSOS DE CHROME SÓ DESTE PERFIL. Contar todo Chrome da máquina mediria
	// o navegador do usuário — que é exatamente o erro que quase cometi ao
	// investigar carga hoje, e que só não virou dano porque conferi antes.
	// E O CONTADOR NAO PODE USAR `grep -c`: ele SAI COM 1 quando conta zero, e
	// zero e' justamente a resposta que este teste quer poder ler. A primeira
	// versao devolveu -1 depois de todo teardown e eu quase li isso como "nao
	// deu para medir" — quando era o resultado certo com o instrumento errado.
	// A mesma armadilha ja' esta' registrada neste repositorio sobre gates
	// dentro de pipe, e cometi de novo.
	chromesForProfile := func() int {
		out, err := exec.Command("bash", "-c",
			"ps aux | grep '[C]hrome' | grep -F "+strconv.Quote(profile)+" | wc -l").Output()
		if err != nil {
			return -1
		}
		n, err := strconv.Atoi(strings.TrimSpace(string(out)))
		if err != nil {
			return -1
		}
		return n
	}

	settle := func() (goroutines, chromes int) {
		// O RUNTIME PRECISA ASSENTAR ANTES DA CONTAGEM. Ler NumGoroutine logo
		// após um Stop conta as que ainda estão morrendo, e isso produz um
		// "vazamento" que some sozinho — falso positivo caro.
		for i := 0; i < 3; i++ {
			runtime.GC()
			time.Sleep(1500 * time.Millisecond)
		}
		return runtime.NumGoroutine(), chromesForProfile()
	}

	g0, c0 := settle()
	t.Logf("BASELINE: goroutines=%d chromes(perfil)=%d", g0, c0)

	type cycle struct{ g, c int }
	var seen []cycle
	for i := 1; i <= 3; i++ {
		runner := engine.NewRunner()
		h := waruntime.NewHolder(core.StartConfig{
			BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
			UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		sess, err := h.Session(ctx)
		if err != nil {
			cancel()
			_ = h.Stop(context.Background())
			t.Fatalf("cycle %d boot: %v", i, err)
		}
		gLive, cLive := runtime.NumGoroutine(), chromesForProfile()
		t.Logf("  cycle %d LIVE:  goroutines=%d chromes=%d (session %t)", i, gLive, cLive, sess != nil)
		cancel()
		via := h.Stop(context.Background())
		t.Logf("  cycle %d stop via %s", i, via)
		g, c := settle()
		t.Logf("  cycle %d AFTER: goroutines=%d chromes=%d", i, g, c)
		seen = append(seen, cycle{g, c})
	}

	// O SINAL E' O CRESCIMENTO ENTRE CICLOS, não a diferença contra a linha de
	// base: pacotes acordam na primeira utilização e deixam goroutines de
	// serviço que nunca mais crescem.
	if len(seen) < 3 {
		t.Fatal("not enough cycles to tell warm-up from growth")
	}
	t.Logf("goroutines por ciclo: %d -> %d -> %d (base %d)",
		seen[0].g, seen[1].g, seen[2].g, g0)
	t.Logf("chromes do perfil por ciclo: %d -> %d -> %d (base %d)",
		seen[0].c, seen[1].c, seen[2].c, c0)

	if seen[2].c > c0 {
		t.Errorf("ORPHAN: %d chrome process(es) of this profile survived teardown "+
			"(baseline %d)", seen[2].c, c0)
	}
	if seen[2].g > seen[1].g && seen[1].g > seen[0].g {
		t.Errorf("LEAK: goroutines grew every cycle (%d -> %d -> %d); a teardown "+
			"that returns leaves something running", seen[0].g, seen[1].g, seen[2].g)
	}
}
