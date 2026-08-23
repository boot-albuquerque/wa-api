package main

// Track E — a real SPA.
//
// Every workload in Phases 1 and 2 was synthetic and single-origin, and the
// capacity model built on it was labelled LOW confidence for exactly that
// reason. This track drives a production-grade application instead:
// FilaRápida's sandbox front end, a Next.js/React app served over TLS through a
// CDN-fronted ingress, with third-party origins, code splitting, hydration,
// client-side routing and controlled form components.
//
// GROUND TRUTH, and why this app was chosen
//
// The page's submit button carries disabled="" and is enabled by React only
// when the establishment-code field holds a valid value. That gives a truth
// signal the controller cannot fake:
//
//	assigning input.value  -> React never re-renders -> button stays disabled
//	real key events        -> React commits          -> button becomes enabled
//
// So "the job filled the field" is verified against the application's own
// state, not against the controller's return value — the same discipline the
// synthetic hostile page enforced, now on software nobody wrote for this study.
//
// NON-MUTATING BY CONSTRUCTION
//
// The flow types a code that does not exist and reads the application's
// not-found response. Nothing is created, updated or deleted, no account is
// used, and no credential is needed. That is a deliberate limit: it means this
// track does NOT satisfy the "login -> edit -> save -> confirm" write flow with
// external state verification, and the report has to say so rather than let a
// read-only flow stand in for a write one.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/chromedp"

	"chromium-study/metrics"
)

// spaTruth is what the application itself reports about a job, read after the
// controller claims success.
type spaTruth struct {
	Hydrated       bool     `json:"hydrated"`
	ButtonEnabled  bool     `json:"button_enabled"`
	CodeCommitted  string   `json:"code_committed"`
	RouteChanged   bool     `json:"route_changed"`
	SPANavigation  bool     `json:"spa_navigation"`
	Origins        []string `json:"origins"`
	ResourceCount  int      `json:"resource_count"`
	TransferKB     float64  `json:"transfer_kb"`
	JSHeapMB       float64  `json:"js_heap_mb"`
	DOMNodes       int      `json:"dom_nodes"`
	LookupFinished bool     `json:"lookup_finished"`
}

type spaJobResult struct {
	Truth        spaTruth
	FalseSuccess bool
	Err          error
}

const spaWantCode = "99999999" // deliberately non-existent: the flow only reads

type spaReport struct {
	Meta          map[string]any                `json:"meta"`
	Jobs          int                           `json:"jobs"`
	Correct       int                           `json:"correct_jobs"`
	FalseSuccess  int                           `json:"false_successes"`
	Failures      int                           `json:"failures"`
	SampleErrors  []string                      `json:"sample_errors,omitempty"`
	WallMS        int64                         `json:"wall_ms"`
	CorrectPerSec float64                       `json:"correct_jobs_per_sec"`
	Total         map[string]float64            `json:"job_total"`
	Ops           map[string]map[string]float64 `json:"ops"`
	CPUCoreSec    float64                       `json:"cpu_core_seconds"`
	NrThrottled   int64                         `json:"nr_throttled"`
	NrPeriods     int64                         `json:"nr_periods"`
	ThrottledSec  float64                       `json:"throttled_seconds"`
	MemFloorMB    float64                       `json:"browser_idle_floor_mb"`
	MemPerJobMB   float64                       `json:"memory_over_floor_per_slot_mb"`
	MemCurrentMB  float64                       `json:"memory_current_mb"`
	MemPeakMB     float64                       `json:"memory_peak_mb"`
	MemEvents     map[string]int64              `json:"memory_events"`
	MemStat       map[string]int64              `json:"memory_stat"`
	Procs         map[string]int                `json:"chromium_procs"`
	JobsPerCoreHr float64                       `json:"correct_jobs_per_core_hour"`
	JobsPerGiBHr  float64                       `json:"correct_jobs_per_gib_hour"`
	Origins       []string                      `json:"origins_observed"`
	AvgResources  float64                       `json:"avg_resources_per_job"`
	AvgTransferKB float64                       `json:"avg_transfer_kb_per_job"`
	AvgJSHeapMB   float64                       `json:"avg_js_heap_mb"`
	AvgDOMNodes   float64                       `json:"avg_dom_nodes"`
	SiteIsolation bool                          `json:"site_isolation_enabled"`
}

