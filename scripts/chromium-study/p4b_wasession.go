package main

// Fase 4B §4 — o piso real de UMA sessão autenticada do WhatsApp Web.
//
// Uma leitura instantânea logo após o login não é o piso: a aplicação continua
// sincronizando, o WebSocket continua recebendo, e o custo real só aparece
// depois que isso assenta. Este modo mantém UMA sessão viva e amostra o estado
// ao longo de vários minutos.
//
// UMA aba, UM perfil, UMA sessão — deliberadamente. A tentativa anterior de
// medir concorrência abrindo abas paralelas no mesmo perfil deu 3 timeouts em 3
// jobs, e a arquitetura do wwebjs (um Client por browser, userDataDir próprio)
// aponta para a mesma restrição. Concorrência não é medida aqui.
//
// Nada é enviado, nenhuma conversa é aberta, e o relatório contém apenas
// contagens, tamanhos e durações.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/chromedp"

	"chromium-study/metrics"
)

type waSessSample struct {
	TSec         float64          `json:"t_sec"`
	MemCurrentMB float64          `json:"memory_current_mb"`
	MemPeakMB    float64          `json:"memory_peak_mb"`
	MemStat      map[string]int64 `json:"memory_stat"`
	CPUCoreSec   float64          `json:"cpu_core_seconds_cumulative"`
	NrThrottled  int64            `json:"nr_throttled_cumulative"`
	Procs        map[string]int   `json:"chromium_procs"`
	DOMNodes     int              `json:"dom_nodes"`
	ChatRows     int              `json:"chat_rows_rendered"`
	JSHeapMB     float64          `json:"js_heap_mb"`
	Resources    int              `json:"resource_count"`
	TransferKB   float64          `json:"transfer_kb"`
	CDPBytes     int64            `json:"cdp_bytes_cumulative"`
	CDPMsgs      int64            `json:"cdp_messages_cumulative"`
	ReadMS       float64          `json:"read_probe_ms"`
	GoroCtl      int              `json:"controller_goroutines"`
	FDsCtl       int              `json:"controller_fds"`
}

