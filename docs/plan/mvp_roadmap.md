# MVP Roadmap

Forward-looking task list distilled from `docs/review_feedback/system_vision_review.md` and `docs/review_feedback/codemaps_review.md`.
Focus: core retrieval quality. Explore Mode, TUI choices, and Phase C are explicitly deferred (see end).

`docs/plan/checklist.md` remains as a historical record of the original phase plan. This document is the canonical next-up tasks.

---

## Summary

| Phase | Goal | Size | Exit criterion |
|---|---|---|---|
| 1 | Evaluation foundation | S | `hit@5` baseline metrics committed; `ragcodepilot eval` CLI works |
| 2 | Hybrid search (BM25 + dense + RRF) | L | Eval shows ≥10pp `hit@5` improvement on exact-symbol queries |
| 3 | Reranking + chunker upgrades | M | Cross-encoder reranker measurably improves `MRR@5`; one new language chunker (Python or Rust) shipped |
| 4 | UX polish | S | JSON output mode, context-lines flag, faster startup *(detail TBD when reached)* |
| 5 | Optional `--answer` mode | M | Ollama-backed answer generation with chunk citations *(detail TBD when reached)* |

**Phases 1-3 are the core retrieval-quality push.** Phases 4-5 are sketched here and will be detailed when reached.

---

## Phase 1 — Evaluation foundation [S]

**Goal:** Build the harness that measures every subsequent change.

**Why now:** Without baseline metrics, you cannot tell if Phase 2 (hybrid), Phase 3 (reranking), or any chunker change helps or hurts. This is the vision review's P1.

**Exit criterion:** `ragcodepilot eval --dataset docs/eval/golden.yaml` runs end-to-end and produces a report with `hit@1/3/5`, `MRR@5`, `recall@10`, and latency percentiles. Baseline numbers checked into the repo.

### Checklist

**Schema & dataset:**

- [x] Define golden query YAML schema. Starting point: the format sketched in `docs/plan/rag_evaluation_metrics.md`.
- [x] Write 5 starter golden queries covering all three categories: **navigation** ("where is X defined"), **concept** ("how does Y work"), **behavior** ("when does Z fail").
- [x] Index ragcodepilot's own repo as the eval target.
- [x] Expand to 20-30 queries over the course of the phase.

**Metrics package:**

- [x] Create `internal/eval/metrics.go` with `HitAtK`, `MRRAtK`, `RecallAtK` functions.
- [x] Capture latencies broken out by stage: embed, qdrant, total.
- [x] Unit tests for each metric (known inputs, expected outputs).

**CLI:**

- [x] Create `internal/eval/runner.go` that loads a YAML dataset, runs each query through the existing `search.Searcher`, computes per-query metrics, aggregates.
- [x] Add `eval` subcommand to `cmd/ragcodepilot/main.go`.
- [x] Support `--output json` and `--output human` modes.
- [x] Support filtering queries by tag (run just navigation, or just concept, etc.).

**Negative tests:**

- [x] Add 3-5 queries that should NOT match well. Confirm top-1 score is below a threshold.

**Baseline:**

- [x] Run eval against current `main`. Commit `docs/eval/baseline_v1.json` with the numbers.

### Files to touch / create

- `internal/eval/metrics.go` *(new)*
- `internal/eval/dataset.go` *(new — YAML loader)*
- `internal/eval/runner.go` *(new)*
- `internal/eval/metrics_test.go` *(new)*
- `cmd/ragcodepilot/main.go` *(extend with `eval` subcommand)*
- `docs/eval/golden.yaml` *(new)*
- `docs/eval/baseline_v1.json` *(new)*

### Out of scope for Phase 1

- No automated CI gating on metric regressions (manual review for now).
- No reranking, no hybrid search — just measure the current system.
- No structural metrics for Explore Mode (those are part of the deferred work).

---

## Phase 2 — Hybrid search [L]

**Goal:** Add BM25 sparse-vector search alongside dense vector search; fuse with Reciprocal Rank Fusion.

**Why now:** Pure vector search misses exact-symbol queries. BM25 catches identifier matches. RRF is a well-understood combiner. Vision review's #3 P1 weakness.

