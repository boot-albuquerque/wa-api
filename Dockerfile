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
COPY src/go.mod src/go.sum ./
RUN go mod download

COPY src/ .
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
WORKDIR /app

COPY --from=builder /app/wa-api         /app/
COPY --from=builder /app/static         /app/static/
COPY --from=builder /app/wa-api.service /app/wa-api.service

RUN chmod +x /app/wa-api && \
    chmod -R 755 /app && \
    chown -R root:root /app

ENTRYPOINT ["/app/wa-api", "--logtype=console", "--color=true"]
