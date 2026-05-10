.PHONY: help build test lint vet tidy check run-index run-search up down clean

# Default target
help:
	@echo "ragcodepilot — available make targets:"
	@echo ""
	@echo "  Development"
	@echo "    build        Build the CLI binary to bin/ragcodepilot"
	@echo "    test         Run all tests with race detector"
	@echo "    lint         Run golangci-lint"
	@echo "    vet          Run go vet"
	@echo "    tidy         Run go mod tidy"
	@echo "    check        Run vet + tidy + lint + test (mirrors CI)"
	@echo ""
	@echo "  Infrastructure"
	@echo "    up           Start Qdrant via Docker Compose"
	@echo "    down         Stop Qdrant"
	@echo ""
	@echo "  Quick run (requires Qdrant running)"
	@echo "    index        Index the current repo (go, ragcodepilot)"
	@echo "    search q=... Search indexed code (e.g. make search q='embedding interface')"
	@echo ""
	@echo "  Cleanup"
	@echo "    clean        Remove build artifacts"

# ─── Build ────────────────────────────────────────────────────────────────────

build:
	go build -o bin/ragcodepilot ./cmd/ragcodepilot

# ─── Quality gates ────────────────────────────────────────────────────────────

test:
	go test ./... -v -race -count=1

lint:
	golangci-lint run ./...

vet:
	go vet ./...

tidy:
	go mod tidy

# Run all checks in the same order as CI
check: vet tidy lint test build

# ─── Infrastructure ───────────────────────────────────────────────────────────

up:
	docker compose up -d
	@echo "Qdrant is running on :6333 (REST) and :6334 (gRPC)"

down:
	docker compose down

# ─── Quick run ────────────────────────────────────────────────────────────────

index:
	go run ./cmd/ragcodepilot index --language go .

search:
	@if [ -z "$(q)" ]; then echo "Usage: make search q='your query here'"; exit 1; fi
	go run ./cmd/ragcodepilot search "$(q)"

# ─── Cleanup ──────────────────────────────────────────────────────────────────

clean:
	rm -rf bin/
