package main

// Track F — soak.
//
// The question is not "did it survive" but "what is the slope". A run that ends
// without an OOM proves nothing if memory was climbing the whole time and the
// run simply ended first, so this mode records a time series and the report
// quotes growth per hour, not a final value.
//
// It drives the REAL SPA at a fixed concurrency, on the topology Track B
// selected, and samples every 15 s:
//   - cgroup memory.current / peak / anon / file, and memory.events
//   - cgroup cpu usage and CFS throttling counters
//   - Go heap, goroutines, OS threads, open FDs
//   - Chromium process and renderer counts
//   - completed / failed / false-success counts so throughput drift is visible
//     against the same clock as the resource series

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/chromedp/chromedp"

	"chromium-study/metrics"
)

type soakSample struct {
	TSec          float64          `json:"t_sec"`
	MemCurrentMB  float64          `json:"memory_current_mb"`
	MemPeakMB     float64          `json:"memory_peak_mb"`
	MemStat       map[string]int64 `json:"memory_stat"`
	MemEvents     map[string]int64 `json:"memory_events"`
	CPUCoreSec    float64          `json:"cpu_core_seconds_cumulative"`
	NrThrottled   int64            `json:"nr_throttled_cumulative"`
	GoHeapMB      float64          `json:"go_heap_mb"`
	GoNumGC       uint32           `json:"go_num_gc"`
	Goroutines    int              `json:"goroutines"`
	Threads       int              `json:"os_threads"`
	FDs           int              `json:"open_fds"`
	Procs         map[string]int   `json:"chromium_procs"`
	Done          int              `json:"jobs_done"`
	Failed        int              `json:"jobs_failed"`
	FalseSuccess  int              `json:"false_successes"`
	BrowserResets int              `json:"browser_restarts"`
}

type soakReport struct {
	Meta    map[string]any `json:"meta"`
	Samples []soakSample   `json:"samples"`
	Final   soakSample     `json:"final"`
	// Slopes are computed here, not left to the reader, because "stable" and
	// "grew 40 MB/h" look identical in a list of samples.
	MemSlopeMBPerHour  float64 `json:"memory_slope_mb_per_hour"`
	HeapSlopeMBPerHour float64 `json:"go_heap_slope_mb_per_hour"`
	FDSlopePerHour     float64 `json:"fd_slope_per_hour"`
	GoroSlopePerHour   float64 `json:"goroutine_slope_per_hour"`
	RendSlopePerHour   float64 `json:"renderer_slope_per_hour"`
	ThroughputFirst    float64 `json:"throughput_first_quarter"`
	ThroughputLast     float64 `json:"throughput_last_quarter"`
	P50First           float64 `json:"p50_first_quarter_ms"`
	P50Last            float64 `json:"p50_last_quarter_ms"`
}

