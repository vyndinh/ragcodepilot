# System Design: Semantic Code Search Application

> Created: May 2026 | Last refreshed: 2026-09-22 (branch-aware architecture snapshot)
> Approach: top-down (use existing vector DB first, study internals later)

## Overall roadmap

```text
Phase A: Build application on Qdrant     <- THIS DOCUMENT
Phase B: Study vector DB internals       <- vector_db_core.md
Phase C: Refactor Rust vector DB to Go   <- Deferred indefinitely (see mvp_roadmap.md)
```

Phase/feature sequencing is owned by [`mvp_roadmap.md`](mvp_roadmap.md) — this
document describes the **current architecture as built**, plus the original
requirements and scale framing. The implementation establishes current behavior;
the roadmap establishes planned work, never proof that a feature has shipped.

**Implementation snapshot:** reconciled with fetched `origin/main` at `a3ac8ff`
on 2026-09-22. This includes additive identifier tokens, named Go type/interface
chunks, combined chunker/tokenizer representation versioning, and mode-calibrated
negative evaluation. This PR changes documentation relative to that base. Saved
v8 evidence predates some chunker changes, so a fresh baseline remains M0 work.

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
| F2 | Parse and chunk code into meaningful units | ✅ Go: AST functions/methods and named types/interfaces; other languages: sliding window + regex naming |
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

The historical 200K-chunk target is an unvalidated scale envelope, not an
acceptance claim or an active multi-repo product requirement.

```text
Dense vector lower bound:
  200,000 chunks * 768 dimensions * 4 bytes = 614,400,000 bytes
                                             (about 586 MiB)
  Sparse vectors, indexes, payloads, replicas and runtime memory are additional.

Illustrative serial embedding arithmetic:
  200,000 chunks * 50 ms/chunk = 10,000 s (about 167 minutes)
  A batch size of 32 alone does not establish parallelism or a 30-minute total.

Saved retrieval evidence:
  v6: small self-corpus; hybrid p95 137 ms
  main v8: 199 Go chunks at capture; hybrid p95 151 ms
  Neither result validates latency at 200K chunks or sustained throughput.
```

A changed index run currently re-embeds all chunks in its repository/language
scope. Planned dense reuse reduces expensive embedding calls; hashing, chunking,
BM25 statistics and sparse writes can remain proportional to scope size. Measure
end-to-end cost, memory, and recovery separately before making capacity claims.
See [local reliability](../improvement/production_readiness_and_features.md).

---

## Step 2: High-level design

### Architecture

```text
CLI: index [--watch] | search [--mode] [--answer] | eval | collections
              |                |                   |
              v                v                   v
        Ingest pipeline   Search service <---- Eval harness
        walk/hash/chunk   query encoding        frozen query set
        enrich + BM25     mode + filters        metrics/report
              |                |
              +-------> Ollama embedder (dense/hybrid)
              |                |
              v                v
          Qdrant <-------- vector query
          named dense + sparse vectors; code + provenance payload
                               |
                               v
                          ranked chunks
                               |
                          optional --answer
                               |
                               v
                          Ollama generator
                          answer + sources
```

### Components

#### 1. CLI (`cmd/ragcodepilot`)

```text
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
model artifact and preprocessing or vectors can be incompatible. Dimension
validation catches only size mismatches. Same-size model changes are not detected,
and the current index version does not fingerprint the embedding model. Use a
new collection for model experiments; matching file hashes can otherwise skip a
necessary rebuild.

#### 2. Ingestion pipeline (`internal/ingest`)

```text
repo path -> walk -> hash files -> classify (unchanged / changed / new / stale /
index-version refresh) -> delete stale -> chunk -> enrich -> compute corpus BM25
stats -> embed (dense) + build sparse -> batch upsert -> late-delete orphaned
chunks of changed files
```

- **Walker**: skips hidden files/directories, configured `skip_dirs`, and `skip_file_patterns`
  (notably `*_test.go` — excluded by default after the Phase 5 dogfooding
  finding; see `retrieval_quality_decisions.md` §2.5).
- **Change detection**: SHA-256 file hash + `index_version` payload per chunk —
  see [`../improvement/reindexing.md`](../improvement/reindexing.md).
- **Chunkers**: Go → AST functions/methods and named types/interfaces
  (`chunker_go.go`); other languages →
  sliding window (~40 lines, 10-line overlap) with regex name extraction.
- **Enrichment**: prepends file path / language / chunk type+name to the text
  sent to the embedder (payload keeps raw code) — `chunk_enrichment.md`.
- **Sparse vectors**: code-aware tokenizer (camelCase/snake_case splitting,
  stop-word + Go-keyword removal, additive identifier tokens and Snowball stemming), BM25 with
  `k1=0.5, b=0.75`, statistics computed over the current repo/language scope — `hybrid_search.md`.
- **Watch mode**: `index --watch` = fsnotify + 500 ms debounce, re-runs the
  pipeline on change — `architecture_decisions.md` §3.

#### 3. Search service (`internal/search`)

```text
query -> embed -> build Qdrant request per mode:
  dense  : named dense vector search
  sparse : named sparse vector search
  hybrid : prefetch dense + sparse -> server-side RRF (k=60)   <- default
filters (language/repo) applied per prefetch stage -> format results
```

Per-stage timings (embed / qdrant / total) are captured for the eval harness.

#### 4. Embedding service (`internal/embedding`)

```text
interface Embedder:
  Embed(texts[]) -> vectors[][]
  Dimension()    -> int
