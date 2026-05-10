# System Design: Semantic Code Search Application

> Created: May 2026 | Approach: top-down (use existing vector DB first, study internals later)

## Overall roadmap

```
Phase A: Build application on Qdrant     ← THIS DOCUMENT
Phase B: Study vector DB internals       ← Vector_DB_core.md
Phase C: Refactor Rust vector DB to Go   ← Future
```

---

## Mapping to full RAG architecture

The full enterprise RAG system (see `rag_parts.md`) has five components. Our project implements a subset focused on retrieval, not generation:

| Full RAG component | Our project equivalent | Status |
|---|---|---|
| **RAG Server** (orchestrator) | CLI + Ingestion Pipeline + Search Service | We build this |
| **Qdrant Server** (vector DB) | Qdrant running in Docker | We use as-is |
| **Embedding Server** | Our `Embedder` interface + implementation | We build this |
| **MongoDB + MinIO** (metadata + files) | Not needed — we read files directly from local filesystem | Skipped |
| **LLM Server** (answer generation) | Not included — we return raw code chunks, not generated answers | Skipped |

Why we skip MongoDB/MinIO: we clone repos locally and index from the filesystem. No file upload workflow needed.

Why we skip the LLM: for code refactoring, you want to read the **actual source code**, not a paraphrased summary. Returning ranked code chunks is more useful than generated text.

---

## Step 1: Requirements and scope

### Goal

Build a Go CLI application that indexes code repositories and enables semantic search over them, using Qdrant as the vector database backend. The primary purpose is to learn how vector DB applications work before refactoring a Rust vector DB to Go.

### Functional requirements

| # | Requirement | Vector DB concept it teaches |
|---|---|---|
| F1 | Ingest code from local Git repositories | Batch upsert, point structure, payloads |
| F2 | Parse and chunk code into meaningful units (functions, classes, blocks) | Data modeling, chunking strategies |
| F3 | Generate vector embeddings for each code chunk | Embeddings, dimensions, distance metrics |
| F4 | Semantic search: natural language query → relevant code | Nearest-neighbor search, scoring |
| F5 | Filtered search: by language, repo, file path | Payload indexing, filtered vector search |
| F6 | Hybrid search: exact keyword match + semantic similarity | Sparse + dense vectors, score fusion |
| F7 | Re-index when code changes (add/update/delete) | Point updates, deletions, collection management |

### Non-functional requirements

| # | Requirement | Notes |
|---|---|---|
| NF1 | Single-user, local deployment | No auth, no multi-tenancy initially |
| NF2 | Written in Go | Builds experience for Phase C (Rust→Go refactor) |
| NF3 | Qdrant as vector DB | Run via Docker, interact via Go SDK |
| NF4 | Must exercise core vector DB features | Collections, points, vectors, payloads, filtering, hybrid search |

### Scale estimate

This is a learning project, not production scale. Estimating helps build the habit.

```
Target: index 5-10 medium repos (~50K code files, ~200K chunks)

Storage:
  200K chunks × 768-dim × 4 bytes/float = ~600 MB vector data
  200K chunks × ~500 bytes avg payload   = ~100 MB metadata
  Total Qdrant storage: ~700 MB (fits in memory on any laptop)

Ingestion:
  200K chunks × ~50ms per embedding = ~2.8 hours (sequential)
  With batching (32 at a time): ~30 min

Search:
  Single user, <10 QPS
  Target latency: <100ms per query
```

---

## Step 2: High-level design

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│                        CLI / TUI                        │
│                                                         │
│  ragcodepilot index <repo-path>    ragcodepilot search "query" │
└──────────┬──────────────────────────────┬───────────────┘
           │                              │
           ▼                              ▼
