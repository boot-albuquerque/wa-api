# Benchmark image: one Chromium, one Go binary, cgroup v2 visible.
#
# The container is the unit of measurement. Every controller runs inside an
# identical container against an identical browser, and memory is read from the
# kernel's own accounting (/sys/fs/cgroup/memory.current) rather than by summing
# RSS across processes that share pages.
FROM debian:bookworm-slim

# procps: ps, for the controller-side RSS reading
# curl: CDP endpoint discovery and readiness probing
# chromium: the browser under test — version pinned by the base image digest,
#           printed at run time into every report
RUN apt-get update && apt-get install -y --no-install-recommends \
      chromium \
      ca-certificates \
      curl \
      procps \
      jq \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /study
COPY study-linux-arm64 /study/study
COPY entrypoint.sh /study/entrypoint.sh
RUN chmod +x /study/entrypoint.sh /study/study

# Non-root would be the production posture, but Chromium's sandbox needs either
# user namespaces or --no-sandbox. The study runs --no-sandbox EXPLICITLY (see
# CanonicalBrowserProfileV1) rather than letting a controller library detect the
# container and disable it silently.
ENTRYPOINT ["/study/entrypoint.sh"]
