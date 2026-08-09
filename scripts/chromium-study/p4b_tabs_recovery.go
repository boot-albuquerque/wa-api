package main

// Fase 4B — duas perguntas que decidem a topologia e um bloqueador de GO.
//
// 1. `watabs`     — o WhatsApp Web aceita mais de uma aba no MESMO perfil?
//                   Se não aceitar, a topologia do alvo final não é escolhida
//                   por CPU nem por RAM: ela é imposta pela aplicação, e vira
//                   1 perfil = 1 sessão = 1 aba. A tentativa anterior de rodar
//                   3 jobs em abas paralelas deu 3 timeouts, mas timeout não é
//                   diagnóstico — este modo lê o que a segunda aba mostra.
//
// 2. `warecover`  — matar o Chromium e subir outro sobre o MESMO perfil deixa a
//                   sessão autenticada? Com o soak de 24h fora de escopo, a
//                   recuperação passa a carregar o peso da garantia.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/chromedp"

	"chromium-study/metrics"
)

// waProfile builds the launch policy including the identity override.
func waProfile() []string {
	p := CanonicalBrowserProfileV1
	if WAUserAgent != "" {
		p = append(append([]string{}, p...), "--user-agent="+WAUserAgent)
	}
	return p
}

// waPageState reports what a given tab is showing, with a text sample so an
// "unknown" is diagnosable instead of merely negative.
func waPageState(ctx context.Context) map[string]any {
	// The deadline lives HERE, not at each call site.
	//
	// This harness has now hung twice for the same reason — a CDP call with no
	// per-operation deadline against a tab that stopped responding — which is
	// precisely the defect this study documented in chromedp's own wait
	// primitives in Phase 1. Fixing it per call site is what let it come back;
	// owning it inside the function is what stops it.
	//
	// A tab that cannot answer within 5 s IS the result: it means the page is
	// gone, frozen or evicted, and the caller needs that as data rather than as
	// an indefinite block.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var raw string
	_ = chromedp.Run(ctx, chromedp.Evaluate(`JSON.stringify((() => {
		const t = document.body ? document.body.innerText : '';
		return {
			url: location.href,
			has_pane: !!document.querySelector('#pane-side'),
			has_qr: !!document.querySelector('canvas[aria-label*="Scan"]'),
			dom_nodes: document.getElementsByTagName('*').length,
			text_length: t.length,
			text_sample: t.slice(0, 240)
		};
	})())`, &raw))
	m := map[string]any{}
	if json.Unmarshal([]byte(raw), &m) != nil || len(m) == 0 {
		// Unresponsive within the deadline — recorded, not hidden.
		m = map[string]any{"unresponsive": true}
	}
	return m
}