┌─────────────────────┐      ┌─────────────────────────┐
│  Ingestion Pipeline │      │     Search Service      │
│                     │      │                         │
│  1. Walk files      │      │  1. Receive query       │
│  2. Parse/chunk     │      │  2. Embed query         │
│  3. Embed chunks    │      │  3. Build Qdrant request│
│  4. Upsert to       │      │     (vector + filters)  │
│     Qdrant          │      │  4. Call Qdrant         │
│                     │      │  5. Format & return     │
└────────┬────────────┘      └───────────┬─────────────┘
         │                               │
         │         ┌─────────────┐       │
         │         │  Embedding  │       │
         ├────────►│   Service   │◄──────┤
         │         │             │       │
         │         │ (API or     │       │
         │         │  local model)│      │
         │         └─────────────┘       │
         │                               │
         ▼                               ▼
    ┌─────────────────────────────────────────┐
    │              Qdrant (Docker)            │
    │                                         │
    │  Collection: "code_chunks"              │
    │  ├── dense vector (auto-detected, cosine)│
    │  ├── sparse vector (BM25, optional)     │
    │  └── payload:                           │
    │       ├── repo: string                  │
    │       ├── file_path: string             │
    │       ├── language: string              │
    │       ├── chunk_type: string            │
    │       ├── name: string                  │
    │       ├── content: string               │
    │       ├── start_line: int               │
    │       └── end_line: int                 │
    └─────────────────────────────────────────┘
```

### Components

#### 1. CLI

Simple command-line tool:

```
ragcodepilot index <repo-path> [--language go,rust] [--collection code_chunks]
ragcodepilot search "how does WAL recovery work?" [--language rust] [--limit 10]
ragcodepilot collections list
ragcodepilot collections delete <name>
```

No web UI initially. CLI is faster to build and sufficient for learning.

#### 2. Ingestion pipeline

Turns a Git repository into searchable vectors:

```
repo path → file walker → language detector → code parser → chunker → enrichment → embedder → Qdrant upsert
```

- **File walker**: recursively walk directory, skip `.git`, `vendor`, `node_modules`, binary files
- **Language detector**: detect by file extension (`.go`, `.rs`, `.py`, `.js`, etc.)
- **Code parser**: extract meaningful units. Start simple (split by function/class boundaries using regex), can improve later with tree-sitter
- **Chunker**: split large functions/files into smaller chunks with overlap. Target ~200-500 tokens per chunk
- **Batch upsert**: send chunks to Qdrant in batches of 32-64 points

#### 3. Search service

Handles query processing and result formatting:

```
user query → embed query → build Qdrant search request → execute → format results
```

Supports three search modes:
- **Semantic only**: dense vector search (default)
- **Filtered**: semantic + payload filter (e.g., language=rust)
- **Hybrid**: dense + sparse (BM25) with RRF fusion (later phase)

#### 4. Embedding service

Abstracts the embedding model behind a simple interface:

```go
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Dimension() int
}
```

Two implementations:
- **Ollama** (`ollama.go`): calls local Ollama server with `nomic-embed-text` model (768d). Vector dimension is auto-detected from the first response and validated on all subsequent calls.
- **Fake** (`fake.go`): generates deterministic pseudo-random vectors for pipeline testing without a real model.

The embedder also includes vector dimension validation (`validate.go`) that prevents model-collection mismatches at both index and search time.

#### 5. Qdrant

Run locally via Docker:

```bash
docker run -p 6333:6333 -p 6334:6334 qdrant/qdrant
```

---

## Step 3: Data flow

### Ingestion flow

```
Git repo → File walker → Language detector → Code parser → Chunker → Enrichment → Embedder → Batch upsert → Qdrant
```

The **enrichment** step prepends structured metadata (file path, language, chunk type/name) to the raw code before embedding. This gives the embedding model human-readable context that significantly improves semantic search for natural-language queries. The raw code stored in Qdrant payload is unchanged — only the embedding input is enriched. See `docs/plan/chunk_enrichment.md` for details.

Key decisions:

| Decision | Choice | Rationale |
|---|---|---|
| Chunk size | 200-500 tokens | Too small = no context. Too large = noisy embeddings |
| Chunk overlap | 50 tokens | Prevents losing context at chunk boundaries |
| Chunk unit | Sliding window (current), function-level planned | Semantic boundaries are better than arbitrary splits |
| Embedding model | `nomic-embed-text` via Ollama (768d) | Local, no API costs, good code/text understanding |
| Vector dimension | Auto-detected from model response | Prevents silent mismatch when switching models |
| Batch size | 32 chunks per embed, 64 points per Qdrant upsert | Balances throughput and memory |
| Point ID | Deterministic hash of `repo + file_path + start_line` | Enables re-indexing without duplicates |

### Search flow

```
User query → Embed query → Search mode selection:
  ├── Semantic:  dense vector search
  ├── Filtered:  dense vector + payload filter
  └── Hybrid:    dense + sparse + RRF fusion
