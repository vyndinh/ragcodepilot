# AGENTS.md

## Rules

- **Never `git push` autonomously.** Always stop after committing locally. Let the user review and push themselves.
- Before working on a feature, skim the `docs/` folder for relevant docs (e.g. `docs/plan/mvp_roadmap.md`, `docs/plan/system_design.md`, `docs/plan/checklist.md` - tracks the original phase plan ); these are the source of truth for cross-cutting features and working conventions
- **`docs/` files use pseudocode, not Go.** When expressing a code idea in any file under `docs/`, write language-agnostic pseudocode (e.g. `function HitAtK(results, k) → bool`) instead of real Go syntax. Real Go code belongs only in `internal/`, `cmd/`, or test files.
- **Use t-shirt sizes for estimates.** When estimating effort in planning docs, use t-shirt sizes (S / M / L / XL) instead of week counts. They communicate relative effort without false precision.

## Project Overview

**ragcodepilot** — a local semantic code search CLI written in Go. It indexes source code from local Git repositories, embeds code chunks using Ollama (`nomic-embed-text`), stores vectors in Qdrant, and retrieves relevant code via natural-language queries from the terminal.

## Build & Test Commands

```bash
# Run tests (all packages, race detector, single run)
go test ./... -v -race -count=1

# Build the CLI binary
go build -o bin/ragcodepilot ./cmd/ragcodepilot

# Run directly without building
go run ./cmd/ragcodepilot <command> [flags]

# Start Qdrant (required for index/search)
docker compose up -d

# Stop Qdrant
docker compose down
```

## Core Architecture

### Workspace Structure

```
ragcodepilot/
  cmd/ragcodepilot/          ← CLI entry point (index, search, collections, version)
    main.go
    main_test.go
  internal/
    config/                  ← YAML config loader (languages, skip_dirs)
    embedding/               ← Embedder interface + Ollama / Fake implementations
    ingest/                  ← Ingestion pipeline: walker → chunker → enrichment → embed → upsert
    model/                   ← Shared types: CodeChunk, SearchResult
    qdrant/                  ← Qdrant gRPC client wrapper (CRUD, search, payload indexes)
    search/                  ← Search orchestration + result formatting
  docs/
    plan/                    ← Design docs, checklist, phase plans
    knowledge/               ← Learning reference docs
    review_feedback/         ← AI review feedback logs
  config.yaml                ← Language definitions + skip directories
  docker-compose.yml         ← Qdrant service
```

### Key Data Flow

**Ingestion:**

```
Git repo → File walker → Language filter → Chunker (Go AST / generic) → Enrichment → Ollama embed → Batch upsert → Qdrant
```

**Search:**

```
Natural-language query → Ollama embed → Qdrant vector search (with optional language/repo filters) → Formatted results
```

## Important Files

| File | Purpose |
|------|---------|
| `cmd/ragcodepilot/main.go` | CLI commands: `index`, `search`, `collections list/delete`, `version` |
| `internal/ingest/pipeline.go` | Orchestrates the full ingestion pipeline (walk → chunk → embed → upsert) |
| `internal/ingest/chunker.go` | Generic sliding-window chunker + `extractName` regex patterns |
| `internal/ingest/chunker_go.go` | Go AST-based function-level chunker |
| `internal/ingest/enrichment.go` | Prepends metadata context to raw code before embedding |
| `internal/embedding/embedder.go` | `Embedder` interface (`Embed`, `Dimension`) |
| `internal/embedding/ollama.go` | Ollama HTTP client for `nomic-embed-text` (768d) |
| `internal/embedding/validate.go` | Vector batch validation (count, dimensions, empty) |
| `internal/qdrant/client.go` | Qdrant gRPC wrapper: collection CRUD, upsert, search, payload indexes |
| `internal/model/chunk.go` | `CodeChunk` and `SearchResult` structs |
| `internal/config/config.go` | YAML config: language→extensions map, skip dirs |
| `config.yaml` | Default language definitions and skip directories |
| `docs/plan/checklist.md` | Current progress tracker across all phases |
| `docs/plan/system_design.md` | Full system design document |

## Local Dev (without Docker)

1. Install Go 1.26+
2. Install and start [Ollama](https://ollama.com) with `ollama pull nomic-embed-text`
3. Start Qdrant: `docker compose up -d` (or run Qdrant binary directly on port 6333/6334)
4. Index a repo: `go run ./cmd/ragcodepilot index --language go .`
5. Search: `go run ./cmd/ragcodepilot search "embedding interface"`
6. Run tests: `go test ./... -v -race -count=1`

## Local Dev (with Docker)

Only Qdrant runs in Docker. The Go CLI runs on the host:

```bash
docker compose up -d                                          # start Qdrant
go run ./cmd/ragcodepilot index --language go .                # index current repo
go run ./cmd/ragcodepilot search --language go "chunker"       # search
docker compose down                                            # stop Qdrant
```
