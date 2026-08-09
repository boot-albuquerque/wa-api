#!/bin/bash
# Launches ONE Chromium with the canonical profile, waits for CDP, records the
# browser-only memory floor, then runs the study binary against it.
#
# The browser-only floor matters: without it, every controller number would
# silently include the browser and the comparison would measure Chromium.
set -uo pipefail

CDP_PORT="${CDP_PORT:-9222}"
PROFILE_DIR="$(mktemp -d)"
CHROME_BIN="${CHROME_BIN:-/usr/bin/chromium}"

# CanonicalBrowserProfileV1 — must stay byte-identical to the Go constant.
# Duplicated deliberately: the launcher is shell and the reporter is Go, and a
# shared file read by both would still not prove the browser was launched with
# what the report claims. The study asserts equality at run time instead (below).
CANONICAL_FLAGS=(
  --headless=new
  --no-sandbox
  --disable-dev-shm-usage
  --disable-gpu
  --no-first-run
  --no-default-browser-check
  --disable-background-networking
  --disable-background-timer-throttling
  --disable-backgrounding-occluded-windows
  --disable-renderer-backgrounding
  --disable-breakpad
  --disable-client-side-phishing-detection
  --disable-default-apps
  --disable-extensions
  --disable-component-extensions-with-background-pages
  --disable-hang-monitor
  --disable-ipc-flooding-protection
  --disable-popup-blocking
  --disable-prompt-on-repost
  --disable-sync
  --metrics-recording-only
  "--disable-features=site-per-process,Translate,TranslateUI,BlinkGenPropertyTrees,AcceptCHFrame,MediaRouter,OptimizationHints"
  --disable-site-isolation-trials
  --force-color-profile=srgb
  --window-size=1280,800
  --lang=en-US
  --remote-debugging-address=0.0.0.0
)

# NO_ISOLATION_FLAGS drops the two site-isolation flags so the lever can be
# measured instead of assumed. Selected with PROFILE=no-isolation.
if [ "${PROFILE:-canonical}" = "no-isolation" ]; then
  FLAGS=()
  for f in "${CANONICAL_FLAGS[@]}"; do
    case "$f" in
      --disable-site-isolation-trials) continue ;;
      --disable-features=*) f="--disable-features=Translate,TranslateUI,BlinkGenPropertyTrees,AcceptCHFrame,MediaRouter,OptimizationHints" ;;
    esac
    FLAGS+=("$f")
  done
else
  FLAGS=("${CANONICAL_FLAGS[@]}")
fi

echo "### environment" >&2
echo "chromium: $($CHROME_BIN --version 2>/dev/null)" >&2
echo "kernel:   $(uname -r) $(uname -m)" >&2
echo "cgroup:   $(cat /sys/fs/cgroup/cgroup.controllers 2>/dev/null | head -c 120)" >&2
echo "mem.max:  $(cat /sys/fs/cgroup/memory.max 2>/dev/null)" >&2
echo "cpu.max:  $(cat /sys/fs/cgroup/cpu.max 2>/dev/null)" >&2
echo "profile:  ${PROFILE:-canonical}" >&2

# Modes that own their own browsers (topology, faults, soak) must NOT have one
# started for them: a stray idle Chromium would sit inside the same cgroup and
# be charged to every topology equally, flattening exactly the differences the
# track exists to measure.
if [ "${SKIP_BROWSER:-0}" = "1" ]; then
  echo "SKIP_BROWSER=1 — the study binary launches its own browsers" >&2
  exec /study/study "$@"
fi

MEM_IDLE=$(cat /sys/fs/cgroup/memory.current)
echo "memory.current before browser: $MEM_IDLE" >&2

"$CHROME_BIN" "${FLAGS[@]}" \
  --remote-debugging-port="$CDP_PORT" \
  --user-data-dir="$PROFILE_DIR" \
  about:blank >/tmp/chromium.log 2>&1 &
CHROME_PID=$!

for i in $(seq 1 100); do
  if curl -sf "http://127.0.0.1:${CDP_PORT}/json/version" >/dev/null 2>&1; then break; fi
  if ! kill -0 "$CHROME_PID" 2>/dev/null; then
    echo "FATAL: chromium exited during startup" >&2
    tail -30 /tmp/chromium.log >&2
    exit 1
  fi
  sleep 0.2
done

# Let the browser reach steady state before charging its floor, otherwise the
# floor absorbs startup allocations and every controller looks cheaper.
sleep 3
MEM_BROWSER_IDLE=$(cat /sys/fs/cgroup/memory.current)
echo "memory.current with idle browser: $MEM_BROWSER_IDLE" >&2
echo "browser-only floor: $(( (MEM_BROWSER_IDLE - MEM_IDLE) / 1048576 )) MiB" >&2

export BROWSER_FLOOR_BYTES=$(( MEM_BROWSER_IDLE - MEM_IDLE ))
export MEM_IDLE_BYTES=$MEM_IDLE

/study/study "$@"
RC=$?

MEM_END=$(cat /sys/fs/cgroup/memory.current)
echo "memory.current at end: $MEM_END ($(( (MEM_END - MEM_IDLE) / 1048576 )) MiB over idle)" >&2
echo "chromium processes at end: $(pgrep -c -f "$PROFILE_DIR" || echo 0)" >&2

kill "$CHROME_PID" 2>/dev/null
exit $RC
