package main

// Fase 6 · H1 — o que são os 4 renderers de UMA sessão.
//
// A Fase 5 mediu 12 processos, dos quais 4 renderers, para uma única sessão do
// WhatsApp, com site isolation já desligada. O spike da ADR-0006 já havia levado
// 9→6 processos com três flags (~198 MB), então contagem de processo é onde a
// memória mora — o heap de JS é 65 MB de 651 MB de anon.
//
// Esta sonda não otimiza nada: ela ATRIBUI. Sem saber o que cada renderer é, uma
// flag que remova um deles é chute, e o efeito colateral só apareceria depois.
//
// # Por que PSS e não RSS
//
// Somar RSS da árvore do Chromium conta páginas compartilhadas várias vezes — são
// 12 processos dividindo binário e bibliotecas. Para ATRIBUIR memória entre
// processos, a medida certa é PSS (`/proc/<pid>/smaps_rollup`), que reparte a
// página compartilhada proporcionalmente entre quem a mapeia. A soma de PSS da
// árvore se aproxima do que o cgroup cobra; a de RSS não.
//
// # Estágios
//
// A atribuição vem da DIFERENÇA entre estágios, não de uma foto:
//
//	boot        browser sozinho, nada navegado
//	primed      depois do primeTab (about:blank)
//	appready    depois do WhatsApp carregado
//
// Um renderer que aparece entre `primed` e `appready` é da aplicação; um que já
// existe em `boot` é do browser, e nenhuma flag de página vai removê-lo.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"chromium-study/metrics"

	"github.com/chromedp/chromedp"
)

type procInfo struct {
	PID    int    `json:"pid"`
	Type   string `json:"type"`
	PSSKiB int    `json:"pss_kib"`
	RSSKiB int    `json:"rss_kib"`
	// Marker distingue renderer de WebUI de renderer de conteúdo. Sem isto, a
	// correlação "2 renderers = 2 browser_ui" seria suposição — e ela decide se
	// remover o browser_ui economizaria alguma coisa.
	Marker string `json:"marker,omitempty"`
}

type targetInfo struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	URL      string `json:"url"`
	Attached bool   `json:"attached"`
}

type targetStage struct {
	Stage      string         `json:"stage"`
	Targets    []targetInfo   `json:"targets"`
	Procs      []procInfo     `json:"procs"`
	ByType     map[string]int `json:"procs_by_type"`
	PSSByType  map[string]int `json:"pss_kib_by_type"`
	TotalPSS   int            `json:"total_pss_kib"`
	CgroupMB   float64        `json:"cgroup_current_mb"`
	ElapsedSec float64        `json:"elapsed_sec"`
}

// sanitizeURL remove query e fragmento.
//
// Não é zelo decorativo: URL de target pode carregar identificador, e o
// relatório é artefato que sai da máquina. Tipo e origem bastam para atribuir.
func sanitizeURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "(ilegivel)"
	}
	u.RawQuery, u.Fragment = "", ""
	s := u.String()
	if len(s) > 96 {
		s = s[:96] + "…"
	}
	return s
}

// readProcs enumera os processos do Chromium com tipo e memória atribuída.
func readProcs() []procInfo {
	var out []procInfo
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return out
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile("/proc/" + e.Name() + "/cmdline")
		if err != nil || len(b) == 0 {
			continue
		}
		cmd := strings.ReplaceAll(string(b), "\x00", " ")
		if !strings.Contains(cmd, "chrom") {
			continue
		}
		p := procInfo{PID: pid, Type: "browser"}
		if i := strings.Index(cmd, "--type="); i >= 0 {
			rest := cmd[i+len("--type="):]
			if j := strings.IndexByte(rest, ' '); j >= 0 {
				rest = rest[:j]
			}
			p.Type = rest
		}
		p.PSSKiB, p.RSSKiB = readProcMem(e.Name())
		p.Marker = rendererMarker(cmd)
		out = append(out, p)
	}
	return out
}

// readProcMem prefere PSS; cai para RSS quando smaps_rollup não existe, e o
// relatório diz qual foi usado por PSS ficar zerado.
func readProcMem(pid string) (pss, rss int) {
	if b, err := os.ReadFile("/proc/" + pid + "/smaps_rollup"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "Pss:") {
				pss = parseMemKiB(line)
				break
			}
		}
	}
	if b, err := os.ReadFile("/proc/" + pid + "/status"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "VmRSS:") {
				rss = parseMemKiB(line)
				break
			}
		}
	}
	return pss, rss
}

