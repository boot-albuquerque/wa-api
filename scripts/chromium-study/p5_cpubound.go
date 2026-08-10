package main

// Fase 5 — CPU correctness boundary contra o WhatsApp real.
//
// A Fase 3 mediu, na SPA de referência, que sob starvation de CPU o controlador
// "tinha sucesso" contra páginas incompletas: 7 de 24 jobs passaram enquanto a
// contagem de nós caía de 1015 para 752. O 4C §8 registrou explicitamente que
// aquele número NÃO vale para este alvo. Este experimento mede o alvo.
//
// # O job, e por que é a busca
//
// Um clique na caixa de busca com verificação de foco é um ciclo
// Resolve→Validate→Act→Verify completo, com input real de mouse, e é
// READ-ONLY: não abre conversa, não envia nada, não altera estado da conta.
// Abrir conversa mediria a mesma coisa com efeito colateral real.
//
// # A testemunha independente
//
// A hostile suite tem `window.__truth` porque a página é nossa. Aqui não é, e
// perguntar ao controlador se ele teve sucesso mediria exatamente o defeito sob
// teste. A solução é instalar OUVINTES antes de agir:
//
//	click  (fase de captura) -> onde o clique caiu de verdade
//	focusin                  -> quem realmente recebeu foco
//
// São observadores puros: não chamam preventDefault, não param propagação, não
// alteram o comportamento do app. E registram apenas BOOLEANOS e CONTADORES —
// nunca texto, nunca conteúdo de conversa.
//
// # O critério é de dois lados
//
// Igual ao da hostile suite, e pela mesma razão que já produziu um PASS falso:
// "a policy recusou tudo sob 0,5 CPU" NÃO é ausência de falso sucesso, é
// false_failure. Um boundary medido só pela ausência de mentira daria o
// resultado degenerado de que a policy é perfeita a 0,1 CPU porque nunca age.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/chromedp"
)

// waSearchSelectors são os candidatos a caixa de busca, em ordem de
// especificidade. O alvo é de terceiro e muda sem aviso, então a lista é
// SONDADA e o que valeu fica registrado no artefato — um seletor fixo no código
// viraria "o experimento não executou" disfarçado de "a policy recusou".
var waSearchSelectors = []string{
	`[data-testid="chat-list-search"]`,
	`#side div[contenteditable="true"][data-tab]`,
	`#side div[contenteditable="true"]`,
	`div[role="textbox"][contenteditable="true"]`,
	`#side [role="textbox"]`,
	`[data-icon="search"]`,
	`[data-icon="search-alt"]`,
	`#side button[aria-label*="Pesquis"]`,
	`[aria-label*="Pesquis"]`,
	`[aria-label*="Search"]`,
}

// jsTopAtCenter diz QUEM está no ponto de clique de cada candidato.
//
// Sem isto, "hit-test falhou" é um beco: não dá para saber se o alvo está
// coberto por um overlay, se o clique cairia num filho (o que seria aceitável)
// ou se a geometria está deslocada. Registra apenas NOME DE TAG e relação
// estrutural — nunca texto, nunca conteúdo de conversa.
const jsTopAtCenter = `(() => {
  const out = [];
  for (const el of document.querySelectorAll(SELECTOR)) {
    const r = el.getBoundingClientRect();
    const cx = r.left + r.width / 2, cy = r.top + r.height / 2;
    const top = document.elementFromPoint(cx, cy);
    out.push({
      tag: el.tagName,
      w: Math.round(r.width), h: Math.round(r.height),
      left: Math.round(r.left), top: Math.round(r.top),
      cx: Math.round(cx), cy: Math.round(cy),
      vw: innerWidth, vh: innerHeight,
      center_in_viewport: cx >= 0 && cy >= 0 && cx < innerWidth && cy < innerHeight,
      top_tag: top ? top.tagName : null,
      top_is_self: top === el,
      top_is_descendant: !!top && el.contains(top),
      top_is_ancestor: !!top && top.contains(el)
    });
  }
  return JSON.stringify(out);
})()`

