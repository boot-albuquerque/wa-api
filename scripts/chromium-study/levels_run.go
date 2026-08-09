package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"runtime"

	"github.com/chromedp/chromedp"

	"chromium-study/workload"
)

// levelReport is one abstraction level measured on the hostile page.
type levelReport struct {
	Level            string                        `json:"level"`
	Jobs             int                           `json:"jobs"`
	Errors           int                           `json:"errors"`
	FalseSuccesses   int                           `json:"false_successes"`
	CorrectJobs      int                           `json:"correct_jobs"`
	JobsPerSec       float64                       `json:"jobs_per_sec"`
	CorrectPerSec    float64                       `json:"correct_jobs_per_sec"`
	CDPSentPerJob    float64                       `json:"cdp_commands_per_job"`
	CDPRecvPerJob    float64                       `json:"cdp_messages_in_per_job"`
	BytesPerJob      float64                       `json:"cdp_bytes_per_job"`
	Total            map[string]float64            `json:"job_total"`
	Ops              map[string]map[string]float64 `json:"ops"`
	AllocBytesPerJob uint64                        `json:"alloc_bytes_per_job"`
	AllocsPerJob     uint64                        `json:"allocs_per_job"`
	GCCycles         uint32                        `json:"gc_cycles"`
}

// RunLevels executes Part H and Part U together.
//
// Both must be measured in the same run: throughput without correctness would
// reward a level for skipping work, and correctness without throughput would
// hide the cost of the guarantees. The hostile page reports ground truth, so a
// job that returned nil but did not do the work is counted as a FALSE SUCCESS,
// not as a success.
func RunLevels(cdpBase string, iters, warmup int, outPath string) error {
	srv, err := workload.Start()
	if err != nil {
		return err
	}
	defer srv.Close()
	pageURL := srv.URL() + "/h"
	if os.Getenv("OVERLAY") == "1" {
		pageURL += "?overlay=1"
	}

	ctx := context.Background()
	proxy, err := StartProxy(ctx, cdpBase)
	if err != nil {
		return fmt.Errorf("proxy: %w", err)
	}
	defer proxy.Close()

	levels := []struct {
		name  string
		level int
	}{
		{"L0-raw-evaluate", L0},
		{"L1-cdproto-equivalent", L1},
		{"L2-chromedp-highlevel", L2},
	}

	out := map[string]any{
		"meta": map[string]any{
			"page":        pageURL,
			"iters":       iters,
			"warmup":      warmup,
			"proxy_ws":    proxy.WS(),
			"started_utc": time.Now().UTC().Format(time.RFC3339),
			"cgroup_max":  readFileTrim("/sys/fs/cgroup/memory.max"),
			"cpu_max":     readFileTrim("/sys/fs/cgroup/cpu.max"),
		},
		"levels": map[string]*levelReport{},
	}

	for _, lv := range levels {
		fmt.Fprintf(os.Stderr, "== %s ==\n", lv.name)
		rep, err := runOneLevel(ctx, proxy, pageURL, lv.name, lv.level, iters, warmup)
		if err != nil {
			fmt.Fprintf(os.Stderr, "   FAILED: %v\n", err)
			continue
		}
		out["levels"].(map[string]*levelReport)[lv.name] = rep
		fmt.Fprintf(os.Stderr,
			"   jobs=%d erros=%d falso_sucesso=%d corretos=%d | %.2f jobs/s (%.2f corretos/s) | "+
				"p50=%.1f p95=%.1f | CDP cmd/job=%.1f | allocs/job=%d\n",
			rep.Jobs, rep.Errors, rep.FalseSuccesses, rep.CorrectJobs, rep.JobsPerSec,
			rep.CorrectPerSec, rep.Total["p50_ms"], rep.Total["p95_ms"], rep.CDPSentPerJob,
			rep.AllocsPerJob)
	}

	b, _ := json.MarshalIndent(out, "", "  ")
	if outPath != "" {
		return os.WriteFile(outPath, b, 0o644)
	}
	fmt.Println(string(b))
	return nil
}

func runOneLevel(ctx context.Context, proxy *CountingProxy, pageURL, name string, level, iters, warmup int) (*levelReport, error) {
	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(ctx, proxy.WS())
	defer cancelAlloc()

	runJob := func(rec *Rec) (JobOutcome, error) {
		tabCtx, tabCancel := chromedp.NewContext(allocCtx)
		defer tabCancel()
		opCtx, opCancel := context.WithTimeout(tabCtx, JobTimeout)
		defer opCancel()
		return runHostileJob(opCtx, level, pageURL, rec)
	}

	warm := NewRec()
	for i := 0; i < warmup; i++ {
		if _, err := runJob(warm); err != nil {
			return nil, fmt.Errorf("warmup: %w", err)
		}
	}

	// Counters reset AFTER warmup so connection setup and first-run costs do
	// not land in the per-job averages.
	proxy.Reset()
	gcBefore, allocBefore, bytesBefore := readMem()

	rec := NewRec()
	var errs, false_, correct int
	start := time.Now()
	for i := 0; i < iters; i++ {
		jt := time.Now()
		outcome, err := runJob(rec)
		if err != nil {
			errs++
			continue
		}
		rec.Total.Add(time.Since(jt))
		if outcome.FalseSuccess {
			false_++
		} else {
			correct++
		}
	}
	wall := time.Since(start)
	gcAfter, allocAfter, bytesAfter := readMem()

	ops := map[string]map[string]float64{}
	for k, v := range rec.Ops {
		ops[k] = v.Stats()
	}
	n := float64(iters)
	return &levelReport{
		Level:            name,
		Jobs:             iters - errs,
		Errors:           errs,
		FalseSuccesses:   false_,
		CorrectJobs:      correct,
		JobsPerSec:       float64(iters-errs) / wall.Seconds(),
		CorrectPerSec:    float64(correct) / wall.Seconds(),
		CDPSentPerJob:    float64(proxy.Sent()) / n,
		CDPRecvPerJob:    float64(proxy.Recv()) / n,
		BytesPerJob:      float64(proxy.BytesOut()+proxy.BytesIn()) / n,
		Total:            rec.Total.Stats(),
		Ops:              ops,
		AllocBytesPerJob: (bytesAfter - bytesBefore) / uint64(iters),
		AllocsPerJob:     (allocAfter - allocBefore) / uint64(iters),
		GCCycles:         gcAfter - gcBefore,
	}, nil
}

// readMem samples the Go allocator directly. GC and allocation pressure are
// part of the cost of an abstraction level, and only MemStats reports them.
func readMem() (gc uint32, mallocs, bytes uint64) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.NumGC, ms.Mallocs, ms.TotalAlloc
}
