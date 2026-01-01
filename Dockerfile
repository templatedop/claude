# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binaries
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/worker ./cmd/worker
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/cli ./cmd/cli

# Worker image
FROM alpine:3.19 AS worker

RUN apk add --no-cache ca-certificates git

WORKDIR /app

COPY --from=builder /bin/worker /app/worker

CMD ["/app/worker"]

# API image
FROM alpine:3.19 AS api

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /bin/api /app/api

EXPOSE 8080

CMD ["/app/api"]

# CLI image
FROM alpine:3.19 AS cli

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /bin/cli /app/claude-orchestrator

ENTRYPOINT ["/app/claude-orchestrator"]
