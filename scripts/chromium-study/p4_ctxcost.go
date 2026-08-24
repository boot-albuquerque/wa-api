package main

// Fase 4, Track B §5/§6 — de onde vem o custo do BrowserContext.
//
// A Fase 3 mediu T3/T6 com 7–12× de CPU e 19–35% de falso sucesso e concluiu
// "não use BrowserContext por sessão". Essa conclusão não separava três coisas:
//
//	(a) o BrowserContext do Chromium;
//	(b) a implementação do chromedp, que falha neste browser;
//	(c) o caminho que a Fase 3 usou para contornar (b) — que criava um target
//	    novo POR JOB através de uma conexão de controle, enquanto o braço de
//	    comparação criava targets pelo caminho normal do chromedp.
//
// (c) sozinho pode explicar o resultado, e por isso a conclusão da Fase 3 não
// pode ser mantida como está.
//
// Este arquivo mede (a) sem (b) e sem (c): tudo por CDP cru, contando targets e
// processos antes e depois de cada contexto, com o chromedp fora do caminho.

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"chromium-study/metrics"
)

type ctxCostSample struct {
	Stage       string         `json:"stage"`
	Contexts    int            `json:"browser_contexts"`
	Targets     map[string]int `json:"targets_by_type"`
	TargetTotal int            `json:"targets_total"`
	Procs       map[string]int `json:"chromium_procs"`
	MemMB       float64        `json:"memory_current_mb"`
	CPUCoreSec  float64        `json:"cpu_core_seconds_cumulative"`
}

// RunCtxCost walks the browser from empty to N contexts, one step at a time,
// recording what each step actually costs.
func RunCtxCost(n int, outPath string) error {
	browsers, err := launchBrowsers(1, CanonicalBrowserProfileV1, 9800)
	if err != nil {
		return err
	}
	defer killAll(browsers)
	time.Sleep(3 * time.Second)

	admin, err := dialAdmin(browsers[0].WSURL)
	if err != nil {
		return err
	}
	defer admin.c.Close()

	cpu0 := metrics.CPUStat()
	out := map[string]any{
		"meta": map[string]any{
			"contexts":    n,
			"chromedp":    false,
			"note":        "raw CDP only — no chromedp connection is ever opened",
			"cpu_max":     readFileTrim("/sys/fs/cgroup/cpu.max"),
			"started_utc": time.Now().UTC().Format(time.RFC3339),
		},
	}
	var samples []ctxCostSample

	sample := func(stage string, nctx int) {
		time.Sleep(1500 * time.Millisecond) // let process creation settle
		byType, total := admin.targetsByType()
		cpu := metrics.CPUStat()
		s := ctxCostSample{
			Stage: stage, Contexts: nctx,
			Targets: byType, TargetTotal: total,
			Procs: metrics.ChromiumProcs(),
			MemMB: float64(metrics.CgroupCurrent()) / 1048576,
			CPUCoreSec: float64(cpu["usage_usec"]-cpu0["usage_usec"]) / 1e6,
		}
		samples = append(samples, s)
		fmt.Fprintf(os.Stderr, "%-28s ctx=%d targets=%d %v | procs=%d rend=%d | mem=%.0fMB cpu=%.1fs\n",
			stage, nctx, total, byType, s.Procs["total"], s.Procs["renderer"], s.MemMB, s.CPUCoreSec)
	}

	sample("0-baseline", 0)

	// Step 1: contexts only, with no page in them. If browser_ui targets appear
	// here, they belong to the context itself and not to anything driven later.
	var ctxIDs []string
	for i := 0; i < n; i++ {
		id, err := admin.createBrowserContext()
		if err != nil {
			return fmt.Errorf("createBrowserContext %d: %w", i, err)
		}
		ctxIDs = append(ctxIDs, id)
		sample(fmt.Sprintf("1-context-%d-created", i+1), i+1)
	}

	// Step 2: one page per context.
	var tids []string
	for i, id := range ctxIDs {
		tid, err := admin.createTarget(id)
		if err != nil {
			return fmt.Errorf("createTarget in ctx %d: %w", i, err)
		}
		tids = append(tids, tid)
		sample(fmt.Sprintf("2-page-in-context-%d", i+1), n)
	}

	// Step 3: the control comparison — the same number of pages, all in the
	// DEFAULT context. Any difference against step 2 is the context's cost and
	// nothing else, because the creating connection and the command are the same.
	for _, tid := range tids {
		admin.closeTarget(tid)
	}
	for _, id := range ctxIDs {
		if err := admin.disposeBrowserContext(id); err != nil {
			fmt.Fprintf(os.Stderr, "dispose %s: %v\n", id, err)
		}
	}
	sample("3-disposed-all", 0)

	var defTids []string
	for i := 0; i < n; i++ {
		tid, err := admin.createTarget("")
		if err != nil {
			return fmt.Errorf("createTarget default %d: %w", i, err)
		}
		defTids = append(defTids, tid)
		sample(fmt.Sprintf("4-page-in-default-%d", i+1), 0)
	}
	for _, tid := range defTids {
		admin.closeTarget(tid)
	}
	sample("5-closed-default-pages", 0)

	out["samples"] = samples
	b, _ := json.MarshalIndent(out, "", "  ")
	if outPath != "" {
		return os.WriteFile(outPath, b, 0o644)
	}
	fmt.Println(string(b))
	return nil
}

