package headless

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"wa-api/internal/headless/engine"
)

// TestProbeBundleScan responde perguntas sobre o BUILD sem exigir sessão.
//
// # Por que existe, e o que o distingue do TestProbeModuleRegistry
//
// O probe do registro exige `APP_READY`, e a exigência é correta: aquele
// caminho é de RESTAURAÇÃO, e tratar página não pronta como sessão utilizável
// seria a confusão que o ADR-0005 D6 proíbe — posse e prontidão são duas
// perguntas.
//
// Mas há perguntas que não são sobre a sessão. "Este build contém um módulo
// chamado X?" é sobre os BUNDLES que a página baixou, e a página de emparelhar
// baixa os mesmos. Medido em 2026-08-23: o probe do registro recusou perfil não
// pareado (`PAIRING_LOADING`) e recusou o perfil de laboratório expirado
// (`OTHER`), então a pergunta de build não chegava a ser feita por falta de uma
// sessão que ela não precisa.
//
// # O que este probe NÃO faz, por desenho (decisão 93b)
//
// Ele LÊ texto de bundle e devolve NOMES e contagens. Não chama `window.require`
// de nada, não toca em conta, não escreve. Afrouxar uma guarda para medir é como
// um instrumento passa a inventar o próprio resultado, então o afrouxamento é
// exatamente do tamanho da pergunta: sem prontidão, e sem ação.
//
// Nenhum conteúdo, nenhuma identidade: só nomes que casam com o padrão.
func TestProbeBundleScan(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_BUNDLESCAN") == "" {
		t.Skip("set WA_PROBE_BUNDLESCAN=1")
	}
	pattern := os.Getenv("WA_PROBE_BUNDLESCAN_RE")
	if pattern == "" {
		t.Fatal("WA_PROBE_BUNDLESCAN_RE is required: a scan without a question " +
			"would print the whole inventory and answer nothing")
	}

	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		dir, _, err := observationProfileDir()
		if err != nil {
			t.Fatalf("resolving the lab profile: %v", err)
		}
		profile = dir
	}

	runner := engine.NewRunner()
	chrome := findChrome(t)
	launcher := &engine.Launcher{BinaryPath: chrome, Runner: runner}
	ctx, cancel := context.WithTimeout(context.Background(), bundleScanDeadline)
	defer cancel()

	browser, err := launcher.Launch(ctx, engine.LaunchConfig{
		ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent,
	})
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	defer engine.CleanStop(context.Background(), runner, browser)

	tab, err := engine.OpenTabWithin(ctx, browser, bundleScanPrimeBudget)
	if err != nil {
		t.Fatalf("open tab: %v", err)
	}
	defer tab.Close()

	if err := tab.Navigate(runner, realSPAURL, "probe/bundlescan/navigate"); err != nil {
		t.Fatalf("navigate: %v", err)
	}

	// A página tem de ter baixado os bundles antes de a varredura significar
	// alguma coisa. Não há relógio na página (invariante 6): quem espera é o Go,
	// e o critério é o próprio número de recursos .js parar de crescer.
	settled := waitForBundles(t, runner, tab)
	t.Logf("bundles estabilizados em %d recursos .js", settled)

	kick := strings.Replace(bundleScanScript, "PATTERN_PLACEHOLDER", strconv.Quote(pattern), 1)
	// GREP: o texto AO REDOR de um literal. Nomes de módulo dizem "o que
	// existe"; isto diz "como se chama", que é a outra metade e que nenhuma
	// lista de nomes dá. Continua a ser leitura de bundle — decisão 93b intacta.
	kick = strings.Replace(kick, "GREP_PLACEHOLDER", strconv.Quote(os.Getenv("WA_PROBE_BUNDLESCAN_GREP")), 1)
	// CARREGABILIDADE (decisão 95): window.require + Object.keys, e NADA mais.
	// Nenhum export é INVOCADO. Distingue "existe no bundle" de "existe e
	// carrega", que é a distinção que a H75 usou para fechar uma busca — e é
	// prova de carregabilidade, nunca de envio.
	kick = strings.Replace(kick, "LOADABLE_PLACEHOLDER",
		strconv.FormatBool(os.Getenv("WA_PROBE_BUNDLESCAN_LOAD") != ""), 1)
	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/bundlescan/kick", func(c context.Context) error {
		return tab.Evaluate(c, kick, &raw)
	}); err != nil {
		t.Fatalf("kick: %v", err)
	}
	t.Logf("pattern: %s", pattern)

	// Park-and-poll: Evaluate não espera promessa (invariante 6).
	deadline := time.Now().Add(bundleScanPollDeadline)
	for {
		var out string
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/bundlescan/poll", func(c context.Context) error {
			return tab.Evaluate(c, bundleScanPoll, &out)
		}); err != nil {
			t.Fatalf("poll: %v", err)
		}
		if strings.Contains(out, `"stage":"done"`) || strings.Contains(out, `"stage":"error"`) {
			t.Logf("resultado: %s", out)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("a varredura não terminou dentro do orçamento; último estado: %s", out)
		}
		time.Sleep(bundleScanPollEvery)
	}
}

