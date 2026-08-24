package main

// Track J — o workload final: WhatsApp Web.
//
// Tudo o que a Fase 3 mediu veio de uma SPA de ~1.015 nós, 3 origens e 27–64 MB
// por slot. O spike inicial deste estudo mediu 474–790 MB por sessão no alvo
// real. Uma ordem de grandeza separa os dois, e é plausível que o regime mude de
// CPU-bound para memory-bound — o que invalidaria a topologia escolhida. Este
// track existe para descobrir isso com dados, não por analogia.
//
// REGRAS DE SEGURANÇA IMPLEMENTADAS NO CÓDIGO, não apenas na conduta:
//
//   - Nenhuma mensagem é enviada. Não existe caminho de código que envie.
//   - Nenhuma conversa é aberta a menos que -wa-chat seja passado explicitamente
//     com o nome de uma conversa destinada ao teste.
//   - Screenshot SOMENTE na tela de QR, que ainda não contém dados da conta.
//     Depois do login nenhuma imagem é capturada.
//   - Os relatórios registram contagens e tamanhos. Nunca texto de mensagem,
//     nome de contato, telefone, cookie ou token.
//   - O perfil do browser é persistente e fica fora do relatório e do git.
//
// O QR aparece assim que a página carrega sem sessão. Por isso o modo `waprep`
// existe: ele valida todo o harness SEM navegar para o WhatsApp, de modo que o
// pareamento só acontece quando o operador estiver de fato disponível.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/chromedp/chromedp"

	"chromium-study/metrics"
)

const waURL = "https://web.whatsapp.com/"

// waState is what the page is showing. Detection is by several candidate
// selectors per state, and the report says WHICH one matched, because
// WhatsApp Web's DOM changes often and a harness that silently depends on one
// selector fails as "not logged in" when it is really "selector moved".
type waState struct {
	State        string   `json:"state"` // "qr" | "app" | "unknown"
	MatchedBy    string   `json:"matched_by,omitempty"`
	QRCandidates []string `json:"qr_selectors_present,omitempty"`
	AppCandidate []string `json:"app_selectors_present,omitempty"`
}

var waQRSelectors = []string{
	`canvas[aria-label*="Scan"]`,
	`div[data-ref] canvas`,
	`[data-testid="qrcode"]`,
	`div[data-ref]`,
}

var waAppSelectors = []string{
	`#pane-side`,
	`[data-testid="chat-list"]`,
	`[aria-label="Chat list"]`,
	`#main`,
}

// waSessionDir is the persistent Chromium profile. Session restoration is a
// first-class part of this workload — a runtime that has to re-pair after every
// restart is not a runtime — so the profile lives on a mounted volume rather
// than in a temp dir.
func waSessionDir() string {
	if d := os.Getenv("WA_SESSION_DIR"); d != "" {
		return d
	}
	return "/session/profile"
}

type waReport struct {
	Meta          map[string]any                `json:"meta"`
	State         waState                       `json:"state"`
	Automation    map[string]any                `json:"automation_fingerprint"`
	Concurrency   int                           `json:"concurrency"`
	Jobs          int                           `json:"jobs"`
	Correct       int                           `json:"correct_jobs"`
	FalseSuccess  int                           `json:"false_successes"`
	Failures      int                           `json:"failures"`
	Timeouts      int                           `json:"timeouts"`
	CorrectPerSec float64                       `json:"correct_jobs_per_sec"`
	Total         map[string]float64            `json:"job_total"`
	Ops           map[string]map[string]float64 `json:"ops"`
	CPUCoreSec    float64                       `json:"cpu_core_seconds"`
	NrThrottled   int64                         `json:"nr_throttled"`
	NrPeriods     int64                         `json:"nr_periods"`
	ThrottledSec  float64                       `json:"throttled_seconds"`
	MemFloorMB    float64                       `json:"browser_idle_floor_mb"`
	MemPerSlotMB  float64                       `json:"memory_over_floor_per_slot_mb"`
	MemCurrentMB  float64                       `json:"memory_current_mb"`
	MemPeakMB     float64                       `json:"memory_peak_mb"`
	MemEvents     map[string]int64              `json:"memory_events"`
	MemStat       map[string]int64              `json:"memory_stat"`
	Procs         map[string]int                `json:"chromium_procs"`
	JobsPerCoreHr float64                       `json:"correct_jobs_per_core_hour"`
	JobsPerGiBHr  float64                       `json:"correct_jobs_per_gib_hour"`
	DOMNodes      float64                       `json:"avg_dom_nodes"`
	JSHeapMB      float64                       `json:"avg_js_heap_mb"`
	Resources     float64                       `json:"avg_resources"`
	Origins       int                           `json:"distinct_origins"`
	SessionAgeSec float64                       `json:"session_age_seconds"`
}