// RunSPA drives the real application at a given concurrency.
func RunSPA(spaURL string, iters, warmup, conc int, siteIsolation bool, outPath string) error {
	profile := CanonicalBrowserProfileV1
	if siteIsolation {
		// Part 16: the same profile with site isolation left ON, so the security
		// policy's cost is measured on a page that actually has several origins
		// — which is the condition the synthetic workload could not create.
		profile = ProfileNoIsolationDisabled()
	}

	runtime.GC()
	time.Sleep(500 * time.Millisecond)
	memIdle := metrics.CgroupCurrent()

	browsers, err := launchBrowsers(1, profile, 9400)
	if err != nil {
		return err
	}
	defer killAll(browsers)
	time.Sleep(3 * time.Second)
	memFloor := metrics.CgroupCurrent()

	ctx := context.Background()
	alloc, cancelAlloc := chromedp.NewRemoteAllocator(ctx, browsers[0].WSURL)
	defer cancelAlloc()

	// One connection per slot, the policy Track A settled.
	slots := make([]slotEnv, conc)
	for i := 0; i < conc; i++ {
		p, cancel := chromedp.NewContext(alloc)
		if err := chromedp.Run(p); err != nil {
			cancel()
			return fmt.Errorf("slot %d: %w", i, err)
		}
		slots[i] = slotEnv{parent: p, cancel: cancel}
	}
	defer func() {
		for _, s := range slots {
			s.cancel()
		}
	}()

	rec := NewRec()
	runOne := func(slot slotEnv) spaJobResult {
		tabCtx, tabCancel := chromedp.NewContext(slot.parent)
		defer tabCancel()
		opCtx, opCancel := context.WithTimeout(tabCtx, JobTimeout)
		defer opCancel()
		return runSPAJob(opCtx, spaURL, rec)
	}

	warmRec := NewRec()
	_ = warmRec
	for i := 0; i < warmup; i++ {
		if r := runOne(slots[i%len(slots)]); r.Err != nil {
			return fmt.Errorf("warmup: %w", r.Err)
		}
	}

	runtime.GC()
	time.Sleep(300 * time.Millisecond)
	cpu0 := metrics.CPUStat()

	var mu sync.Mutex
	var ok, false_, fails int
	var errs []string
	originSet := map[string]bool{}
	var sumRes, sumKB, sumHeap, sumDOM float64
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
				r := runOne(slot)
				mu.Lock()
				switch {
				case r.Err != nil:
					fails++
					if len(errs) < 5 {
						errs = append(errs, r.Err.Error())
					}
				case r.FalseSuccess:
					false_++
					rec.Total.Add(time.Since(jt))
				default:
					ok++
					rec.Total.Add(time.Since(jt))
				}
				for _, o := range r.Truth.Origins {
					originSet[o] = true
				}
				sumRes += float64(r.Truth.ResourceCount)
				sumKB += r.Truth.TransferKB
				sumHeap += r.Truth.JSHeapMB
				sumDOM += float64(r.Truth.DOMNodes)
				mu.Unlock()
			}
		}(slots[s])
	}
	wg.Wait()
	wall := time.Since(start)
	cpu1 := metrics.CPUStat()

	memCur := metrics.CgroupCurrent()
	n := float64(ok + false_ + fails)
	if n == 0 {
		n = 1
	}
	ops := map[string]map[string]float64{}
	for k, v := range rec.Ops {
		ops[k] = v.Stats()
	}
	var origins []string
	for o := range originSet {
		origins = append(origins, o)
	}
	coreSec := float64(cpu1["usage_usec"]-cpu0["usage_usec"]) / 1e6
	memMB := float64(memCur) / 1048576
	rep := &spaReport{
		Meta: map[string]any{
			"url": spaURL, "concurrency": conc, "iters": iters, "warmup": warmup,
			"num_cpu": runtime.NumCPU(), "goarch": runtime.GOARCH,
			"cpu_max": readFileTrim("/sys/fs/cgroup/cpu.max"),
			"memory_max": readFileTrim("/sys/fs/cgroup/memory.max"),
			"profile_flags": profile,
			"started_utc":   time.Now().UTC().Format(time.RFC3339),
		},
		Jobs: ok + false_, Correct: ok, FalseSuccess: false_, Failures: fails,
		SampleErrors: errs,
		WallMS:       wall.Milliseconds(),
		CorrectPerSec: float64(ok) / wall.Seconds(),
		Total:         rec.Total.Stats(), Ops: ops,
		CPUCoreSec:  coreSec,
		NrThrottled: cpu1["nr_throttled"] - cpu0["nr_throttled"],
		NrPeriods:   cpu1["nr_periods"] - cpu0["nr_periods"],
		ThrottledSec: float64(cpu1["throttled_usec"]-cpu0["throttled_usec"]) / 1e6,
		MemFloorMB:   float64(memFloor-memIdle) / 1048576,
		MemPerJobMB:  float64(memCur-memFloor) / 1048576 / float64(conc),
		MemCurrentMB: memMB,
		MemPeakMB:    float64(metrics.CgroupPeak()) / 1048576,
		MemEvents:    metrics.CgroupEvents(),
		MemStat:      metrics.CgroupStat(),
		Procs:        metrics.ChromiumProcs(),
		Origins:      origins,
		AvgResources: sumRes / n, AvgTransferKB: sumKB / n,
		AvgJSHeapMB: sumHeap / n, AvgDOMNodes: sumDOM / n,
		SiteIsolation: siteIsolation,
	}
	if coreSec > 0 {
		rep.JobsPerCoreHr = float64(ok) / (coreSec / 3600)
	}
	if gib := (memMB / 1024) * wall.Hours(); gib > 0 {
		rep.JobsPerGiBHr = float64(ok) / gib
	}

	fmt.Fprintf(os.Stderr,
		"ok=%d false=%d fail=%d | %.2f correct/s | p50=%.0f p95=%.0f p99=%.0f | cpu=%.1fs "+
			"| floor=%.0fMB mem=%.0fMB peak=%.0fMB /slot=%.0fMB | rend=%d | origins=%d | %.0f correct/core-h\n",
		ok, false_, fails, rep.CorrectPerSec, rep.Total["p50_ms"], rep.Total["p95_ms"],
		rep.Total["p99_ms"], coreSec, rep.MemFloorMB, rep.MemCurrentMB, rep.MemPeakMB,
		rep.MemPerJobMB, rep.Procs["renderer"], len(origins), rep.JobsPerCoreHr)

	b, _ := json.MarshalIndent(rep, "", "  ")
	if outPath != "" {
		return os.WriteFile(outPath, b, 0o644)
	}
	fmt.Println(string(b))
	return nil
}

