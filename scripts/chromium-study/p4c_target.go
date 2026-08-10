package main

// Fase 4C Tracks B e C — a unidade de isolamento do alvo, e se autenticação
// sobrevive a crash.
//
// Substitui p4b_tabs_recovery.go, removido: aquele arquivo travou duas vezes por
// chamadas CDP sem prazo. Tudo aqui passa pelo Runner, que aplica a
// DeadlinePolicy, e cada experimento roda sob Watchdog com orçamento próprio.
// Nenhuma função deste arquivo chama chromedp.Run diretamente.
//
// Ambos os experimentos são READ-ONLY: navegam, leem estado estrutural e matam
// o próprio browser. Não enviam nada, não abrem conversa, não registram
// conteúdo.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/chromedp"

	"chromium-study/metrics"
)

// waProfile é a launch policy do alvo: o perfil canônico mais a identidade
// explícita.
//
// A identidade existe por COMPATIBILIDADE, e §27 delimita o escopo: o alvo
// recusa o token HeadlessChrome/151 e aceita a identidade correspondente à
// versão instalada. Não há aqui nenhuma tentativa de tornar a automação
// indetectável — o UA usado é registrado em todo relatório e o resto do
// fingerprint fica como está.
// WAWindowSize sobrepõe o --window-size para os modos que tocam o WhatsApp.
//
// Existe por medida, não por preferência. Com o canônico 1280x800 o viewport
// efetivo é 1280x657 — a moldura come 143 px — e o WhatsApp renderiza `#side`
// com 715x830 em (-165,-42): a barra lateral inteira fica RECORTADA, com todo
// elemento clicável fora da área visível e hit-test falhando em todos.
//
// O efeito é grave e silencioso: o caminho ingênuo clica em coordenadas fora da
// lateral e afirma sucesso (wrong_target 5/5 medido), enquanto a policy recusa
// tudo. Ler isso como "a policy é conservadora demais" seria a conclusão errada
// a partir de uma janela mal dimensionada.
//
// Não altera `CanonicalBrowserProfileV1`: aquele perfil é a linha de base de
// todas as fases anteriores, e mudá-lo trocaria em silêncio o baseline delas.
var WAWindowSize = ""

func waProfile() []string {
	p := append([]string{}, CanonicalBrowserProfileV1...)
	if WAUserAgent != "" {
		p = append(p, "--user-agent="+WAUserAgent)
	}
	if WAWindowSize != "" {
		// Chromium usa a ÚLTIMA ocorrência da flag, então acrescentar sobrepõe
		// a do perfil canônico sem precisar removê-la — e o relatório continua
		// mostrando as duas, que é o que torna a sobreposição auditável.
		p = append(p, "--window-size="+WAWindowSize)
	}
	return p
}

// pageClass é a classificação exigida por §10.
//
// Um timeout é classe própria e NUNCA é lido como evidência de single-tab —
// §12 é explícito, e a Fase 4B errou exatamente aí.
type pageClass string

const (
	classAppReady      pageClass = "APP_READY"
	classRedirect      pageClass = "REDIRECT"
	classConflict      pageClass = "SESSION_CONFLICT"
	classErrorPage     pageClass = "ERROR_PAGE"
	classLoginRequired pageClass = "LOGIN_REQUIRED"
	classUnresponsive  pageClass = "UNRESPONSIVE"
	classOther         pageClass = "OTHER"
)

// pageSnapshot é o estado estrutural de uma aba. Só forma, nunca conteúdo:
// `text_sample` existe para reconhecer telas de erro/conflito do próprio
// WhatsApp e é truncado, e a lista de chats jamais é lida como texto.
type pageSnapshot struct {
	URL        string    `json:"url"`
	Title      string    `json:"title"`
	ReadyState string    `json:"ready_state"`
	HasPane    bool      `json:"has_pane_side"`
	HasQR      bool      `json:"has_qr"`
	DOMNodes   int       `json:"dom_nodes"`
	TextLen    int       `json:"text_length"`
	TextSample string    `json:"text_sample"`
	Class      pageClass `json:"class"`
	ProbeError string    `json:"probe_error,omitempty"`
}