// jsInstallWitness instala os ouvintes e zera o registro.
//
// Roda ANTES de cada tentativa. Em fase de captura para ver o clique antes de
// qualquer handler do app, e sem tocar no evento.
const jsInstallWitness = `(() => {
  const sel = SELECTOR;
  if (!window.__p5w) {
    window.__p5w = {clicks: 0, onTarget: 0, offTarget: 0, focusedTarget: false};
    document.addEventListener('click', (e) => {
      const w = window.__p5w;
      w.clicks++;
      const t = document.querySelector(w.sel);
      if (t && (e.target === t || t.contains(e.target))) { w.onTarget++; }
      else { w.offTarget++; }
    }, true);
    document.addEventListener('focusin', (e) => {
      const w = window.__p5w;
      const t = document.querySelector(w.sel);
      if (t && (e.target === t || t.contains(e.target))) { w.focusedTarget = true; }
    }, true);
  }
  const w = window.__p5w;
  w.sel = sel; w.clicks = 0; w.onTarget = 0; w.offTarget = 0; w.focusedTarget = false;
  return 'ok';
})()`

// jsReadWitness devolve só contadores e booleanos.
const jsReadWitness = `JSON.stringify(window.__p5w || {clicks:0,onTarget:0,offTarget:0,focusedTarget:false})`

// jsReset devolve o foco ao estado inicial entre tentativas.
//
// Usa blur() direto de propósito: isto é PREPARAÇÃO, não a ação sob teste. Usar
// atalho de JS no que está sendo medido seria o defeito; usá-lo para montar a
// precondição só torna as tentativas independentes.
const jsReset = `(() => { if (document.activeElement && document.activeElement.blur) {
  document.activeElement.blur(); } return 'ok'; })()`

type witness struct {
	Clicks     int  `json:"clicks"`
	OnTarget   int  `json:"onTarget"`
	OffTarget  int  `json:"offTarget"`
	FocusedTgt bool `json:"focusedTarget"`
}

type cpuBoundAttempt struct {
	Iter       int     `json:"iteration"`
	Agent      string  `json:"agent"`
	Reported   bool    `json:"agent_reported_success"`
	Witness    witness `json:"witness"`
	Outcome    string  `json:"outcome"`
	Refusal    string  `json:"refusal,omitempty"`
	DecisionMS int64   `json:"decision_ms"`
	DOMNodes   int     `json:"dom_nodes_at_attempt"`
	ChatRows   int     `json:"chat_rows_at_attempt"`
}

// classifyCPUBound usa a TESTEMUNHA como verdade, nunca o relato do agente.
//
// Diferente da hostile suite num ponto: aqui todo caso é ExpectAct=true. A
// caixa de busca existe, é interagível e um agente correto DEVE conseguir
// focá-la. Portanto uma recusa é sempre false_failure — não há recusa correta
// neste job, e é isso que fecha a porta para a solução degenerada.
func classifyCPUBound(reported bool, w witness) InteractionOutcome {
	switch {
	case reported && w.FocusedTgt:
		return OutcomeCorrectSuccess
	case reported && w.OffTarget > 0:
		return OutcomeWrongTarget
	case reported:
		// Afirmou sucesso e o foco não foi para o alvo.
		return OutcomeFalseSuccess
	case w.FocusedTgt:
		// Recusou, mas o foco chegou: recusa que mente para o outro lado.
		return OutcomeFalseFailure
	default:
		return OutcomeFalseFailure
	}
}

func readWitness(ctx context.Context, r *Runner, label string) witness {
	var raw string
	_ = r.Do(ctx, OpStateProbe, label, func(ctx context.Context) error {
		return chromedp.Run(ctx, chromedp.Evaluate(jsReadWitness, &raw))
	})
	var w witness
	_ = json.Unmarshal([]byte(raw), &w)
	return w
}