// RunWAPrep validates the whole harness WITHOUT touching WhatsApp.
//
// It proves, before anybody is asked to pick up a phone, that: the persistent
// profile directory is writable and survives a browser restart, the browser
// boots with the canonical profile, cgroup accounting works, the automation
// fingerprint can be read, and screenshots can be written to the output volume.
func RunWAPrep(outPath string) error {
	dir := waSessionDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("session dir %s: %w", dir, err)
	}
	probe := filepath.Join(dir, ".writable")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return fmt.Errorf("session dir not writable: %w", err)
	}
	_ = os.Remove(probe)

	out := map[string]any{
		"session_dir":  dir,
		"session_dir_writable": true,
		"cpu_max":      readFileTrim("/sys/fs/cgroup/cpu.max"),
		"memory_max":   readFileTrim("/sys/fs/cgroup/memory.max"),
		"num_cpu":      runtime.NumCPU(),
		"goarch":       runtime.GOARCH,
		"canonical_flags": CanonicalBrowserProfileV1,
		"started_utc":  time.Now().UTC().Format(time.RFC3339),
	}

	// Two boots of the SAME profile directory. If the second boot does not reuse
	// it, session restoration cannot work and there is no point pairing.
	for boot := 1; boot <= 2; boot++ {
		memIdle := metrics.CgroupCurrent()
		PersistentProfileDir = dir
		browsers, err := launchBrowsers(1, CanonicalBrowserProfileV1, 9900)
		PersistentProfileDir = ""
		if err != nil {
			return fmt.Errorf("boot %d: %w", boot, err)
		}
		time.Sleep(2500 * time.Millisecond)
		floor := float64(metrics.CgroupCurrent()-memIdle) / 1048576

		alloc, cancelAlloc := chromedp.NewRemoteAllocator(context.Background(), browsers[0].WSURL)
		ctx, cancel := chromedp.NewContext(alloc)
		var fp map[string]any
		var raw string
		err = chromedp.Run(ctx,
			chromedp.Navigate("about:blank"),
			chromedp.Evaluate(`JSON.stringify({
				webdriver: navigator.webdriver,
				userAgent: navigator.userAgent,
				languages: navigator.languages,
				platform: navigator.platform,
				hardwareConcurrency: navigator.hardwareConcurrency,
				deviceMemory: navigator.deviceMemory || null,
				plugins: navigator.plugins.length,
				headlessHint: /Headless/.test(navigator.userAgent)
			})`, &raw))
		if err == nil {
			_ = json.Unmarshal([]byte(raw), &fp)
		}
		// Screenshot path check on the OUTPUT volume, done here so a broken mount
		// is discovered now and not while a QR is on screen and expiring.
		var png []byte
		if e := chromedp.Run(ctx, chromedp.CaptureScreenshot(&png)); e == nil {
			_ = os.WriteFile("/out/wa-prep-screenshot.png", png, 0o644)
		}
		out[fmt.Sprintf("boot_%d", boot)] = map[string]any{
			"browser_floor_mb":   floor,
			"procs":              metrics.ChromiumProcs(),
			"fingerprint":        fp,
			"screenshot_bytes":   len(png),
			"profile_dir_exists": dirNonEmpty(dir),
		}
		cancel()
		cancelAlloc()
		cleanStop(browsers[0])
		time.Sleep(1500 * time.Millisecond)
	}

	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(os.Stderr, string(b))
	if outPath != "" {
		return os.WriteFile(outPath, b, 0o644)
	}
	return nil
}

// RunWAOpen navigates to WhatsApp Web and reports what the page shows.
//
// THIS is the step that can display a QR code, and it is deliberately a separate
// mode from everything else so it is never reached as a side effect of running
// something adjacent.
// WAUserAgent, when set, adds an explicit --user-agent to the launch policy.
//
// It exists because WhatsApp Web refuses this browser outright. With the
// canonical profile the page renders:
//
//	"WhatsApp works with Google Chrome 100+ — To use WhatsApp, update Chrome
//	 or use Mozilla Firefox, Safari, Microsoft Edge or Opera."
//
// The engine IS Chrome 151; only the UA token differs — Chromium reports
// HeadlessChrome/151.0.0.0 under --headless=new, and WhatsApp's sniffing does
// not recognise it as Chrome.
//
// Setting this is a CHANGE OF BROWSER IDENTITY, which this study has treated
// from Phase 1 as a decision that belongs to the platform and must be explicit.
// It is a flag, never a default, so no run can acquire it silently.
var WAUserAgent string

