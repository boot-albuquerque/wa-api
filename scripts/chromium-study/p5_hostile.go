package main

// Fase 5 §16 — Hostile Interaction Suite.
//
// A InteractionPolicy é, até aqui, uma hipótese sobre como eliminar falso
// sucesso. Esta suite é o que a transforma em evidência ou a reprova.
//
// Cada cenário tem GROUND TRUTH no próprio documento: a página registra em
// window.__truth se o clique legítimo no alvo pretendido aconteceu. Sem isso a
// suite mediria a opinião do controlador sobre si mesmo, que é precisamente o
// defeito sob teste.
//
// O critério bloqueante é assimétrico de propósito: false_success = 0. Uma
// recusa correta (correct_failure) é resultado DESEJÁVEL — vale mais recusar
// uma interação duvidosa do que afirmar que ela funcionou.

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/chromedp/chromedp"
)

// Oito cenários de §16. `expectAct` diz se um agente CORRETO deveria conseguir
// agir; quando false, a resposta certa é recusar.
type hostileCase struct {
	Name      string
	Path      string
	Selector  string
	Post      string // póscondição observável
	ExpectAct bool
	Why       string
}

var hostileCases = []hostileCase{
	{"overlay-sobre-botao", "/overlay", "#target", "window.__truth.clicked === true", false,
		"um overlay transparente cobre o botao; o clique atinge o overlay"},
	{"removido-apos-query", "/removed", "#target", "window.__truth.clicked === true", false,
		"o no e removido logo apos casar com o seletor"},
	{"node-replacement", "/replace", "#target", "window.__truth.clicked === true", false,
		"o no e substituido por outro identico, no estilo React"},
	{"disabled-vira-enabled", "/enable", "#target", "window.__truth.clicked === true", true,
		"comeca disabled e habilita; o agente deve esperar, nao desistir"},
	{"selector-duplicado", "/dup", ".target", "window.__truth.clicked === true", false,
		"dois nos interagiveis casam; escolher um e arbitrario"},
	{"zero-size", "/zerosize", "#target", "window.__truth.clicked === true", false,
		"caixa de dimensao zero nao e clicavel por um humano"},
	{"fora-do-viewport", "/offscreen", "#target", "window.__truth.clicked === true", false,
		"fora da area visivel sem rolagem"},
	{"modal-animando", "/modal", "#target", "window.__truth.clicked === true", true,
		"o alvo se move; clicar antes de parar acerta a posicao errada"},
}

const hostileShell = `<!doctype html><html><body>
<script>window.__truth = {clicked:false, wrongClicks:0};</script>%s</body></html>`

func hostileHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, hostileShell, body)
	}
}

