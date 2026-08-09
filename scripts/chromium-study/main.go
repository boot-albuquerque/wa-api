// Command chromium-study benchmarks CDP controllers against ONE externally
// launched browser, inside a Linux container with cgroup v2 accounting.
//
// Layering, kept strictly apart because conflating them is the classic error
// this study exists to avoid:
//
//	browser         chromium binary, one version, launched once
//	launch policy   the flag set (CanonicalBrowserProfileV1), identical for all
//	controller      chromedp / rod / playwright / raw CDP — what is under test
//	workload        the SPA and the nine steps driven against it
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"chromium-study/metrics"
	"chromium-study/workload"
)

// Lat is an alias so controllers.go does not import the metrics package.
type Lat = metrics.Latencies

// JobTimeout bounds one workload job for every controller. It exists because
// two of the three libraries under test wait forever by default.
var JobTimeout = 60 * time.Second

// CanonicalBrowserProfileV1 is the launch policy every controller runs against.
//
// It is written out here, in one place, because the central hypothesis of this
// study is that most of the memory difference attributed to controller
// libraries is really this list. Any controller that would add or change a flag
// is instead pointed at an already-running browser, so the list cannot drift
// per controller.
//
// Rationale per group:
//   - headless=new: the supported headless implementation; old headless is a
//     different browser with different memory behaviour.
//   - no-sandbox: required to run as a non-root-capable process in a plain
//     container. It is a SECURITY REDUCTION, stated explicitly rather than
//     inherited from a library that detects containers and flips it silently.
//   - disable-dev-shm-usage: containers default to a 64MB /dev/shm; without
//     this Chromium crashes under load. The study also measures the alternative
//     (mounting a larger /dev/shm) separately.
//   - site isolation flags: the single largest memory lever found so far, so
//     they are set explicitly and identically for everyone.
var CanonicalBrowserProfileV1 = []string{
	"--headless=new",
	"--no-sandbox",
	"--disable-dev-shm-usage",
	"--disable-gpu",
	"--no-first-run",
	"--no-default-browser-check",
	"--disable-background-networking",
	"--disable-background-timer-throttling",
	"--disable-backgrounding-occluded-windows",
	"--disable-renderer-backgrounding",
	"--disable-breakpad",
	"--disable-client-side-phishing-detection",
	"--disable-default-apps",
	"--disable-extensions",
	"--disable-component-extensions-with-background-pages",
	"--disable-hang-monitor",
	"--disable-ipc-flooding-protection",
	"--disable-popup-blocking",
	"--disable-prompt-on-repost",
	"--disable-sync",
	"--metrics-recording-only",
	"--disable-features=site-per-process,Translate,TranslateUI,BlinkGenPropertyTrees,AcceptCHFrame,MediaRouter,OptimizationHints",
	"--disable-site-isolation-trials",
	"--force-color-profile=srgb",
	"--window-size=1280,800",
	"--lang=en-US",
	"--remote-debugging-address=0.0.0.0",
}

// ProfileNoIsolationDisabled is CanonicalBrowserProfileV1 with the two site
// isolation flags removed, to measure that lever alone rather than assert it.
func ProfileNoIsolationDisabled() []string {
	out := make([]string, 0, len(CanonicalBrowserProfileV1))
	for _, f := range CanonicalBrowserProfileV1 {
		if f == "--disable-site-isolation-trials" {
			continue
		}
		if strings.HasPrefix(f, "--disable-features=") {
			f = "--disable-features=Translate,TranslateUI,BlinkGenPropertyTrees,AcceptCHFrame,MediaRouter,OptimizationHints"
		}
		out = append(out, f)
	}
	return out
}

type runReport struct {
	Meta        map[string]any        `json:"meta"`
	Controllers map[string]*ctlReport `json:"controllers"`
	Browser     map[string]any        `json:"browser"`
	Cgroup      map[string]any        `json:"cgroup"`
}

type ctlReport struct {
	Jobs          int                           `json:"jobs"`
	Failures      int                           `json:"failures"`
	WallMS        int64                         `json:"wall_ms"`
	JobsPerSec    float64                       `json:"jobs_per_sec"`
	ConnectMS     float64                       `json:"connect_ms"`
	Total         map[string]float64            `json:"job_total"`
	Ops           map[string]map[string]float64 `json:"ops"`
	CtlBefore     metrics.Snapshot              `json:"controller_before"`
	CtlAfter      metrics.Snapshot              `json:"controller_after"`
	CgroupDeltaB  int64                         `json:"cgroup_delta_bytes"`
	RendererKills int64                         `json:"renderer_kills"`
}

