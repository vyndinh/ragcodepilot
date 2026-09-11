# System Design: Semantic Code Search Application

> Created: May 2026 | Last refreshed: 2026-07-06 (reflects shipped Phases 1–2 + 5 v0)
> Approach: top-down (use existing vector DB first, study internals later)

## Overall roadmap

```
Phase A: Build application on Qdrant     ← THIS DOCUMENT
Phase B: Study vector DB internals       ← vector_db_core.md
Phase C: Refactor Rust vector DB to Go   ← Deferred indefinitely (see mvp_roadmap.md)
```

Phase/feature sequencing is owned by [`mvp_roadmap.md`](mvp_roadmap.md) — this
document describes the **current architecture as built**, plus the original
requirements and scale framing. Where the two disagree, the roadmap wins.

---

## Mapping to full RAG architecture

The full enterprise RAG system (see `rag_parts.md`) has five components. The
project now implements four of them:

| Full RAG component | Our project equivalent | Status |
|---|---|---|
| **RAG Server** (orchestrator) | CLI + Ingestion Pipeline + Search Service | Built |
| **Qdrant Server** (vector DB) | Qdrant running in Docker | Used as-is |
| **Embedding Server** | `Embedder` interface + Ollama / Fake implementations | Built |
| **MongoDB + MinIO** (metadata + files) | Not needed — files read directly from local filesystem | Skipped |
| **LLM Server** (answer generation) | `Generator` interface + Ollama (`qwen2.5-coder:7b` default), opt-in via `search --answer` | **Built (Phase 5 v0)** |

Why we skip MongoDB/MinIO: we clone repos locally and index from the
filesystem. No file upload workflow needed.

Answer generation is **opt-in**: the default `search` path returns ranked raw
code chunks (for code work you usually want the actual source). `--answer`
layers a grounded, citation-formatted LLM answer on top of the same retrieval.
See [`phase5_v0_answer_mode.md`](phase5_v0_answer_mode.md).

---

## Step 1: Requirements and scope

### Goal

Build a Go CLI application that indexes code repositories and enables semantic
search — and, opt-in, RAG answers — over them, using Qdrant as the vector
database backend. Originally a learning vehicle for vector-DB applications;
now evolving toward a full local RAG pipeline (see `mvp_roadmap.md` product
direction).

### Functional requirements

| # | Requirement | Status |
|---|---|---|
| F1 | Ingest code from local Git repositories | ✅ |
| F2 | Parse and chunk code into meaningful units | ✅ Go: AST function-level; other languages: sliding window + regex naming |
| F3 | Generate vector embeddings for each code chunk | ✅ dense (Ollama) + sparse (BM25), enriched input |
| F4 | Semantic search: natural language query → relevant code | ✅ |
| F5 | Filtered search: by language, repo | ✅ payload-indexed filters |
| F6 | Hybrid search: exact keyword match + semantic similarity | ✅ BM25 + dense + server-side RRF (default mode) |
| F7 | Re-index when code changes (add/update/delete) | ✅ file-hash + index-version change detection; `index --watch` |
| F8 | Measure retrieval quality | ✅ `eval` harness, golden set, committed baselines |
| F9 | Generate grounded answers with citations | ✅ v0 (`--answer`, frozen prompt, greedy decoding) |

### Non-functional requirements

| # | Requirement | Notes |
|---|---|---|
| NF1 | Single-user, local deployment | No auth, no multi-tenancy. Local-first: no cloud APIs |
| NF2 | Written in Go | Builds experience for the (deferred) Phase C refactor |
| NF3 | Qdrant as vector DB | Docker, gRPC SDK |
| NF4 | Deterministic where possible | Greedy decoding for answers; deterministic chunk IDs; reproducible eval |
| NF5 | Default path stays stable | New capability ships behind opt-in flags; default byte-identical |

### Scale estimate

Original target framing (kept as the design envelope):

```
Target: index 5-10 medium repos (~50K code files, ~200K chunks)

Storage:
  200K chunks × 768-dim × 4 bytes/float ≈ 600 MB dense vectors
  + sparse vectors + payloads             ≈ 100–200 MB
  Total Qdrant storage: <1 GB (fits on any laptop)

Ingestion:
  Embedding is the bottleneck (~50ms/chunk); batching (32) → ~30 min for 200K chunks

Search:
  Single user, <10 QPS; measured hybrid p95 ≈ 120–140 ms (baseline_v4/v6)
```