func startHostileServer() (*hangServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()

	// Em todos: só o clique no ALVO pretendido marca truth.clicked. Qualquer
	// outro caminho incrementa wrongClicks, então "acertou outra coisa" é
	// distinguível de "não fez nada".
	mux.HandleFunc("/overlay", hostileHandler(`
<button id=target style="position:absolute;top:50px;left:50px;width:120px;height:40px"
        onclick="window.__truth.clicked=true">ok</button>
<div id=cover style="position:absolute;top:0;left:0;width:400px;height:200px;background:rgba(0,0,0,0.01)"
     onclick="window.__truth.wrongClicks++"></div>`))

	mux.HandleFunc("/removed", hostileHandler(`
<button id=target style="width:120px;height:40px" onclick="window.__truth.clicked=true">ok</button>
<script>setTimeout(()=>{const e=document.getElementById('target'); if(e) e.remove();}, 150);</script>`))

	mux.HandleFunc("/replace", hostileHandler(`
<div id=host><button id=target style="width:120px;height:40px"
   onclick="window.__truth.clicked=true">ok</button></div>
<script>setInterval(()=>{
  const h=document.getElementById('host');
  h.innerHTML='<button id=target style="width:120px;height:40px" onclick="window.__truth.wrongClicks++">ok</button>';
}, 180);</script>`))

	mux.HandleFunc("/enable", hostileHandler(`
<button id=target disabled style="width:120px;height:40px"
        onclick="window.__truth.clicked=true">ok</button>
<script>setTimeout(()=>{document.getElementById('target').disabled=false;}, 800);</script>`))

	mux.HandleFunc("/dup", hostileHandler(`
<button class=target style="width:120px;height:40px" onclick="window.__truth.wrongClicks++">a</button>
<button class=target style="width:120px;height:40px" onclick="window.__truth.clicked=true">b</button>`))

	mux.HandleFunc("/zerosize", hostileHandler(`
<button id=target style="width:0;height:0;overflow:hidden;padding:0;border:0"
        onclick="window.__truth.clicked=true">ok</button>`))

	mux.HandleFunc("/offscreen", hostileHandler(`
<button id=target style="position:absolute;top:5000px;left:50px;width:120px;height:40px"
        onclick="window.__truth.clicked=true">ok</button>`))

	mux.HandleFunc("/modal", hostileHandler(`
<button id=target style="position:absolute;top:10px;left:10px;width:120px;height:40px"
        onclick="window.__truth.clicked=true">ok</button>
<script>
let x=10; const el=document.getElementById('target');
const t=setInterval(()=>{ x+=6; el.style.left=x+'px'; if(x>200){clearInterval(t);} }, 50);
</script>`))

	s := &hangServer{ln: ln, srv: &http.Server{Handler: mux}}
	go func() { _ = s.srv.Serve(ln) }()
	return s, nil
}

type hostileResult struct {
	Case      string `json:"case"`
	Agent     string `json:"agent"`
	ExpectAct bool   `json:"agent_should_act"`
	Acted     bool   `json:"agent_reported_success"`
	Truth     bool   `json:"ground_truth_clicked"`
	Wrong     int    `json:"ground_truth_wrong_clicks"`
	Outcome   string `json:"outcome"`
	Refusal   string `json:"refusal,omitempty"`
}

// classify compara o que o agente AFIRMOU com o que a página REGISTROU.
func classifyOutcome(expectAct, reported, truth bool, wrong int) InteractionOutcome {
	switch {
	case reported && truth:
		return OutcomeCorrectSuccess
	case reported && wrong > 0:
		// Afirmou sucesso e o clique caiu em outro elemento.
		return OutcomeWrongTarget
	case reported && !truth:
		// O pior caso: afirmou sucesso sem que nada tenha acontecido.
		return OutcomeFalseSuccess
	case !reported && truth:
		return OutcomeFalseFailure
	case !reported && !expectAct:
		return OutcomeCorrectFailure
	default:
		// Deveria ter conseguido agir e recusou. NÃO é correct_failure: uma
		// policy que recusa tudo satisfaz false_success=0 trivialmente, e
		// classificar isto como acerto esconde a solução degenerada.
		return OutcomeFalseFailure
	}
}

func readTruth(ctx context.Context, r *Runner, label string) (bool, int) {
	var raw string
	_ = r.Do(ctx, OpStateProbe, label, func(ctx context.Context) error {
		return chromedp.Run(ctx, chromedp.Evaluate(
			`JSON.stringify(window.__truth||{clicked:false,wrongClicks:0})`, &raw))
	})
	var t struct {
		Clicked bool `json:"clicked"`
		Wrong   int  `json:"wrongClicks"`
	}
	_ = json.Unmarshal([]byte(raw), &t)
	return t.Clicked, t.Wrong
}