func main() {
	var (
		cdpURL   = flag.String("cdp", "http://127.0.0.1:9222", "external browser CDP endpoint")
		ctlNames = flag.String("controllers", "cdp-min,chromedp,chromedp-cdproto,rod-canon,rod-default", "comma list")
		wl       = flag.String("workload", "a", "a|b")
		iters    = flag.Int("iters", 30, "measured jobs per controller")
		warmup   = flag.Int("warmup", 5, "warmup jobs, discarded")
		conc     = flag.Int("concurrency", 1, "concurrent jobs")
		out      = flag.String("out", "", "write JSON report here")
		jobTO    = flag.Duration("job-timeout", 60*time.Second, "per-job deadline")
		chaosInt = flag.Duration("chaos", 0, "if >0, crash a renderer every interval during the measured phase")
		mode     = flag.String("mode", "controllers", "controllers|levels|prof|topology")
		profDir  = flag.String("profdir", "/out/prof", "where mode=prof writes pprof files")
		onlyTopo = flag.String("topology", "", "run only this topology name")
		spaURL   = flag.String("spa-url", "", "real SPA to drive in mode=spa")
		siteIso  = flag.Bool("site-isolation", false, "leave Chromium site isolation ON")
		soakDur  = flag.Duration("soak", time.Hour, "mode=soak wall-clock duration")
		nBrowse  = flag.Int("browsers", 1, "mode=soak browser instances")
		reps     = flag.Int("reps", 5, "independent repetitions per arm")
		waChat   = flag.String("wa-chat", "", "mode=wacap: search term for an AUTHORISED test conversation; empty means no interaction at all")
		waWait   = flag.Duration("wa-wait", 5*time.Minute, "mode=waopen: how long to wait for QR pairing")
		fault    = flag.String("fault", "sigkill", "mode=warecover: failure mode under ablation — sigkill | graceful")
		stopVia  = flag.String("stop", "sigterm", "mode=walifecycle: how to shut the browser down — sigterm | browserclose")
		reclaim  = flag.Bool("reclaim", true, "mode=walifecycle: delete Singleton files on each boot — the variable under ablation")
		waUA     = flag.String("wa-ua", "", "explicit --user-agent; CHANGES BROWSER IDENTITY, never defaulted")
	)
	flag.Parse()
	JobTimeout = *jobTO

	// Part H/U run their own server and their own counting proxy.
	if *mode == "levels" {
		must(RunLevels(*cdpURL, *iters, *warmup, *out))
		return
	}
	if *mode == "linkprobe" {
		must(RunLinkProbe(*spaURL))
		return
	}
	if *mode == "ctxprobe" {
		must(RunCtxProbe(*cdpURL))
		return
	}
	// Fase 4 Track B: what a BrowserContext actually costs, raw CDP only.
	if *mode == "ctxcost" {
		must(RunCtxCost(4, *out))
		return
	}
	// Track J. Three separate modes on purpose: only `waopen` can display a QR.
	if *mode == "waprep" {
		must(RunWAPrep(*out))
		return
	}
	if *mode == "waopen" {
		WAUserAgent = *waUA
		must(RunWAOpen(*waWait, *out))
		return
	}
	// Fase 4C §7: o harness prova a si mesmo antes de tocar no alvo.
	if *mode == "deadlinetest" {
		must(RunDeadlineSelfTest(*out))
		return
	}
	if *mode == "watabs" {
		WAUserAgent = *waUA
		must(RunTargetTabs(*reps, *out))
		return
	}
	if *mode == "walifecycle" {
		WAUserAgent = *waUA
		ReclaimSingletons = *reclaim
		if *stopVia != "sigterm" && *stopVia != "browserclose" {
			must(fmt.Errorf("-stop deve ser sigterm ou browserclose, recebido %q", *stopVia))
		}
		LifecycleStop = *stopVia
		must(RunSessionLifecycle(*reps, *out))
		return
	}
	if *mode == "warecover" {
		WAUserAgent = *waUA
		if *fault != "sigkill" && *fault != "graceful" {
			must(fmt.Errorf("-fault deve ser sigkill ou graceful, recebido %q", *fault))
		}
		RecoveryFault = *fault
		must(RunTargetRecovery(*reps, *out))
		return
	}
	if *mode == "wasession" {
		WAUserAgent = *waUA
		must(RunWASession(*soakDur, *out))
		return
	}
	if *mode == "wacap" {
		WAUserAgent = *waUA
		must(RunWACapacity(*conc, *iters, *waChat, *out))
		return
	}
	if *mode == "ctxrepro" {
		must(RunCtxRepro(*out))
		return
	}
	// Fase 4 Track A: connection policy.
	if *mode == "connpolicy" {
		must(RunConnPolicy(*iters, *warmup, *reps, *out))
		return
	}
	// Track F: soak on the real SPA.
	if *mode == "soak" {
		must(RunSoak(*spaURL, *soakDur, *conc, *nBrowse, *out))
		return
	}
	// Track E: the real SPA.
	if *mode == "spa" {
		must(RunSPA(*spaURL, *iters, *warmup, *conc, *siteIso, *out))
		return
	}
	// Track B: browser/context/page topology.
	if *mode == "topology" {
		must(RunTopology(*iters, *warmup, *conc, *onlyTopo, *out))
		return
	}
	// Track A: one controller, profiled.
	if *mode == "prof" {
		name := strings.TrimSpace(strings.Split(*ctlNames, ",")[0])
		must(RunProf(*cdpURL, name, *iters, *warmup, *conc, *profDir, *out))
		return
	}

	srv, err := workload.Start()
	must(err)
	defer srv.Close()

	// The container reaches its own server over loopback; the browser is in the
	// same network namespace, so loopback is correct and adds no network stack
	// variance between controllers.
	pageURL := srv.URL() + "/" + *wl

	wsURL, browserVersion, err := browserWS(*cdpURL)
	must(err)

	rep := &runReport{
		Meta: map[string]any{
			"go_version":      runtime.Version(),
			"goos":            runtime.GOOS,
			"goarch":          runtime.GOARCH,
			"num_cpu":         runtime.NumCPU(),
			"workload":        *wl,
			"iters":           *iters,
			"warmup":          *warmup,
			"concurrency":     *conc,
			"page_url":        pageURL,
			"canonical_flags": CanonicalBrowserProfileV1,
			"started_utc":     time.Now().UTC().Format(time.RFC3339),
		},
		Controllers: map[string]*ctlReport{},
		Browser:     browserVersion,
		Cgroup: map[string]any{
			"memory_current_at_start": metrics.CgroupCurrent(),
			"memory_stat_at_start":    metrics.CgroupStat(),
			"memory_max":              readFileTrim("/sys/fs/cgroup/memory.max"),
			"cpu_max":                 readFileTrim("/sys/fs/cgroup/cpu.max"),
		},
	}

	for _, name := range strings.Split(*ctlNames, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		fmt.Fprintf(os.Stderr, "== %s ==\n", name)
		r, err := runController(name, wsURL, pageURL, *iters, *warmup, *conc, *chaosInt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "   FAILED: %v\n", err)
			continue
		}
		rep.Controllers[name] = r
		fmt.Fprintf(os.Stderr, "   jobs=%d fail=%d jobs/s=%.2f p50=%.1fms p95=%.1fms p99=%.1fms ctlRSS=%dKB\n",
			r.Jobs, r.Failures, r.JobsPerSec, r.Total["p50_ms"], r.Total["p95_ms"], r.Total["p99_ms"], r.CtlAfter.RSSKB)
	}

	rep.Cgroup["memory_current_at_end"] = metrics.CgroupCurrent()
	rep.Cgroup["memory_stat_at_end"] = metrics.CgroupStat()
	rep.Meta["server_requests"] = srv.Requests()

	b, _ := json.MarshalIndent(rep, "", "  ")
	if *out != "" {
		must(os.WriteFile(*out, b, 0o644))
		fmt.Fprintf(os.Stderr, "report -> %s\n", *out)
	} else {
		fmt.Println(string(b))
	}
}

