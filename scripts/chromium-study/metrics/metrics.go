// Package metrics collects the process- and container-level numbers the study
// reports, keeping the controller side and the browser side strictly apart.
//
// The separation is the whole point: a controller benchmark that accidentally
// counts browser memory measures the browser. Every function here states which
// side it observes.
package metrics

import (
	"os"
	"os/exec"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Snapshot is the CONTROLLER side only: the Go process running the benchmark.
type Snapshot struct {
	RSSKB      int64  `json:"controller_rss_kb"`
	Goroutines int    `json:"goroutines"`
	Threads    int    `json:"os_threads"`
	FDs        int    `json:"open_fds"`
	HeapAllocB uint64 `json:"heap_alloc_bytes"`
	HeapSysB   uint64 `json:"heap_sys_bytes"`
}

// Self observes the current process. RSS comes from ps rather than from Go's
// runtime because runtime.MemStats reports the Go heap, not resident memory —
// they answer different questions, and only RSS is what a cgroup charges.
func Self() Snapshot {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return Snapshot{
		RSSKB:      rssKB(os.Getpid()),
		Goroutines: runtime.NumGoroutine(),
		Threads:    osThreads(),
		FDs:        openFDs(),
		HeapAllocB: ms.HeapAlloc,
		HeapSysB:   ms.HeapSys,
	}
}

func rssKB(pid int) int64 {
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return -1
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return -1
	}
	return v
}

// osThreads counts real OS threads via the runtime's threadcreate profile,
// which is what shows up as tasks in a cgroup's pids controller.
func osThreads() int {
	pr := pprof.Lookup("threadcreate")
	if pr == nil {
		return -1
	}
	return pr.Count()
}

func openFDs() int {
	// /dev/fd is present on both macOS and Linux and counts this process's
	// descriptors without shelling out to lsof, which is slow enough to
	// perturb a latency measurement.
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		return -1
	}
	return len(entries)
}

// ProcTree is the BROWSER side: every process whose command line carries the
// marker (a unique --user-data-dir or port), summed.
type ProcTree struct {
	RSSKB int64 `json:"browser_rss_kb"`
	Procs int   `json:"browser_procs"`
}

// BrowserByMarker attributes browser processes by a unique string in their
// command line. Attributing by marker rather than by PID keeps the method
// identical across controllers, including the ones that do not expose the
// browser PID at all.
//
// Caveat this cannot fix on macOS: summing RSS double-counts memory shared
// between the browser's processes. On Linux the study uses cgroup accounting
// instead; see CgroupCurrent.
func BrowserByMarker(marker string) ProcTree {
	out, err := exec.Command("ps", "-axww", "-o", "rss=,command=").Output()
	if err != nil {
		return ProcTree{RSSKB: -1}
	}
	var t ProcTree
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, marker) {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		kb, err := strconv.ParseInt(f[0], 10, 64)
		if err != nil {
			continue
		}
		t.RSSKB += kb
		t.Procs++
	}
	return t
}

// CgroupCurrent reads cgroup v2 memory.current: the number the kernel charges
// and the number Kubernetes uses to OOM-kill. Returns -1 outside Linux/cgroup
// v2. This is the authoritative memory figure when it is available; sum(RSS) is
// the fallback, and the two are not interchangeable.
func CgroupCurrent() int64 {
	b, err := os.ReadFile("/sys/fs/cgroup/memory.current")
	if err != nil {
		return -1
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return -1
	}
	return v
}

// CgroupStat returns selected fields of cgroup v2 memory.stat (anon, file,
// shmem, ...), which is how shared memory becomes visible instead of being
// silently multiplied by process count.
func CgroupStat() map[string]int64 {
	b, err := os.ReadFile("/sys/fs/cgroup/memory.stat")
	if err != nil {
		return nil
	}
	m := map[string]int64{}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		v, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil {
			continue
		}
		switch f[0] {
		case "anon", "file", "shmem", "slab", "kernel_stack", "sock":
			m[f[0]] = v
		}
	}
	return m
}

// CgroupPeak reads memory.peak — the high-water mark since the cgroup was
// created. memory.current at the end of a run misses a spike that was reclaimed
// before the sample, and a spike is what triggers an OOM kill, so both are
// reported. Returns -1 when the kernel is too old to expose it.
func CgroupPeak() int64 {
	return readInt("/sys/fs/cgroup/memory.peak")
}