// RunHostileSuite executa §16.
func RunHostileSuite(outPath string) error {
	wd := StartWatchdog("hostile-suite", 10*time.Minute)
	defer wd.Stop()

	srv, err := startHostileServer()
	if err != nil {
		return err
	}
	defer srv.Close()

	browsers, err := launchBrowsers(1, CanonicalBrowserProfileV1, 9995)
	if err != nil {
		return err
	}
	defer killAll(browsers)
	time.Sleep(2 * time.Second)

	alloc, cancelAlloc := chromedp.NewRemoteAllocator(wd.Ctx(), browsers[0].WSURL)
	defer cancelAlloc()

	r := NewRunner()
	pol := NewInteractionPolicy(r)
	var results []hostileResult

	run := func(hc hostileCase, agent string) hostileResult {
		tab, cancelTab := chromedp.NewContext(alloc)
		defer cancelTab()
		if err := primeTab(tab); err != nil {
			return hostileResult{Case: hc.Name, Agent: agent, Outcome: string(OutcomeTimeout),
				Refusal: "aba nao inicializou"}
		}
		label := agent + "/" + hc.Name
		if err := r.Do(tab, OpNavigate, label+"/nav", func(ctx context.Context) error {
			return chromedp.Run(ctx, chromedp.Navigate(srv.URL()+hc.Path))
		}); err != nil {
			return hostileResult{Case: hc.Name, Agent: agent, Outcome: string(OutcomeTimeout)}
		}

		res := hostileResult{Case: hc.Name, Agent: agent, ExpectAct: hc.ExpectAct}
		var actErr error
		if agent == "chromedp-direto" {
			// O caminho ingênuo: clica e considera o retorno sem erro como
			// sucesso. É o comportamento que a policy existe para substituir.
			actErr = r.Do(tab, OpAction, label+"/click", func(ctx context.Context) error {
				return chromedp.Run(ctx, chromedp.Click(hc.Selector, chromedp.ByQuery))
			})
		} else {
			actErr = pol.Click(tab, hc.Selector, hc.Post, label)
		}
		res.Acted = actErr == nil
		if actErr != nil {
			res.Refusal = actErr.Error()
		}
		res.Truth, res.Wrong = readTruth(tab, r, label+"/truth")
		res.Outcome = string(classifyOutcome(hc.ExpectAct, res.Acted, res.Truth, res.Wrong))
		return res
	}

	for _, hc := range hostileCases {
		for _, agent := range []string{"chromedp-direto", "interaction-policy"} {
			res := run(hc, agent)
			results = append(results, res)
			fmt.Fprintf(os.Stderr, "%-22s %-19s -> %-17s truth=%v wrong=%d\n",
				hc.Name, agent, res.Outcome, res.Truth, res.Wrong)
		}
	}

	count := func(agent string, o InteractionOutcome) int {
		n := 0
		for _, r := range results {
			if r.Agent == agent && r.Outcome == string(o) {
				n++
			}
		}
		return n
	}

	summary := map[string]any{}
	verdict := "PASS"
	for _, agent := range []string{"chromedp-direto", "interaction-policy"} {
		fs := count(agent, OutcomeFalseSuccess)
		wt := count(agent, OutcomeWrongTarget)
		summary[agent] = map[string]int{
			"correct_success": count(agent, OutcomeCorrectSuccess),
			"correct_failure": count(agent, OutcomeCorrectFailure),
			"false_success":   fs,
			"false_failure":   count(agent, OutcomeFalseFailure),
			"wrong_target":    wt,
			"timeout":         count(agent, OutcomeTimeout),
		}
		// O critério de §16 é necessário mas não suficiente. Sem exigir os
		// sucessos que os casos actionable preveem, "recusar sempre" passa.
		ff := count(agent, OutcomeFalseFailure)
		if agent == "interaction-policy" && (fs > 0 || wt > 0 || ff > 0) {
			verdict = "FAIL"
		}
		fmt.Fprintf(os.Stderr, "\n%s: false_success=%d wrong_target=%d correct_success=%d correct_failure=%d\n",
			agent, fs, wt, count(agent, OutcomeCorrectSuccess), count(agent, OutcomeCorrectFailure))
	}
	fmt.Fprintf(os.Stderr, "\nHOSTILE SUITE (criterio: false_success=0 na policy): %s\n", verdict)

	return writeJSON(outPath, map[string]any{
		"experiment":  "hostile-interaction-suite",
		"verdict":     verdict,
		"cases":       len(hostileCases),
		"summary":     summary,
		"results":     results,
		"watchdog":    wd.Verdict(r.Log),
		"started_utc": time.Now().UTC().Format(time.RFC3339),
	})
}
