# ragcodepilot

**Search your codebase using plain English, right from the terminal.**

`ragcodepilot` is a local **RAG** (Retrieval-Augmented Generation) code search CLI. The full RAG idea has three steps:

1. **Retrieve** — find the most relevant pieces of source code for a given question.
2. **Augment** — feed those code snippets into a prompt as context.
3. **Generate** — have an LLM produce an answer grounded in the retrieved code.

**Retrieval is fully implemented** — you can point the tool at a local repository, index it, and search with natural language queries like *"how does the chunking work?"*. **Answer generation (v0) is now available** via the opt-in `--answer` flag, which feeds the retrieved chunks to a local Ollama model (`qwen2.5-coder:7b`) and prints a synthesized answer above its sources. Support for additional LLM providers (OpenAI, Anthropic, etc.) is planned — see the [roadmap](docs/plan/mvp_roadmap.md).

Answer mode is **opt-in** — without it, the tool works as a pure code search engine. Both scripting (CLI one-liners) and interactive (REPL) usage are supported.

**How retrieval works:** The tool breaks source code into small chunks (functions, blocks of lines), converts each chunk into a numerical "fingerprint" (embedding) that captures its meaning, and stores everything in a local vector database ([Qdrant](https://qdrant.tech/)). It also builds a keyword index (BM25) alongside the embeddings and combines both signals for better results. When you search, your query is matched against the stored chunks to find the closest results.

## Current status

- Go CLI with Qdrant as the vector database backend.
- Local Ollama embedder using `nomic-embed-text` (768-dimensional vectors, auto-detected).
- Go files are chunked at the function level using `go/parser` AST; other languages use a 40-line sliding window.
- Chunk enrichment prepends file/language/function metadata to embedding input for improved search relevance.
- Search with dense vector lookup and optional language and repo payload filtering.
- **Answer mode (`--answer`)**: feeds retrieved chunks to a local Ollama generative model (`qwen2.5-coder:7b` by default) and prints a synthesized answer above its sources. Opt-in; the default `search` path is unchanged.
- Embedding dimension auto-detection and validation (collection mismatch produces clear error with fix instructions).
- Incremental re-indexing: only changed files are re-embedded; stale chunks from deleted/renamed files are cleaned up.
- **Retrieval evaluation harness** (`ragcodepilot eval`) with golden dataset, `hit@k`, `MRR@5`, `recall@10`, and per-stage latency percentiles.
- Collection list and delete commands.
- `config.yaml` is auto-loaded during indexing when present; built-in defaults are used only when it is absent.

Hybrid search is implemented: BM25 sparse vectors (`k1=0.5`, `b=0.75`) + dense vectors + Reciprocal Rank Fusion (`--mode dense|sparse|hybrid`, default `hybrid`), with additive Snowball stemming on the BM25 path. Current baseline (`baseline_v6`, 182 chunks, 23 golden queries): `hit@5 = 0.895`, `hit@1 = 0.579`, `MRR@5 = 0.673`, `recall@5 = 0.789`, `recall@10 = 0.921`. (Indexing excludes `*_test.go` by default — see [`config.yaml`](config.yaml) `skip_file_patterns`; this recovered `hit@5` from 0.789 to 0.895 by keeping test chunks out of the top-K.) Cross-encoder reranking, the Rust AST chunker, and UX polish are tracked on the roadmap — see [`docs/plan/mvp_roadmap.md`](docs/plan/mvp_roadmap.md).

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
- [Ollama](https://ollama.com/) with the `nomic-embed-text` model (required for indexing/search)
- For `--answer` mode only: a generative model (`qwen2.5-coder:7b` by default)

## Getting started

Start Qdrant:

```bash
docker compose up -d qdrant
```

Pull the embedding model (one-time):

```bash
ollama pull nomic-embed-text
```

For `--answer` mode, also pull the generative model (one-time):

```bash
ollama pull qwen2.5-coder:7b
```

**Tip — avoid cold-start latency:** the first `--answer` call after Ollama
starts spends 5–30s loading the generative model into memory. To keep the
model resident across calls (subsequent answers take 1–5s), set:

```bash
export OLLAMA_KEEP_ALIVE=-1
```

This pins models in Ollama's process (costs a few GB of RAM). It is the single
biggest perceived-speed improvement for `--answer` — see
[`docs/knowledge/architecture_decisions.md`](docs/knowledge/architecture_decisions.md) §4.

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

Index once, then stay running and re-index on file changes (incremental):

```bash
go run ./cmd/ragcodepilot index --language go --watch .
# Edits are detected via fsnotify; debounced and re-indexed via the same
# pipeline. Press Ctrl-C to exit. Respects skip_dirs and skip_file_patterns
# in config.yaml, just like a one-shot index.
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

Generate an answer from the retrieved chunks (RAG mode):

```bash
go run ./cmd/ragcodepilot search --answer "how does change detection work?"
```

This prints a synthesized `Answer:` followed by the `Sources:` chunks that fed
it. Use `--ollama-generative-model` to swap the model, or `--generator fake` for
a hermetic (no-Ollama) canned response. Without `--answer`, `search` behaves
exactly as before (retrieval only).

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

Evaluate answer mode too (reference-free answer metrics, real Ollama):

```bash
go run ./cmd/ragcodepilot eval --answer
```

This runs the normal retrieval eval **and** generates an answer per query, then
reports deterministic, reference-free answer metrics alongside the retrieval
numbers:

- **well-formed rate** — answers that are non-empty.
- **cited rate** / **all-citations-valid** — positive-query answers that cite
  `[N]` chunks, and whether those citations point at chunks that were actually
  provided (no dangling refs).
- **refusal rate (negative)** — negative queries where the model correctly said
  "not enough information" instead of hallucinating. This is the hallucination
  floor. (Refusal is detected by a phrase heuristic — a diagnostic, not ground
  truth.)

Answer metrics are **reported, never gated** — they never change the exit code.
Generation runs greedy (temperature 0) so re-runs are reproducible. Note this is
much slower than retrieval-only eval (one LLM call per query).

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
| `-watch` | `false` | After the initial index, watch repo for changes and re-index incrementally (blocks until Ctrl-C) |
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
| `-answer` | `false` | Generate an answer from the retrieved chunks (RAG mode) |
| `-generator` | `ollama` | Generator for `--answer`: `ollama`, `fake` |
| `-ollama-generative-model` | `qwen2.5-coder:7b` | Ollama generative model for `--answer` |
| `-answer-limit` | `5` | Number of top chunks fed to the generator for `--answer` |
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
| `-subtype` | (all) | Filter queries by subtype (e.g. `structural` under navigation; combines with `-type` as AND) |
| `-mode` | `hybrid` | Retrieval mode: `dense`, `sparse`, `hybrid` |
| `-embedder` | `ollama` | Embedder to use: `ollama`, `fake` |
| `-ollama-url` | `http://localhost:11434` | Ollama server URL |
| `-ollama-model` | `nomic-embed-text` | Ollama embedding model |
| `-answer` | `false` | Also generate answers and score reference-free answer metrics |
| `-generator` | `ollama` | Generator for `--answer`: `ollama`, `fake` |
| `-ollama-generative-model` | `qwen2.5-coder:7b` | Ollama generative model for `--answer` |
| `-answer-limit` | `5` | Top chunks fed to the generator for `--answer` (retrieval metrics still use `--limit`) |
| `-qdrant-host` | `localhost` | Qdrant host |
| `-qdrant-port` | `6334` | Qdrant gRPC port |

## Configuration

`config.yaml` controls language extension mappings, skipped directories, and skipped file patterns for indexing.

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

skip_file_patterns:
  - "*_test.go"
```

`skip_file_patterns` is a list of globs matched against each file's base name
(via Go's `filepath.Match`) to exclude files from indexing.

- **Default:** when the key is omitted, it defaults to `["*_test.go"]`, so Go
  test files are excluded. This keeps test fixtures (including eval golden
  queries) out of retrieval and reduces top-K crowding from test chunks.
- **Disable:** set `skip_file_patterns: []` to index everything, including test
  files.
- **Other languages:** add patterns such as `"*.test.ts"`, `"*_test.py"`, or
  `"*.spec.js"` as needed.

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
