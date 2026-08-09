package main

// Track B — topology: how many Chromium processes, BrowserContexts and pages a
// worker pod should run for a fixed amount of concurrent work.
//
// One policy is held CONSTANT across every topology, and it has to be stated
// before any number is read: each concurrent worker slot gets its OWN CDP
// websocket connection. That is not a neutral choice — it is the conclusion of
// Track A, where forcing chromedp onto a single shared connection cost most of
// its throughput. Leaving connections to vary with the topology would have made
// this track re-measure Track A's effect and label it "contexts".
//
// What varies here is only: how many browser processes, how many
// BrowserContexts inside them, and how many pages share a context.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"

	"chromium-study/metrics"
	"chromium-study/workload"
)

// topoSpec is one point in the topology space.
//
// Contexts == 0 means "use the browser's default BrowserContext", which is what
// every previous phase did implicitly. It is kept as a distinct value rather
// than folded into 1, because the default context is not just a context: it is
// the one that already exists, shares the profile directory's state, and cannot
// be disposed.
type topoSpec struct {
	Name     string `json:"name"`
	Browsers int    `json:"browsers"`
	Contexts int    `json:"contexts_per_browser"`
	Slots    int    `json:"concurrent_slots"`
	Note     string `json:"note"`
}

func topologySet(conc int) []topoSpec {
	return []topoSpec{
		{"T1-1b-default-1slot", 1, 0, 1, "serial reference"},
		{"T2-1b-default-Nslot", 1, 0, conc, "one browser, one context, N pages"},
		{"T3-1b-Nctx", 1, conc, conc, "one browser, one context per slot"},
		{"T4-2b-default", 2, 0, conc, "two browsers, default contexts"},
		{"T5-Nb-1slot-each", conc, 0, conc, "one browser per slot"},
		{"T6-2b-2ctx", 2, 2, conc, "two browsers, two contexts each"},
	}
}

type topoResult struct {
	Spec           topoSpec                      `json:"spec"`
	Jobs           int                           `json:"jobs"`
	Correct        int                           `json:"correct_jobs"`
	FalseSuccess   int                           `json:"false_successes"`
	Failures       int                           `json:"failures"`
	SampleErrors   []string                      `json:"sample_errors,omitempty"`
	WallMS         int64                         `json:"wall_ms"`
	JobsPerSec     float64                       `json:"jobs_per_sec"`
	CorrectPerSec  float64                       `json:"correct_jobs_per_sec"`
	Total          map[string]float64            `json:"job_total"`
	Ops            map[string]map[string]float64 `json:"ops"`
	CPUCoreSec     float64                       `json:"cpu_core_seconds"`
	CPUUserSec     float64                       `json:"cpu_user_seconds"`
	CPUSysSec      float64                       `json:"cpu_system_seconds"`
	NrThrottled    int64                         `json:"nr_throttled"`
	NrPeriods      int64                         `json:"nr_periods"`
	ThrottledSec   float64                       `json:"throttled_seconds"`
	MemCurrentMB   float64                       `json:"memory_current_mb"`
	MemPeakMB      float64                       `json:"memory_peak_mb"`
	MemFloorMB     float64                       `json:"memory_idle_floor_mb"`
	MemEvents      map[string]int64              `json:"memory_events"`
	MemStat        map[string]int64              `json:"memory_stat"`
	Procs          map[string]int                `json:"chromium_procs"`
	Goroutines     int                           `json:"goroutines"`
	Threads        int                           `json:"os_threads"`
	FDs            int                           `json:"open_fds"`
	JobsPerCoreHr  float64                       `json:"correct_jobs_per_core_hour"`
	JobsPerGiBHr   float64                       `json:"correct_jobs_per_gib_hour"`
	ConnectionsMax int                           `json:"cdp_connections"`
}