// rendererMarker extrai só os sinais que identificam o PAPEL do renderer.
// Nunca o cmdline inteiro: ele carrega o caminho do perfil.
func rendererMarker(cmd string) string {
	var m []string
	for _, tok := range []string{"--extension-process", "--utility-sub-type=",
		"--service-sandbox-type=", "--renderer-client-id=", "--shared-files"} {
		if strings.Contains(cmd, tok) {
			m = append(m, strings.TrimSuffix(tok, "="))
		}
	}
	// O que realmente separa WebUI de conteúdo: o site alocado ao processo.
	if i := strings.Index(cmd, "--site-per-process"); i >= 0 {
		m = append(m, "site-per-process")
	}
	if i := strings.Index(cmd, "--time-ticks-at-unix-epoch"); i >= 0 {
		_ = i
	}
	return strings.Join(m, " ")
}

func parseMemKiB(line string) int {
	f := strings.Fields(line)
	if len(f) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(f[1])
	return n
}

func (a *adminConn) getTargets() ([]targetInfo, error) {
	res, err := a.c.call("", "Target.getTargets", map[string]any{})
	if err != nil {
		return nil, err
	}
	var r struct {
		TargetInfos []struct {
			TargetID string `json:"targetId"`
			Type     string `json:"type"`
			URL      string `json:"url"`
			Attached bool   `json:"attached"`
		} `json:"targetInfos"`
	}
	if err := json.Unmarshal(res, &r); err != nil {
		return nil, err
	}
	out := make([]targetInfo, 0, len(r.TargetInfos))
	for _, t := range r.TargetInfos {
		out = append(out, targetInfo{ID: t.TargetID, Type: t.Type,
			URL: sanitizeURL(t.URL), Attached: t.Attached})
	}
	return out, nil
}

func snapshotStage(name string, admin *adminConn) targetStage {
	st := targetStage{Stage: name, ByType: map[string]int{}, PSSByType: map[string]int{}}
	if tg, err := admin.getTargets(); err == nil {
		st.Targets = tg
	}
	st.Procs = readProcs()
	for _, p := range st.Procs {
		st.ByType[p.Type]++
		st.PSSByType[p.Type] += p.PSSKiB
		st.TotalPSS += p.PSSKiB
	}
	st.CgroupMB = float64(metrics.CgroupCurrent()) / 1048576
	return st
}

func printStage(st targetStage) {
	fmt.Fprintf(os.Stderr, "\n### %s — t=%.0fs · cgroup %.0f MB · PSS total %.0f MB\n",
		st.Stage, st.ElapsedSec, st.CgroupMB, float64(st.TotalPSS)/1024)
	for t, n := range st.ByType {
		fmt.Fprintf(os.Stderr, "  %-10s x%d  %.0f MB (PSS)\n", t, n, float64(st.PSSByType[t])/1024)
	}
	for _, tg := range st.Targets {
		fmt.Fprintf(os.Stderr, "  target %-16s attached=%-5v %s\n", tg.Type, tg.Attached, tg.URL)
	}
}

// CensusBootOnly para no estágio `boot`, sem navegar.
//
// Existe porque o achado do boot — dois browser_ui de omnibox popup — NÃO
// depende do WhatsApp. Varrer flags contra ele custa ~10 s por tentativa e
// nenhum pareamento, enquanto o caminho completo custa minutos e uma sessão.
var CensusBootOnly = false

// CensusExtraFlags são flags acrescentadas ao perfil canônico só nesta sonda.
var CensusExtraFlags []string

// CensusProbeTimeout e CensusSettleBudget existem para UM experimento: separar
// "renderer travado" de "renderer ocupado".
//
// Com 5 s por sondagem, 12 tentativas seguidas estouraram. Isso é compatível com
// as duas leituras — página parada e página ocupada respondem igual a um prazo
// curto. Subir só o prazo, mantendo o resto, é o que as distingue: se a resposta
// volta em 20 s, era ocupação; se não volta nem em 30 s por vários minutos, é
// outra coisa.
var CensusProbeTimeout time.Duration
var CensusSettleBudget = 90 * time.Second