// CgroupEvents reads memory.events: low/high/max/oom/oom_kill. A run that never
// touched its memory ceiling must show zeros here; without that, "memory was
// not the bottleneck" is an assumption rather than an observation.
func CgroupEvents() map[string]int64 {
	return readPairs("/sys/fs/cgroup/memory.events", nil)
}

// CPUStat reads cgroup v2 cpu.stat. usage_usec is the CPU actually consumed by
// everything in the container — controller AND browser — which is what turns a
// throughput number into jobs per core-hour. nr_throttled/throttled_usec say
// whether the CFS quota, rather than the software, set the ceiling.
func CPUStat() map[string]int64 {
	return readPairs("/sys/fs/cgroup/cpu.stat", map[string]bool{
		"usage_usec": true, "user_usec": true, "system_usec": true,
		"nr_periods": true, "nr_throttled": true, "throttled_usec": true,
	})
}

// ChromiumProcs counts browser processes by CDP process type, read from
// /proc/*/cmdline.
//
// Renderer count is reported separately from the total because it is the number
// that scales with pages and origins, and because "process count went up" is
// otherwise ambiguous between a new tab and a new site-isolated frame.
func ChromiumProcs() map[string]int {
	out := map[string]int{}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return out
	}
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		b, err := os.ReadFile("/proc/" + e.Name() + "/cmdline")
		if err != nil || len(b) == 0 {
			continue
		}
		cmd := strings.ReplaceAll(string(b), "\x00", " ")
		if !strings.Contains(cmd, "chrom") {
			continue
		}
		out["total"]++
		switch {
		case strings.Contains(cmd, "--type=renderer"):
			out["renderer"]++
		case strings.Contains(cmd, "--type=gpu-process"):
			out["gpu"]++
		case strings.Contains(cmd, "--type=utility"):
			out["utility"]++
		case strings.Contains(cmd, "--type=zygote"):
			out["zygote"]++
		case !strings.Contains(cmd, "--type="):
			out["browser"]++
		default:
			out["other"]++
		}
	}
	return out
}

func readInt(p string) int64 {
	b, err := os.ReadFile(p)
	if err != nil {
		return -1
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return -1
	}
	return v
}

func readPairs(p string, keep map[string]bool) map[string]int64 {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	m := map[string]int64{}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		if keep != nil && !keep[f[0]] {
			continue
		}
		v, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil {
			continue
		}
		m[f[0]] = v
	}
	return m
}

// Latencies accumulates durations and reports order statistics. The study
// reports percentiles because a mean hides exactly the tail this workload cares
// about.
type Latencies struct {
	samples []time.Duration
}

func (l *Latencies) Add(d time.Duration) { l.samples = append(l.samples, d) }

// Stats returns n, mean, p50, p95, p99, max and stddev in milliseconds.
func (l *Latencies) Stats() map[string]float64 {
	n := len(l.samples)
	if n == 0 {
		return map[string]float64{"n": 0}
	}
	s := make([]time.Duration, n)
	copy(s, l.samples)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000.0 }
	pct := func(p float64) float64 {
		// Nearest-rank, the definition that does not interpolate between
		// samples that were never observed.
		i := int(p*float64(n)+0.5) - 1
		if i < 0 {
			i = 0
		}
		if i >= n {
			i = n - 1
		}
		return ms(s[i])
	}
	var sum float64
	for _, d := range s {
		sum += ms(d)
	}
	mean := sum / float64(n)
	var vs float64
	for _, d := range s {
		diff := ms(d) - mean
		vs += diff * diff
	}
	return map[string]float64{
		"n": float64(n), "mean_ms": mean, "p50_ms": pct(0.50), "p95_ms": pct(0.95),
		"p99_ms": pct(0.99), "max_ms": ms(s[n-1]), "stddev_ms": sqrt(vs / float64(n)),
	}
}

func sqrt(f float64) float64 {
	if f <= 0 {
		return 0
	}
	x := f
	for i := 0; i < 40; i++ {
		x = 0.5 * (x + f/x)
	}
	return x
}