func newController(name string) (Controller, error) {
	switch name {
	case "cdp-min":
		return &cdpMin{}, nil
	case "chromedp":
		return &chromeDP{}, nil
	case "chromedp-cdproto":
		return &chromeDP{lowLevel: true}, nil
	case "rod-canon":
		return &rodCtl{noDefault: true}, nil
	case "rod-default":
		return &rodCtl{}, nil
	case "rod-fastpoll":
		return &rodCtl{noDefault: true, fastPoll: true}, nil
	case "chromedp-shared-conn":
		return &chromeDP{shared: true}, nil
	case "rod-noraf":
		return &rodNoRAF{rodCtl: rodCtl{noDefault: true}}, nil
	case "rod-noraf-fastpoll":
		return &rodNoRAF{rodCtl: rodCtl{noDefault: true, fastPoll: true}}, nil
	}
	// rod-multiconn-N: N independent CDP connections, the Track A ablation.
	if n, ok := strings.CutPrefix(name, "rod-multiconn-"); ok {
		size, err := strconv.Atoi(n)
		if err != nil || size < 1 {
			return nil, fmt.Errorf("bad pool size in %q", name)
		}
		return &rodCtl{noDefault: true, poolSize: size}, nil
	}
	return nil, fmt.Errorf("unknown controller %q", name)
}