// RunTargetCensus executa H1.
func RunTargetCensus(outPath string) error {
	wd := StartWatchdog("target-census", 25*time.Minute)
	defer wd.Stop()

	dir := waSessionDir()
	PersistentProfileDir = dir
	profile := waProfile()
	if len(CensusExtraFlags) > 0 {
		profile = append(append([]string{}, profile...), CensusExtraFlags...)
	}
	browsers, err := launchBrowsers(1, profile, 9950)
	PersistentProfileDir = ""
	if err != nil {
		return err
	}
	defer cleanStop(browsers[0])
	time.Sleep(2 * time.Second)

	admin, err := dialAdmin(browsers[0].WSURL)
	if err != nil {
		return fmt.Errorf("conexao administrativa: %w", err)
	}

	// Tempo por estágio: a primeira corrida estourou o orçamento e o
	// cancelamento matou a aba ANTES do estágio appready — o delta de PSS saiu
	// negativo, que foi o sinal de que a medida era inválida. Sem tempo por
	// estágio não dá para saber onde o orçamento foi.
	t0 := time.Now()
	var stages []targetStage
	add := func(name string) targetStage {
		st := snapshotStage(name, admin)
		st.ElapsedSec = time.Since(t0).Seconds()
		printStage(st)
		stages = append(stages, st)
		return st
	}

	boot := add("boot")

	if CensusBootOnly {
		fmt.Fprintf(os.Stderr, "\nboot-only: renderers=%d  PSS=%.0f MB  flags_extra=%v\n",
			boot.ByType["renderer"], float64(boot.TotalPSS)/1024, CensusExtraFlags)

		// TESTE CAUSAL: os 2 renderers do boot SÃO os browser_ui?
		//
		// Correlacionar contagem com lista de target seria suposição, e ela
		// decide se remover o browser_ui economiza alguma coisa. Fechar e
		// recontar responde direto.
		closed := 0
		for _, tg := range boot.Targets {
			if tg.Type == "browser_ui" && tg.ID != "" {
				admin.closeTarget(tg.ID)
				closed++
			}
		}
		time.Sleep(2 * time.Second)
		after := add("after_ui_close")
		fmt.Fprintf(os.Stderr,
			"\nCAUSAL: fechei %d browser_ui -> renderers %d->%d, PSS %.0f->%.0f MB\n",
			closed, boot.ByType["renderer"], after.ByType["renderer"],
			float64(boot.TotalPSS)/1024, float64(after.TotalPSS)/1024)
		return writeJSON(outPath, map[string]any{
			"experiment":        "h1-target-census-boot",
			"stages":            stages,
			"extra_flags":       CensusExtraFlags,
			"renderers":         boot.ByType["renderer"],
			"pss_kib":           boot.TotalPSS,
			"browser_ui_closed": closed,
			"cpu_stat":          metrics.CPUStat(),
			"started_utc":       time.Now().UTC().Format(time.RFC3339),
		})
	}

	alloc, cancelAlloc := chromedp.NewRemoteAllocator(wd.Ctx(), browsers[0].WSURL)
	defer cancelAlloc()
	tab, cancelTab := chromedp.NewContext(alloc)
	defer cancelTab()

	r := NewRunner()
	if err := primeTab(tab); err != nil {
		return fmt.Errorf("aba nao inicializou: %w", err)
	}
	primed := add("primed")

	if err := navigateTarget(tab, r, "census/navigate"); err != nil {
		return err
	}
	// waitAppReady FALHAR NÃO ABORTA esta sonda, e isso é o desenho, não
	// tolerância a erro.
	//
	// A pergunta do experimento é "a página volta a responder, e quando?".
	// Desistir aos 15 s responde antes de perguntar: um renderer ocupado e um
	// renderer travado falham igual num prazo curto, e só o tempo até a resposta
	// os separa. O laço de settle abaixo é a medida; o app-ready é contexto.
	appReadyErr := ""
	if err := waitAppReady(tab, r, "census/ready"); err != nil {
		appReadyErr = err.Error()
		snap := snapshot(tab, r, "census/precondition")
		fmt.Fprintf(os.Stderr, "app-ready NAO alcancado (classe=%s qr=%v): %v\n",
			snap.Class, snap.HasQR, err)
		fmt.Fprintf(os.Stderr, "seguindo mesmo assim — a resposta tardia E o dado\n")
	}
	// Estado estacionário: contar durante o sync inicial mediria o transiente.
	//
	// NÃO usar chromedp.Poll aqui, e a razão custou 25 minutos de corrida cega:
	// o WithPollingTimeout do Poll é implementado DENTRO DA PÁGINA, com um timer
	// do próprio JS. Renderer travado derrota o próprio timeout, e aí nada do
	// lado Go interrompe — a operação fica fora da DeadlinePolicy sem parecer
	// que está.
	//
	// Aqui o laço é do lado Go e cada sondagem passa pelo Runner: prazo por
	// operação, registro no OpLog, e um teto explícito de repetições. É o mesmo
	// desenho da InteractionPolicy, que nunca travou.
	if CensusProbeTimeout > 0 {
		r.Policy.StateProbe = CensusProbeTimeout
		fmt.Fprintf(os.Stderr, "probe timeout override: %s · budget %s\n",
			CensusProbeTimeout, CensusSettleBudget)
	}
	settleDeadline := time.Now().Add(CensusSettleBudget)
	settled := false
	settleStart := time.Now()
	for i := 0; time.Now().Before(settleDeadline); i++ {
		var ok bool
		err := r.Do(tab, OpStateProbe, fmt.Sprintf("census/settle%d", i),
			func(ctx context.Context) error {
				return chromedp.Run(ctx, chromedp.Evaluate(`(() => {
					const p = document.querySelector('#pane-side');
					if (!p) return false;
					return p.querySelectorAll('[role="listitem"], [role="row"]').length > 0 &&
					       !document.querySelector('progress, [role="progressbar"]');
				})()`, &ok))
			})
		el := time.Since(settleStart)
		if err != nil {
			fmt.Fprintf(os.Stderr, "settle%d t=%.0fs: %v\n", i, el.Seconds(), err)
		} else {
			fmt.Fprintf(os.Stderr, "settle%d t=%.0fs: RESPONDEU ok=%v\n", i, el.Seconds(), ok)
		}
		if ok {
			settled = true
			break
		}
		time.Sleep(2 * time.Second)
	}
	fmt.Fprintf(os.Stderr, "sync settled=%v\n", settled)
	time.Sleep(3 * time.Second)
	appready := add("appready")

	// A atribuição: o que NASCEU com a aplicação.
	delta := map[string]int{}
	for t, n := range appready.ByType {
		delta[t] = n - boot.ByType[t]
	}
	deltaPSS := float64(appready.TotalPSS-boot.TotalPSS) / 1024

	fmt.Fprintf(os.Stderr, "\n=== ATRIBUICAO ===\n")
	fmt.Fprintf(os.Stderr, "processos boot=%d primed=%d appready=%d\n",
		len(boot.Procs), len(primed.Procs), len(appready.Procs))
	for t, d := range delta {
		if d != 0 {
			fmt.Fprintf(os.Stderr, "  %-10s %+d entre boot e appready\n", t, d)
		}
	}
	fmt.Fprintf(os.Stderr, "PSS da aplicacao: %.0f MB (appready - boot)\n", deltaPSS)
	fmt.Fprintf(os.Stderr, "renderers no boot: %d  -> teto do ganho por flag de pagina\n",
		boot.ByType["renderer"])

	return writeJSON(outPath, map[string]any{
		"experiment":        "h1-target-census",
		"stages":            stages,
		"delta_by_type":     delta,
		"app_pss_mb":        deltaPSS,
		"renderers_at_boot": boot.ByType["renderer"],
		"cpu_stat":          metrics.CPUStat(),
		"memory_events":     metrics.CgroupEvents(),
		// Sem isto a sonda nao responde "onde parou" — que e exatamente o que o
		// OpLog da 4C existe para responder, e o que faltou na corrida cega.
		"watchdog":        wd.Verdict(r.Log),
		"sync_settled":    settled,
		"app_ready_error": appReadyErr,
		"probe_timeout":   r.Policy.StateProbe.String(),
		"settle_budget":   CensusSettleBudget.String(),
		"window_flag":     WAWindowSize,
		"started_utc":     time.Now().UTC().Format(time.RFC3339),
	})
}

var _ = context.Background