// classify mapeia o snapshot para a matriz de §10.
func classify(s pageSnapshot, probeErr error) pageClass {
	if probeErr != nil {
		return classUnresponsive
	}
	if s.HasPane {
		return classAppReady
	}
	if s.HasQR {
		return classLoginRequired
	}
	low := lower(s.TextSample)
	switch {
	case contains(low, "open in another") || contains(low, "outra janela") ||
		contains(low, "another window") || contains(low, "aberto em outro") ||
		contains(low, "only be used in one") || contains(low, "outra aba"):
		return classConflict
	case contains(low, "update") && contains(low, "chrome"):
		return classErrorPage
	case s.URL != "" && !contains(lower(s.URL), "web.whatsapp.com"):
		return classRedirect
	case s.TextLen == 0:
		return classUnresponsive
	}
	return classOther
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}

func contains(h, n string) bool {
	if len(n) > len(h) {
		return false
	}
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return true
		}
	}
	return false
}

// snapshot lê o estado de uma aba sob o prazo de StateProbe.
func snapshot(ctx context.Context, r *Runner, label string) pageSnapshot {
	var raw string
	err := r.Do(ctx, OpStateProbe, label, func(ctx context.Context) error {
		return chromedp.Run(ctx, chromedp.Evaluate(`JSON.stringify((() => {
			const t = document.body ? document.body.innerText : '';
			return {
				url: location.href,
				title: document.title,
				ready_state: document.readyState,
				has_pane_side: !!document.querySelector('#pane-side'),
				has_qr: !!document.querySelector('canvas[aria-label*="Scan"]'),
				dom_nodes: document.getElementsByTagName('*').length,
				text_length: t.length,
				text_sample: t.slice(0, 200)
			};
		})())`, &raw))
	})
	var s pageSnapshot
	if err == nil {
		_ = json.Unmarshal([]byte(raw), &s)
	} else {
		s.ProbeError = err.Error()
	}
	s.Class = classify(s, err)
	return s
}

// primeTab materializa o target ANTES de qualquer operação sob prazo.
//
// Necessário por uma interação entre chromedp e a DeadlinePolicy que custou uma
// corrida inteira do Track B: no chromedp o target é criado preguiçosamente no
// primeiro Run, e as goroutines que o gerenciam derivam do contexto passado
// NESSE primeiro Run. Como Runner.Do sempre cancela o contexto filho ao
// retornar, deixar a criação acontecer dentro de um Do mata o target assim que
// a primeira operação termina — e toda operação seguinte na mesma aba falha
// instantaneamente com "context canceled", nunca com deadline_exceeded.
//
// O self-test de §7 não expôs isso porque cada caso usava aba nova e uma única
// operação; a interação só aparece a partir da segunda operação na mesma aba.
//
// Aqui o primeiro Run roda no contexto de vida da aba, sem prazo próprio — é
// criação de target local, não espera por página remota. O watchdog do
// experimento continua sendo o limite superior.
func primeTab(tab context.Context) error {
	return chromedp.Run(tab)
}

// waitAppReady espera a aplicação ficar operacional, sob o prazo de Query.
func waitAppReady(ctx context.Context, r *Runner, label string) error {
	return r.Do(ctx, OpQuery, label, func(ctx context.Context) error {
		// WithPollingInterval é obrigatório aqui, não uma afinação.
		//
		// A estratégia padrão do chromedp.Poll é requestAnimationFrame, e rAF
		// não dispara em target headless fora de primeiro plano. O predicado
		// então NUNCA é avaliado: o Poll queima o prazo inteiro sem testar a
		// condição uma vez sequer. Foi o que ocorreu na primeira corrida válida
		// do Track B — todo Query/ready estourou 15 s enquanto o StateProbe
		// seguinte encontrava #pane-side em ~22 ms, uma contradição que só se
		// resolve assim.
		//
		// É o mesmo mecanismo que a Fase 3 isolou como causa do teto de
		// concorrência do go-rod (WaitStableRAF -> Page.WaitRepaint). Duas
		// bibliotecas diferentes, o mesmo erro: presumir rAF em aba de fundo.
		//
		// O intervalo explícito troca rAF por setTimeout, que dispara mesmo
		// throttled. O timeout do Poll fica generoso de propósito — quem deve
		// vencer é o prazo da policy. E o valor 0 NÃO significa "sem limite":
		// faz o Poll errar de imediato.
		return chromedp.Run(ctx, chromedp.Poll(`!!document.querySelector('#pane-side')`, nil,
			chromedp.WithPollingInterval(100*time.Millisecond),
			chromedp.WithPollingTimeout(5*time.Minute)))
	})
}