**Known gap vs this envelope:** the current corpus is ~182 chunks (this repo,
tests excluded), and a re-index with *any* change re-embeds the full corpus
because sparse IDF is corpus-wide. Diff-proportional re-embedding must land
before the 200K-chunk envelope is real — see
[`../improvement/production_readiness_and_features.md`](../improvement/production_readiness_and_features.md) §1.1.

---

## Step 2: High-level design

### Architecture

```
┌───────────────────────────────────────────────────────────────────┐
│                              CLI                                  │
│  index [--watch]   search [--mode] [--answer]   collections   eval│
└──────┬───────────────────────┬────────────────────────┬───────────┘
       ▼                       ▼                        ▼
┌───────────────┐   ┌────────────────────┐   ┌────────────────────┐
│   Ingestion   │   │   Search Service   │   │   Eval Harness     │
│   Pipeline    │   │                    │   │                    │
│ walk → hash → │   │ embed query        │   │ golden.yaml →      │
│ classify →    │   │ → dense/sparse/    │   │ run each query →   │
│ chunk →       │   │   hybrid (RRF)     │   │ hit@k, MRR, recall,│
│ enrich →      │   │ → filters          │   │ latency, neg pass  │
│ embed →       │   │ → format results   │   │ (+ Tier B answer   │
│ upsert        │   │ → [--answer] LLM   │   │    metrics)        │
└──────┬────────┘   └─────┬──────────┬───┘   └─────────┬──────────┘
       │                  │          │                 │
       │        ┌─────────▼───┐  ┌───▼──────────┐      │
       │        │  Embedder   │  │  Generator   │      │
       ├───────►│ (Ollama     │  │ (Ollama      │      │
       │        │  nomic-     │  │  qwen2.5-    │      │
       │        │  embed-text │  │  coder:7b /  │      │
       │        │  / Fake)    │  │  Fake)       │      │
       │        └─────────────┘  └──────────────┘      │
       ▼                                               ▼
    ┌──────────────────────────────────────────────────────┐
    │                  Qdrant (Docker)                     │
    │  Collection: "code_chunks"                           │
    │  ├── named dense vector  ("dense", auto-detected dim,│
    │  │                        cosine)                    │
    │  ├── named sparse vector ("sparse", BM25 weights)    │
    │  └── payload: repo, file_path, language, chunk_type, │
    │      name, content, start_line, end_line, indexed_at,│
    │      file_hash, index_version                        │
    └──────────────────────────────────────────────────────┘
```

### Components

#### 1. CLI (`cmd/ragcodepilot`)

```
ragcodepilot index <repo-path> [--language go,rust] [--collection X] [--watch]
ragcodepilot search "query" [--mode dense|sparse|hybrid] [--language ...] [--repo ...]
                            [--limit N] [--answer] [--answer-limit N]
                            [--generator ollama|fake] [--ollama-generative-model M]
ragcodepilot eval [--dataset docs/eval/golden.yaml] [--output human|json]
                  [--type T] [--subtype S] [--answer]
ragcodepilot collections list | delete <name>
ragcodepilot version
```

All commands share `--qdrant-host/--qdrant-port`, `--embedder ollama|fake`,
`--ollama-url`, `--ollama-model`. Index and search must use the same embedding
model or vectors are incompatible (dimension validation catches mismatches).

#### 2. Ingestion pipeline (`internal/ingest`)

```
repo path → walk → hash files → classify (unchanged / changed / new / stale /
index-version refresh) → delete stale → chunk → enrich → compute corpus BM25
stats → embed (dense) + build sparse → batch upsert → late-delete orphaned
chunks of changed files
```

- **Walker**: skips hidden dirs, configured `skip_dirs`, and `skip_file_patterns`
  (notably `*_test.go` — excluded by default after the Phase 5 dogfooding
  finding; see `retrieval_quality_decisions.md` §2.5).
- **Change detection**: SHA-256 file hash + `index_version` payload per chunk —
  see [`../improvement/reindexing.md`](../improvement/reindexing.md).