// discoverSearchTarget sonda os candidatos e devolve o primeiro interagível.
//
// Registra o que encontrou para TODOS, não só para o vencedor: quando o alvo
// mudar, a diferença entre "o seletor sumiu" e "o seletor existe mas não é
// interagível" é o que separa uma correção de cinco minutos de uma investigação
// de uma tarde.
func discoverSearchTarget(ctx context.Context, pol *InteractionPolicy) (string, []map[string]any) {
	var report []map[string]any
	chosen := ""
	for _, sel := range waSearchSelectors {
		cs, err := pol.probe(ctx, sel, "cpubound/discover")
		entry := map[string]any{"selector": sel}
		if err != nil {
			entry["error"] = err.Error()
			report = append(report, entry)
			continue
		}
		countable := func(cs []rawCandidate) (int, int) {
			a, h := 0, 0
			for _, c := range cs {
				if c.Attached && c.Visible && c.Enabled && c.W > 0 && c.H > 0 && c.InView {
					a++
					if c.Hit {
						h++
					}
				}
			}
			return a, h
		}
		actionable, hittable := countable(cs)

		// Se há candidato único mas sem ponto de clique, ROLA e sonda de novo —
		// porque é exatamente isso que o Validate da policy faz.
		//
		// A primeira versão exigia hittable ANTES de qualquer rolagem, e por
		// isso rejeitava o alvo que a policy alcançaria: a caixa de busca do
		// WhatsApp aparece em top=-25 e só fica clicável depois da rolagem. Um
		// dublê mais ESTRITO que a produção derruba o experimento por um motivo
		// que a produção não teria — a face oposta da armadilha nº 1 do
		// ARMADILHAS.md, e igualmente enganosa.
		if len(cs) > 0 && actionable == 1 && hittable == 0 {
			if pol.scrollTargetIntoView(ctx, sel, "cpubound/discover") {
				if cs2, e := pol.probe(ctx, sel, "cpubound/rediscover"); e == nil {
					cs = cs2
					actionable, hittable = countable(cs)
					entry["scrolled_into_view"] = true
				}
			}
		}
		entry["matched"] = len(cs)
		entry["actionable"] = actionable
		entry["hittable"] = hittable
		// Só vale diagnosticar quando há algo casado e o hit-test não fecha:
		// é aí que "quem está por cima" decide se o alvo é recuperável.
		if len(cs) > 0 && hittable == 0 {
			var raw string
			if e := pol.R.Do(ctx, OpStateProbe, "cpubound/topat", func(ctx context.Context) error {
				return chromedp.Run(ctx, chromedp.Evaluate(
					jsSubstituteSelector(jsTopAtCenter, sel), &raw))
			}); e == nil {
				var tops []map[string]any
				if json.Unmarshal([]byte(raw), &tops) == nil {
					entry["top_at_center"] = tops
				}
			}
		}
		report = append(report, entry)
		// Exige EXATAMENTE um interagível E que ele passe no HIT-TEST.
		//
		// A primeira versão exigia só `actionable == 1` e escolheu
		// `[aria-label*="Pesquis"]`, um elemento com geometria real mas coberto
		// no ponto central. Resultado: a policy recusou 5/5 por hit-test em
		// ~190 ms, o chromedp direto clicou no que estava por cima e afirmou
		// sucesso 5/5, e o experimento reprovou a policy por fazer exatamente a
		// coisa certa.
		//
		// O dado do hit-test já vinha na sondagem e eu o descartava. Escolher um
		// alvo que a própria policy recusaria por desenho não mede a policy —
		// mede o meu seletor.
		if chosen == "" && actionable == 1 && hittable == 1 {
			chosen = sel
		}
	}
	return chosen, report
}