// RunSoak drives the SPA for a wall-clock duration at fixed concurrency.
func RunSoak(spaURL string, dur time.Duration, conc, browsers int, outPath string) error {
	runtime.GC()
	time.Sleep(500 * time.Millisecond)

	bs, err := launchBrowsers(browsers, CanonicalBrowserProfileV1, 9600)
	if err != nil {
		return err
	}
	defer killAll(bs)
	time.Sleep(3 * time.Second)

	ctx := context.Background()
	allocs := make([]context.Context, len(bs))
	for i, b := range bs {
		a, cancel := chromedp.NewRemoteAllocator(ctx, b.WSURL)
		defer cancel()
		allocs[i] = a
	}
	slots := make([]slotEnv, conc)
	for i := 0; i < conc; i++ {
		p, cancel := chromedp.NewContext(allocs[i*len(bs)/conc])
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

	rep := &soakReport{Meta: map[string]any{
		"url": spaURL, "duration": dur.String(), "concurrency": conc,
		"browsers": browsers, "num_cpu": runtime.NumCPU(), "goarch": runtime.GOARCH,
		"cpu_max": readFileTrim("/sys/fs/cgroup/cpu.max"),
		"memory_max": readFileTrim("/sys/fs/cgroup/memory.max"),
		"started_utc": time.Now().UTC().Format(time.RFC3339),
	}}

	var mu sync.Mutex
	var done, failed, falseS int
	// Per-quarter latency buckets: the drift question needs early and late
	// distributions, and a single accumulator over the whole run cannot show it.
	quarters := make([]*Lat, 4)
	for i := range quarters {
		quarters[i] = &Lat{}
	}
	quarterDone := make([]int, 4)

	cpu0 := metrics.CPUStat()
	start := time.Now()
	deadline := start.Add(dur)
	stop := make(chan struct{})

	var wgS sync.WaitGroup
	wgS.Add(1)
	go func() {
		defer wgS.Done()
		tick := time.NewTicker(15 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				cpu := metrics.CPUStat()
				var ms runtime.MemStats
				runtime.ReadMemStats(&ms)
				self := metrics.Self()
				mu.Lock()
				s := soakSample{
					TSec:         time.Since(start).Seconds(),
					MemCurrentMB: float64(metrics.CgroupCurrent()) / 1048576,
					MemPeakMB:    float64(metrics.CgroupPeak()) / 1048576,
					MemStat:      metrics.CgroupStat(),
					MemEvents:    metrics.CgroupEvents(),
					CPUCoreSec:   float64(cpu["usage_usec"]-cpu0["usage_usec"]) / 1e6,
					NrThrottled:  cpu["nr_throttled"] - cpu0["nr_throttled"],
					GoHeapMB:     float64(ms.HeapAlloc) / 1048576,
					GoNumGC:      ms.NumGC,
					Goroutines:   self.Goroutines, Threads: self.Threads, FDs: self.FDs,
					Procs: metrics.ChromiumProcs(),
					Done:  done, Failed: failed, FalseSuccess: falseS,
				}
				rep.Samples = append(rep.Samples, s)
				mu.Unlock()
				fmt.Fprintf(os.Stderr, "  t=%5.0fs done=%4d fail=%3d false=%3d mem=%6.0fMB heap=%5.1fMB gor=%4d fd=%4d rend=%3d\n",
					s.TSec, s.Done, s.Failed, s.FalseSuccess, s.MemCurrentMB, s.GoHeapMB,
					s.Goroutines, s.FDs, s.Procs["renderer"])
			}
		}
	}()

	rec := NewRec()
	var wg sync.WaitGroup
	for i := 0; i < conc; i++ {
		wg.Add(1)
		go func(slot slotEnv) {
			defer wg.Done()
			for time.Now().Before(deadline) {
				jt := time.Now()
				tabCtx, tabCancel := chromedp.NewContext(slot.parent)
				opCtx, opCancel := context.WithTimeout(tabCtx, JobTimeout)
				r := runSPAJob(opCtx, spaURL, rec)
				opCancel()
				tabCancel()
				el := time.Since(jt)
				q := int(time.Since(start) * 4 / dur)
				if q > 3 {
					q = 3
				}
				mu.Lock()
				switch {
				case r.Err != nil:
					failed++
				case r.FalseSuccess:
					falseS++
					done++
				default:
					done++
				}
				if r.Err == nil {
					quarters[q].Add(el)
					quarterDone[q]++
				}
				mu.Unlock()
			}
		}(slots[i])
	}
	wg.Wait()
	close(stop)
	wgS.Wait()

	if len(rep.Samples) > 0 {
		rep.Final = rep.Samples[len(rep.Samples)-1]
	}
	// Least-squares slope over the samples, in units per hour.
	slope := func(get func(soakSample) float64) float64 {
		n := float64(len(rep.Samples))
		if n < 3 {
			return 0
		}
		var sx, sy, sxy, sxx float64
		for _, s := range rep.Samples {
			x := s.TSec / 3600
			y := get(s)
			sx += x
			sy += y
			sxy += x * y
			sxx += x * x
		}
		den := n*sxx - sx*sx
		if den == 0 {
			return 0
		}
		return (n*sxy - sx*sy) / den
	}
	rep.MemSlopeMBPerHour = slope(func(s soakSample) float64 { return s.MemCurrentMB })
	rep.HeapSlopeMBPerHour = slope(func(s soakSample) float64 { return s.GoHeapMB })
	rep.FDSlopePerHour = slope(func(s soakSample) float64 { return float64(s.FDs) })
	rep.GoroSlopePerHour = slope(func(s soakSample) float64 { return float64(s.Goroutines) })
	rep.RendSlopePerHour = slope(func(s soakSample) float64 { return float64(s.Procs["renderer"]) })

	qsec := dur.Seconds() / 4
	rep.ThroughputFirst = float64(quarterDone[0]) / qsec
	rep.ThroughputLast = float64(quarterDone[3]) / qsec
	rep.P50First = quarters[0].Stats()["p50_ms"]
	rep.P50Last = quarters[3].Stats()["p50_ms"]

	fmt.Fprintf(os.Stderr,
		"SOAK %s: done=%d fail=%d false=%d | mem %+.1f MB/h heap %+.2f MB/h fd %+.1f/h gor %+.1f/h rend %+.2f/h | "+
			"throughput %.2f -> %.2f jobs/s | p50 %.0f -> %.0f ms\n",
		dur, done, failed, falseS, rep.MemSlopeMBPerHour, rep.HeapSlopeMBPerHour,
		rep.FDSlopePerHour, rep.GoroSlopePerHour, rep.RendSlopePerHour,
		rep.ThroughputFirst, rep.ThroughputLast, rep.P50First, rep.P50Last)

	b, _ := json.MarshalIndent(rep, "", "  ")
	if outPath != "" {
		return os.WriteFile(outPath, b, 0o644)
	}
	fmt.Println(string(b))
	return nil
}
