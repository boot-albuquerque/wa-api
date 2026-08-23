#!/usr/bin/env python3
"""Consolidates the per-experiment JSON reports into the study's tables.

Prints only what was measured. Any cell the harness could not observe is left
as '-' rather than filled with a plausible number.
"""
import json
import glob
import os
import sys

RESULTS = sys.argv[1] if len(sys.argv) > 1 else "results"


def load(name):
    p = os.path.join(RESULTS, name + ".json")
    if not os.path.exists(p):
        return None
    with open(p) as f:
        return json.load(f)


def fmt(v, nd=1):
    if v is None:
        return "-"
    return f"{v:.{nd}f}"


def table_controllers(rep, title):
    if not rep:
        print(f"\n## {title}: (não executado)")
        return
    print(f"\n## {title}")
    meta = rep["meta"]
    br = rep.get("browser") or {}
    print(f"   browser: {br.get('Browser','?')}  | goarch={meta.get('goarch')} "
          f"cpus={meta.get('num_cpu')} conc={meta.get('concurrency')} "
          f"iters={meta.get('iters')} workload={meta.get('workload')}")
    cg = rep.get("cgroup") or {}
    start = cg.get("memory_current_at_start")
    end = cg.get("memory_current_at_end")
    if isinstance(start, int) and isinstance(end, int):
        print(f"   cgroup memory.current: início {start/2**20:.0f} MiB → fim {end/2**20:.0f} MiB "
              f"(limite {cg.get('memory_max')})")
    hdr = (f"{'controller':<18} {'jobs':>5} {'fail':>5} {'jobs/s':>7} {'p50':>8} {'p95':>8} "
           f"{'p99':>8} {'max':>8} {'ctlRSS_KB':>10} {'gor':>5} {'thr':>5} {'fd':>4} {'conn_ms':>8}")
    print("   " + hdr)
    print("   " + "-" * len(hdr))
    for name, c in sorted(rep["controllers"].items()):
        t = c["job_total"]
        a = c["controller_after"]
        print("   " + f"{name:<18} {c['jobs']:>5} {c['failures']:>5} {c['jobs_per_sec']:>7.2f} "
              f"{fmt(t.get('p50_ms')):>8} {fmt(t.get('p95_ms')):>8} {fmt(t.get('p99_ms')):>8} "
              f"{fmt(t.get('max_ms')):>8} {a['controller_rss_kb']:>10} {a['goroutines']:>5} "
              f"{a['os_threads']:>5} {a['open_fds']:>4} {c['connect_ms']:>8.1f}")


def table_ops(rep, title):
    if not rep:
        return
    print(f"\n### {title} — por operação (p50 / p95 ms)")
    ctls = sorted(rep["controllers"].items())
    if not ctls:
        return
    ops = sorted(ctls[0][1]["ops"].keys())
    print("   " + f"{'op':<14}" + "".join(f"{n:>26}" for n, _ in ctls))
    for op in ops:
        row = f"{op:<14}"
        for _, c in ctls:
            s = c["ops"].get(op, {})
            row += f"{fmt(s.get('p50_ms'))+' / '+fmt(s.get('p95_ms')):>26}"
        print("   " + row)


def main():
    print("=" * 100)
    print("RESULTADOS CONSOLIDADOS —", os.path.abspath(RESULTS))
    print("=" * 100)

    a = load("normA-rep1")
    b = load("normB-rep1")
    table_controllers(a, "Workload A (SPA leve) — browser canônico, serial")
    table_ops(a, "Workload A")
    table_controllers(b, "Workload B (SPA pesada) — browser canônico, serial")
    table_ops(b, "Workload B")

    noiso = load("iso-off")
    if noiso:
        table_controllers(noiso, "Política de lançamento SEM disable-site-isolation (mesmo controller)")

    print("\n## Escala — mesma browser instance, N páginas concorrentes")
    hdr = f"{'exp':<18} {'controller':<14} {'conc':>5} {'jobs':>5} {'fail':>5} {'jobs/s':>8} {'p50':>8} {'p95':>8} {'p99':>8} {'cgroup_end_MiB':>15}"
    print("   " + hdr)
    print("   " + "-" * len(hdr))
    for f in sorted(glob.glob(os.path.join(RESULTS, "conc-*.json"))):
        rep = json.load(open(f))
        end = (rep.get("cgroup") or {}).get("memory_current_at_end")
        endm = f"{end/2**20:.0f}" if isinstance(end, int) else "-"
        for name, c in sorted(rep["controllers"].items()):
            t = c["job_total"]
            print("   " + f"{os.path.basename(f)[:-5]:<18} {name:<14} {rep['meta']['concurrency']:>5} "
                  f"{c['jobs']:>5} {c['failures']:>5} {c['jobs_per_sec']:>8.2f} "
                  f"{fmt(t.get('p50_ms')):>8} {fmt(t.get('p95_ms')):>8} {fmt(t.get('p99_ms')):>8} {endm:>15}")


if __name__ == "__main__":
    main()