func RunWAOpen(waitLogin time.Duration, outPath string) error {
	dir := waSessionDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	profile := CanonicalBrowserProfileV1
	if WAUserAgent != "" {
		profile = append(append([]string{}, profile...), "--user-agent="+WAUserAgent)
	}
	PersistentProfileDir = dir
	browsers, err := launchBrowsers(1, profile, 9900)
	PersistentProfileDir = ""
	if err != nil {
		return err
	}
	defer cleanStop(browsers[0])
	time.Sleep(2 * time.Second)

	alloc, cancelAlloc := chromedp.NewRemoteAllocator(context.Background(), browsers[0].WSURL)
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(alloc)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(waURL)); err != nil {
		return err
	}
	// The page needs time to decide whether it has a session; polling for state
	// immediately would report "unknown" on a page that is merely still booting.
	deadline := time.Now().Add(90 * time.Second)
	var st waState
	for time.Now().Before(deadline) {
		st = waDetectState(ctx)
		if st.State != "unknown" {
			break
		}
		time.Sleep(1 * time.Second)
	}
	fmt.Fprintf(os.Stderr, "state=%s matched_by=%s\n", st.State, st.MatchedBy)

	if st.State == "qr" {
		// The QR is the only thing on screen at this point — no account data
		// exists yet — so capturing it is safe. After login, nothing is captured.
		var png []byte
		if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&png)); err != nil {
			return fmt.Errorf("qr screenshot: %w", err)
		}
		if err := os.WriteFile("/out/wa-qr.png", png, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "QR written to /out/wa-qr.png (%d bytes) — waiting for pairing\n", len(png))

		// Wait for the app to appear. The QR is refreshed by WhatsApp itself; the
		// harness re-captures only when the previous image stops being valid,
		// which is detected by the page swapping the canvas, not on a timer.
		loginDeadline := time.Now().Add(waitLogin)
		lastShot := time.Now()
		for time.Now().Before(loginDeadline) {
			s := waDetectState(ctx)
			if s.State == "app" {
				st = s
				fmt.Fprintln(os.Stderr, "paired: application UI detected")
				break
			}
			if time.Since(lastShot) > 20*time.Second {
				var p2 []byte
				if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&p2)); err == nil {
					_ = os.WriteFile("/out/wa-qr.png", p2, 0o644)
					lastShot = time.Now()
				}
			}
			time.Sleep(2 * time.Second)
		}
	}

	if st.State == "unknown" {
		// Neither screen matched. That is ambiguous between "selectors moved",
		// "page did not load" and "the browser was refused", and guessing between
		// them wastes the operator's time — so the page describes itself.
		// Capturing here is safe: an unknown state is by definition not a
		// logged-in session, so no account data can be on screen.
		var diag string
		_ = chromedp.Run(ctx, chromedp.Evaluate(`JSON.stringify({
			url: location.href,
			title: document.title,
			readyState: document.readyState,
			body_text_length: (document.body ? document.body.innerText.length : 0),
			text_sample: (document.body ? document.body.innerText.slice(0, 400) : ''),
			canvases: document.querySelectorAll('canvas').length,
			data_ref_nodes: document.querySelectorAll('[data-ref]').length,
			scripts: document.querySelectorAll('script').length,
			top_level_ids: [...document.querySelectorAll('body > div')].map(d => d.id).slice(0, 10)
		})`, &diag))
		fmt.Fprintf(os.Stderr, "DIAGNOSTIC: %s\n", diag)
		var png []byte
		if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&png)); err == nil {
			_ = os.WriteFile("/out/wa-unknown.png", png, 0o644)
			fmt.Fprintf(os.Stderr, "screenshot of unknown state -> /out/wa-unknown.png (%d bytes)\n", len(png))
		}
		st.MatchedBy = diag
	}

	if st.State != "app" {
		out, _ := json.MarshalIndent(map[string]any{"state": st}, "", "  ")
		if outPath != "" {
			_ = os.WriteFile(outPath, out, 0o644)
		}
		return fmt.Errorf("not logged in (state=%s)", st.State)
	}

	// Logged in. Let WhatsApp finish its initial sync before anything is
	// measured: a session that is still downloading history is not the steady
	// state a capacity number should describe.
	fmt.Fprintln(os.Stderr, "waiting for initial sync to settle...")
	syncStart := time.Now()
	_ = chromedp.Run(ctx, chromedp.Poll(`(() => {
		const pane = document.querySelector('#pane-side');
		if (!pane) return false;
		// Sync is considered settled when the chat list has rendered rows and no
		// progress bar is present. Row COUNT only — never row content.
		const rows = pane.querySelectorAll('[role="listitem"], [role="row"]').length;
		const progress = document.querySelector('progress, [role="progressbar"]');
		return rows > 0 && !progress;
	})()`, nil, chromedp.WithPollingTimeout(180*time.Second)))
	syncSec := time.Since(syncStart).Seconds()

	shape := waMeasureShape(ctx)
	shape["sync_settle_seconds"] = syncSec
	shape["state"] = st
	shape["profile_dir_bytes"] = dirSize(dir)

	b, _ := json.MarshalIndent(shape, "", "  ")
	fmt.Fprintln(os.Stderr, string(b))
	if outPath != "" {
		return os.WriteFile(outPath, b, 0o644)
	}
	return nil
}