- **Chunkers**: Go → AST function-level (`chunker_go.go`); other languages →
  sliding window (~40 lines, 10-line overlap) with regex name extraction.
- **Enrichment**: prepends file path / language / chunk type+name to the text
  sent to the embedder (payload keeps raw code) — `chunk_enrichment.md`.
- **Sparse vectors**: code-aware tokenizer (camelCase/snake_case splitting,
  stop-word + Go-keyword removal, additive Snowball stemming), BM25 with
  `k1=0.5, b=0.75`, IDF computed corpus-wide per run — `hybrid_search.md`.
- **Watch mode**: `index --watch` = fsnotify + 500 ms debounce, re-runs the
  pipeline on change — `architecture_decisions.md` §3.

#### 3. Search service (`internal/search`)

```
query → embed → build Qdrant request per mode:
  dense  : named dense vector search
  sparse : named sparse vector search
  hybrid : prefetch dense + sparse → server-side RRF (k=60)   ← default
filters (language/repo) applied per prefetch stage → format results
```

Per-stage timings (embed / qdrant / total) are captured for the eval harness.

#### 4. Embedding service (`internal/embedding`)

```
interface Embedder:
  Embed(texts[]) → vectors[][]
  Dimension()    → int
```

- **Ollama** (`nomic-embed-text`, 768d): dimension auto-detected from the first
  batch and validated on every subsequent one (`validate.go`) — prevents silent
  model/collection mismatches.
- **Fake**: deterministic pseudo-random vectors for tests.

#### 5. Answer generation (`internal/answer`) — Phase 5 v0

```
interface Generator:
  Generate(ctx, query, chunks[]) → answer text

interface Warmer (optional):        # type-asserted; pulls model load out of the timed call
  Warmup(ctx) → error
```

- **OllamaGenerator** (default model `qwen2.5-coder:7b`): frozen v0 system
  prompt (golden-tested wording), greedy decoding (temperature 0, fixed seed),
  numbered-chunk context with `[N]` citations.
- **FakeGenerator** for plumbing tests.
- Opt-in only; without `--answer` the search path is byte-identical to pre-v0.

#### 6. Eval harness (`internal/eval`)

Golden YAML dataset (39 queries: navigation / concept / behavior / negative,
with a 16-query `structural` subtype) → runs the real search path → reports
`hit@1/3/5`, `MRR@5`, `recall@5/10` + recall gap, `negative_pass_rate`,
per-stage latency percentiles, per-type breakdown. With `--answer`, adds
reference-free Tier B answer metrics (citation validity, refusal-on-negative,
well-formedness) — report-only, never gated. Baselines are committed under
`docs/eval/`; `baseline_v6.json` is canonical. See
[`../eval/README.md`](../eval/README.md).

#### 7. Qdrant (`internal/qdrant`)

gRPC wrapper: collection CRUD with named dense+sparse vector schema, payload
indexes (repo / language / file_path), batch upsert, unified search (all three
modes), scroll for file states, targeted deletes for stale/orphaned chunks.

---

## Step 3: Data flow

### Ingestion flow

```
Git repo → Walk → Hash → Classify → Chunk (AST | window) → Enrich
        → BM25 corpus stats → Embed dense + build sparse → Batch upsert
        → Late-delete orphaned chunks
```

Key decisions:

| Decision | Choice | Rationale |
|---|---|---|
| Chunk unit | Go: per-function (AST); others: sliding window ~40 lines / 10 overlap | Semantic boundaries beat arbitrary splits |
| Embedding input | Enriched (path + language + type/name header) | Large lift on natural-language queries |
| Embedding model | `nomic-embed-text` via Ollama (768d) | Local, free, adequate; code-specialized model is an open lever (`cheaper_levers.md`) |
| Vector dimension | Auto-detected + validated | Prevents silent mismatch when switching models |
| Sparse algorithm | BM25 `k1=0.5, b=0.75` + Snowball stemming | Eval-driven: +15.8pp hit@1 vs TF-IDF with no hit@5 loss (`hybrid_search.md` §3) |
| Test files | `*_test.go` excluded by default | They crowded top-K; excluding lifted hit@5 0.789→0.895 |
| Batch size | 32 embed / upsert batch | Throughput vs memory |
| Point ID | Deterministic hash of repo + file path + symbol name + chunk index | Stable across line shifts; re-index without duplicates |
| Change detection | SHA-256 file hash + `index_version` | mtime is unreliable; version catches tokenizer changes |

