#!/bin/bash
# Track A — the ablation, run as four arms.
#
# Two baselines and two ablations, each in its OWN container so that one arm's
# leftover connections, goroutines or browser state cannot be charged to the
# next. Phase 2 learned that the hard way: rod's Close() killed the shared
# browser and only a rotated controller order made it visible.
#
# The prediction under test, stated before the run:
#   chromedp             fast   (one CDP connection per job, by accident)
#   chromedp-shared-conn SLOWER (same library, forced onto one connection)
#   rod-canon            slow   (one connection for all jobs)
#   rod-multiconn-8      FASTER (same library, one connection per worker)
#
# If chromedp-shared-conn stays fast, or rod-multiconn-8 stays slow, the
# per-connection hypothesis is refuted and the mechanism is elsewhere.
set -uo pipefail
cd "$(dirname "$0")"
OUT=results-p3/trackA
mkdir -p "$OUT"

CPUS="${CPUS:-8}"
CONC="${CONC:-8}"
ITERS="${ITERS:-80}"

run() { # run <arm> <mode> <extra-args...>
  local arm="$1"; shift
  local mode="$1"; shift
  echo "=== $arm ($mode, conc=$CONC, cpus=$CPUS) ==="
  docker run --rm --cpus="$CPUS" --memory=4g \
    -v "$PWD/results-p3:/out" \
    chromium-study:p3 \
    -mode "$mode" -controllers "$arm" -concurrency "$CONC" -iters "$ITERS" \
    -warmup 5 -job-timeout 60s "$@" 2>&1 | tail -12
}

for arm in chromedp chromedp-shared-conn rod-canon "rod-multiconn-$CONC"; do
  # Throughput arm: no profiling, so the number is comparable with Phase 2.
  run "$arm" controllers -out "/out/trackA/thr-$arm-c$CONC.json"
  # Mechanism arm: full mutex/block/CPU profiling, throughput NOT comparable.
  run "$arm" prof -out "/out/trackA/prof-$arm-c$CONC.json" \
      -profdir "/out/trackA/pprof-$arm-c$CONC"
done
echo "done"