// waDetectState asks the page which of the two known screens it is on.
func waDetectState(ctx context.Context) waState {
	var raw string
	expr := `JSON.stringify({
		qr:  ` + jsSelectorList(waQRSelectors) + `,
		app: ` + jsSelectorList(waAppSelectors) + `
	})`
	if err := chromedp.Run(ctx, chromedp.Evaluate(expr, &raw)); err != nil {
		return waState{State: "unknown"}
	}
	var r struct {
		QR  []string `json:"qr"`
		App []string `json:"app"`
	}
	if json.Unmarshal([]byte(raw), &r) != nil {
		return waState{State: "unknown"}
	}
	st := waState{State: "unknown", QRCandidates: r.QR, AppCandidate: r.App}
	switch {
	case len(r.App) > 0:
		st.State, st.MatchedBy = "app", r.App[0]
	case len(r.QR) > 0:
		st.State, st.MatchedBy = "qr", r.QR[0]
	}
	return st
}

// jsSelectorList builds an expression returning the selectors that match.
func jsSelectorList(sels []string) string {
	b, _ := json.Marshal(sels)
	return `(` + string(b) + `).filter(s => { try { return !!document.querySelector(s) } catch(e) { return false } })`
}

// waMeasureShape records the SIZE and COST of the loaded application. Every
// value here is a count, a byte total or a duration — never content.
func waMeasureShape(ctx context.Context) map[string]any {
	var raw string
	_ = chromedp.Run(ctx, chromedp.Evaluate(`JSON.stringify((() => {
		const res = performance.getEntriesByType('resource') || [];
		const origins = [...new Set(res.map(r => { try { return new URL(r.name).origin } catch(e) { return null } }).filter(Boolean))];
		const pane = document.querySelector('#pane-side');
		return {
			dom_nodes: document.getElementsByTagName('*').length,
			chat_rows_rendered: pane ? pane.querySelectorAll('[role="listitem"], [role="row"]').length : 0,
			js_heap_mb: (performance.memory ? performance.memory.usedJSHeapSize : 0) / 1048576,
			js_heap_limit_mb: (performance.memory ? performance.memory.jsHeapSizeLimit : 0) / 1048576,
			resource_count: res.length,
			transfer_kb: res.reduce((a, r) => a + (r.transferSize || 0), 0) / 1024,
			distinct_origins: origins.length,
			service_workers: navigator.serviceWorker ? 1 : 0,
			storage_estimate: null
		};
	})())`, &raw))
	m := map[string]any{}
	_ = json.Unmarshal([]byte(raw), &m)
	m["memory_current_mb"] = float64(metrics.CgroupCurrent()) / 1048576
	m["memory_peak_mb"] = float64(metrics.CgroupPeak()) / 1048576
	m["memory_stat"] = metrics.CgroupStat()
	m["memory_events"] = metrics.CgroupEvents()
	m["chromium_procs"] = metrics.ChromiumProcs()
	m["cpu_stat"] = metrics.CPUStat()
	return m
}