// RunWASession keeps one authenticated session alive and samples it.
func RunWASession(dur time.Duration, outPath string) error {
	dir := waSessionDir()
	memIdle := metrics.CgroupCurrent()

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
	memPreLogin := metrics.CgroupCurrent()

	ctx := context.Background()
	// Route through the counting proxy so the session's ongoing CDP traffic is
	// measured rather than assumed to be idle.
	px, err := StartProxy(ctx, browsers[0].HTTPBase)
	if err != nil {
		return err
	}
	defer px.Close()

	alloc, cancelAlloc := chromedp.NewRemoteAllocator(ctx, px.WS())
	defer cancelAlloc()
	tab, cancelTab := chromedp.NewContext(alloc)
	defer cancelTab()

	if err := chromedp.Run(tab, chromedp.Navigate(waURL)); err != nil {
		return err
	}
	loginStart := time.Now()
	if err := chromedp.Run(tab, chromedp.Poll(`!!document.querySelector('#pane-side')`, nil,
		chromedp.WithPollingTimeout(120*time.Second))); err != nil {
		return fmt.Errorf("session not restored: %w", err)
	}
	restoreSec := time.Since(loginStart).Seconds()
	fmt.Fprintf(os.Stderr, "session restored in %.2fs — sampling for %s\n", restoreSec, dur)

	cpu0 := metrics.CPUStat()
	px.Reset()
	var samples []waSessSample
	start := time.Now()
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	deadline := time.Now().Add(dur)

	for time.Now().Before(deadline) {
		<-tick.C
		// A read probe on every sample: this is the primitive the workload
		// actually uses, so its latency belongs in the same series as the
		// resources it competes for.
		probeStart := time.Now()
		var raw string
		// Every probe carries its own short deadline. Without one, a single hung
		// CDP call blocks the sampling loop forever: the first version of this
		// mode ran 38 minutes on an 8-minute budget for exactly that reason —
		// the same defect this study flagged in chromedp's untimed waits.
		probeCtx, probeCancel := context.WithTimeout(tab, 10*time.Second)
		err := chromedp.Run(probeCtx, chromedp.Evaluate(`JSON.stringify((() => {
			const pane = document.querySelector('#pane-side');
			const res = performance.getEntriesByType('resource') || [];
			return {
				dom_nodes: document.getElementsByTagName('*').length,
				chat_rows: pane ? pane.querySelectorAll('[role="listitem"], [role="row"]').length : 0,
				js_heap_mb: (performance.memory ? performance.memory.usedJSHeapSize : 0) / 1048576,
				resource_count: res.length,
				transfer_kb: res.reduce((a, r) => a + (r.transferSize || 0), 0) / 1024
			};
		})())`, &raw))
		probeCancel()
		readMS := float64(time.Since(probeStart).Microseconds()) / 1000
		var page struct {
			DOMNodes   int     `json:"dom_nodes"`
			ChatRows   int     `json:"chat_rows"`
			JSHeapMB   float64 `json:"js_heap_mb"`
			Resources  int     `json:"resource_count"`
			TransferKB float64 `json:"transfer_kb"`
		}
		if err == nil {
			_ = json.Unmarshal([]byte(raw), &page)
		}
		cpu := metrics.CPUStat()
		self := metrics.Self()
		s := waSessSample{
			TSec:         time.Since(start).Seconds(),
			MemCurrentMB: float64(metrics.CgroupCurrent()) / 1048576,
			MemPeakMB:    float64(metrics.CgroupPeak()) / 1048576,
			MemStat:      metrics.CgroupStat(),
			CPUCoreSec:   float64(cpu["usage_usec"]-cpu0["usage_usec"]) / 1e6,
			NrThrottled:  cpu["nr_throttled"] - cpu0["nr_throttled"],
			Procs:        metrics.ChromiumProcs(),
			DOMNodes:     page.DOMNodes, ChatRows: page.ChatRows,
			JSHeapMB: page.JSHeapMB, Resources: page.Resources, TransferKB: page.TransferKB,
			CDPBytes: px.BytesIn() + px.BytesOut(),
			CDPMsgs:  px.Sent() + px.Recv(),
			ReadMS:   readMS,
			GoroCtl:  self.Goroutines, FDsCtl: self.FDs,
		}
		samples = append(samples, s)
		fmt.Fprintf(os.Stderr,
			"t=%4.0fs mem=%.0fMB anon=%.0f peak=%.0f | cpu=%.1fs thr=%d | rend=%d proc=%d | dom=%d rows=%d heap=%.1fMB | cdp=%.0fKB/%d msgs | read=%.0fms\n",
			s.TSec, s.MemCurrentMB, float64(s.MemStat["anon"])/1048576, s.MemPeakMB,
			s.CPUCoreSec, s.NrThrottled, s.Procs["renderer"], s.Procs["total"],
			s.DOMNodes, s.ChatRows, s.JSHeapMB, float64(s.CDPBytes)/1024, s.CDPMsgs, s.ReadMS)
	}

	out := map[string]any{
		"meta": map[string]any{
			"duration": dur.String(), "sessions": 1, "tabs": 1,
			"cpu_max": readFileTrim("/sys/fs/cgroup/cpu.max"),
			"memory_max": readFileTrim("/sys/fs/cgroup/memory.max"),
			"user_agent_overridden": WAUserAgent != "",
			"started_utc":           time.Now().UTC().Format(time.RFC3339),
		},
		"memory_idle_mb":       float64(memIdle) / 1048576,
		"browser_floor_mb":     float64(memPreLogin-memIdle) / 1048576,
		"session_restore_sec":  restoreSec,
		"samples":              samples,
		"profile_dir_bytes":    dirSize(dir),
		"memory_events_at_end": metrics.CgroupEvents(),
	}
	if len(samples) >= 3 {
		f, l := samples[0], samples[len(samples)-1]
		hours := (l.TSec - f.TSec) / 3600
		if hours > 0 {
			out["anon_slope_mb_per_hour"] = (float64(l.MemStat["anon"]) - float64(f.MemStat["anon"])) / 1048576 / hours
			out["mem_slope_mb_per_hour"] = (l.MemCurrentMB - f.MemCurrentMB) / hours
			out["cdp_bytes_per_hour"] = float64(l.CDPBytes-f.CDPBytes) / hours
		}
		out["session_ram_mb"] = l.MemCurrentMB - out["browser_floor_mb"].(float64) - float64(memIdle)/1048576
	}

	b, _ := json.MarshalIndent(out, "", "  ")
	if outPath != "" {
		return os.WriteFile(outPath, b, 0o644)
	}
	fmt.Println(string(b))
	return nil
}