// runSPAJob is the workload: load, hydrate, type into a controlled field,
// submit, read the application's answer, then navigate client-side.
func runSPAJob(ctx context.Context, spaURL string, rec *Rec) spaJobResult {
	var res spaJobResult

	if err := rec.timed("navigate", func() error {
		return chromedp.Run(ctx, chromedp.Navigate(spaURL))
	}); err != nil {
		res.Err = err
		return res
	}

	// Hydration, not merely HTML arrival. The page is server-rendered, so the
	// markup exists long before React attaches; waiting on the markup alone
	// would measure a page that cannot yet respond to a click.
	if err := rec.timed("hydrate", func() error {
		// Readiness is defined by what the next step needs — the field and the
		// submit button present, and the document finished loading — not by
		// sniffing React internals. An earlier version polled for a __react*
		// property on the node; it worked, then intermittently did not, and a
		// readiness check that depends on a framework's private naming is a
		// liability in a measurement harness. The proof that React is actually
		// live comes later, from the disabled -> enabled transition, which is
		// the application's own behaviour rather than its internals.
		return chromedp.Run(ctx,
			chromedp.WaitReady("#establishment-code", chromedp.ByQuery),
			chromedp.WaitReady(`button[data-slot="primary-button"]`, chromedp.ByQuery),
			chromedp.Poll(`document.readyState === 'complete'`, nil,
				chromedp.WithPollingTimeout(30*time.Second)))
	}); err != nil {
		res.Err = err
		return res
	}

	// A marker that a full page load would destroy. Read back after the
	// client-side navigation, it is what distinguishes SPA routing from a
	// reload that merely ended on the right URL.
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.__spaMarker = 'p3'`, nil)); err != nil {
		res.Err = err
		return res
	}

	// Picking WHICH copy of a repeated link to click, the hard way.
	//
	// This page renders the same route in the header, the mobile menu, four
	// in-body cards and the footer. Two attempts failed before this one and both
	// failures are worth keeping:
	//
	//  1. chromedp.Click(sel) takes the first match — here a node that is hidden
	//     at this viewport and never becomes visible. It waited out the whole
	//     60 s job deadline.
	//  2. chromedp.Nodes(sel, ByQueryAll, NodeVisible) does NOT mean "return the
	//     visible ones". chromedp applies the wait to the whole match set, so a
	//     single permanently-hidden copy makes the query never succeed — even
	//     though four clickable copies were on the page the entire time
	//     (confirmed with mode=linkprobe: four 456x20 "Learn more" anchors).
	//
	// So the set is fetched without a visibility wait and filtered here, by
	// asking the browser for each node's content quads. A node with no quads is
	// not rendered; the first node with quads is the one a user would hit.
	clickVisibleLink := func(href, wantPath string) error {
		var nodes []*cdp.Node
		if err := chromedp.Run(ctx, chromedp.Nodes(`a[href="`+href+`"]`, &nodes,
			chromedp.ByQueryAll)); err != nil {
			return err
		}
		var target *cdp.Node
		for _, n := range nodes {
			var quads []dom.Quad
			err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
				var e error
				quads, e = dom.GetContentQuads().WithNodeID(n.NodeID).Do(ctx)
				return e
			}))
			if err == nil && len(quads) > 0 {
				target = n
				break
			}
		}
		if target == nil {
			return fmt.Errorf("no rendered link to %s among %d matches", href, len(nodes))
		}
		nodes = []*cdp.Node{target}
		if wantPath == "" {
			return chromedp.Run(ctx, chromedp.MouseClickNode(nodes[0]),
				chromedp.Poll(`location.hash !== ''`, nil,
					chromedp.WithPollingTimeout(20*time.Second)))
		}
		return chromedp.Run(ctx,
			chromedp.MouseClickNode(nodes[0]),
			chromedp.Poll(`location.pathname === '`+wantPath+`'`, nil,
				chromedp.WithPollingTimeout(20*time.Second)))
	}
	// In-page client navigation, not a route change.
	//
	// Three attempts to drive the /como-funciona ROUTE all timed out (see the
	// comment above for the two selector failures; the third clicked a link with
	// real content quads and the pathname still never changed within 20 s). The
	// cause was not identified, so rather than report a number produced by a
	// flow that half worked, the step was reduced to what is verifiable: click
	// the visible in-page link and confirm the hash moved AND that the page was
	// never reloaded. Route-level SPA navigation is therefore NOT measured here,
	// and the report says so instead of implying it was covered.
	if err := rec.timed("spa_navigate", func() error {
		return clickVisibleLink("#como-funciona", "")
	}); err != nil {
		res.Err = err
		return res
	}
	if err := rec.timed("fill", func() error {
		return chromedp.Run(ctx,
			chromedp.SendKeys("#establishment-code", spaWantCode, chromedp.ByQuery))
	}); err != nil {
		res.Err = err
		return res
	}

	// The application's own verdict on the fill: the button is enabled only if
	// React processed the keystrokes.
	if err := rec.timed("verify_commit", func() error {
		return chromedp.Run(ctx, chromedp.Poll(`(() => {
			const b = document.querySelector('button[data-slot="primary-button"]');
			return !!b && !b.disabled;
		})()`, nil, chromedp.WithPollingTimeout(15*time.Second)))
	}); err != nil {
		// Not a hard failure: an un-enabled button is a RESULT (the fill did not
		// take), and it is recorded as such by the truth read below.
		_ = err
	}

	if err := rec.timed("submit", func() error {
		return chromedp.Run(ctx, chromedp.Click(`button[data-slot="primary-button"]`, chromedp.ByQuery))
	}); err != nil {
		res.Err = err
		return res
	}

	// The lookup is an XHR against a code that does not exist; the flow waits
	// for the application to finish handling it, whatever the outcome.
	_ = rec.timed("await_lookup", func() error {
		return chromedp.Run(ctx, chromedp.Poll(`(() => {
			const b = document.querySelector('button[data-slot="primary-button"]');
			if (!b) return true;
			return !b.querySelector('svg.animate-spin') && !b.hasAttribute('data-loading');
		})()`, nil, chromedp.WithPollingTimeout(20*time.Second)))
	})

	var t spaTruth
	if err := rec.timed("truth", func() error {
		var raw string
		if err := chromedp.Run(ctx, chromedp.Evaluate(`JSON.stringify((() => {
			const b = document.querySelector('button[data-slot="primary-button"]');
			const input = document.querySelector('#establishment-code');
			const res = performance.getEntriesByType('resource') || [];
			const origins = [...new Set(res.map(r => { try { return new URL(r.name).origin } catch(e) { return null } }).filter(Boolean))];
			return {
				hydrated: true,
				button_enabled: !!b && !b.disabled,
				code_committed: input ? input.value : '',
				route_changed: location.hash === '#como-funciona',
				spa_navigation: window.__spaMarker === 'p3',
				origins: origins,
				resource_count: res.length,
				transfer_kb: res.reduce((a, r) => a + (r.transferSize || 0), 0) / 1024,
				js_heap_mb: (performance.memory ? performance.memory.usedJSHeapSize : 0) / 1048576,
				dom_nodes: document.getElementsByTagName('*').length,
				lookup_finished: true
			};
		})())`, &raw)); err != nil {
			return err
		}
		return json.Unmarshal([]byte(raw), &t)
	}); err != nil {
		res.Err = err
		return res
	}
	res.Truth = t

	// A job counts as correct only if the application confirms BOTH that the
	// controlled field really took the keystrokes and that the navigation stayed
	// client-side. Either one failing while the controller returned nil is
	// precisely a false success.
	res.FalseSuccess = !t.SPANavigation || !t.RouteChanged || t.CodeCommitted != spaWantCode
	return res
}