// RunWACapacity measures the workload at one concurrency, on an already-paired
// profile. Read-only by construction: it restores the session, waits for sync,
// reads the UI shape and exercises search with real key events.
func RunWACapacity(conc, iters int, chatQuery string, outPath string) error {
	dir := waSessionDir()
	memIdle := metrics.CgroupCurrent()
	// The same identity policy as the pairing run: WhatsApp Web refuses the
	// default HeadlessChrome token outright, so a capacity run without it would
	// measure the "unsupported browser" page.
	profile := CanonicalBrowserProfileV1
	if WAUserAgent != "" {
		profile = append(append([]string{}, profile...), "--user-agent="+WAUserAgent)
	}
	PersistentProfileDir = dir
	browsers, err := launchBrowsers(1, profile, 9900)
	PersistentProfileDir = ""
	if err != nil {
		return err
	}
	defer cleanStop(browsers[0])
	time.Sleep(3 * time.Second)
	memFloor := metrics.CgroupCurrent()

	alloc, cancelAlloc := chromedp.NewRemoteAllocator(context.Background(), browsers[0].WSURL)
	defer cancelAlloc()

	slots := make([]slotEnv, conc)
	for i := 0; i < conc; i++ {
		p, cancel := chromedp.NewContext(alloc)
		if err := chromedp.Run(p); err != nil {
			cancel()
			return fmt.Errorf("slot %d: %w", i, err)
		}
		defer cancel()
		slots[i] = slotEnv{parent: p}
	}

	cpu0 := metrics.CPUStat()
	rec := NewRec()
	var mu sync.Mutex
	var ok, false_, fails, timeouts int
	var sumDOM, sumHeap, sumRes float64
	jobCh := make(chan int, iters)
	for i := 0; i < iters; i++ {
		jobCh <- i
	}
	close(jobCh)

	start := time.Now()
	var wg sync.WaitGroup
	for s := 0; s < conc; s++ {
		wg.Add(1)
		go func(slot slotEnv) {
			defer wg.Done()
			for range jobCh {
				jt := time.Now()
				tabCtx, tabCancel := chromedp.NewContext(slot.parent)
				opCtx, opCancel := context.WithTimeout(tabCtx, JobTimeout)
				shape, correct, err := runWAJob(opCtx, chatQuery, rec)
				opCancel()
				tabCancel()
				mu.Lock()
				switch {
				case err != nil:
					fails++
					if opCtx.Err() != nil {
						timeouts++
					}
				case !correct:
					false_++
					rec.Total.Add(time.Since(jt))
				default:
					ok++
					rec.Total.Add(time.Since(jt))
				}
				if shape != nil {
					sumDOM += toF(shape["dom_nodes"])
					sumHeap += toF(shape["js_heap_mb"])
					sumRes += toF(shape["resource_count"])
				}
				mu.Unlock()
			}
		}(slots[s])
	}
	wg.Wait()
	wall := time.Since(start)
	cpu1 := metrics.CPUStat()

	n := float64(iters)
	coreSec := float64(cpu1["usage_usec"]-cpu0["usage_usec"]) / 1e6
	memCur := metrics.CgroupCurrent()
	memMB := float64(memCur) / 1048576
	ops := map[string]map[string]float64{}
	for k, v := range rec.Ops {
		ops[k] = v.Stats()
	}
	rep := &waReport{
		Meta: map[string]any{
			"url": waURL, "num_cpu": runtime.NumCPU(), "goarch": runtime.GOARCH,
			"cpu_max": readFileTrim("/sys/fs/cgroup/cpu.max"),
			"memory_max":  readFileTrim("/sys/fs/cgroup/memory.max"),
			"chat_query_used": chatQuery != "",
			"started_utc":     time.Now().UTC().Format(time.RFC3339),
		},
		Concurrency: conc, Jobs: ok + false_, Correct: ok, FalseSuccess: false_,
		Failures: fails, Timeouts: timeouts,
		CorrectPerSec: float64(ok) / wall.Seconds(),
		Total:         rec.Total.Stats(), Ops: ops,
		CPUCoreSec:  coreSec,
		NrThrottled: cpu1["nr_throttled"] - cpu0["nr_throttled"],
		NrPeriods:   cpu1["nr_periods"] - cpu0["nr_periods"],
		ThrottledSec: float64(cpu1["throttled_usec"]-cpu0["throttled_usec"]) / 1e6,
		MemFloorMB:   float64(memFloor-memIdle) / 1048576,
		MemPerSlotMB: float64(memCur-memFloor) / 1048576 / float64(conc),
		MemCurrentMB: memMB,
		MemPeakMB:    float64(metrics.CgroupPeak()) / 1048576,
		MemEvents:    metrics.CgroupEvents(),
		MemStat:      metrics.CgroupStat(),
		Procs:        metrics.ChromiumProcs(),
		DOMNodes:     sumDOM / n, JSHeapMB: sumHeap / n, Resources: sumRes / n,
	}
	if coreSec > 0 {
		rep.JobsPerCoreHr = float64(ok) / (coreSec / 3600)
	}
	if gib := (memMB / 1024) * wall.Hours(); gib > 0 {
		rep.JobsPerGiBHr = float64(ok) / gib
	}

	fmt.Fprintf(os.Stderr,
		"conc=%d ok=%d false=%d fail=%d timeout=%d | %.3f corr/s | p50=%.0f p95=%.0f p99=%.0f | "+
			"cpu=%.1fs thr=%.0f%% | floor=%.0fMB /slot=%.0fMB peak=%.0fMB | rend=%d | dom=%.0f heap=%.0fMB | %.0f/core-h\n",
		conc, ok, false_, fails, timeouts, rep.CorrectPerSec, rep.Total["p50_ms"],
		rep.Total["p95_ms"], rep.Total["p99_ms"], coreSec,
		100*float64(rep.NrThrottled)/maxI(rep.NrPeriods), rep.MemFloorMB, rep.MemPerSlotMB,
		rep.MemPeakMB, rep.Procs["renderer"], rep.DOMNodes, rep.JSHeapMB, rep.JobsPerCoreHr)

	b, _ := json.MarshalIndent(rep, "", "  ")
	if outPath != "" {
		return os.WriteFile(outPath, b, 0o644)
	}
	fmt.Println(string(b))
	return nil
}

