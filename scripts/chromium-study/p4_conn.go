package main

// Track A (Fase 4) — ConnectionPolicy, isolada.
//
// A Fase 3 registrou "uma conexão CDP por slot" como política, e essa decisão
// não estava sustentada pelos seus próprios dados: as duas médias comparadas
// (14,44–20,42 j/s contra 19,82–20,70 j/s) se sobrepõem. O que aquele
// experimento realmente refutou foi a hipótese de que a contagem de conexões
// explicava o teto do rod — não que uma política fosse melhor que a outra.
//
// Aqui a pergunta é só uma, com tudo o mais fixo: 2 Chromium, 4 slots, mesmo
// perfil, mesmo workload, mesmo número de jobs, N repetições independentes.
//
//	C1  SHARED_PER_BROWSER   uma conexão por browser; 2 slots dividem cada uma
//	C2  PER_SLOT             uma conexão por slot
//	C3  HYBRID               uma conexão de controle (ciclo de vida de target)
//	                         + uma conexão de driving compartilhada por browser
//
// Bytes e mensagens CDP são contados no fio, por um proxy por browser, porque
// nenhuma das três configurações expõe isso e instrumentar a biblioteca mediria
// coisas diferentes em cada arm.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"

	"chromium-study/metrics"
	"chromium-study/workload"
)

type connPolicy string

const (
	sharedPerBrowser connPolicy = "C1-SHARED_PER_BROWSER"
	perSlot          connPolicy = "C2-PER_SLOT"
	hybrid           connPolicy = "C3-HYBRID"
)

type connRun struct {
	Policy        connPolicy         `json:"policy"`
	Rep           int                `json:"rep"`
	Correct       int                `json:"correct_jobs"`
	FalseSuccess  int                `json:"false_successes"`
	Failures      int                `json:"failures"`
	CorrectPerSec float64            `json:"correct_jobs_per_sec"`
	Total         map[string]float64 `json:"job_total"`
	CPUCoreSec    float64            `json:"cpu_core_seconds"`
	MemCurrentMB  float64            `json:"memory_current_mb"`
	Connections   int                `json:"cdp_connections"`
	CDPMsgOut     int64              `json:"cdp_messages_out"`
	CDPMsgIn      int64              `json:"cdp_messages_in"`
	CDPBytes      int64              `json:"cdp_bytes_total"`
	CDPBytesPerJb float64            `json:"cdp_bytes_per_job"`
	AllocsPerJob  uint64             `json:"allocs_per_job"`
	AllocMBPerJob float64            `json:"alloc_mb_per_job"`
	Goroutines    int                `json:"goroutines_after"`
	Threads       int                `json:"os_threads_after"`
	FDs           int                `json:"open_fds_after"`
	ConnFailures  int                `json:"connection_failures"`
}

type connReport struct {
	Meta    map[string]any            `json:"meta"`
	Runs    []connRun                 `json:"runs"`
	Summary map[string]map[string]any `json:"summary"`
}