// RunWATabs opens a session, then a SECOND tab on the same profile.
func RunWATabs(outPath string) error {
	PersistentProfileDir = waSessionDir()
	browsers, err := launchBrowsers(1, waProfile(), 9900)
	PersistentProfileDir = ""
	if err != nil {
		return err
	}
	defer gracefulStop(browsers[0])
	time.Sleep(3 * time.Second)

	alloc, cancelAlloc := chromedp.NewRemoteAllocator(context.Background(), browsers[0].WSURL)
	defer cancelAlloc()

	tab1, c1 := chromedp.NewContext(alloc)
	defer c1()
	if err := chromedp.Run(tab1, chromedp.Navigate(waURL)); err != nil {
		return err
	}
	t0 := time.Now()
	err1 := chromedp.Run(tab1, chromedp.Poll(`!!document.querySelector('#pane-side')`, nil,
		chromedp.WithPollingTimeout(90*time.Second)))
	tab1Sec := time.Since(t0).Seconds()
	st1 := waPageState(tab1)
	fmt.Fprintf(os.Stderr, "tab1: pane=%v after %.1fs err=%v\n", st1["has_pane"], tab1Sec, err1)

	// The second tab, same browser, same profile, same session.
	tab2, c2 := chromedp.NewContext(alloc)
	defer c2()
	if err := chromedp.Run(tab2, chromedp.Navigate(waURL)); err != nil {
		return err
	}
	t1 := time.Now()
	err2 := chromedp.Run(tab2, chromedp.Poll(`!!document.querySelector('#pane-side')`, nil,
		chromedp.WithPollingTimeout(45*time.Second)))
	tab2Sec := time.Since(t1).Seconds()
	st2 := waPageState(tab2)
	fmt.Fprintf(os.Stderr, "tab2: pane=%v after %.1fs err=%v\n", st2["has_pane"], tab2Sec, err2)
	fmt.Fprintf(os.Stderr, "tab2 text: %v\n", st2["text_sample"])

	// Re-read tab 1: if the application enforces exclusivity it may have EVICTED
	// the first tab when the second one claimed the session. Which side loses is
	// the difference between "second tab fails" and "opening a second tab breaks
	// the session you already had".
	st1After := waPageState(tab1)
	fmt.Fprintf(os.Stderr, "tab1 after tab2: pane=%v\n", st1After["has_pane"])

	out := map[string]any{
		"question":        "does WhatsApp Web allow a second tab on the same profile?",
		"tab1_ready_sec":  tab1Sec,
		"tab1_error":      errStr(err1),
		"tab1_state":      st1,
		"tab2_ready_sec":  tab2Sec,
		"tab2_error":      errStr(err2),
		"tab2_state":      st2,
		"tab1_state_after_tab2": st1After,
		"chromium_procs":  metrics.ChromiumProcs(),
		"memory_current_mb": float64(metrics.CgroupCurrent()) / 1048576,
		"started_utc":     time.Now().UTC().Format(time.RFC3339),
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	if outPath != "" {
		return os.WriteFile(outPath, b, 0o644)
	}
	fmt.Println(string(b))
	return nil
}

// RunWARecover kills Chromium under a live session and measures what it costs
// to come back on the same profile.
func RunWARecover(outPath string) error {
	dir := waSessionDir()
	PersistentProfileDir = dir
	browsers, err := launchBrowsers(1, waProfile(), 9900)
	PersistentProfileDir = ""
	if err != nil {
		return err
	}
	time.Sleep(3 * time.Second)

	alloc, cancelAlloc := chromedp.NewRemoteAllocator(context.Background(), browsers[0].WSURL)
	tab, cancelTab := chromedp.NewContext(alloc)
	if err := chromedp.Run(tab, chromedp.Navigate(waURL)); err != nil {
		return err
	}
	if err := chromedp.Run(tab, chromedp.Poll(`!!document.querySelector('#pane-side')`, nil,
		chromedp.WithPollingTimeout(120*time.Second))); err != nil {
		cancelTab()
		cancelAlloc()
		gracefulStop(browsers[0])
		return fmt.Errorf("session not up before fault: %w", err)
	}
	before := metrics.ChromiumProcs()
	selfBefore := metrics.Self()
	fmt.Fprintf(os.Stderr, "session up: procs=%v goroutines=%d fds=%d\n",
		before, selfBefore.Goroutines, selfBefore.FDs)

	// FAULT: SIGKILL the browser process only. Children are left to be
	// reparented, which is exactly the orphan case a pod restart must survive.
	killAt := time.Now()
	if err := browsers[0].SIGKILL(); err != nil {
		return err
	}

	// Detection: the first operation that fails. A short deadline per probe so a
	// dead connection cannot hold the measurement for tens of seconds — the
	// defect that invalidated the Phase 1 chaos test.
	var detectSec float64
	for time.Since(killAt) < 30*time.Second {
		probeCtx, probeCancel := context.WithTimeout(tab, 2*time.Second)
		err := chromedp.Run(probeCtx, chromedp.Evaluate(`1`, nil))
		probeCancel()
		if err != nil {
			detectSec = time.Since(killAt).Seconds()
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	cancelTab()
	cancelAlloc()
	time.Sleep(1 * time.Second)
	orphans := metrics.ChromiumProcs()
	fmt.Fprintf(os.Stderr, "detected in %.2fs | orphan procs right after kill: %v\n", detectSec, orphans)

	// RECOVERY: new Chromium, same profile.
	recStart := time.Now()
	PersistentProfileDir = dir
	b2, err := launchBrowsers(1, waProfile(), 9901)
	PersistentProfileDir = ""
	if err != nil {
		return fmt.Errorf("relaunch: %w", err)
	}
	defer gracefulStop(b2[0])
	bootSec := time.Since(recStart).Seconds()

	alloc2, cancelAlloc2 := chromedp.NewRemoteAllocator(context.Background(), b2[0].WSURL)
	defer cancelAlloc2()
	tab2, cancelTab2 := chromedp.NewContext(alloc2)
	defer cancelTab2()
	if err := chromedp.Run(tab2, chromedp.Navigate(waURL)); err != nil {
		return err
	}
	authErr := chromedp.Run(tab2, chromedp.Poll(`!!document.querySelector('#pane-side')`, nil,
		chromedp.WithPollingTimeout(120*time.Second)))
	recoverSec := time.Since(recStart).Seconds()
	st := waPageState(tab2)
	selfAfter := metrics.Self()

	stillAuth := authErr == nil && st["has_pane"] == true
	fmt.Fprintf(os.Stderr,
		"RECOVERY: boot=%.1fs total=%.1fs still_authenticated=%v qr_shown=%v | procs=%v | goroutines %d->%d fds %d->%d\n",
		bootSec, recoverSec, stillAuth, st["has_qr"], metrics.ChromiumProcs(),
		selfBefore.Goroutines, selfAfter.Goroutines, selfBefore.FDs, selfAfter.FDs)

	out := map[string]any{
		"fault":                  "Chromium SIGKILL (browser process only)",
		"detection_latency_sec":  detectSec,
		"browser_boot_sec":       bootSec,
		"recovery_latency_sec":   recoverSec,
		"still_authenticated":    stillAuth,
		"qr_shown_after_recovery": st["has_qr"],
		"required_new_login":     !stillAuth,
		"procs_before_fault":     before,
		"procs_orphaned":         orphans,
		"procs_after_recovery":   metrics.ChromiumProcs(),
		"controller_goroutines":  map[string]int{"before": selfBefore.Goroutines, "after": selfAfter.Goroutines},
		"controller_fds":         map[string]int{"before": selfBefore.FDs, "after": selfAfter.FDs},
		"memory_current_mb":      float64(metrics.CgroupCurrent()) / 1048576,
		"memory_peak_mb":         float64(metrics.CgroupPeak()) / 1048576,
		"jobs_lost":              1,
		"blast_radius_note":      "one session per profile: a browser death costs exactly the jobs of that one session",
		"started_utc":            time.Now().UTC().Format(time.RFC3339),
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	if outPath != "" {
		return os.WriteFile(outPath, b, 0o644)
	}
	fmt.Println(string(b))
	return nil
}

func errStr(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}