func navigateTarget(ctx context.Context, r *Runner, label string) error {
	return r.Do(ctx, OpNavigate, label, func(ctx context.Context) error {
		return chromedp.Run(ctx, chromedp.Navigate(waURL))
	})
}

// ---------------------------------------------------------------------------
// Track B — multi-tab
// ---------------------------------------------------------------------------

type tabTrial struct {
	Rep      int          `json:"rep"`
	PageA    pageSnapshot `json:"page_a_initial"`
	PageB    pageSnapshot `json:"page_b"`
	PageAEnd pageSnapshot `json:"page_a_after_b"`
	Verdict  string       `json:"verdict"`
}

// RunTargetTabs executa §9–§13: uma aba, depois duas, com N repetições.
func RunTargetTabs(reps int, outPath string) error {
	wd := StartWatchdog("single-tab", time.Duration(reps)*2*time.Minute+time.Minute)
	defer wd.Stop()
	r := NewRunner()

	var trials []tabTrial
	for rep := 1; rep <= reps; rep++ {
		if wd.Expired() {
			break
		}
		t, err := oneTabTrial(wd.Ctx(), r, rep)
		if err != nil {
			fmt.Fprintf(os.Stderr, "rep %d FALHOU: %v\n", rep, err)
			continue
		}
		trials = append(trials, *t)
		fmt.Fprintf(os.Stderr, "rep %d: A=%s  B=%s  A_depois=%s  -> %s\n",
			rep, t.PageA.Class, t.PageB.Class, t.PageAEnd.Class, t.Verdict)
	}

	verdict := "INCONCLUSIVE"
	if len(trials) > 0 {
		agree := trials[0].Verdict
		all := true
		for _, t := range trials {
			if t.Verdict != agree {
				all = false
			}
		}
		if all && len(trials) >= reps {
			verdict = agree
		}
	}
	fmt.Fprintf(os.Stderr, "\nMULTI-TAB: %s (%d/%d repeticoes concordantes)\n",
		verdict, len(trials), reps)

	return writeJSON(outPath, map[string]any{
		"experiment":  "whatsapp-multi-tab",
		"reps":        reps,
		"verdict":     verdict,
		"trials":      trials,
		"watchdog":    wd.Verdict(r.Log),
		"operations":  r.Log.Records(),
		"started_utc": time.Now().UTC().Format(time.RFC3339),
	})
}

func oneTabTrial(parent context.Context, r *Runner, rep int) (*tabTrial, error) {
	// Browser limpo por repetição (§13): estado de aba herdado entre repetições
	// mascararia justamente o efeito sob teste.
	PersistentProfileDir = waSessionDir()
	browsers, err := launchBrowsers(1, waProfile(), 9960)
	PersistentProfileDir = ""
	if err != nil {
		return nil, err
	}
	defer cleanStop(browsers[0])
	time.Sleep(2 * time.Second)

	alloc, cancelAlloc := chromedp.NewRemoteAllocator(parent, browsers[0].WSURL)
	defer cancelAlloc()

	tabA, cancelA := chromedp.NewContext(alloc)
	defer cancelA()
	if err := primeTab(tabA); err != nil {
		return nil, fmt.Errorf("aba A nao inicializou: %w", err)
	}
	if err := navigateTarget(tabA, r, fmt.Sprintf("rep%d/A/navigate", rep)); err != nil {
		return nil, err
	}
	_ = waitAppReady(tabA, r, fmt.Sprintf("rep%d/A/ready", rep))
	snapA := snapshot(tabA, r, fmt.Sprintf("rep%d/A/state", rep))
	if snapA.Class != classAppReady {
		return nil, fmt.Errorf("controle falhou: aba A ficou %s", snapA.Class)
	}

	tabB, cancelB := chromedp.NewContext(alloc)
	defer cancelB()
	if err := primeTab(tabB); err != nil {
		return nil, fmt.Errorf("aba B nao inicializou: %w", err)
	}
	if err := navigateTarget(tabB, r, fmt.Sprintf("rep%d/B/navigate", rep)); err != nil {
		// Navegar falhar já é um resultado; a classificação vem do snapshot.
		_ = err
	}
	_ = waitAppReady(tabB, r, fmt.Sprintf("rep%d/B/ready", rep))
	snapB := snapshot(tabB, r, fmt.Sprintf("rep%d/B/state", rep))

	// §11: A ainda vale depois que B entrou?
	snapAEnd := snapshot(tabA, r, fmt.Sprintf("rep%d/A/state-after-B", rep))

	t := &tabTrial{Rep: rep, PageA: snapA, PageB: snapB, PageAEnd: snapAEnd}
	switch {
	case snapAEnd.Class == classAppReady && snapB.Class == classAppReady:
		t.Verdict = "MULTI_TAB_SUPPORTED"
	case snapAEnd.Class == classAppReady && snapB.Class != classAppReady:
		t.Verdict = "ONE_ACTIVE_APP_PER_PROFILE"
	case snapAEnd.Class != classAppReady && snapB.Class == classAppReady:
		t.Verdict = "SESSION_MIGRATES_TO_NEWEST_TAB"
	default:
		t.Verdict = "BOTH_BLOCKED_OTHER_MECHANISM"
	}
	return t, nil
}