// RunConnPolicy executes every policy `reps` times, rotating the order between
// repetitions so that a warm cache or a hot CPU cannot be charged to whichever
// policy happens to run first.
func RunConnPolicy(iters, warmup, reps int, outPath string) error {
	srv, err := workload.Start()
	if err != nil {
		return err
	}
	defer srv.Close()
	pageURL := srv.URL() + "/h"

	policies := []connPolicy{sharedPerBrowser, perSlot, hybrid}
	rep := &connReport{
		Meta: map[string]any{
			"browsers": 2, "slots": 4, "iters_per_run": iters, "warmup": warmup,
			"reps": reps, "num_cpu": runtime.NumCPU(), "goarch": runtime.GOARCH,
			"cpu_max": readFileTrim("/sys/fs/cgroup/cpu.max"),
			"memory_max": readFileTrim("/sys/fs/cgroup/memory.max"),
			"workload":    "hostile page, L2 chromedp, ground truth verified",
			"started_utc": time.Now().UTC().Format(time.RFC3339),
		},
		Summary: map[string]map[string]any{},
	}

	for r := 0; r < reps; r++ {
		order := make([]connPolicy, len(policies))
		copy(order, policies)
		// Deterministic rotation, not randomness: Math.random is unavailable in
		// this harness by policy, and a rotation gives every arm every position.
		for i := range order {
			order[i] = policies[(i+r)%len(policies)]
		}
		for _, p := range order {
			fmt.Fprintf(os.Stderr, "== %s rep %d ==\n", p, r+1)
			run, err := runConnArm(p, pageURL, iters, warmup, r+1)
			if err != nil {
				fmt.Fprintf(os.Stderr, "   FAILED: %v\n", err)
				continue
			}
			rep.Runs = append(rep.Runs, *run)
			fmt.Fprintf(os.Stderr,
				"   ok=%d false=%d fail=%d | %.2f corr/s | p50=%.0f p95=%.0f p99=%.0f | "+
					"cpu=%.1fs | conns=%d | cdp=%.0fB/job | allocs=%d/job\n",
				run.Correct, run.FalseSuccess, run.Failures, run.CorrectPerSec,
				run.Total["p50_ms"], run.Total["p95_ms"], run.Total["p99_ms"],
				run.CPUCoreSec, run.Connections, run.CDPBytesPerJb, run.AllocsPerJob)
		}
	}

	// Aggregate per policy: median and range, not mean alone. One slow run
	// distorts a mean of five, and the decision rule for this track is a
	// 5–10% threshold that a distorted mean could cross by itself.
	for _, p := range policies {
		var tp, p99, cpu, bytes []float64
		for _, r := range rep.Runs {
			if r.Policy != p {
				continue
			}
			tp = append(tp, r.CorrectPerSec)
			p99 = append(p99, r.Total["p99_ms"])
			cpu = append(cpu, r.CPUCoreSec)
			bytes = append(bytes, r.CDPBytesPerJb)
		}
		if len(tp) == 0 {
			continue
		}
		rep.Summary[string(p)] = map[string]any{
			"n":                    len(tp),
			"throughput_median":    median(tp),
			"throughput_min":       minOf(tp),
			"throughput_max":       maxOf(tp),
			"p99_median_ms":        median(p99),
			"cpu_core_sec_median":  median(cpu),
			"cdp_bytes_per_job_med": median(bytes),
		}
	}

	b, _ := json.MarshalIndent(rep, "", "  ")
	if outPath != "" {
		return os.WriteFile(outPath, b, 0o644)
	}
	fmt.Println(string(b))
	return nil
}