// targetsByType asks the browser what exists, rather than inferring it from
// events the connection may have missed.
func (a *adminConn) targetsByType() (map[string]int, int) {
	res, err := a.c.call("", "Target.getTargets", map[string]any{})
	if err != nil {
		return map[string]int{"error": 1}, 0
	}
	var r struct {
		TargetInfos []struct {
			Type             string `json:"type"`
			URL              string `json:"url"`
			BrowserContextID string `json:"browserContextId"`
		} `json:"targetInfos"`
	}
	if err := json.Unmarshal(res, &r); err != nil {
		return map[string]int{"unmarshal_error": 1}, 0
	}
	m := map[string]int{}
	for _, t := range r.TargetInfos {
		m[t.Type]++
	}
	return m, len(r.TargetInfos)
}

// RunCtxRepro is the minimal reproduction of the Fase 3 defect.
//
// Established so far: the same two CDP commands succeed on a raw websocket and
// fail through chromedp's browser connection, on the same browser, in either
// order. The remaining difference between the two connections is what chromedp
// configures on its own before doing anything else. This walks that difference
// one command at a time, on a RAW connection, so the trigger — if it is one of
// them — is named rather than guessed.
func RunCtxRepro(outPath string) error {
	browsers, err := launchBrowsers(1, CanonicalBrowserProfileV1, 9850)
	if err != nil {
		return err
	}
	defer killAll(browsers)
	time.Sleep(2 * time.Second)

	type step struct {
		Name    string `json:"name"`
		Setup   string `json:"setup_command"`
		SetupOK bool   `json:"setup_ok"`
		CtxOK   bool   `json:"create_context_ok"`
		TgtOK   bool   `json:"create_target_ok"`
		TgtErr  string `json:"create_target_error,omitempty"`
	}
	var steps []step

	// Each case gets a FRESH connection: connection state is the variable under
	// test, so reusing one would let an earlier case contaminate a later one.
	cases := []struct {
		name   string
		method string
		params map[string]any
	}{
		{"raw, no setup", "", nil},
		{"after setDiscoverTargets", "Target.setDiscoverTargets",
			map[string]any{"discover": true}},
		{"after setAutoAttach(flatten)", "Target.setAutoAttach",
			map[string]any{"autoAttach": true, "waitForDebuggerOnStart": false, "flatten": true}},
		{"after setAutoAttach(no flatten)", "Target.setAutoAttach",
			map[string]any{"autoAttach": true, "waitForDebuggerOnStart": false, "flatten": false}},
	}

	for _, c := range cases {
		a, err := dialAdmin(browsers[0].WSURL)
		if err != nil {
			return err
		}
		s := step{Name: c.name, Setup: c.method}
		if c.method != "" {
			_, err := a.c.call("", c.method, c.params)
			s.SetupOK = err == nil
			if err != nil {
				fmt.Fprintf(os.Stderr, "%-34s setup FAILED: %v\n", c.name, err)
			}
		} else {
			s.SetupOK = true
		}
		id, err := a.createBrowserContext()
		s.CtxOK = err == nil
		if err == nil {
			_, err = a.createTarget(id)
			s.TgtOK = err == nil
			if err != nil {
				s.TgtErr = err.Error()
			}
			_ = a.disposeBrowserContext(id)
		}
		steps = append(steps, s)
		fmt.Fprintf(os.Stderr, "%-34s ctx=%v target=%v %s\n", c.name, s.CtxOK, s.TgtOK, s.TgtErr)
		a.c.Close()
	}

	out := map[string]any{
		"meta": map[string]any{
			"browser":     "Chromium 151.0.7922.108 --headless=new",
			"question":    "which connection-level setup makes createTarget(browserContextId) fail",
			"started_utc": time.Now().UTC().Format(time.RFC3339),
		},
		"steps": steps,
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	if outPath != "" {
		return os.WriteFile(outPath, b, 0o644)
	}
	fmt.Println(string(b))
	return nil
}
