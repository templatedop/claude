# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binaries with version info
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o /bin/worker ./cmd/worker

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o /bin/api ./cmd/api

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o /bin/cli ./cmd/cli

# Worker image
FROM alpine:3.19 AS worker

RUN apk add --no-cache ca-certificates git tzdata

# Create non-root user
RUN addgroup -g 1000 orchestrator && \
    adduser -u 1000 -G orchestrator -h /app -D orchestrator

WORKDIR /app

# Create directories for storage and workspace
RUN mkdir -p /app/storage /app/workspace /app/config && \
    chown -R orchestrator:orchestrator /app

COPY --from=builder /bin/worker /app/worker

USER orchestrator

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD pgrep -x worker || exit 1

CMD ["/app/worker"]

# API image
FROM alpine:3.19 AS api

RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 orchestrator && \
    adduser -u 1000 -G orchestrator -h /app -D orchestrator

WORKDIR /app

COPY --from=builder /bin/api /app/api

USER orchestrator

EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

CMD ["/app/api"]

# CLI image
FROM alpine:3.19 AS cli

RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 orchestrator && \
    adduser -u 1000 -G orchestrator -h /app -D orchestrator

WORKDIR /app

COPY --from=builder /bin/cli /app/claude-orchestrator

USER orchestrator

ENTRYPOINT ["/app/claude-orchestrator"]

# All-in-one image (for development)
FROM alpine:3.19 AS all

RUN apk add --no-cache ca-certificates git tzdata bash

# Create non-root user
RUN addgroup -g 1000 orchestrator && \
    adduser -u 1000 -G orchestrator -h /app -D orchestrator

WORKDIR /app

# Create directories
RUN mkdir -p /app/storage /app/workspace /app/config && \
    chown -R orchestrator:orchestrator /app

COPY --from=builder /bin/worker /app/worker
COPY --from=builder /bin/api /app/api
COPY --from=builder /bin/cli /app/claude-orchestrator

USER orchestrator

EXPOSE 8080

# Default to running the worker
CMD ["/app/worker"]
