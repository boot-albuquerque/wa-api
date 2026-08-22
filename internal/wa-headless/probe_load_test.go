package waheadless

import (
	"context"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/chats"
	"wa-api/internal/wa-headless/capabilities/message"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeLoad measures one session under real concurrent load, and checks the
// promise the H177 fix made but never verified under volume: that the per-call
// page globals are RELEASED.
//
// H177 proved two concurrent reads stop crossing. Two is not load. The release
// was added because a per-call key trades a crossed answer for one page global
// per read — the "conserto que quebra outra coisa" of Regra 4 — and a leak of
// that shape only shows at volume.
//
// THE COUNT OF SURVIVING GLOBALS IS THE POINT. Latency and goroutines are
// reported for the record; the assertion is that the page does not accumulate.
func TestProbeLoad(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_LOAD") == "" {
		t.Skip("set WA_PROBE_LOAD=1")
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	eval := sess.Tab().Evaluate

	globals := func(what string) int {
		var raw string
		const script = `(() => {
			try {
				let n = 0;
				for (const k of Object.keys(window)) { if (k.indexOf("__waHeadless") === 0) { n++; } }
				return String(n);
			} catch (e) { return "-1"; }
		})()`
		if err := eval(ctx, script, &raw); err != nil {
			t.Fatalf("counting globals (%s): %v", what, err)
		}
		n, _ := strconv.Atoi(raw)
		return n
	}

	// A LINHA DE BASE INCLUI OS GLOBAIS QUE VIVEM DE PROPOSITO — a assinatura do
	// messagemeta e' um deles, e conta-la como vazamento seria acusar o desenho.
	base := globals("antes")
	gBase := runtime.NumGoroutine()
	t.Logf("ANTES: globais __waHeadless*=%d goroutines=%d", base, gBase)

	var msgID string
	if err := eval(ctx, `(() => {
		try {
			for (const m of window.require("WAWebCollections").Msg.getModelsArray()) {
				if (m.id && m.id.id) { return m.id.id; }
			}
			return "";
		} catch (e) { return ""; }
	})()`, &msgID); err != nil {
		t.Fatalf("picking a message: %v", err)
	}
	if msgID == "" {
		t.Skip("no loaded message to read")
	}

	// CARGA DE VERDADE, e a primeira medicao nao era. 72 chamadas em 110ms e'
	// amostra: nao estressa nada e nao deixa um vazamento por chamada aparecer.
	// Mil chamadas fazem as duas coisas.
	const workers, each = 40, 25
	r := message.New(runner, eval)
	c := chats.New(runner, eval)
	var mu sync.Mutex
	var lat []time.Duration
	fails := 0

	var wg sync.WaitGroup
	start := time.Now()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				t0 := time.Now()
				var e error
				// MISTURA DE CAPACIDADES, nao a mesma repetida: o defeito da H177
				// era entre chamadas DIFERENTES na mesma sessao.
				switch (w + i) % 3 {
				case 0:
					_, e = r.OriginOf(ctx, msgID, "probe/load/origin")
				case 1:
					_, e = r.CurrentOf(ctx, msgID, "probe/load/current")
				default:
					_, e = c.List(ctx, 20, "probe/load/list")
				}
				mu.Lock()
				lat = append(lat, time.Since(t0))
				if e != nil {
					fails++
				}
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	elapsed := time.Since(start)

	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	p50, p95, worst := lat[len(lat)/2], lat[len(lat)*95/100], lat[len(lat)-1]
	t.Logf("%d chamadas em %s — p50=%s p95=%s pior=%s, %d erros",
		len(lat), elapsed.Round(time.Millisecond), p50.Round(time.Millisecond),
		p95.Round(time.Millisecond), worst.Round(time.Millisecond), fails)

	if fails > 0 {
		t.Errorf("%d de %d chamadas falharam sob carga", fails, len(lat))
	}

	// O PONTO: os globais por chamada foram liberados.
	after := globals("depois")
	gAfter := runtime.NumGoroutine()
	t.Logf("DEPOIS: globais=%d goroutines=%d", after, gAfter)
	if after > base {
		t.Fatalf("a pagina acumulou %d global(is) apos %d chamadas (%d -> %d); a "+
			"chave por chamada trocou resposta cruzada por vazamento",
			after-base, len(lat), base, after)
	}
}
