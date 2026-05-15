# ragcodepilot

`ragcodepilot` is a local semantic code search CLI. It indexes source code from local repositories, stores code chunks and vector embeddings in Qdrant, then searches the indexed code from the terminal using natural language.

`search` returns ranked code chunks; opt-in answer generation via a local LLM is available behind `--answer` ([Phase 5 v0 plan](docs/plan/phase5_v0_answer_mode.md)).

## Current status

- Go CLI with Qdrant as the vector database backend.
- Local Ollama embedder using `nomic-embed-text` (768-dimensional vectors, auto-detected).
- Go files are chunked at the function level using `go/parser` AST; other languages use a 40-line sliding window.
- Chunk enrichment prepends file/language/function metadata to embedding input for improved search relevance.
- Search with dense vector lookup and optional language and repo payload filtering.
- Embedding dimension auto-detection and validation (collection mismatch produces clear error with fix instructions).
- Incremental re-indexing: only changed files are re-embedded; stale chunks from deleted/renamed files are cleaned up.
- **Retrieval evaluation harness** (`ragcodepilot eval`) with golden dataset, `hit@k`, `MRR@5`, `recall@10`, and per-stage latency percentiles.
- Collection list and delete commands.
- `config.yaml` is auto-loaded during indexing when present; built-in defaults are used only when it is absent.

Hybrid search is implemented: BM25 sparse vectors (`k1=0.5`, `b=0.75`) + dense vectors + Reciprocal Rank Fusion (`--mode dense|sparse|hybrid`, default `hybrid`), with additive Snowball stemming on the BM25 path. Latest baseline (`baseline_v4`): `hit@5 = 0.895`, `hit@1 = 0.579`, `MRR@5 = 0.699`. Cross-encoder reranking, the Rust AST chunker, and UX polish are tracked on the roadmap — see [`docs/plan/mvp_roadmap.md`](docs/plan/mvp_roadmap.md).

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
  |-- eval --dataset <golden.yaml>
  |     -> load golden queries
  |     -> run each through search.Searcher
  |     -> compute hit@k, MRR@5, recall@10, latency percentiles
  |     -> JSON or human-readable report
  |
  |-- collections list|delete
```

Qdrant runs locally through Docker Compose. The Go CLI runs on the host machine and connects to Qdrant over gRPC on port `6334`. Ollama runs locally and serves embeddings on port `11434`.

## Prerequisites

- Go `1.26.3`
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
go run ./cmd/ragcodepilot version
```

Build the project:

```bash
mkdir -p bin
go build -o bin/ragcodepilot ./cmd/ragcodepilot
```

Index this repository (Go files only):

```bash
go run ./cmd/ragcodepilot index --language go .
```

Search indexed code:

```bash
go run ./cmd/ragcodepilot search --language go --limit 5 "embedding interface"
```

Search with repo filter:

```bash
go run ./cmd/ragcodepilot search --repo ragcodepilot --limit 3 "ingestion pipeline"
```

Combine language and repo filters:

```bash
go run ./cmd/ragcodepilot search --language go --repo ragcodepilot --limit 3 "how does chunking work?"
```

List collections:

```bash
go run ./cmd/ragcodepilot collections list
```

Delete the default collection (required when changing embedding model or enrichment logic):

```bash
go run ./cmd/ragcodepilot collections delete code_chunks
```

Run retrieval evaluation against the golden dataset:

```bash
go run ./cmd/ragcodepilot eval --dataset docs/eval/golden.yaml
```

Run eval with JSON output (for diffing baselines):

```bash
go run ./cmd/ragcodepilot eval --output json > docs/eval/baseline_v2.json
```

Filter eval to a specific query type:

```bash
go run ./cmd/ragcodepilot eval --type navigation
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
| `-repo` | (all) | Comma-separated repo name filter (e.g., `ragcodepilot`) |
| `-mode` | `hybrid` | Retrieval mode: `dense`, `sparse`, `hybrid` |
| `-limit` | `5` | Maximum number of results |
| `-embedder` | `ollama` | Embedder to use: `ollama`, `fake` |
| `-ollama-url` | `http://localhost:11434` | Ollama server URL |
| `-ollama-model` | `nomic-embed-text` | Ollama embedding model |
| `-qdrant-host` | `localhost` | Qdrant host |
| `-qdrant-port` | `6334` | Qdrant gRPC port |

### Eval

| Flag | Default | Description |
|------|---------|-------------|
| `-dataset` | `docs/eval/golden.yaml` | Path to the golden YAML dataset |
| `-collection` | `code_chunks` | Qdrant collection name |
| `-output` | `human` | Output format: `human`, `json` |
| `-limit` | `10` | Per-query result limit (must be ≥ 10 for recall@10) |
| `-type` | (all) | Filter queries by type (`navigation`, `concept`, `behavior`, `negative`) |
| `-mode` | `hybrid` | Retrieval mode: `dense`, `sparse`, `hybrid` |
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
go build -o bin/ragcodepilot ./cmd/ragcodepilot
```

Inject a version at build time:

```bash
mkdir -p bin
go build -ldflags "-X main.version=dev-local" -o bin/ragcodepilot ./cmd/ragcodepilot
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
go run ./cmd/ragcodepilot collections delete code_chunks
go run ./cmd/ragcodepilot index --language go .
```

## Known limitations

- Function-level chunking is Go-only (AST-based). Other languages use a sliding window.
- Sparse vectors use BM25 with a softened `k1=0.5` (Elasticsearch's default `k1=1.2` is calibrated for long, mixed-length documents; code chunks are short and uniform, so milder TF saturation gave a much cleaner result on the May 2026 eval — hit@1 +21pp vs TF-IDF). The original plural/singular token-mismatch regression on the `hasher_concept` query was resolved on 2026-05-15 by additive Snowball stemming (`baseline_v4`). See [`docs/plan/hybrid_search.md`](docs/plan/hybrid_search.md) §3 for the full history and eval matrix.
- Embedding dimension is auto-detected; switching models requires collection delete + re-index.

## Further docs

- [MVP Roadmap](docs/plan/mvp_roadmap.md)
- [Phase 5 v0 — `--answer` mode plan](docs/plan/phase5_v0_answer_mode.md)
- [Hybrid search design + eval matrix](docs/plan/hybrid_search.md)
- [Retrieval quality decisions](docs/knowledge/retrieval_quality_decisions.md)
- [System design](docs/plan/system_design.md)
- [Progress checklist](docs/plan/checklist.md)
- [Evaluation harness](docs/eval/README.md)
- [Function-level chunker](docs/plan/function_level_chunker.md)
- [Embedding dimension validation](docs/plan/embedding_dimension_validation.md)
- [Chunk enrichment](docs/plan/chunk_enrichment.md)
- [RAG architecture parts](docs/knowledge/rag_parts.md)
- [Embeddings explained](docs/knowledge/embeddings_explained.md)
- [Vector database platform comparison](docs/knowledge/compare.md)