// ---------------------------------------------------------------------------
// Track C — browser crash recovery
// ---------------------------------------------------------------------------

type recoveryTrial struct {
	Rep            int     `json:"rep"`
	DetectSec      float64 `json:"cdp_disconnect_detected_sec"`
	LaunchSec      float64 `json:"browser_launch_sec"`
	AppReadySec    float64 `json:"time_to_app_ready_sec"`
	TotalSec       float64 `json:"total_recovery_sec"`
	QRRequired     bool    `json:"qr_required"`
	Outcome        string  `json:"outcome"`
	ProfileBytes   int64   `json:"profile_bytes_after"`
	MemPeakMB      float64 `json:"memory_peak_mb"`
	OrphanProcs    int     `json:"orphan_procs_after_kill"`
	ReclaimedLocks int     `json:"singleton_files_reclaimed"`
	Note           string  `json:"note,omitempty"`
}

// RecoveryFault seleciona a forma de parada do Track C: "sigkill" ou
// "graceful". É a variável independente da ablação de §17.
var RecoveryFault = "sigkill"

// RunTargetRecovery executa §16–§20 com N repetições.
func RunTargetRecovery(reps int, outPath string) error {
	wd := StartWatchdog("browser-recovery", time.Duration(reps)*3*time.Minute+time.Minute)
	defer wd.Stop()
	r := NewRunner()

	var trials []recoveryTrial
	for rep := 1; rep <= reps; rep++ {
		if wd.Expired() {
			break
		}
		t, err := oneRecoveryTrial(wd.Ctx(), r, rep)
		if err != nil {
			fmt.Fprintf(os.Stderr, "rep %d FALHOU: %v\n", rep, err)
			trials = append(trials, recoveryTrial{Rep: rep, Outcome: "TIMEOUT"})
			continue
		}
		trials = append(trials, *t)
		fmt.Fprintf(os.Stderr,
			"rep %d: detect=%.2fs launch=%.2fs ready=%.2fs total=%.2fs qr=%v -> %s\n",
			rep, t.DetectSec, t.LaunchSec, t.AppReadySec, t.TotalSec, t.QRRequired, t.Outcome)
	}

	// A contagem de perda de auth precisa incluir PRECONDITION_LOGIN_REQUIRED.
	//
	// Contar só RECOVERED_LOGIN_REQUIRED subnotifica o desfecho mais grave:
	// quando a credencial já se perdeu, o baseline da repetição SEGUINTE
	// também falha por falta de auth, e essas repetições sumiam da conta. Foi
	// assim que a primeira corrida imprimiu "perda de auth: 0" com três
	// repetições deslogadas.
	//
	// firstLossRep é o dado que interessa para produção: depois de QUANTOS
	// ciclos de falha a sessão morre. Uma taxa média sobre repetições
	// esconderia isso, porque as repetições não são independentes — uma vez
	// perdida, a credencial não volta.
	var ok, authLoss, firstLossRep int
	var totals []float64
	for _, t := range trials {
		switch t.Outcome {
		case "RECOVERED_AUTHENTICATED":
			ok++
			totals = append(totals, t.TotalSec)
		case "RECOVERED_LOGIN_REQUIRED", "PRECONDITION_LOGIN_REQUIRED":
			authLoss++
			if firstLossRep == 0 {
				firstLossRep = t.Rep
			}
		}
	}
	summary := map[string]any{
		"successes":                ok,
		"attempts":                 len(trials),
		"authentication_loss":      authLoss,
		"recovery_p50_sec":         median(totals),
		"recovery_p95_sec":         pct95(totals),
		"authentication_loss_rate": rate(authLoss, len(trials)),
		"first_loss_rep":           firstLossRep,
	}
	// survived_kill_cycles só existe quando houve perda E as repetições
	// anteriores realmente exerceram a falha. Se o baseline já falhava, nenhum
	// ciclo foi exercido e o número seria ficção — a primeira versão chegou a
	// imprimir -1 e depois 4 para uma corrida em que nenhum SIGKILL ocorreu.
	if firstLossRep > 0 {
		exercised := 0
		for _, t := range trials {
			if t.Rep < firstLossRep && (t.Outcome == "RECOVERED_AUTHENTICATED" || t.Outcome == "RECOVERED_LOGIN_REQUIRED") {
				exercised++
			}
		}
		summary["fault_cycles_actually_exercised"] = exercised
	}
	fmt.Fprintf(os.Stderr,
		"\nBROWSER RECOVERY: %d/%d autenticadas | perda de auth: %d (1a na rep %d) | p50=%.1fs p95=%.1fs\n",
		ok, len(trials), authLoss, firstLossRep, median(totals), pct95(totals))

	return writeJSON(outPath, map[string]any{
		"experiment":  "browser-crash-recovery",
		"fault":       RecoveryFault,
		"reps":        reps,
		"summary":     summary,
		"trials":      trials,
		"watchdog":    wd.Verdict(r.Log),
		"operations":  r.Log.Records(),
		"started_utc": time.Now().UTC().Format(time.RFC3339),
	})
}

