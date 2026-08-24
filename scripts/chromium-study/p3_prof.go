package main

// Track A — causal profiling of the concurrency ceiling.
//
// Phase 2 established WHAT happens (rod stops scaling around 2 jobs/s while
// chromedp reaches 6+ on the same browser). This track exists to establish WHY,
// and the bar set for it is explicit: a profile alone is correlation. A stack
// holding a lock is not a cause until removing that serialization changes the
// throughput in the predicted direction.
//
// So this file produces two things per run:
//
//   1. profiles — CPU, heap, allocs, block, mutex, goroutine — captured only
//      during the MEASURED phase, never during warmup, because connection setup
//      allocates and would otherwise dominate the allocation profiles;
//   2. a goroutine census sampled on a timer, which is the only artifact that
//      shows a queue GROWING rather than being deep at one instant.
//
// The ablation itself is not a code change inside rod. It is a topology change
// applied identically to both libraries: N independent CDP connections instead
// of one shared connection. If rod recovers with N connections and chromedp is
// indifferent to them, the serialization is per-connection. That is a
// falsifiable prediction, and the control arm exists so a general "more
// connections is faster" effect cannot be mistaken for a rod-specific one.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"time"

	"context"

	"chromium-study/metrics"
	"chromium-study/workload"
)

// profSample is one instant of the controller process's internal state.
type profSample struct {
	TMS        int64          `json:"t_ms"`
	Goroutines int            `json:"goroutines"`
	Threads    int            `json:"threads"`
	HeapMB     float64        `json:"heap_mb"`
	CgroupMB   float64        `json:"cgroup_mb"`
	Done       int            `json:"jobs_done"`
	TopStacks  map[string]int `json:"top_stacks,omitempty"`
}

type profReport struct {
	Meta       map[string]any `json:"meta"`
	Samples    []profSample   `json:"samples"`
	Jobs       int            `json:"jobs"`
	Failures   int            `json:"failures"`
	WallMS     int64          `json:"wall_ms"`
	JobsPerSec float64        `json:"jobs_per_sec"`
	Total      map[string]float64
	Ops        map[string]map[string]float64 `json:"ops"`
	BlockTop   []stackCost                   `json:"block_top"`
	MutexTop   []stackCost                   `json:"mutex_top"`
	GoroTop    []stackCost                   `json:"goroutine_top"`
}

// stackCost is one entry of a debug=1 pprof text profile, reduced to the two
// numbers that matter: how much (cycles for block/mutex, count for goroutine)
// and where.
type stackCost struct {
	Value int64    `json:"value"`
	Count int64    `json:"count,omitempty"`
	Stack []string `json:"stack"`
}

// RunProf executes Track A for one controller.
func RunProf(cdpBase, ctlName string, iters, warmup, conc int, profDir, outPath string) error {
	if err := os.MkdirAll(profDir, 0o755); err != nil {
		return err
	}

	// Both rates at 1 mean "record every event". That is expensive and it
	// perturbs the measurement — which is acceptable here BECAUSE this track
	// reports mechanism, not throughput. The throughput numbers quoted as
	// results come from the unprofiled runs of Phase 2 and from the ablation
	// arm below, which runs with profiling off.
	runtime.SetMutexProfileFraction(1)
	runtime.SetBlockProfileRate(1)
	defer runtime.SetMutexProfileFraction(0)
	defer runtime.SetBlockProfileRate(0)

	srv, err := workload.Start()
	if err != nil {
		return err
	}
	defer srv.Close()
	pageURL := srv.URL() + "/a"

	wsURL, browserVersion, err := browserWS(cdpBase)
	if err != nil {
		return err
	}

	ctl, err := newController(ctlName)
	if err != nil {
		return err
	}
	ctx := context.Background()
	if err := ctl.Connect(ctx, wsURL); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer ctl.Close()

	warm := NewRec()
	for i := 0; i < warmup; i++ {
		if err := ctl.RunJob(ctx, pageURL, warm); err != nil {
			return fmt.Errorf("warmup: %w", err)
		}
	}

	// Reset the accumulating profiles so warmup contention is not attributed to
	// the measured phase. Go has no explicit reset, but reading a profile does
	// not clear it, so instead the sample counters are snapshotted and the
	// deltas are what gets reported for block/mutex.
	runtime.GC()

	cpuFile, err := os.Create(filepath.Join(profDir, "cpu.pb.gz"))
	if err != nil {
		return err
	}
	if err := pprof.StartCPUProfile(cpuFile); err != nil {
		return err
	}

	rep := &profReport{
		Meta: map[string]any{
			"controller":  ctlName,
			"concurrency": conc,
			"iters":       iters,
			"num_cpu":     runtime.NumCPU(),
			"goarch":      runtime.GOARCH,
			"browser":     browserVersion,
			"started_utc": time.Now().UTC().Format(time.RFC3339),
			"cpu_max":     readFileTrim("/sys/fs/cgroup/cpu.max"),
			"memory_max":  readFileTrim("/sys/fs/cgroup/memory.max"),
			"profiled":    true,
		},
	}

	var done, fails int64
	var mu sync.Mutex
	stop := make(chan struct{})
	var wgSample sync.WaitGroup
	wgSample.Add(1)
	start := time.Now()
	go func() {
		defer wgSample.Done()
		tick := time.NewTicker(250 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				s := metrics.Self()
				var ms runtime.MemStats
				runtime.ReadMemStats(&ms)
				mu.Lock()
				d := done
				mu.Unlock()
				smp := profSample{
					TMS:        time.Since(start).Milliseconds(),
					Goroutines: runtime.NumGoroutine(),
					Threads:    s.Threads,
					HeapMB:     float64(ms.HeapAlloc) / 1048576,
					CgroupMB:   float64(metrics.CgroupCurrent()) / 1048576,
					Done:       int(d),
				}
				// A census every fourth sample: parsing the goroutine profile is
				// itself work, and doing it every tick would show up in the CPU
				// profile as the harness rather than the subject.
				if len(rep.Samples)%4 == 0 {
					smp.TopStacks = goroutineCensus()
				}
				rep.Samples = append(rep.Samples, smp)
			}
		}
	}()

	rec := NewRec()
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
			done++
			rec.Total.Add(time.Since(jt))
			mu.Unlock()
		}()
	}
	wg.Wait()
	wall := time.Since(start)
	close(stop)
	wgSample.Wait()
	pprof.StopCPUProfile()
	_ = cpuFile.Close()

	for _, name := range []string{"heap", "allocs", "block", "mutex", "goroutine"} {
		f, err := os.Create(filepath.Join(profDir, name+".pb.gz"))
		if err != nil {
			return err
		}
		if err := pprof.Lookup(name).WriteTo(f, 0); err != nil {
			return err
		}
		_ = f.Close()
	}

	rep.BlockTop = topStacks("block", 12)
	rep.MutexTop = topStacks("mutex", 12)
	rep.GoroTop = topStacks("goroutine", 12)

	ops := map[string]map[string]float64{}
	for k, v := range rec.Ops {
		ops[k] = v.Stats()
	}
	rep.Jobs = int(done)
	rep.Failures = int(fails)
	rep.WallMS = wall.Milliseconds()
	rep.JobsPerSec = float64(done) / wall.Seconds()
	rep.Total = rec.Total.Stats()
	rep.Ops = ops

	b, _ := json.MarshalIndent(rep, "", "  ")
	if outPath != "" {
		return os.WriteFile(outPath, b, 0o644)
	}
	fmt.Println(string(b))
	return nil
}