**Exit criterion:** Eval shows ≥10pp `hit@5` improvement on exact-symbol queries (a tag in the golden set), with no regression on concept queries.

### Checklist

**Schema:**

- [ ] Research Qdrant's named sparse vector API; confirm exact request shape.
- [ ] Update collection schema: dense vector + sparse vector slot.
- [ ] Migration path: when an old collection is detected, force re-index (no in-place migration).

**Sparse vector generation:**

- [ ] Implement BM25 tokenizer in `internal/embedding/bm25.go`. Code-aware tokenization (split on camelCase, snake_case, treat identifiers vs prose differently).
- [ ] Generate sparse vector alongside dense in the ingest pipeline.
- [ ] Tests for tokenization edge cases (CamelCase, snake_case, mixed, numbers).

**Hybrid search:**

- [ ] Extend `qdrant.Client.Search` to issue both dense and sparse queries.
- [ ] Implement RRF fusion in `internal/search/rrf.go`. Standard formula: `score(d) = Σ 1 / (k + rank_i(d))` for `k=60`.
- [ ] Add `--mode dense|sparse|hybrid` flag. Default: `hybrid`.
- [ ] Unit tests for RRF with known input rankings.

**Evaluation:**

- [ ] Tag golden queries by type (`exact-symbol`, `concept`, `behavior`).
- [ ] Run eval in all three modes. Compare `hit@5` per tag.
- [ ] Commit `docs/eval/baseline_v2.json` showing the delta.

**Documentation:**

- [ ] Write `docs/plan/hybrid_search.md` documenting the schema change, RRF parameters, and the eval results.

### Files to touch / create

- `internal/embedding/bm25.go` *(new)*
- `internal/embedding/bm25_test.go` *(new)*
- `internal/qdrant/client.go` *(extend `EnsureCollection`, `Upsert`, `Search` for sparse)*
- `internal/search/rrf.go` *(new)*
- `internal/search/searcher.go` *(wire RRF into the search flow)*
- `internal/ingest/pipeline.go` *(generate sparse vectors alongside dense)*
- `cmd/ragcodepilot/main.go` *(`--mode` flag)*
- `docs/plan/hybrid_search.md` *(new)*
- `docs/eval/baseline_v2.json` *(new)*

### Out of scope for Phase 2

- No learned sparse models (SPLADE, etc.) — BM25 only.
- No reranking yet — that's Phase 3.
- No tuning beyond a single RRF `k` value (60 is the standard).

---

## Phase 3 — Reranking + chunker upgrades [M]

**Goal:** Add a cross-encoder reranker on top-50 retrieval, and replace one language's regex chunker with proper AST-based chunking.

**Why now:** Vision review's P2 #4 and #5 weaknesses. Reranking lifts precision on ambiguous queries. AST chunking lifts chunk quality for languages beyond Go.

**Exit criterion:**

- Reranking turned on shows measurable `MRR@5` improvement on the ambiguous-query subset (no regression on others).
- One non-Go language has AST-based chunking with chunk-quality metric ≥ Go baseline.

### Checklist

**Reranking sub-phase:**

- [ ] Decide reranker model. Candidates:
  - A small cross-encoder served via Ollama (if a suitable model exists).
  - Local sentence-transformers via a Python sidecar.
  - A Go reimplementation of a small reranker.
  - Document the decision in `docs/plan/reranking.md`.
- [ ] Build `internal/rerank/` package with a `Reranker` interface and one implementation.
- [ ] Add `--rerank` flag to `search` (default: off until eval confirms it helps).
- [ ] Implement the flow: retrieve top-50 (hybrid) → rerank → return top-10.
- [ ] Eval: tag ambiguous queries; compare `MRR@5` with and without rerank.
- [ ] Latency budget: rerank should add ≤200ms warm. If it adds more, document the tradeoff.
- [ ] Tests: fake reranker for wiring tests; real reranker tested manually.

**Chunker upgrade sub-phase:**

- [ ] Pick one language. **Rust** (matches the Phase C ambition). Document the choice.
- [ ] Evaluate AST parsing options:
  - tree-sitter-go bindings (CGo, build complexity).
  - Pure-Go ports (e.g. `go-tree-sitter-bare`).
  - Calling a Python sidecar for Python AST (avoids CGo but adds a process).