func runConnArm(policy connPolicy, pageURL string, iters, warmup, repNo int) (*connRun, error) {
	const nBrowsers, nSlots = 2, 4

	browsers, err := launchBrowsers(nBrowsers, CanonicalBrowserProfileV1, 9700)
	if err != nil {
		return nil, err
	}
	defer killAll(browsers)
	time.Sleep(2 * time.Second)

	ctx := context.Background()

	// One counting proxy per browser: every CDP byte of every arm crosses it,
	// so the three policies are measured by the same instrument.
	proxies := make([]*CountingProxy, nBrowsers)
	for i, b := range browsers {
		px, err := StartProxy(ctx, b.HTTPBase)
		if err != nil {
			return nil, fmt.Errorf("proxy %d: %w", i, err)
		}
		defer px.Close()
		proxies[i] = px
	}

	allocs := make([]context.Context, nBrowsers)
	for i := range browsers {
		a, cancel := chromedp.NewRemoteAllocator(ctx, proxies[i].WS())
		defer cancel()
		allocs[i] = a
	}

	var connFailures int
	var conns int
	slots := make([]slotEnv, nSlots)
	var admins []*adminConn

	switch policy {
	case perSlot:
		// Each slot derives straight from the allocator, so each dials.
		for s := 0; s < nSlots; s++ {
			bi := s * nBrowsers / nSlots
			p, cancel := chromedp.NewContext(allocs[bi])
			if err := chromedp.Run(p); err != nil {
				cancel()
				connFailures++
				return nil, fmt.Errorf("slot %d: %w", s, err)
			}
			defer cancel()
			slots[s] = slotEnv{parent: p}
			conns++
		}

	case sharedPerBrowser:
		// One connection per browser, allocated once; slots inherit its Browser.
		perBrowser := make([]context.Context, nBrowsers)
		for i := range browsers {
			p, cancel := chromedp.NewContext(allocs[i])
			if err := chromedp.Run(p); err != nil {
				cancel()
				connFailures++
				return nil, fmt.Errorf("browser %d conn: %w", i, err)
			}
			defer cancel()
			perBrowser[i] = p
			conns++
		}
		for s := 0; s < nSlots; s++ {
			slots[s] = slotEnv{parent: perBrowser[s*nBrowsers/nSlots]}
		}

	case hybrid:
		// Control plane and data plane on separate connections: target lifecycle
		// on a raw CDP connection, driving on one shared chromedp connection per
		// browser. This is the shape the Fase 3 BrowserContext workaround forced,
		// measured here on its own merits rather than as a side effect.
		perBrowser := make([]context.Context, nBrowsers)
		for i := range browsers {
			a, err := dialAdmin(browsers[i].WSURL)
			if err != nil {
				connFailures++
				return nil, fmt.Errorf("admin %d: %w", i, err)
			}
			defer a.c.Close()
			admins = append(admins, a)
			conns++

			p, cancel := chromedp.NewContext(allocs[i])
			if err := chromedp.Run(p); err != nil {
				cancel()
				connFailures++
				return nil, fmt.Errorf("driving %d: %w", i, err)
			}
			defer cancel()
			perBrowser[i] = p
			conns++
		}
		for s := 0; s < nSlots; s++ {
			bi := s * nBrowsers / nSlots
			slots[s] = slotEnv{parent: perBrowser[bi], admin: admins[bi]}
		}
	}

	rec := NewRec()
	runOne := func(slot slotEnv) (JobOutcome, error) {
		var opts []chromedp.ContextOption
		if slot.admin != nil {
			tid, err := slot.admin.createTarget("")
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

	for i := 0; i < warmup; i++ {
		if _, err := runOne(slots[i%nSlots]); err != nil {
			return nil, fmt.Errorf("warmup: %w", err)
		}
	}

	runtime.GC()
	time.Sleep(300 * time.Millisecond)
	for _, px := range proxies {
		px.Reset()
	}
	cpu0 := metrics.CPUStat()
	var ms0 runtime.MemStats
	runtime.ReadMemStats(&ms0)

	var mu sync.Mutex
	var ok, false_, fails int
	jobCh := make(chan int, iters)
	for i := 0; i < iters; i++ {
		jobCh <- i
	}
	close(jobCh)

	start := time.Now()
	var wg sync.WaitGroup
	for s := 0; s < nSlots; s++ {
		wg.Add(1)
		go func(slot slotEnv) {
			defer wg.Done()
			for range jobCh {
				jt := time.Now()
				outcome, err := runOne(slot)
				mu.Lock()
				switch {
				case err != nil:
					fails++
				case outcome.FalseSuccess:
					false_++
					rec.Total.Add(time.Since(jt))
				default:
					ok++
					rec.Total.Add(time.Since(jt))
				}
				mu.Unlock()
			}
		}(slots[s])
	}
	wg.Wait()
	wall := time.Since(start)

	cpu1 := metrics.CPUStat()
	var ms1 runtime.MemStats
	runtime.ReadMemStats(&ms1)
	self := metrics.Self()

	var msgOut, msgIn, bytesTotal int64
	for _, px := range proxies {
		msgOut += px.Sent()
		msgIn += px.Recv()
		bytesTotal += px.BytesOut() + px.BytesIn()
	}
	n := float64(iters)
	return &connRun{
		Policy: policy, Rep: repNo,
		Correct: ok, FalseSuccess: false_, Failures: fails,
		CorrectPerSec: float64(ok) / wall.Seconds(),
		Total:         rec.Total.Stats(),
		CPUCoreSec:    float64(cpu1["usage_usec"]-cpu0["usage_usec"]) / 1e6,
		MemCurrentMB:  float64(metrics.CgroupCurrent()) / 1048576,
		Connections:   conns,
		CDPMsgOut:     msgOut, CDPMsgIn: msgIn, CDPBytes: bytesTotal,
		CDPBytesPerJb: float64(bytesTotal) / n,
		AllocsPerJob:  (ms1.Mallocs - ms0.Mallocs) / uint64(iters),
		AllocMBPerJob: float64(ms1.TotalAlloc-ms0.TotalAlloc) / 1048576 / n,
		Goroutines:    self.Goroutines, Threads: self.Threads, FDs: self.FDs,
		ConnFailures:  connFailures,
	}, nil
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[len(s)/2]
}

func minOf(v []float64) float64 {
	m := v[0]
	for _, x := range v {
		if x < m {
			m = x
		}
	}
	return m
}

func maxOf(v []float64) float64 {
	m := v[0]
	for _, x := range v {
		if x > m {
			m = x
		}
	}
	return m
}