// RunTopology executes every topology in the set with the SAME total number of
// jobs, so throughput and cost per job are directly comparable.
func RunTopology(iters, warmup, conc int, only string, outPath string) error {
	srv, err := workload.Start()
	if err != nil {
		return err
	}
	defer srv.Close()
	pageURL := srv.URL() + "/h"

	out := map[string]any{
		"meta": map[string]any{
			"iters_per_topology": iters,
			"warmup":             warmup,
			"base_concurrency":   conc,
			"num_cpu":            runtime.NumCPU(),
			"goarch":             runtime.GOARCH,
			"cpu_max":            readFileTrim("/sys/fs/cgroup/cpu.max"),
			"memory_max":         readFileTrim("/sys/fs/cgroup/memory.max"),
			"canonical_flags":    CanonicalBrowserProfileV1,
			"connection_policy":  "one CDP websocket per concurrent slot (Track A result held constant)",
			"started_utc":        time.Now().UTC().Format(time.RFC3339),
		},
		"results": []*topoResult{},
	}

	for _, spec := range topologySet(conc) {
		if only != "" && spec.Name != only {
			continue
		}
		fmt.Fprintf(os.Stderr, "== %s (browsers=%d contexts=%d slots=%d) ==\n",
			spec.Name, spec.Browsers, spec.Contexts, spec.Slots)
		r, err := runTopology(spec, pageURL, iters, warmup)
		if err != nil {
			fmt.Fprintf(os.Stderr, "   FAILED: %v\n", err)
			continue
		}
		out["results"] = append(out["results"].([]*topoResult), r)
		fmt.Fprintf(os.Stderr,
			"   ok=%d false=%d fail=%d | %.2f correct/s | p50=%.0f p99=%.0f | "+
				"cpu=%.1fs mem=%.0fMB peak=%.0fMB rend=%d | %.0f correct/core-h\n",
			r.Correct, r.FalseSuccess, r.Failures, r.CorrectPerSec, r.Total["p50_ms"],
			r.Total["p99_ms"], r.CPUCoreSec, r.MemCurrentMB, r.MemPeakMB,
			r.Procs["renderer"], r.JobsPerCoreHr)
	}

	b, _ := json.MarshalIndent(out, "", "  ")
	if outPath != "" {
		return os.WriteFile(outPath, b, 0o644)
	}
	fmt.Println(string(b))
	return nil
}

// slotEnv is one worker slot: its own connection, bound to one browser and one
// BrowserContext.
type slotEnv struct {
	parent     context.Context
	cancel     context.CancelFunc
	browserCtx string // empty means the browser's default context
	admin      *adminConn
}

// adminConn is the raw CDP control connection for one browser.
type adminConn struct{ c *cdpMin }

func dialAdmin(wsURL string) (*adminConn, error) {
	c := &cdpMin{}
	if err := c.Connect(context.Background(), wsURL); err != nil {
		return nil, err
	}
	return &adminConn{c: c}, nil
}

func (a *adminConn) createBrowserContext() (string, error) {
	res, err := a.c.call("", "Target.createBrowserContext", map[string]any{})
	if err != nil {
		return "", err
	}
	var r struct {
		BrowserContextID string `json:"browserContextId"`
	}
	if err := json.Unmarshal(res, &r); err != nil {
		return "", err
	}
	return r.BrowserContextID, nil
}

func (a *adminConn) createTarget(browserCtxID string) (string, error) {
	params := map[string]any{"url": "about:blank"}
	// An empty browserContextId must be OMITTED, not sent as "": Chromium
	// validates the field when present and would reject the empty string, and
	// the caller uses "" to mean "the default context".
	if browserCtxID != "" {
		params["browserContextId"] = browserCtxID
	}
	res, err := a.c.call("", "Target.createTarget", params)
	if err != nil {
		return "", err
	}
	var r struct {
		TargetID string `json:"targetId"`
	}
	if err := json.Unmarshal(res, &r); err != nil {
		return "", err
	}
	return r.TargetID, nil
}

func (a *adminConn) closeTarget(id string) {
	_, _ = a.c.call("", "Target.closeTarget", map[string]any{"targetId": id})
}

func (a *adminConn) disposeBrowserContext(id string) error {
	_, err := a.c.call("", "Target.disposeBrowserContext", map[string]any{"browserContextId": id})
	return err
}