// RunCPUBoundary mede o job sob o teto de CPU do container.
func RunCPUBoundary(iters int, outPath string) error {
	wd := StartWatchdog("cpu-boundary", 15*time.Minute)
	defer wd.Stop()

	dir := waSessionDir()
	PersistentProfileDir = dir
	browsers, err := launchBrowsers(1, waProfile(), 9960)
	PersistentProfileDir = ""
	if err != nil {
		return err
	}
	defer cleanStop(browsers[0])
	time.Sleep(2 * time.Second)

	alloc, cancelAlloc := chromedp.NewRemoteAllocator(wd.Ctx(), browsers[0].WSURL)
	defer cancelAlloc()
	tab, cancelTab := chromedp.NewContext(alloc)
	defer cancelTab()

	r := NewRunner()
	pol := NewInteractionPolicy(r)

	if err := primeTab(tab); err != nil {
		return fmt.Errorf("aba nao inicializou: %w", err)
	}
	if err := navigateTarget(tab, r, "cpubound/navigate"); err != nil {
		return err
	}
	if err := waitAppReady(tab, r, "cpubound/ready"); err != nil {
		// Sem app pronto não há job. Classificar isto como "a policy recusou"
		// seria atribuir à policy uma falha de precondição.
		//
		// Mas devolver o erro CRU é o defeito que a 4C já pagou uma vez: o
		// baseline falhar com "Inspected target navigated or closed (-32000)"
		// é, na maioria das vezes, a credencial tendo se perdido e o app
		// redirecionando para o login — o que destrói o target sob inspeção. Um
		// experimento que devolve isso como erro genérico transforma o desfecho
		// MAIS importante da corrida numa mensagem de biblioteca.
		//
		// Por isso: classifica antes de desistir, e grava o artefato.
		snap := snapshot(tab, r, "cpubound/precondition")
		outcome := "PRECONDITION_UNRESPONSIVE"
		if snap.Class == classLoginRequired || snap.HasQR {
			outcome = "PRECONDITION_LOGIN_REQUIRED"
		}
		fmt.Fprintf(os.Stderr, "PRECONDICAO=%s classe=%s qr=%v perfil=%d bytes\n",
			outcome, snap.Class, snap.HasQR, dirSize(dir))
		_ = writeJSON(outPath, map[string]any{
			"experiment":    "cpu-correctness-boundary",
			"verdict":       "NOT_EXECUTED",
			"outcome":       outcome,
			"cpu_max":       readFileTrim("/sys/fs/cgroup/cpu.max"),
			"state":         snap,
			"profile_bytes": dirSize(dir),
			"error":         err.Error(),
			"started_utc":   time.Now().UTC().Format(time.RFC3339),
		})
		return fmt.Errorf("PRECONDICAO %s: %w", outcome, err)
	}

	// Estado estacionário antes de medir: uma sessão ainda sincronizando não é
	// o regime que um número de capacidade deve descrever.
	_ = chromedp.Run(tab, chromedp.Poll(`(() => {
		const pane = document.querySelector('#pane-side');
		if (!pane) return false;
		const rows = pane.querySelectorAll('[role="listitem"], [role="row"]').length;
		return rows > 0 && !document.querySelector('progress, [role="progressbar"]');
	})()`, nil, chromedp.WithPollingTimeout(180*time.Second),
		chromedp.WithPollingInterval(500*time.Millisecond)))

	// A geometria de `#side` assenta, ou já nasce errada?
	//
	// Distinguir isso decide o fix: se converge, a causa é espera insuficiente
	// (requisito nº 5 da 4C — app-ready precisa de folga) e basta esperar; se
	// não converge, é layout e nenhuma espera resolve. Medir é mais barato que
	// discutir, e sem esta amostragem eu escolheria uma das duas por intuição.
	for i := 0; i < 4; i++ {
		var g string
		_ = r.Do(tab, OpStateProbe, fmt.Sprintf("cpubound/settle%d", i), func(ctx context.Context) error {
			return chromedp.Run(ctx, chromedp.Evaluate(`JSON.stringify((() => {
				const s = document.querySelector('#side');
				if (!s) return null;
				const r = s.getBoundingClientRect();
				let t = 'none', p = s;
				while (p && p !== document.documentElement) {
					const tr = getComputedStyle(p).transform;
					if (tr && tr !== 'none') { t = p.tagName + ':' + tr; break; }
					p = p.parentElement;
				}
				return {x: Math.round(r.left), y: Math.round(r.top),
				        w: Math.round(r.width), h: Math.round(r.height),
				        vw: innerWidth, vh: innerHeight, ancestor_transform: t};
			})())`, &g))
		})
		fmt.Fprintf(os.Stderr, "side_settle[%d] %s\n", i, g)
		time.Sleep(2 * time.Second)
	}

	sel, discovery := discoverSearchTarget(tab, pol)
	discJSON, _ := json.Marshal(discovery)
	fmt.Fprintf(os.Stderr, "discovery: %s\n", discJSON)
	if sel == "" {
		// Enumera o que É clicável na barra lateral, em vez de me fazer chutar
		// mais um seletor. Reporta apenas ESTRUTURA — tag, role, data-icon,
		// geometria, hit-test — e nunca texto: aria-label e conteúdo de nó
		// podem carregar nome de contato.
		var raw string
		_ = r.Do(tab, OpStateProbe, "cpubound/enumerate", func(ctx context.Context) error {
			return chromedp.Run(ctx, chromedp.Evaluate(`JSON.stringify((() => {
				const side = document.querySelector('#side') || document.body;
				const out = [];
				for (const el of side.querySelectorAll('button,[role="button"],input,[role="textbox"],[data-icon]')) {
					const r = el.getBoundingClientRect();
					if (r.width <= 0 || r.height <= 0) continue;
					const x = Math.max(r.left,0), y = Math.max(r.top,0);
					const x1 = Math.min(r.right,innerWidth), y1 = Math.min(r.bottom,innerHeight);
					if (!(x1 > x && y1 > y)) continue;
					const top = document.elementFromPoint((x+x1)/2, (y+y1)/2);
					out.push({
						tag: el.tagName,
						role: el.getAttribute('role') || null,
						icon: el.getAttribute('data-icon') || null,
						testid: el.getAttribute('data-testid') || null,
						has_aria_label: !!el.getAttribute('aria-label'),
						w: Math.round(r.width), h: Math.round(r.height),
						x: Math.round(r.left), y: Math.round(r.top),
						hit: !!top && (top === el || el.contains(top))
					});
				}
				return out.slice(0, 40);
			})())`, &raw))
		})
		fmt.Fprintf(os.Stderr, "clicaveis_no_side: %s\n", raw)

		// Geometria de layout. Quando TODO elemento da barra lateral tem x
		// negativo, a hipótese "o seletor está errado" já não explica nada — o
		// que explica é o viewport. Só números; nenhuma captura de tela, porque
		// a tela pós-login carrega nome de contato.
		var geo string
		_ = r.Do(tab, OpStateProbe, "cpubound/layout", func(ctx context.Context) error {
			return chromedp.Run(ctx, chromedp.Evaluate(`JSON.stringify((() => {
				const de = document.documentElement, side = document.querySelector('#side');
				const sr = side ? side.getBoundingClientRect() : null;
				const app = document.querySelector('#app') || document.body;
				const ar = app.getBoundingClientRect();
				return {
					inner: [innerWidth, innerHeight],
					outer: [outerWidth, outerHeight],
					dpr: devicePixelRatio,
					doc_scroll: [de.scrollLeft, de.scrollTop],
					doc_size: [de.scrollWidth, de.scrollHeight],
					body_scroll: [document.body.scrollLeft, document.body.scrollTop],
					visual: (window.visualViewport ? {
						w: visualViewport.width, h: visualViewport.height,
						ox: visualViewport.offsetLeft, oy: visualViewport.offsetTop,
						scale: visualViewport.scale
					} : null),
					side_rect: sr ? {x: Math.round(sr.left), y: Math.round(sr.top),
					                 w: Math.round(sr.width), h: Math.round(sr.height)} : null,
					app_rect: {x: Math.round(ar.left), y: Math.round(ar.top),
					           w: Math.round(ar.width), h: Math.round(ar.height)},
					html_zoom: getComputedStyle(de).zoom,
					html_transform: getComputedStyle(de).transform,
					app_transform: getComputedStyle(app).transform
				};
			})())`, &geo))
		})
		fmt.Fprintf(os.Stderr, "layout: %s\n", geo)
		// Falhar alto. Um experimento que segue sem alvo produz uma tabela de
		// recusas e a leitura errada de que a policy quebrou sob CPU.
		return fmt.Errorf("nenhum seletor de busca interagivel: %s", discJSON)
	}
	fmt.Fprintf(os.Stderr, "alvo escolhido: %s\n", sel)

	shape := waMeasureShape(tab)
	var attempts []cpuBoundAttempt

	for i := 1; i <= iters && !wd.Expired(); i++ {
		for _, agent := range []string{"chromedp-direto", "interaction-policy"} {
			_ = r.Do(tab, OpStateProbe, "cpubound/reset", func(ctx context.Context) error {
				return chromedp.Run(ctx, chromedp.Evaluate(jsReset, nil))
			})
			time.Sleep(300 * time.Millisecond)

			var ok string
			if err := r.Do(tab, OpStateProbe, "cpubound/witness", func(ctx context.Context) error {
				return chromedp.Run(ctx, chromedp.Evaluate(
					jsSubstituteSelector(jsInstallWitness, sel), &ok))
			}); err != nil {
				continue
			}

			var actErr error
			t0 := time.Now()
			if agent == "chromedp-direto" {
				actErr = r.Do(tab, OpAction, "cpubound/click", func(ctx context.Context) error {
					return chromedp.Run(ctx, chromedp.Click(sel, chromedp.ByQuery))
				})
			} else {
				actErr = pol.Click(tab, sel,
					`(() => { const t = document.querySelector(`+jsQuote(sel)+`);
					   return !!t && (document.activeElement === t ||
					                  t.contains(document.activeElement)); })()`,
					"cpubound")
			}
			a := cpuBoundAttempt{
				Iter:       i,
				Agent:      agent,
				Reported:   actErr == nil,
				DecisionMS: time.Since(t0).Milliseconds(),
			}
			if actErr != nil {
				a.Refusal = actErr.Error()
			}
			a.Witness = readWitness(tab, r, "cpubound/readwitness")
			a.Outcome = string(classifyCPUBound(a.Reported, a.Witness))

			// Completude do DOM NA TENTATIVA: é a variável que a Fase 3
			// identificou como o que degrada sob starvation, e medi-la só no
			// início esconderia justamente a degradação progressiva.
			sh := waMeasureShape(tab)
			a.DOMNodes = asInt(sh["dom_nodes"])
			a.ChatRows = asInt(sh["chat_rows_rendered"])

			attempts = append(attempts, a)
			fmt.Fprintf(os.Stderr, "iter=%d %-19s -> %-16s focused=%v off=%d dom=%d rows=%d %dms\n",
				i, agent, a.Outcome, a.Witness.FocusedTgt, a.Witness.OffTarget,
				a.DOMNodes, a.ChatRows, a.DecisionMS)
		}
	}

	count := func(agent string, o InteractionOutcome) int {
		n := 0
		for _, a := range attempts {
			if a.Agent == agent && a.Outcome == string(o) {
				n++
			}
		}
		return n
	}
	summary := map[string]any{}
	verdict := "PASS"
	for _, agent := range []string{"chromedp-direto", "interaction-policy"} {
		fs, wt := count(agent, OutcomeFalseSuccess), count(agent, OutcomeWrongTarget)
		ff, cs := count(agent, OutcomeFalseFailure), count(agent, OutcomeCorrectSuccess)
		summary[agent] = map[string]int{
			"correct_success": cs, "false_success": fs,
			"wrong_target": wt, "false_failure": ff,
		}
		if agent == "interaction-policy" && (fs > 0 || wt > 0 || ff > 0) {
			verdict = "FAIL"
		}
		fmt.Fprintf(os.Stderr, "\n%s: correct_success=%d false_success=%d wrong_target=%d false_failure=%d\n",
			agent, cs, fs, wt, ff)
	}
	fmt.Fprintf(os.Stderr, "\nCPU BOUNDARY (cpu.max=%s): %s\n",
		readFileTrim("/sys/fs/cgroup/cpu.max"), verdict)

	return writeJSON(outPath, map[string]any{
		"experiment":    "cpu-correctness-boundary",
		"verdict":       verdict,
		"cpu_max":       readFileTrim("/sys/fs/cgroup/cpu.max"),
		"selector_used": sel,
		"discovery":     discovery,
		"settle_budget": pol.SettleBudget.String(),
		"shape_initial": shape,
		"summary":       summary,
		"attempts":      attempts,
		"watchdog":      wd.Verdict(r.Log),
		"started_utc":   time.Now().UTC().Format(time.RFC3339),
	})
}

func jsQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func asInt(v any) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 0
}