func runController(name, wsURL, pageURL string, iters, warmup, conc int, chaosEvery time.Duration) (*ctlReport, error) {
	ctl, err := newController(name)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()

	// Settle the Go runtime before the "before" snapshot so the delta reflects
	// the controller, not leftover allocations from the previous one.
	runtime.GC()
	time.Sleep(300 * time.Millisecond)
	before := metrics.Self()
	cgBefore := metrics.CgroupCurrent()

	t0 := time.Now()
	if err := ctl.Connect(ctx, wsURL); err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	connectMS := float64(time.Since(t0).Microseconds()) / 1000.0
	defer ctl.Close()

	warm := NewRec()
	for i := 0; i < warmup; i++ {
		if err := ctl.RunJob(ctx, pageURL, warm); err != nil {
			return nil, fmt.Errorf("warmup: %w", err)
		}
	}

	// Chaos starts only for the MEASURED phase: crashing during warmup would
	// mix recovery cost into the baseline the warmup exists to establish.
	var chaos *Chaos
	if chaosEvery > 0 {
		var cerr error
		chaos, cerr = StartChaos(ctx, wsURL, chaosEvery)
		if cerr != nil {
			return nil, fmt.Errorf("chaos: %w", cerr)
		}
	}

	rec := NewRec()
	var fails int
	var mu sync.Mutex
	start := time.Now()
	if conc <= 1 {
		for i := 0; i < iters; i++ {
			jt := time.Now()
			if err := ctl.RunJob(ctx, pageURL, rec); err != nil {
				fails++
				continue
			}
			rec.Total.Add(time.Since(jt))
		}
	} else {
		sem := make(chan struct{}, conc)
		var wg sync.WaitGroup
		for i := 0; i < iters; i++ {
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				jt := time.Now()
				if err := ctl.RunJob(ctx, pageURL, rec); err != nil {
					mu.Lock()
					fails++
					mu.Unlock()
					return
				}
				mu.Lock()
				rec.Total.Add(time.Since(jt))
				mu.Unlock()
			}()
		}
		wg.Wait()
	}
	wall := time.Since(start)
	var crashes int64
	if chaos != nil {
		crashes = chaos.Stop()
	}

	after := metrics.Self()
	ops := map[string]map[string]float64{}
	names := make([]string, 0, len(rec.Ops))
	for k := range rec.Ops {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		ops[k] = rec.Ops[k].Stats()
	}

	ok := iters - fails
	return &ctlReport{
		Jobs:          ok,
		Failures:      fails,
		WallMS:        wall.Milliseconds(),
		JobsPerSec:    float64(ok) / wall.Seconds(),
		ConnectMS:     connectMS,
		Total:         rec.Total.Stats(),
		Ops:           ops,
		CtlBefore:     before,
		CtlAfter:      after,
		CgroupDeltaB:  metrics.CgroupCurrent() - cgBefore,
		RendererKills: crashes,
	}, nil
}

// browserWS resolves the browser-level WebSocket URL and records the exact
// browser build, so a report can never be compared against one from a
// different Chromium.
func browserWS(base string) (string, map[string]any, error) {
	out, err := exec.Command("curl", "-s", "--max-time", "10", base+"/json/version").Output()
	if err != nil {
		return "", nil, fmt.Errorf("cdp endpoint %s: %w", base, err)
	}
	var v map[string]any
	if err := json.Unmarshal(out, &v); err != nil {
		return "", nil, fmt.Errorf("cdp endpoint %s returned %q", base, string(out))
	}
	ws, _ := v["webSocketDebuggerUrl"].(string)
	if ws == "" {
		return "", nil, fmt.Errorf("no webSocketDebuggerUrl in %s", string(out))
	}
	return ws, v, nil
}

func readFileTrim(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}