→ Qdrant returns ranked results → Format and display
```

### Data model (Qdrant point)

```json
{
  "id": "a1b2c3d4-...",
  "vector": {
    "dense": [0.12, -0.31, 0.88, "..."]
  },
  "payload": {
    "repo": "qdrant/qdrant",
    "file_path": "src/segment/src/wal.rs",
    "language": "rust",
    "chunk_type": "function",
    "name": "recover_from_wal",
    "content": "fn recover_from_wal(&self) -> Result<()> { ... }",
    "start_line": 142,
    "end_line": 187,
    "indexed_at": "2026-05-07T00:00:00Z"
  }
}
```

---

## Step 4: Build phases

### Phase 1: Minimal semantic search ✅

- ✅ Set up Go project structure
- ✅ Run Qdrant in Docker
- ✅ Implement simple file walker + text chunker (sliding window with overlap)
- ✅ Implement embedder — Ollama with `nomic-embed-text` (768d) + fake embedder for testing
- ✅ Embedding dimension auto-detection and validation (see `docs/plan/embedding_dimension_validation.md`)
- ✅ Chunk enrichment — prepend file path, language, and chunk type/name metadata before embedding (see `docs/plan/chunk_enrichment.md`)
- ✅ Upsert chunks to Qdrant
- ✅ Implement basic semantic search via CLI
- ✅ Collection management commands (list, delete)
- ✅ Search result formatting with scores and metadata
- ✅ Externalized configuration (`config.yaml` with language/extension mappings)
- **Goal**: `ragcodepilot index . && ragcodepilot search "WAL recovery"` works ✅

### Phase 2: Filtering and better parsing (in progress)

- ✅ Add language detection by file extension
- ✅ Add `--language` payload filtering on both index and search
- Add `--repo` payload filtering on search
- ✅ Function-level chunking for Go files (AST-based, see `docs/plan/function_level_chunker.md`)
- Improve chunker: regex heuristics for Python/Rust
- Add re-indexing (detect changed files, update/delete stale points)
- **Goal**: filtered search works, chunks are meaningful code units

### Phase 3: Hybrid search

- Add sparse vectors for BM25-style keyword matching
- Implement hybrid search with RRF fusion
- Add exact function name search alongside semantic
- **Goal**: hybrid search finds code by both meaning and keywords

### Phase 4: Learn internals

- Study Qdrant internals: how does it store vectors? HNSW? Segments?
- Map what you learned to `Vector_DB_core.md` concepts
- **Goal**: ready to start Phase C (Rust→Go vector DB refactor)

---

## Tradeoffs and decisions

| Tradeoff | Decision | Why |
|---|---|---|
| API embedding vs local | Local Ollama from the start | No API costs, works offline, sufficient quality with `nomic-embed-text` |
| Hardcoded vs auto-detected dimension | Auto-detected from first embedding response | Prevents silent search failures when switching models |
| Raw code vs enriched embedding input | Enriched: prepend file path, language, chunk type/name | Dramatically improves natural-language query matching on unfamiliar repos |
| Tree-sitter parsing vs regex | Start with regex, consider tree-sitter or Go AST later | Regex is simpler; AST gives better chunks but adds complexity |
| CLI vs web UI | CLI only | Faster to build; sufficient for learning |
| Single collection vs per-repo | Single collection with repo as payload filter | Simpler; cross-repo search works naturally |
| Chunk size | ~40 lines with 10-line overlap | Balanced between precision and context |

---

## Go project structure

```
ragcodepilot/
├── cmd/
│   └── ragcodepilot/
│       └── main.go              # CLI entry point
├── internal/
│   ├── ingest/
│   │   ├── walker.go            # File system walker
│   │   ├── chunker.go           # Code chunking logic
│   │   ├── enrichment.go        # Prepend metadata to chunks before embedding
│   │   └── pipeline.go          # Orchestrates walk → chunk → enrich → embed → upsert
│   ├── search/
│   │   └── searcher.go          # Query embedding + Qdrant search + formatting
│   ├── embedding/
│   │   ├── embedder.go          # Embedder interface
│   │   ├── ollama.go            # Ollama local model (nomic-embed-text)
│   │   ├── fake.go              # Deterministic random vectors for testing
│   │   └── validate.go          # Vector dimension validation
│   ├── qdrant/
│   │   └── client.go            # Qdrant client wrapper (with dimension validation)
│   ├── config/
│   │   └── config.go            # YAML config loader + language detection
│   └── model/
│       └── chunk.go             # CodeChunk, SearchResult types
├── docs/
│   ├── plan/                    # Design documents and checklists
│   └── knowledge/               # Learning notes (embeddings, RAG, comparisons)
├── config.yaml                  # Language/extension mappings + skip dirs
├── docker-compose.yml           # Qdrant service
├── go.mod
└── go.sum
```

---

## FEEDBACK: Suggested Improvements

FEEDBACK: The overall architecture is well-scoped for a learning-oriented semantic code search tool. The strongest design choice is explicitly skipping LLM answer generation and returning actual source chunks, which matches the goal of code understanding and future vector DB internals work.

### FEEDBACK: Treat evaluation as a first-class subsystem

FEEDBACK: Add evaluation directly to the system design because retrieval quality will guide Phase 2 filtering, Phase 3 hybrid search, and future reranking decisions.

Suggested addition to the architecture:

```text
Evaluation Runner
  ├── loads eval datasets
  ├── runs the same Search Service path used by the CLI
  ├── compares returned chunks against expected files/symbols
  ├── records retrieval metrics and latency metrics
  └── writes JSON reports for regression comparison
