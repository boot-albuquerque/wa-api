FROM golang:1.25-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    g++ \
    pkg-config \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ENV CGO_ENABLED=1
RUN go build -o wa-api ./cmd/core

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    netcat-openbsd \
    postgresql-client \
    openssl \
    curl \
    ffmpeg \
    tzdata \
    && rm -rf /var/lib/apt/lists/*

ENV TZ="America/Sao_Paulo"
# WA_API_PORT e' a mesma env var que pkg/bootstrap/main.go:72 le para escolher a porta.
# Sobrescrever com `docker run -e WA_API_PORT=...` mantem EXPOSE/HEALTHCHECK em sincronia.
ENV WA_API_PORT=8080
WORKDIR /app

COPY --from=builder /app/wa-api /app/

RUN chmod +x /app/wa-api && \
    chmod -R 755 /app && \
    groupadd -r waapi && useradd -r -g waapi waapi && \
    chown -R waapi:waapi /app

USER waapi

EXPOSE $WA_API_PORT

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:$WA_API_PORT/livez || exit 1

ENTRYPOINT ["/app/wa-api", "--logtype=console", "--color=true"]
