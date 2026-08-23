package waheadless

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/chats"
	"wa-api/internal/wa-headless/capabilities/message"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeCPUUnderLoad measures what the module COSTS in CPU, idle and busy.
//
// THERE IS A REAL SUSPICION BEHIND IT, not just an item to tick. Every capability
// here uses park-and-poll: the page parks an answer and Go polls for it, because
// Evaluate does not await promises (invariant 6). Polling is a loop, and many
// concurrent calls are many loops — if the cost per call grew with concurrency,
// this is where it would show.
//
// CPU IS SAMPLED AS A DELTA OF CUMULATIVE TIME, not as `%cpu`. The %cpu column is
// an average since the process STARTED, so on a session that has been up for
// minutes it would flatten exactly the burst this test wants to see.
func TestProbeCPUUnderLoad(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_CPU") == "" {
		t.Skip("set WA_PROBE_CPU=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	eval := sess.Tab().Evaluate
	pid, err := h.BrowserPID()
	if err != nil {
		t.Fatalf("browser pid: %v", err)
	}

	// SEGUNDOS DE CPU ACUMULADOS PELA ARVORE. `ps -o time=` devolve
	// [dd-]hh:mm:ss, e somar isso da arvore inteira e' o unico jeito de nao
	// perder o que os renderers gastam.
	cpuSeconds := func() float64 {
		out, err := exec.Command("bash", "-c",
			"ps -o time=,ppid=,pid= -ax | awk -v p="+strconv.Itoa(pid)+" '$2==p || $3==p {print $1}'").Output()
		if err != nil {
			return -1
		}
		total := 0.0
		for _, line := range strings.Fields(string(out)) {
			parts := strings.Split(strings.TrimSpace(line), ":")
			mult := []float64{1}
			for len(mult) < len(parts) {
				mult = append([]float64{mult[0] * 60}, mult...)
			}
			for i, p := range parts {
				v, err := strconv.ParseFloat(p, 64)
				if err != nil {
					continue
				}
				total += v * mult[i]
			}
		}
		return total
	}

	measure := func(what string, d time.Duration, work func()) float64 {
		c0 := cpuSeconds()
		t0 := time.Now()
		work()
		for time.Since(t0) < d {
			time.Sleep(200 * time.Millisecond)
		}
		spent := cpuSeconds() - c0
		wall := time.Since(t0).Seconds()
		t.Logf("%-8s  %.2fs de CPU em %.1fs de relogio  (%.0f%% de um nucleo)",
			what, spent, wall, spent/wall*100)
		return spent / wall
	}

	var msgID string
	_ = eval(ctx, `(() => { try { for (const m of window.require("WAWebCollections").Msg.getModelsArray()) { if (m.id && m.id.id) { return m.id.id; } } return ""; } catch (e) { return ""; } })()`, &msgID)
	if msgID == "" {
		t.Skip("no loaded message to read")
	}

	// OCIOSO PRIMEIRO, e por 30s: a linha de base tem de ser do mesmo tamanho
	// que a medicao, senao compara janelas diferentes.
	idle := measure("ocioso", 30*time.Second, func() {})

	r := message.New(runner, eval)
	c := chats.New(runner, eval)
	busy := measure("carga", 30*time.Second, func() {
		var wg sync.WaitGroup
		for w := 0; w < 12; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				deadline := time.Now().Add(28 * time.Second)
				for i := 0; time.Now().Before(deadline); i++ {
					if (w+i)%2 == 0 {
						_, _ = r.OriginOf(ctx, msgID, "probe/cpu/origin")
					} else {
						_, _ = c.List(ctx, 20, "probe/cpu/list")
					}
				}
			}(w)
		}
		wg.Wait()
	})

	t.Logf("razao carga/ocioso: %.1fx", busy/max(idle, 0.001))

	// O QUE IMPORTA E' O OCIOSO SER BARATO. Uma sessao parada que queima CPU e'
	// o defeito que escala com o numero de sessoes na maquina — e' ele que
	// decide quantas cabem, nao o pico sob carga.
	if idle > 0.25 {
		t.Errorf("uma sessao OCIOSA consome %.0f%% de um nucleo; isso multiplica "+
			"por sessao e decide quantas cabem numa maquina", idle*100)
	}
}
