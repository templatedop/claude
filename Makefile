.PHONY: all build test clean run-worker run-api run-cli docker-build docker-up docker-down help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
BINARY_DIR=bin

# Binary names
WORKER_BINARY=$(BINARY_DIR)/worker
API_BINARY=$(BINARY_DIR)/api
CLI_BINARY=$(BINARY_DIR)/claude-orchestrator

all: build

## build: Build all binaries
build: $(WORKER_BINARY) $(API_BINARY) $(CLI_BINARY)

$(BINARY_DIR):
	mkdir -p $(BINARY_DIR)

$(WORKER_BINARY): $(BINARY_DIR)
	$(GOBUILD) -o $(WORKER_BINARY) ./cmd/worker

$(API_BINARY): $(BINARY_DIR)
	$(GOBUILD) -o $(API_BINARY) ./cmd/api

$(CLI_BINARY): $(BINARY_DIR)
	$(GOBUILD) -o $(CLI_BINARY) ./cmd/cli

## test: Run all tests
test:
	$(GOTEST) -v ./...

## test-coverage: Run tests with coverage
test-coverage:
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

## clean: Clean build artifacts
clean:
	rm -rf $(BINARY_DIR)
	rm -f coverage.out coverage.html

## deps: Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

## run-worker: Run the worker locally
run-worker: $(WORKER_BINARY)
	./$(WORKER_BINARY)

## run-api: Run the API server locally
run-api: $(API_BINARY)
	./$(API_BINARY)

## run-cli: Run the CLI (with arguments after --)
run-cli: $(CLI_BINARY)
	./$(CLI_BINARY) $(ARGS)

## docker-build: Build Docker images
docker-build:
	docker-compose build

## docker-up: Start all services
docker-up:
	docker-compose up -d

## docker-down: Stop all services
docker-down:
	docker-compose down

## docker-logs: View logs from all services
docker-logs:
	docker-compose logs -f

## temporal-up: Start only Temporal (for local development)
temporal-up:
	docker-compose up -d temporal temporal-ui postgres

## temporal-down: Stop Temporal
temporal-down:
	docker-compose down temporal temporal-ui postgres

## lint: Run linter
lint:
	golangci-lint run ./...

## fmt: Format code
fmt:
	gofmt -s -w .

## vet: Run go vet
vet:
	$(GOCMD) vet ./...

## help: Show this help message
help:
	@echo "Claude Orchestrator - Available Commands:"
	@echo ""
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'
	@echo ""
	@echo "Environment Variables:"
	@echo "  ANTHROPIC_API_KEY  - Anthropic API key (required for worker)"
	@echo "  TEMPORAL_ADDRESS   - Temporal server address (default: localhost:7233)"
	@echo "  TEMPORAL_NAMESPACE - Temporal namespace (default: default)"
	@echo "  WORKING_DIR        - Working directory for file operations"
	@echo "  PORT               - API server port (default: 8080)"