const (
	bundleScanDeadline     = 4 * time.Minute
	bundleScanPrimeBudget  = 45 * time.Second
	bundleScanPollDeadline = 3 * time.Minute
	bundleScanPollEvery    = 2 * time.Second
	bundleScanSettleRounds = 3
	bundleScanSettleEvery  = 2 * time.Second
)

// waitForBundles espera o número de recursos .js parar de crescer, e devolve-o.
//
// Contar uma vez seria medir o instante em que a página começou, não o conjunto
// que ela baixou — e uma varredura sobre metade dos bundles reportaria ausência
// de módulos que existem. É o erro que este probe existe para não cometer.
func waitForBundles(t *testing.T, r *engine.Runner, tab *engine.Tab) int {
	t.Helper()
	const count = `String(performance.getEntriesByType('resource')` +
		`.filter(e => /\.js(\?|$)/.test(e.name)).length)`
	last, stable := -1, 0
	for stable < bundleScanSettleRounds {
		var out string
		if err := r.Do(context.Background(), engine.OpStateProbe, "probe/bundlescan/settle", func(c context.Context) error {
			return tab.Evaluate(c, count, &out)
		}); err != nil {
			t.Fatalf("contando bundles: %v", err)
		}
		n, err := strconv.Atoi(strings.Trim(out, `"`))
		if err != nil {
			t.Fatalf("contagem ilegível %q: %v", out, err)
		}
		if n == last && n > 0 {
			stable++
		} else {
			stable, last = 0, n
		}
		time.Sleep(bundleScanSettleEvery)
	}
	return last
}

// bundleScanScript varre o TEXTO dos bundles já baixados e devolve os nomes que
// casam com o padrão. Nunca chama window.require (decisão 93b): resolver um
// módulo é agir, e a pergunta é sobre existir.
const bundleScanScript = `(() => {
	window.__waBundleScan = { stage: 'pending' };
	const pattern = new RegExp(PATTERN_PLACEHOLDER);
	const GREP = GREP_PLACEHOLDER;
	const LOAD = LOADABLE_PLACEHOLDER;
	const hits = [];
	const loaded = [];
	(async () => {
		try {
			const urls = performance.getEntriesByType('resource')
				.map(e => e.name).filter(n => /\.js(\?|$)/.test(n));
			const seen = new Set();
			let bytes = 0, fetched = 0, failed = 0;
			for (const u of urls) {
				try {
					const r = await fetch(u);
					if (!r.ok) { failed++; continue; }
					const txt = await r.text();
					bytes += txt.length; fetched++;
					const m = txt.match(/\bWA[A-Z][A-Za-z0-9_]{3,60}\b/g);
					if (m) { for (const n of m) seen.add(n); }
					if (GREP) {
						let at = -1;
						while ((at = txt.indexOf(GREP, at + 1)) !== -1 && hits.length < 6) {
							hits.push(txt.slice(Math.max(0, at - 320), at + 320));
						}
					}
				} catch (e) { failed++; }
			}
			const all = Array.from(seen).sort();
			const matched = all.filter(n => pattern.test(n));
			if (LOAD) {
				for (const n of matched.slice(0, 12)) {
					let keys = null, why = null, kind = null;
					try {
						const mod = window.require(n);
						// Object.keys apenas. Nenhum export e' chamado.
						keys = mod ? Object.keys(mod).slice(0, 20) : [];
						// typeof e' reflexao pura: nao invoca nada, e distingue
						// "componente React" de "funcao exportada". A H75
						// descartou um modulo por ter so' displayName, que e'
						// exatamente o que Object.keys de uma FUNCAO mostra.
						kind = typeof mod;
					} catch (e) { why = String((e && e.message) || e).slice(0, 80); }
					loaded.push({ name: n, kind: kind, keys: keys, why: why });
				}
			}
			window.__waBundleScan = {
				stage: 'done', urls: urls.length, fetched: fetched, failed: failed,
				bytes: bytes, distinct: all.length,
				matched: matched.length, names: matched.slice(0, 40),
				grepHits: hits, loaded: loaded
			};
		} catch (e) {
			window.__waBundleScan = { stage: 'error', why: String((e && e.message) || e) };
		}
	})();
	return 'kicked';
})()`

const bundleScanPoll = `JSON.stringify(window.__waBundleScan || {stage:'missing'})`