```

Recommended repository structure:

```text
ragcodepilot/
├── internal/
│   ├── eval/
│   │   ├── dataset.go
│   │   ├── runner.go
│   │   ├── metrics.go
│   │   ├── report.go
│   │   └── compare.go
│   ├── ingest/
│   ├── search/
│   ├── embedding/
│   ├── qdrant/
│   ├── config/
│   └── model/
├── docs/
│   └── eval/
│       ├── ragcodepilot_retrieval.yaml
│       ├── ragcodepilot_filters.yaml
│       ├── ragcodepilot_negative.yaml
│       └── README.md
├── eval/
│   └── results/
│       └── .gitkeep
└── testdata/
    └── repos/
        └── tiny-go-repo/
```

FEEDBACK: Keep eval code in `internal/eval`, human-maintained datasets in `docs/eval`, generated reports in `eval/results`, and small deterministic test repositories in `testdata/repos`.

### FEEDBACK: Add an evaluation command to the CLI

FEEDBACK: Add `ragcodepilot eval` alongside `index`, `search`, and `collections`.

Suggested CLI shape:

```bash
ragcodepilot eval retrieval --dataset docs/eval/ragcodepilot_retrieval.yaml --collection code_chunks --top-k 5
ragcodepilot eval filters --dataset docs/eval/ragcodepilot_filters.yaml --collection code_chunks
ragcodepilot eval compare --baseline eval/results/base.json --candidate eval/results/new.json
```

FEEDBACK: The eval runner should call the same search path as normal CLI search. Avoid a separate evaluation-only search implementation because it can drift from production behavior.

### FEEDBACK: Strengthen the point ID strategy

FEEDBACK: The current point ID strategy uses `repo + file_path + start_line`, which is deterministic but fragile. `start_line` changes whenever lines are inserted above a function, which can create new IDs for unchanged functions and leave stale vectors behind.

Recommended payload fields:

```json
{
  "repo": "ragcodepilot",
  "branch": "main",
  "commit": "abc123",
  "file_path": "internal/ingest/chunker.go",
  "file_hash": "sha256...",
  "content_hash": "sha256...",
  "language": "go",
  "chunk_type": "function",
  "symbol": "ChunkFile",
  "chunk_index": 0,
  "start_line": 42,
  "end_line": 88
}
```

Recommended ID strategy:

```text
function chunk:
  repo + file_path + symbol + chunk_type + chunk_index

anonymous/sliding chunk:
  repo + file_path + content_hash