func oneRecoveryTrial(parent context.Context, r *Runner, rep int) (*recoveryTrial, error) {
	dir := waSessionDir()
	t := &recoveryTrial{Rep: rep}

	PersistentProfileDir = dir
	browsers, err := launchBrowsers(1, waProfile(), 9970)
	PersistentProfileDir = ""
	if err != nil {
		return nil, err
	}
	time.Sleep(2 * time.Second)

	alloc, cancelAlloc := chromedp.NewRemoteAllocator(parent, browsers[0].WSURL)
	tab, cancelTab := chromedp.NewContext(alloc)
	if err := primeTab(tab); err != nil {
		cancelTab()
		cancelAlloc()
		cleanStop(browsers[0])
		return nil, fmt.Errorf("aba baseline nao inicializou: %w", err)
	}
	if err := navigateTarget(tab, r, fmt.Sprintf("rec%d/baseline/navigate", rep)); err != nil {
		cancelTab()
		cancelAlloc()
		cleanStop(browsers[0])
		return nil, err
	}
	if err := waitAppReady(tab, r, fmt.Sprintf("rec%d/baseline/ready", rep)); err != nil {
		// O baseline falhar NÃO é falha de execução por padrão. A causa mais
		// provável é a mais importante: a credencial já se perdeu numa
		// repetição anterior, e a aplicação redireciona para o login —
		// destruindo o target sob inspeção, o que o CDP reporta como
		// "Inspected target navigated or closed (-32000)".
		//
		// Na primeira corrida do Track C isso foi devolvido como erro genérico
		// e o laço o converteu em TIMEOUT, produzindo a linha "perda de auth: 0"
		// enquanto três repetições tinham perdido exatamente a auth. Um
		// classificador que engole o desfecho mais importante do experimento é
		// pior do que não ter classificador.
		//
		// Por isso: classifica antes de desistir.
		snap := snapshot(tab, r, fmt.Sprintf("rec%d/baseline/state", rep))
		cancelTab()
		cancelAlloc()
		cleanStop(browsers[0])
		if snap.Class == classLoginRequired || snap.HasQR {
			t.Outcome = "PRECONDITION_LOGIN_REQUIRED"
			t.QRRequired = true
			t.ProfileBytes = dirSize(dir)
			t.Note = "credencial ja perdida antes desta repeticao"
			return t, nil
		}
		t.Outcome = "PRECONDITION_UNRESPONSIVE"
		t.Note = "baseline nao ficou pronto, classe=" + string(snap.Class) + ": " + err.Error()
		return t, nil
	}

	// A FORMA da parada é a variável sob ablação.
	//
	// sigkill: só o processo principal morre, filhos ficam órfãos — é o caso
	//          que um restart de pod precisa sobreviver, e o caso COMUM em
	//          Kubernetes (OOM kill, evicção, grace period estourado).
	// graceful: SIGTERM e espera, que é o que um preStop bem feito produz.
	//
	// Sem rodar as duas com o mesmo N, atribuir a perda de credencial ao
	// SIGKILL é correlação: número de ciclos e tempo decorrido ficam
	// confundidos com a forma de parada.
	killAt := time.Now()
	if RecoveryFault == "graceful" {
		gracefulStop(browsers[0]) //ablation:stop-form
	} else if err := browsers[0].SIGKILL(); err != nil {
		cancelTab()
		cancelAlloc()
		return nil, err
	}

	// Detecção sob o prazo de RecoveryProbe, repetida até o orçamento do laço.
	for time.Since(killAt) < 30*time.Second {
		err := r.Do(tab, OpRecoveryProbe, fmt.Sprintf("rec%d/detect", rep),
			func(ctx context.Context) error {
				return chromedp.Run(ctx, chromedp.Evaluate(`1`, nil))
			})
		if err != nil {
			t.DetectSec = time.Since(killAt).Seconds()
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	cancelTab()
	cancelAlloc()
	time.Sleep(800 * time.Millisecond)
	t.OrphanProcs = metrics.ChromiumProcs()["total"]

	// §18: reclaim só é legítimo sob posse exclusiva. Aqui ela é trivialmente
	// verdadeira — este processo é o único dono do diretório — e o número de
	// arquivos recuperados é registrado como parte do custo.
	t.ReclaimedLocks = countSingletons(dir)

	recStart := time.Now()
	PersistentProfileDir = dir
	b2, err := launchBrowsers(1, waProfile(), 9971)
	PersistentProfileDir = ""
	if err != nil {
		t.Outcome = "PROFILE_ERROR"
		return t, nil
	}
	defer cleanStop(b2[0])
	t.LaunchSec = time.Since(recStart).Seconds()

	alloc2, cancelAlloc2 := chromedp.NewRemoteAllocator(parent, b2[0].WSURL)
	defer cancelAlloc2()
	tab2, cancelTab2 := chromedp.NewContext(alloc2)
	defer cancelTab2()
	if err := primeTab(tab2); err != nil {
		t.Outcome = "PROFILE_ERROR"
		t.Note = "aba nao inicializou apos relaunch: " + err.Error()
		return t, nil
	}

	if err := navigateTarget(tab2, r, fmt.Sprintf("rec%d/recover/navigate", rep)); err != nil {
		t.Outcome = "TIMEOUT"
		t.TotalSec = time.Since(recStart).Seconds()
		return t, nil
	}
	authErr := waitAppReady(tab2, r, fmt.Sprintf("rec%d/recover/ready", rep))
	t.AppReadySec = time.Since(recStart).Seconds() - t.LaunchSec
	t.TotalSec = time.Since(recStart).Seconds()

	snap := snapshot(tab2, r, fmt.Sprintf("rec%d/recover/state", rep))
	t.QRRequired = snap.HasQR
	t.ProfileBytes = dirSize(dir)
	t.MemPeakMB = float64(metrics.CgroupPeak()) / 1048576

	switch {
	case authErr == nil && snap.Class == classAppReady:
		t.Outcome = "RECOVERED_AUTHENTICATED"
	case snap.Class == classLoginRequired:
		t.Outcome = "RECOVERED_LOGIN_REQUIRED"
	case snap.Class == classUnresponsive:
		t.Outcome = "TIMEOUT"
	default:
		t.Outcome = "PROFILE_ERROR"
	}
	return t, nil
}

func countSingletons(dir string) int {
	n := 0
	for _, name := range []string{"SingletonLock", "SingletonCookie", "SingletonSocket"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			n++
		}
	}
	return n
}

func pct95(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	i := int(0.95*float64(len(s))+0.5) - 1
	if i < 0 {
		i = 0
	}
	if i >= len(s) {
		i = len(s) - 1
	}
	return s[i]
}

func rate(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total)
}
