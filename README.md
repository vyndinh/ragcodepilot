# ragsearch

`ragsearch` is a local semantic code search CLI. It indexes source code from local repositories, stores code chunks and vector embeddings in Qdrant, then searches the indexed code from the terminal using natural language.

This project is a learning implementation for vector database application design. It focuses on retrieval, not answer generation: search returns ranked code chunks, not LLM-generated summaries.

## Current status

- Go CLI with Qdrant as the vector database backend.
- Local Ollama embedder using `nomic-embed-text` (768-dimensional vectors, auto-detected).
- Go files are chunked at the function level using `go/parser` AST; other languages use a 40-line sliding window.
- Chunk enrichment prepends file/language/function metadata to embedding input for improved search relevance.
- Search with dense vector lookup and optional language and repo payload filtering.
- Embedding dimension auto-detection and validation (collection mismatch produces clear error with fix instructions).
- Collection list and delete commands.
- `config.yaml` is auto-loaded during indexing when present; built-in defaults are used only when it is absent.

Planned but not complete: regex chunkers for Python/Rust, re-index/delete-stale behavior, sparse vectors, and hybrid BM25/vector search.

## Architecture

```text
CLI
  |
  |-- index <repo-path>
  |     -> file walker
  |     -> language detector
  |     -> chunker (Go AST or sliding window)
  |     -> enrichment (prepend metadata to embedding input)
  |     -> Ollama embedder (nomic-embed-text)
  |     -> Qdrant upsert
  |
  |-- search <query>
  |     -> Ollama query embedder
  |     -> dimension validation
  |     -> Qdrant vector search (with language/repo filters)
  |     -> terminal result formatter
  |
  |-- collections list|delete
```

Qdrant runs locally through Docker Compose. The Go CLI runs on the host machine and connects to Qdrant over gRPC on port `6334`. Ollama runs locally and serves embeddings on port `11434`.

## Prerequisites

- Go `1.26.1`
- Docker and Docker Compose
- [Ollama](https://ollama.com/) with `nomic-embed-text` model

## Getting started

Start Qdrant:

```bash
docker compose up -d qdrant
```

Pull the embedding model (one-time):

```bash
ollama pull nomic-embed-text
```

Check the CLI from source:

```bash
go run ./cmd/ragsearch version
```

Build the project:

```bash
mkdir -p bin
go build -o bin/ragsearch ./cmd/ragsearch
```

Index this repository (Go files only):

```bash
go run ./cmd/ragsearch index --language go .
```

Search indexed code:

```bash
go run ./cmd/ragsearch search --language go --limit 5 "embedding interface"
```

Search with repo filter:

```bash
go run ./cmd/ragsearch search --repo ragsearch --limit 3 "ingestion pipeline"
```

Combine language and repo filters:

```bash
go run ./cmd/ragsearch search --language go --repo ragsearch --limit 3 "how does chunking work?"
```

List collections:

```bash
go run ./cmd/ragsearch collections list
```

Delete the default collection (required when changing embedding model or enrichment logic):

```bash
go run ./cmd/ragsearch collections delete code_chunks
```

Stop Qdrant:

```bash
docker compose down
```

## CLI flags

### Index

| Flag | Default | Description |
|------|---------|-------------|
| `-collection` | `code_chunks` | Qdrant collection name |
| `-language` | (all) | Comma-separated language filter (e.g., `go,rust`) |
| `-embedder` | `ollama` | Embedder to use: `ollama`, `fake` |
| `-ollama-url` | `http://localhost:11434` | Ollama server URL |
| `-ollama-model` | `nomic-embed-text` | Ollama embedding model |
| `-qdrant-host` | `localhost` | Qdrant host |
| `-qdrant-port` | `6334` | Qdrant gRPC port |

### Search

| Flag | Default | Description |
|------|---------|-------------|
| `-collection` | `code_chunks` | Qdrant collection name |
| `-language` | (all) | Comma-separated language filter (e.g., `go,rust`) |
| `-repo` | (all) | Comma-separated repo name filter (e.g., `ragsearch`) |
| `-limit` | `5` | Maximum number of results |
| `-embedder` | `ollama` | Embedder to use: `ollama`, `fake` |
| `-ollama-url` | `http://localhost:11434` | Ollama server URL |
| `-ollama-model` | `nomic-embed-text` | Ollama embedding model |
| `-qdrant-host` | `localhost` | Qdrant host |
| `-qdrant-port` | `6334` | Qdrant gRPC port |

## Configuration

`config.yaml` controls language extension mappings and skipped directories for indexing.

When running `index`, the CLI uses this precedence:

1. Load `./config.yaml` if it exists.
2. Use built-in defaults if `./config.yaml` does not exist.
3. Fail fast if `./config.yaml` exists but cannot be read or parsed.

Example:

```yaml
languages:
  go: [".go"]
  rust: [".rs"]
  python: [".py"]

skip_dirs:
  - .git
  - vendor
  - node_modules
  - target
```

The search command does not load `config.yaml`. It filters against language and repo values stored in Qdrant payloads during indexing.

## Development

Run all tests with race detection:

```bash
go test -race ./...
```

Run tests for a specific package:

```bash
go test -race -v ./internal/ingest/...
go test -race -v ./internal/qdrant/...
```

Build the project:

```bash
mkdir -p bin
go build -o bin/ragsearch ./cmd/ragsearch
```

Inject a version at build time:

```bash
mkdir -p bin
go build -ldflags "-X main.version=dev-local" -o bin/ragsearch ./cmd/ragsearch
```

Check formatting:

```bash
gofmt -l ./internal/
```

## Re-indexing after changes

You must delete the collection and re-index when:

- Changing the embedding model (different vector dimensions)
- Changing the enrichment logic (different embedding input text)
- Switching between `fake` and `ollama` embedders

```bash
go run ./cmd/ragsearch collections delete code_chunks
go run ./cmd/ragsearch index --language go .
```

## Known limitations

- Function-level chunking is Go-only (AST-based). Other languages use a sliding window.
- Hybrid search, sparse vectors, BM25, and RRF fusion are not implemented yet.
- Re-indexing does not delete stale chunks when files are removed or renamed.
- Embedding dimension is auto-detected; switching models requires collection delete + re-index.

## Further docs

- [System design](docs/plan/system_design.md)
- [Progress checklist](docs/plan/checklist_2026-05-07.md)
- [Function-level chunker](docs/plan/function_level_chunker.md)
- [Embedding dimension validation](docs/plan/embedding_dimension_validation.md)
- [Chunk enrichment](docs/plan/chunk_enrichment.md)
- [RAG architecture parts](docs/knowledge/rag_parts.md)
- [Embeddings explained](docs/knowledge/embeddings_explained.md)
- [Vector database platform comparison](docs/knowledge/compare.md)