```

- **Ollama** (`nomic-embed-text`, 768d): dimension auto-detected from the first
  batch and validated on every subsequent one (`validate.go`) — detects size
  mismatches, not same-dimensional model/preprocessing incompatibility.
- **Fake**: deterministic pseudo-random vectors for tests.

#### 5. Answer generation (`internal/answer`) — Phase 5 v0

```text
interface Generator:
  Generate(ctx, query, chunks[]) -> answer text

interface Warmer (optional):        # type-asserted; pulls model load out of the timed call
  Warmup(ctx) -> error
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
`docs/eval/`. v6/v7 are historical reports on this branch; the latest saved
baseline is `baseline_v8.json`. Its 0.50 negative pass rate uses the corrected
RRF ceiling (0.02), now also in this branch. The historical v6/v7 1.00 rates
used an ineffective cosine threshold. They do not establish negative-query
safety; score families require separate calibration. See
[`../eval/README.md`](../eval/README.md).

#### 7. Qdrant (`internal/qdrant`)

gRPC wrapper: collection CRUD with named dense+sparse vector schema, payload
indexes (repo / language / file_path), batch upsert, unified search (all three
modes), scroll for file states, targeted deletes for stale/orphaned chunks.

---

## Step 3: Data flow

### Ingestion flow

```text
Git repo -> Walk -> Hash -> Classify -> Chunk (AST | window) -> Enrich
        -> BM25 corpus stats -> Embed dense + build sparse -> Batch upsert
        -> Late-delete orphaned chunks
```

Key decisions:

| Decision | Choice | Rationale |
|---|---|---|
| Chunk unit | Go: functions/methods and named types/interfaces (AST); others: sliding window ~40 lines / 10 overlap | Semantic boundaries beat arbitrary splits |
| Embedding input | Enriched (path + language + type/name header) | Large lift on natural-language queries |
| Embedding model | `nomic-embed-text` via Ollama (768d) | Local, free, adequate; code-specialized model is an open lever (`cheaper_levers.md`) |
| Vector dimension | Auto-detected + validated | Detects size mismatch; model/preprocessing provenance remains a gap |
| Sparse algorithm | BM25 `k1=0.5, b=0.75` + additive identifiers and Snowball stemming | Eval-driven: +15.8pp hit@1 vs TF-IDF with no hit@5 loss (`hybrid_search.md` §3) |
| Test files | `*_test.go` excluded by default | They crowded top-K; excluding lifted hit@5 0.789→0.895 |
| Batch size | 32 embed / upsert batch | Throughput vs memory |
| Point ID | Hash of repo + file path + symbol + chunk index; unnamed chunks use start line | Named IDs can survive line shifts; changed shapes need stale-ID cleanup |
| Change detection | SHA-256 file hash + combined tokenizer/chunker `index_version` | Detects tracked representation changes; model/enrichment fingerprinting remains planned |

### Search flow

```text
User query -> Embed -> mode (dense | sparse | hybrid+RRF) + filters
-> ranked chunks -> format
-> [--answer] top --answer-limit chunks -> warm generator -> grounded answer + [N] citations + sources
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
    "index_version": "sparse-bm25-snowball-ident-v2+go-types-v1"
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
- **Next:** M0 fresh baseline plus one external repository, then M3 dense reuse
  with retry/cleanup. M4 existing-command errors and .gitignore can proceed independently.
- **Optional:** M1 sources-first output before generation.
- **Deferred:** streaming, standalone doctor, automated retrieval CI, scanners,
  and M5 retrieval layers/prototypes until repeated failures justify them.
- **Removed from active scope:** M2 MCP/agent-first integration, multi-repo
  workspace commands, and shared/team deployment.

---

## Tradeoffs and decisions

| Tradeoff | Decision | Why |
|---|---|---|
| API embedding vs local | Local Ollama | No API costs, offline, privacy; local-first is a product constraint |
| Answer generation | Opt-in `--answer`, frozen prompt, greedy | Default retrieval unchanged; generation repeatability depends on fixed model/runtime |
| Tree-sitter vs regex vs AST | Go AST now; regex fallback; tree-sitter deferred | Best chunks where it matters most, least complexity |
| CLI vs daemon | Thin CLI; Qdrant+Ollama are the daemons | See `architecture_decisions.md` — daemon solves problems we don't have |
| Collection scope | Existing repo filters; separate collections for distinct evaluation/worktree scopes | Basename repo IDs can collide; independently indexed scopes have different BM25 statistics |
| Eval gating | Manual pinned-input comparisons; automated retrieval CI deferred | Frozen paired evidence and named failures precede automated quality gates |

---

## Go project structure

```text
ragcodepilot/
+-- cmd/ragcodepilot/            # CLI entry point (index, search, eval, collections, version)
+-- internal/
|   +-- ingest/                  # walker, hasher, chunker (+ Go AST), enrichment, pipeline, watcher
|   +-- search/                  # query embedding + mode selection + Qdrant search + timings
|   +-- embedding/               # Embedder iface, ollama, fake, validate, sparse (BM25 tokenizer/stats)
|   +-- answer/                  # Generator iface, ollama, fake, prompt (frozen v0), metrics, results
|   +-- eval/                    # dataset loader, runner, metrics, report formatting
|   +-- qdrant/                  # gRPC client wrapper (schema, upsert, search, scroll, deletes)
|   +-- config/                  # YAML config: languages, skip_dirs, skip_file_patterns
|   `-- model/                   # CodeChunk, FileIndexState, SearchResult
+-- docs/
|   +-- plan/                    # design docs + roadmap (this file)
|   +-- knowledge/               # decision docs + learning notes
|   +-- eval/                    # golden set, baselines, compare.py
|   +-- improvement/             # re-indexing, incremental roadmap, production readiness
|   `-- review_feedback/         # review logs
+-- config.yaml
+-- docker-compose.yml           # Qdrant service
`-- go.mod / go.sum
```