### Search flow

```
User query → Embed → mode (dense | sparse | hybrid+RRF) + filters
→ ranked chunks → format
→ [--answer] top --answer-limit chunks → warm generator → grounded answer + [N] citations + sources
```

### Data model (Qdrant point)

```json
{
  "id": "a1b2c3d4-...",
  "vector": {
    "dense":  [0.12, -0.31, "..."],
    "sparse": {"indices": [17, 542, "..."], "values": [1.9, 0.7, "..."]}
  },
  "payload": {
    "repo": "ragcodepilot",
    "file_path": "internal/ingest/chunker.go",
    "language": "go",
    "chunk_type": "function",
    "name": "ChunkFile",
    "content": "…raw source…",
    "start_line": 42,
    "end_line": 87,
    "indexed_at": "2026-07-06T00:00:00Z",
    "file_hash": "9f2c…",
    "index_version": "sparse-v2"
  }
}
```

---

## Step 4: Build status

> Historical phase-by-phase checklists live in [`checklist.md`](checklist.md);
> current sequencing lives in [`mvp_roadmap.md`](mvp_roadmap.md). Snapshot:

- ✅ **P1 — Eval foundation**: harness, golden set, committed baselines.
- ✅ **P2 — Hybrid search**: BM25 sparse + dense + server-side RRF, default mode.
- ✅ **P5 v0 — `--answer` mode**: grounded answers via local Ollama, Tier B answer eval.
- ✅ **Re-indexing + watch mode**: hash/version change detection, fsnotify watch.
- ▶ **Next per the 2026-07-06 roadmap restructure**: milestone **M0** (decide
  the navigation bet: GraphRAG prerequisites + symbol-table spike), then
  M1 streaming answers → M2 agent integration (`--json`, MCP server) →
  M3 real-repo trust → M4 hardening → M5 evidence-scoped retrieval levers.
- ⏸ Reranking, Rust AST chunker, GraphRAG — gated inside M5; see roadmap.

---

## Tradeoffs and decisions

| Tradeoff | Decision | Why |
|---|---|---|
| API embedding vs local | Local Ollama | No API costs, offline, privacy; local-first is a product constraint |
| Answer generation | Opt-in `--answer`, frozen prompt, greedy | Default path stays deterministic and fast; answers reproducible |
| Tree-sitter vs regex vs AST | Go AST now; regex fallback; tree-sitter deferred | Best chunks where it matters most, least complexity |
| CLI vs daemon | Thin CLI; Qdrant+Ollama are the daemons | See `architecture_decisions.md` — daemon solves problems we don't have |
| Single collection vs per-repo | Single collection, repo payload filter | Cross-repo search works naturally |
| Eval gating | Report-only, manual judgment | Determinism first; CI gating is a known gap (production_readiness doc §1.4) |

---

## Go project structure

```
ragcodepilot/
├── cmd/ragcodepilot/            # CLI entry point (index, search, eval, collections, version)
├── internal/
│   ├── ingest/                  # walker, hasher, chunker (+ Go AST), enrichment, pipeline, watcher
│   ├── search/                  # query embedding + mode selection + Qdrant search + timings
│   ├── embedding/               # Embedder iface, ollama, fake, validate, sparse (BM25 tokenizer/stats)
│   ├── answer/                  # Generator iface, ollama, fake, prompt (frozen v0), metrics, results
│   ├── eval/                    # dataset loader, runner, metrics, report formatting
│   ├── qdrant/                  # gRPC client wrapper (schema, upsert, search, scroll, deletes)
│   ├── config/                  # YAML config: languages, skip_dirs, skip_file_patterns
│   └── model/                   # CodeChunk, FileIndexState, SearchResult
├── docs/
│   ├── plan/                    # design docs + roadmap (this file)
│   ├── knowledge/               # decision docs + learning notes
│   ├── eval/                    # golden set, baselines, compare.py
│   ├── improvement/             # re-indexing, incremental roadmap, production readiness
│   └── review_feedback/         # review logs
├── config.yaml
├── docker-compose.yml           # Qdrant service
└── go.mod / go.sum
```