- [ ] Implement chunker in `internal/ingest/chunker_<lang>.go`.
- [ ] Per-function chunk extraction matching the Go AST chunker's contract.
- [ ] Eval: pick 5 golden queries targeting this language; show `hit@5` improvement vs the regex fallback.
- [ ] Tests for the new chunker (corpus of small sample files).

### Files to touch / create

- `internal/rerank/reranker.go` *(new — interface)*
- `internal/rerank/<impl>.go` *(new — implementation)*
- `internal/rerank/rerank_test.go` *(new)*
- `internal/ingest/chunker_<lang>.go` *(new)*
- `internal/ingest/chunker_<lang>_test.go` *(new)*
- `internal/ingest/pipeline.go` *(route to the new chunker by language)*
- `internal/search/searcher.go` *(wire reranker)*
- `cmd/ragcodepilot/main.go` *(`--rerank` flag)*
- `docs/plan/reranking.md` *(new — design doc)*
- `docs/eval/baseline_v3.json` *(new — post-Phase-3 metrics)*

### Out of scope for Phase 3

- Only **ONE** non-Go language. Multi-language chunking is deferred.
- LLM-as-reranker is not on the table (vision review's "avoid early").
- Reranker tuning beyond reasonable defaults — just confirm it helps or doesn't.

---

## Phase 4 — UX polish [S, sketch only]

**Goal:** Make result output composable and developer-friendly.

**Likely checklist (flesh out when reached):**

- [ ] `--json` output mode for `search`
- [ ] `--context-lines N` flag (print N lines around each chunk)
- [ ] Result grouping by file
- [ ] Faster cold-start (pre-warm Ollama; pin via `OLLAMA_KEEP_ALIVE`)
- [ ] Update README with new flags

**Do not start until Phase 3's exit criterion is met.**

---

## Phase 5 — Optional `--answer` mode [M, sketch only]

**Goal:** Bridge to "understand" use case via opt-in LLM synthesis. Default remains retrieval-only.

**Likely checklist (flesh out when reached):**

- [ ] Design the prompt template: chunks → answer with citations.
- [ ] Implement `--answer` flag on `search`.
- [ ] Output: answer text + chunk-ID citations referencing real `internal/.../file.go:line` locations.
- [ ] Guardrail: if retrieval top-1 score is below threshold, refuse to generate (avoid hallucinating on weak retrieval).
- [ ] Eval extension: faithfulness check on a sample of generated answers.

**Don't start unless a real user need has surfaced.**

---

## Deferred decisions (revisit triggers)

Explicit out-of-scope list. Each item has a trigger that should cause us to reopen it.

| Item | Status | Revisit trigger |
|---|---|---|
| **Explore Mode** (call graph + clustering + drill-down) | Deferred per `docs/review_feedback/codemaps_review.md` | After Phase 3 exit criterion is met AND retrieval quality is solid. |
| **TUI implementation choice** (stdin-loop vs bubbletea) | Deferred per user request | When Explore Mode is reopened. Restart from §2.5 of `codemaps_review.md`. |
| **Tree-sitter for languages beyond Phase 3's pick** | Deferred | After Phase 3 ships and the first non-Go chunker proves the pattern. |
| **Phase C** (custom vector DB in Go per `docs/plan/vecdb/`) | Deferred indefinitely | After Phase 5 ships AND explicit decision that learning goals outweigh continued product investment. |
| **Watch mode / incremental re-indexing** | Deferred | After Phase 4 (UX polish). |
| **Multi-modal embeddings** (separate code / prose / docstring vectors) | Deferred | Only if eval shows the single-vector approach has a clear ceiling we can't lift with reranking. |
| **IDE plugin** | Deferred | After Phase 5; never in MVP. |

---

## Related docs

- `docs/review_feedback/system_vision_review.md` — source of the phase numbering and overall strategy.
- `docs/review_feedback/codemaps_review.md` — why Explore Mode is deferred.
- `docs/brainstorm/codemaps_analysis.md` — the original Explore Mode proposal (to reopen later).
- `docs/plan/rag_evaluation_metrics.md` — eval harness spec (input for Phase 1).
- `docs/plan/checklist.md` — historical record of the original phase plan.