```

FEEDBACK: Keep `start_line` and `end_line` as payload for display, but avoid relying on line number as the primary identity.

### FEEDBACK: Add stale-delete and re-indexing design details

FEEDBACK: Re-indexing is listed in Phase 2, but the design should define how old points are removed.

Suggested approach:

```text
1. Compute file hash for each indexed file.
2. Skip unchanged files.
3. For changed files, delete old points where repo + file_path match.
4. Re-chunk, re-embed, and upsert new points.
5. For deleted files, delete all points where repo + file_path match.
```

FEEDBACK: This is simpler and safer than trying to update individual chunks when function boundaries or line numbers change.

### FEEDBACK: Make payload indexes explicit

FEEDBACK: Since filtered search is a functional requirement, the system design should specify which Qdrant payload fields should be indexed.

Recommended payload indexes:

```text
repo
file_path
language
chunk_type
name / symbol
indexed_at
```

FEEDBACK: This makes filtered vector search faster and makes filter behavior easier to validate in the eval harness.

### FEEDBACK: Clarify chunking strategy by language

FEEDBACK: The design mentions sliding-window chunking, Go AST function-level chunking, and future regex heuristics for Python/Rust. Add a table that makes the intended behavior explicit.

Suggested table:

| Language | Initial strategy | Target strategy |
|---|---|---|
| Go | AST function-level chunking | AST function + method + type-aware chunks |
| Rust | sliding window / regex | tree-sitter or Rust parser |
| Python | sliding window / regex | tree-sitter or AST |
| Markdown | heading-based chunks | heading-based chunks |
| Unknown | sliding window | sliding window |

FEEDBACK: This helps explain why some languages should score better than others during evaluation.

### FEEDBACK: Add score interpretation and thresholds

FEEDBACK: Qdrant similarity scores are useful, but the design should avoid treating them as absolute truth across models or search modes.

Suggested note:

```text
Search scores are ranking signals, not universal confidence values.
Thresholds should be calibrated per embedding model, distance metric, and search mode.
Negative eval cases should be used to choose practical "no strong match" thresholds.
```

FEEDBACK: This will matter when adding hybrid search because dense scores, sparse scores, and fused scores are not directly comparable.

### FEEDBACK: Add observability hooks

FEEDBACK: The search and ingestion flows should expose timing information so evaluation can separate total latency from embedding latency and Qdrant latency.

Suggested timing fields:

```json
{
  "timing": {
    "total_ms": 91,
    "query_embedding_ms": 37,
    "qdrant_search_ms": 42,
    "formatting_ms": 3
  }
}
```

FEEDBACK: This supports the performance metrics proposed in the evaluation document and helps diagnose whether regressions come from embedding, Qdrant, or result formatting.

### FEEDBACK: Add a baseline workflow before hybrid search

FEEDBACK: Before Phase 3 hybrid search, create and save a baseline report for the current semantic-only system.

Suggested workflow:

```bash
ragcodepilot index . --collection code_chunks
ragcodepilot eval retrieval --dataset docs/eval/ragcodepilot_retrieval.yaml --collection code_chunks --out eval/results/semantic_baseline.json
```

FEEDBACK: After adding hybrid search, compare against the baseline:

```bash
ragcodepilot eval compare --baseline eval/results/semantic_baseline.json --candidate eval/results/hybrid_candidate.json
```

FEEDBACK: Hybrid search should improve exact symbol/config queries without significantly hurting semantic concept queries or p95 latency.

### FEEDBACK: Add explicit non-goals

FEEDBACK: The design already says no web UI, no MongoDB/MinIO, and no LLM server. Add evaluation-related non-goals too.

Suggested non-goals:

```text
- No generated-answer evaluation until an LLM answer layer exists.
- No large public benchmark integration in the first version.
- No multi-user analytics or production observability stack.
- No automatic relevance labeling in the first version.
```

FEEDBACK: These boundaries keep the project focused and prevent the eval harness from becoming too large too early.

### FEEDBACK: Recommended near-term implementation order

FEEDBACK: The next implementation sequence should be:

```text
1. Add docs/eval/ragcodepilot_retrieval.yaml with 20-30 queries.
2. Implement internal/eval dataset loader.
3. Implement hit@k, MRR@5, recall@5.
4. Add latency timing to the search service.
5. Add ragcodepilot eval retrieval command.
6. Save JSON reports to eval/results.
7. Add filter-specific eval cases.
8. Add negative cases with calibrated score thresholds.
9. Add compare mode.
10. Use the evaluator before changing hybrid search or reranking.
```

FEEDBACK: This creates a scoreboard before making bigger search-quality changes.