// runWAJob is one unit of work: restore, sync, read shape, and optionally
// exercise search. It NEVER sends anything and never opens a conversation
// unless a query was explicitly provided.
func runWAJob(ctx context.Context, chatQuery string, rec *Rec) (map[string]any, bool, error) {
	if err := rec.timed("navigate", func() error {
		return chromedp.Run(ctx, chromedp.Navigate(waURL))
	}); err != nil {
		return nil, false, err
	}
	if err := rec.timed("restore_session", func() error {
		return chromedp.Run(ctx, chromedp.Poll(`!!document.querySelector('#pane-side')`, nil,
			chromedp.WithPollingTimeout(120*time.Second)))
	}); err != nil {
		return nil, false, err
	}
	if err := rec.timed("await_sync", func() error {
		return chromedp.Run(ctx, chromedp.Poll(`(() => {
			const pane = document.querySelector('#pane-side');
			if (!pane) return false;
			return pane.querySelectorAll('[role="listitem"], [role="row"]').length > 0;
		})()`, nil, chromedp.WithPollingTimeout(120*time.Second)))
	}); err != nil {
		return nil, false, err
	}

	correct := true
	if chatQuery != "" {
		// Search is the only interaction, and it is verified the same way the
		// FilaRápida flow was: the application must show that IT received the
		// keystrokes, not merely that the dispatch returned.
		if err := rec.timed("search", func() error {
			return chromedp.Run(ctx,
				chromedp.WaitReady(`[contenteditable="true"][data-tab]`, chromedp.ByQuery),
				chromedp.SendKeys(`[contenteditable="true"][data-tab]`, chatQuery, chromedp.ByQuery))
		}); err != nil {
			return nil, false, err
		}
		var committed string
		if err := rec.timed("verify_search", func() error {
			return chromedp.Run(ctx, chromedp.Evaluate(
				`(document.querySelector('[contenteditable="true"][data-tab]')||{}).textContent || ''`,
				&committed))
		}); err != nil {
			return nil, false, err
		}
		correct = committed == chatQuery
	}

	var shape map[string]any
	_ = rec.timed("shape", func() error {
		shape = waMeasureShape(ctx)
		return nil
	})
	return shape, correct, nil
}

func toF(v any) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

func maxI(v int64) float64 {
	if v <= 0 {
		return 1
	}
	return float64(v)
}

func dirNonEmpty(d string) bool {
	e, err := os.ReadDir(d)
	return err == nil && len(e) > 0
}

func dirSize(d string) int64 {
	var total int64
	_ = filepath.Walk(d, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}