// goroutineCensus counts goroutines by their top two frames.
//
// The question this answers is not "how many goroutines exist" — that number is
// in every sample already — but "how many are parked in the SAME place". A
// controller whose ceiling is a serialization point shows a growing pile at one
// stack; a controller that is merely busy shows goroutines spread across the
// job's own call sites.
func goroutineCensus() map[string]int {
	var sb strings.Builder
	if err := pprof.Lookup("goroutine").WriteTo(&sb, 1); err != nil {
		return nil
	}
	out := map[string]int{}
	for _, blk := range strings.Split(sb.String(), "\n\n") {
		lines := strings.Split(strings.TrimSpace(blk), "\n")
		if len(lines) < 2 || !strings.HasPrefix(lines[0], "goroutine profile") {
			// entry form: "N @ 0x... 0x..." then indented frames
			if len(lines) < 2 {
				continue
			}
			var n int
			if _, err := fmt.Sscanf(lines[0], "%d @", &n); err != nil {
				continue
			}
			key := []string{}
			for _, l := range lines[1:] {
				l = strings.TrimSpace(l)
				if l == "" || strings.HasPrefix(l, "#") {
					continue
				}
				key = append(key, shortFrame(l))
				if len(key) == 2 {
					break
				}
			}
			if len(key) > 0 {
				out[strings.Join(key, " <- ")] += n
			}
		}
	}
	return out
}

// topStacks parses a debug=1 text profile into the heaviest stacks.
//
// Parsing text rather than shipping the toolchain into the container is
// deliberate: the container must stay the measurement environment, not become a
// development one. The .pb.gz files are written alongside for offline analysis
// with `go tool pprof`, and the numbers in the report come from the same data.
func topStacks(name string, n int) []stackCost {
	var sb strings.Builder
	p := pprof.Lookup(name)
	if p == nil {
		return nil
	}
	if err := p.WriteTo(&sb, 1); err != nil {
		return nil
	}
	var out []stackCost
	for _, blk := range strings.Split(sb.String(), "\n") {
		line := strings.TrimSpace(blk)
		if line == "" {
			continue
		}
		// block/mutex: "<cycles> <count> @ 0x..."; goroutine: "<count> @ 0x..."
		if !strings.Contains(line, " @ ") {
			continue
		}
		head := strings.SplitN(line, " @ ", 2)[0]
		fields := strings.Fields(head)
		var sc stackCost
		switch len(fields) {
		case 1:
			fmt.Sscanf(fields[0], "%d", &sc.Value)
		case 2:
			fmt.Sscanf(fields[0], "%d", &sc.Value)
			fmt.Sscanf(fields[1], "%d", &sc.Count)
		default:
			continue
		}
		out = append(out, sc)
	}
	// The frames belong to the lines after each header; re-walk to attach them.
	blocks := strings.Split(sb.String(), "\n")
	idx := -1
	for _, line := range blocks {
		t := strings.TrimSpace(line)
		if strings.Contains(t, " @ ") {
			idx++
			continue
		}
		if idx >= 0 && idx < len(out) && strings.HasPrefix(line, "#") {
			if len(out[idx].Stack) < 6 {
				out[idx].Stack = append(out[idx].Stack, shortFrame(t))
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value > out[j].Value })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// shortFrame keeps the package-qualified symbol and drops file paths, which are
// build-machine specific and would make two runs look different when they are
// not.
func shortFrame(s string) string {
	s = strings.TrimPrefix(s, "#")
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "\t"); i >= 0 {
		s = s[:i]
	}
	f := strings.Fields(s)
	if len(f) >= 2 {
		s = f[1]
	}
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	return s
}
