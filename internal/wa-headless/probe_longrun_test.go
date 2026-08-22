package waheadless

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/chats"
	"wa-api/internal/wa-headless/capabilities/message"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeLongRunning holds ONE session for several minutes with steady work and
// samples what grows.
//
// The teardown baseline (H174) says a session that ends leaves nothing behind.
// That is a different question from whether a session that STAYS grows while it
// stays, and the enunciado of Fase 2 asks both.
//
// WHAT IT WATCHES: the browser's resident memory, this process's goroutines, the
// page's own globals, and the latency of the same read repeated. A leak shows as
// a trend, not as a number, so it samples over time and reports the series — the
// assertion is on GROWTH between the first and last thirds, not on any absolute.
func TestProbeLongRunning(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_LONGRUN") == "" {
		t.Skip("set WA_PROBE_LONGRUN=1 (holds a session for several minutes)")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}
	minutes := 8
	if v := os.Getenv("WA_LONGRUN_MINUTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			minutes = n
		}
	}

	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(minutes+6)*time.Minute)
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

	// RSS DE TODA A ARVORE, nao so' do processo pai: o Chrome e' multiprocesso e
	// medir so' o pai reportaria estabilidade enquanto um renderer cresce.
	rssKB := func() int {
		out, err := exec.Command("bash", "-c",
			"ps -o rss=,ppid=,pid= -ax | awk -v p="+strconv.Itoa(pid)+" '$2==p || $3==p {s+=$1} END {print s+0}'").Output()
		if err != nil {
			return -1
		}
		n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
		return n
	}
	globals := func() int {
		var raw string
		const s = `(() => { let n = 0; for (const k of Object.keys(window)) { if (k.indexOf("__waHeadless") === 0) { n++; } } return String(n); })()`
		if err := eval(ctx, s, &raw); err != nil {
			return -1
		}
		n, _ := strconv.Atoi(raw)
		return n
	}

	var msgID string
	_ = eval(ctx, `(() => { try { for (const m of window.require("WAWebCollections").Msg.getModelsArray()) { if (m.id && m.id.id) { return m.id.id; } } return ""; } catch (e) { return ""; } })()`, &msgID)

	r := message.New(runner, eval)
	c := chats.New(runner, eval)
	type sample struct {
		at      time.Duration
		rss     int
		gor     int
		globals int
		lat     time.Duration
	}
	var series []sample
	start := time.Now()
	deadline := start.Add(time.Duration(minutes) * time.Minute)

	for time.Now().Before(deadline) {
		t0 := time.Now()
		if msgID != "" {
			_, _ = r.OriginOf(ctx, msgID, "probe/longrun/origin")
		}
		_, _ = c.List(ctx, 20, "probe/longrun/list")
		lat := time.Since(t0)
		series = append(series, sample{time.Since(start), rssKB(), runtime.NumGoroutine(), globals(), lat})
		time.Sleep(10 * time.Second)
	}

	if len(series) < 6 {
		t.Fatalf("only %d samples; not enough to tell a trend from noise", len(series))
	}
	for _, s := range series {
		t.Logf("  t=%-6s rss=%7dKB goroutines=%3d globais=%d lat=%s",
			s.at.Round(time.Second), s.rss, s.gor, s.globals, s.lat.Round(time.Millisecond))
	}

	// A COMPARACAO E' ENTRE TERCOS, nao entre extremos: o primeiro minuto ainda
	// tem aquecimento e uma unica amostra final pode ser ruido.
	third := len(series) / 3
	avg := func(xs []sample, f func(sample) int) int {
		sum := 0
		for _, x := range xs {
			sum += f(x)
		}
		return sum / len(xs)
	}
	firstRSS := avg(series[:third], func(s sample) int { return s.rss })
	lastRSS := avg(series[len(series)-third:], func(s sample) int { return s.rss })
	firstG := avg(series[:third], func(s sample) int { return s.gor })
	lastG := avg(series[len(series)-third:], func(s sample) int { return s.gor })
	t.Logf("RSS medio: %dKB -> %dKB (%+d%%)", firstRSS, lastRSS,
		(lastRSS-firstRSS)*100/max(firstRSS, 1))
	t.Logf("goroutines medias: %d -> %d", firstG, lastG)

	if lastG > firstG+4 {
		t.Errorf("goroutines cresceram de %d para %d ao longo de %d minutos", firstG, lastG, minutes)
	}
	if firstRSS > 0 && lastRSS > firstRSS*3/2 {
		t.Errorf("a memoria residente do navegador cresceu mais de 50%% (%dKB -> %dKB) "+
			"em %d minutos de trabalho constante", firstRSS, lastRSS, minutes)
	}
	if last := series[len(series)-1]; last.globals > 0 {
		t.Errorf("a pagina terminou com %d global(is) __waHeadless*", last.globals)
	}
}