func runTopology(spec topoSpec, pageURL string, iters, warmup int) (*topoResult, error) {
	// Idle floor BEFORE any browser exists: without it, "memory per job" would
	// include the browser's fixed cost and every dense topology would look
	// artificially efficient.
	runtime.GC()
	time.Sleep(500 * time.Millisecond)
	memIdle := metrics.CgroupCurrent()

	browsers, err := launchBrowsers(spec.Browsers, CanonicalBrowserProfileV1, 9300)
	if err != nil {
		return nil, err
	}
	defer killAll(browsers)
	time.Sleep(2 * time.Second) // let the browsers reach steady state

	ctx := context.Background()

	// One raw control connection per browser, opened before any chromedp
	// connection so its role stays unambiguous.
	admins := make([]*adminConn, len(browsers))
	for i, b := range browsers {
		a, err := dialAdmin(b.WSURL)
		if err != nil {
			return nil, fmt.Errorf("admin conn: %w", err)
		}
		defer a.c.Close()
		admins[i] = a
	}

	// One allocator per browser. The allocator itself holds no connection; each
	// context derived from it dials its own.
	allocs := make([]context.Context, len(browsers))
	for i, b := range browsers {
		a, cancel := chromedp.NewRemoteAllocator(ctx, b.WSURL)
		defer cancel()
		allocs[i] = a
	}

	// BrowserContext and target lifecycle runs on a DEDICATED raw CDP control
	// connection per browser, not on chromedp's connection.
	//
	// This is a design choice forced by a reproduced defect, and it is worth
	// stating because it is also the shape a production runtime should have —
	// the launch/lifecycle policy owning contexts, the driving library owning
	// pages.
	//
	// The defect: chromedp's WithNewBrowserContext() fails on Chromium
	// 151.0.7922.108 with --headless=new — "Failed to open new tab - no browser
	// is open (-32000)". Isolated with a wire trace (mode=ctxprobe):
	//   * the identical frames replayed on a raw websocket SUCCEED, including
	//     every explicitly-false optional field chromedp serializes;
	//   * they still succeed after chromedp has connected, so it is not that
	//     chromedp puts the browser into a bad state;
	//   * but the same command issued THROUGH chromedp's browser connection
	//     (cdp.WithExecutor with its Browser) fails.
	// The discriminating factor was not identified further. What is established
	// is that per-context isolation is not reachable through chromedp's own API
	// on this browser, which is a production-relevant limitation regardless of
	// its cause.

	// (Historical note: an earlier version of this comment claimed the commands
	// were issued with cdproto through chromedp; that path is what fails.)
	//
	// That option does not work on this browser: on Chromium 151.0.7922.108 with
	// --headless=new it fails with "Failed to open new tab - no browser is open
	// (-32000)", while the same two commands issued directly over the same
	// websocket succeed. It was isolated with a wire trace (mode=ctxprobe):
	// chromedp sends Target.createBrowserContext{disposeOnDetach:true} followed
	// by Target.createTarget{browserContextId,...}, and replaying those exact
	// frames by hand — including every explicitly-false optional field — works.
	// The discriminating factor was NOT identified, so this is a workaround
	// around a REPRODUCED failure, not an explanation of it.
	//
	// The consequence for production is the finding, not the workaround: the
	// library's supported API for per-session isolation is unusable on this
	// browser version, so any design that relies on BrowserContexts has to own
	// that code path itself.

	// Build the slots. Slot -> browser is a contiguous split so that a 2-browser
	// topology puts slots 0..n/2 on browser 0, not interleaved; interleaving
	// would spread each browser's load over the whole run and hide the moment a
	// single browser saturates.
	slots := make([]slotEnv, spec.Slots)
	for s := 0; s < spec.Slots; s++ {
		bi := s * spec.Browsers / spec.Slots
		p, cancel := chromedp.NewContext(allocs[bi])
		if err := chromedp.Run(p); err != nil {
			cancel()
			return nil, fmt.Errorf("slot %d connect: %w", s, err)
		}
		var ctxID string
		if spec.Contexts > 0 {
			id, err := admins[bi].createBrowserContext()
			if err != nil {
				cancel()
				return nil, fmt.Errorf("slot %d browser context: %w", s, err)
			}
			ctxID = id
		}
		slots[s] = slotEnv{parent: p, cancel: cancel, browserCtx: ctxID, admin: admins[bi]}
	}
	defer func() {
		for _, s := range slots {
			s.cancel()
		}
	}()

	runOne := func(slot slotEnv, rec *Rec) (JobOutcome, error) {
		var opts []chromedp.ContextOption
		if slot.browserCtx != "" {
			tid, err := slot.admin.createTarget(slot.browserCtx)
			if err != nil {
				return JobOutcome{}, err
			}
			defer slot.admin.closeTarget(tid)
			opts = append(opts, chromedp.WithTargetID(target.ID(tid)))
		}
		tabCtx, tabCancel := chromedp.NewContext(slot.parent, opts...)
		defer tabCancel()
		opCtx, opCancel := context.WithTimeout(tabCtx, JobTimeout)
		defer opCancel()
		return runHostileJob(opCtx, L2, pageURL, rec)
	}

	warm := NewRec()
	for i := 0; i < warmup; i++ {
		if _, err := runOne(slots[i%len(slots)], warm); err != nil {
			return nil, fmt.Errorf("warmup: %w", err)
		}
	}

	// Counters are read AFTER warmup: page creation, context attach and the
	// first navigation of each slot are startup costs, not per-job costs.
	runtime.GC()
	time.Sleep(300 * time.Millisecond)
	cpu0 := metrics.CPUStat()
	memBefore := metrics.CgroupCurrent()

	rec := NewRec()
	var mu sync.Mutex
	var ok, false_, fails int
	var errSamples []string
	start := time.Now()
	var wg sync.WaitGroup
	jobCh := make(chan int, iters)
	for i := 0; i < iters; i++ {
		jobCh <- i
	}
	close(jobCh)
	for s := 0; s < spec.Slots; s++ {
		wg.Add(1)
		go func(slot slotEnv) {
			defer wg.Done()
			for range jobCh {
				jt := time.Now()
				outcome, err := runOne(slot, rec)
				mu.Lock()
				if err != nil {
					fails++
					if len(errSamples) < 5 {
						errSamples = append(errSamples, err.Error())
					}
				} else {
					rec.Total.Add(time.Since(jt))
					if outcome.FalseSuccess {
						false_++
					} else {
						ok++
					}
				}
				mu.Unlock()
			}
		}(slots[s])
	}
	wg.Wait()
	wall := time.Since(start)

	cpu1 := metrics.CPUStat()
	memAfter := metrics.CgroupCurrent()
	self := metrics.Self()

	usec := func(k string) float64 { return float64(cpu1[k]-cpu0[k]) / 1e6 }
	ops := map[string]map[string]float64{}
	for k, v := range rec.Ops {
		ops[k] = v.Stats()
	}
	coreSec := usec("usage_usec")
	memMB := float64(memAfter) / 1048576
	peakMB := float64(metrics.CgroupPeak()) / 1048576
	// GiB-hours charges the memory actually resident during the run, which is
	// the figure a cluster bills for, rather than the pod's limit.
	gibHours := (memMB / 1024) * (wall.Hours())
	res := &topoResult{
		Spec: spec, Jobs: ok + false_, Correct: ok, FalseSuccess: false_, Failures: fails,
		SampleErrors: errSamples,
		WallMS:     wall.Milliseconds(),
		JobsPerSec: float64(ok+false_) / wall.Seconds(),
		CorrectPerSec: func() float64 {
			return float64(ok) / wall.Seconds()
		}(),
		Total: rec.Total.Stats(), Ops: ops,
		CPUCoreSec: coreSec, CPUUserSec: usec("user_usec"), CPUSysSec: usec("system_usec"),
		NrThrottled: cpu1["nr_throttled"] - cpu0["nr_throttled"],
		NrPeriods:   cpu1["nr_periods"] - cpu0["nr_periods"],
		ThrottledSec: float64(cpu1["throttled_usec"]-cpu0["throttled_usec"]) / 1e6,
		MemCurrentMB: memMB,
		MemPeakMB:    peakMB,
		MemFloorMB:   float64(memBefore-memIdle) / 1048576,
		MemEvents:    metrics.CgroupEvents(),
		MemStat:      metrics.CgroupStat(),
		Procs:        metrics.ChromiumProcs(),
		Goroutines:   self.Goroutines, Threads: self.Threads, FDs: self.FDs,
		ConnectionsMax: spec.Slots,
	}
	if coreSec > 0 {
		res.JobsPerCoreHr = float64(ok) / (coreSec / 3600)
	}
	if gibHours > 0 {
		res.JobsPerGiBHr = float64(ok) / gibHours
	}
	return res, nil
}
