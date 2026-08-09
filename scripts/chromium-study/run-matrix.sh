#!/bin/bash
# Runs the experiment matrix, one container per experiment.
#
# One container per experiment on purpose: cgroup accounting is per container,
# so sharing one container across experiments would blend their memory and make
# every number after the first meaningless.
#
# Controller order is rotated between repetitions so that a systematic effect of
# running first (cold page cache, cold JIT in the renderer) cannot be mistaken
# for a property of whichever controller happened to be listed first.
set -uo pipefail

OUT="${OUT:-$(pwd)/results}"
IMG="${IMG:-chromium-study:v1}"
CPUS="${CPUS:-4}"
MEM="${MEM:-4g}"
mkdir -p "$OUT"

run() {
  local name="$1"; shift
  local profile="${PROFILE:-canonical}"
  echo "=== $name (profile=$profile cpus=$CPUS mem=$MEM) ==="
  docker run --rm \
    --cpus="$CPUS" --memory="$MEM" --memory-swap="$MEM" \
    -e PROFILE="$profile" \
    -v "$OUT:/out" \
    "$IMG" "$@" -out "/out/${name}.json" \
    > "$OUT/${name}.stdout" 2> "$OUT/${name}.stderr"
  local rc=$?
  grep -E 'browser-only floor|jobs=|FAILED|memory.current at end' "$OUT/${name}.stderr" | sed 's/^/    /'
  echo "    rc=$rc"
}

A="cdp-min,chromedp,chromedp-cdproto,rod-canon,rod-default"
B="chromedp,chromedp-cdproto,rod-canon,rod-default,cdp-min"
C="rod-canon,rod-default,cdp-min,chromedp,chromedp-cdproto"

# Parts 2/3/16/17/18 — three independent repetitions, rotated order.
run normA-rep1 -controllers "$A" -workload a -iters 100 -warmup 10 -concurrency 1 -job-timeout 30s
run normA-rep2 -controllers "$B" -workload a -iters 100 -warmup 10 -concurrency 1 -job-timeout 30s
run normA-rep3 -controllers "$C" -workload a -iters 100 -warmup 10 -concurrency 1 -job-timeout 30s

# Part 5 — heavy SPA.
run normB-rep1 -controllers "$A" -workload b -iters 60 -warmup 8 -concurrency 1 -job-timeout 30s
run normB-rep2 -controllers "$C" -workload b -iters 60 -warmup 8 -concurrency 1 -job-timeout 30s

# Parts 3/8 — the site-isolation lever, isolated: same controller, same
# workload, only the launch policy differs.
run iso-on  -controllers chromedp -workload b -iters 40 -warmup 6 -concurrency 1 -job-timeout 30s
PROFILE=no-isolation run iso-off -controllers chromedp -workload b -iters 40 -warmup 6 -concurrency 1 -job-timeout 30s

# Parts 6/7 — concurrency ladder: N pages sharing ONE browser.
for c in 2 4 8 16; do
  run "conc-${c}" -controllers chromedp,rod-canon -workload a -iters $((c*15)) -warmup 6 -concurrency "$c" -job-timeout 60s
done

echo "results in $OUT"
