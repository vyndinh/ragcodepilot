# MVP Roadmap

Forward-looking task list distilled from `docs/review_feedback/system_vision_review.md` and `docs/review_feedback/codemaps_review.md`.
Focus: core retrieval quality. Explore Mode, TUI choices, and Phase C are explicitly deferred (see end).

`docs/plan/checklist.md` remains as a historical record of the original phase plan. This document is the canonical next-up tasks.

Product direction: ragcodepilot is evolving toward a full local RAG pipeline. Phases 1-3 build the retrieval foundation; answer generation adds the "G" only after retrieval is measurable and strong enough to provide trustworthy context.

---

## Summary

| Phase | Goal | Size | Exit criterion | Status |
|---|---|---|---|---|
| 1 | Evaluation foundation | S | `hit@5` baseline metrics committed; `ragcodepilot eval` CLI works | ✅ Done |
| 2 | Hybrid search (BM25 + dense + RRF) | L | Eval shows ≥10pp `hit@5` improvement on exact-symbol queries | ✅ Done — current baseline `baseline_v6.json`: hit@5 = 0.895, hit@1 = 0.579, MRR@5 = 0.673, recall@5 = 0.789, recall@10 = 0.921. BM25 `k1=0.5` + additive Snowball stemming; `*_test.go` excluded from indexing. See `retrieval_quality_decisions.md` §2.5 for the v4 → v6 lineage (Phase 2 corpus → Phase 5 corpus + hygiene). |
| 5 v0 | Minimal `--answer` mode (RAG seam) | S | `ragcodepilot search --answer "q"` returns LLM answer + source chunks via local Ollama | ✅ Shipped — answer-mode dogfooding triggered `*_test.go` exclusion + GraphRAG plan |
| 6 | **GraphRAG — structural retrieval layer** | L | `--graph` lifts `hit@5` on **≥60% of the named `structural` queries** (per-query gate; top-5 inclusion for LLM context), no regression >2pp elsewhere, `negative_pass_rate` stays 1.00. See `graphrag.md` Goal section for the authoritative criterion. | **▶ Next** — plan at `graphrag.md` |
| 3 | Reranking (cross-encoder) | M | — | ⏸ **Deprioritized 2026-05-28.** Reranking only reorders within top-50; cannot add structural signal. Goal is top-5 inclusion for `--answer`, which GraphRAG attacks directly. Revisit only if GraphRAG ships and `concept`-query `hit@5` remains the bottleneck. |
| 3.5 | Rust AST chunker | M | Rust chunker ships with per-function chunks matching the Go contract | ⏸ Deferred — split out from old Phase 3. Independent of reranking; pick up after GraphRAG if multi-language coverage becomes the binding gap. |
| 4 | UX polish | S | JSON output mode, context-lines flag, faster startup *(detail TBD when reached)* | ⏸ Deferred (some items may be shaped by Phase 5 v0 output) |

**Pivot — 2026-05-14.** Phase 5 v0 was pulled ahead of Phases 3 and 4 after Phase 2's hit@5 of 0.895 cleared the vision review's "retrieval is strong enough to feed an LLM" gate, and the user confirmed the RAG product direction. The original Phase 5 condition was "don't start unless a real user need has surfaced" — that gate just opened. Phase 3 (Rust chunker) and Phase 4 (UX polish) are parked, not cancelled; their full plans remain in `phase3_rust_chunker.md` and the Phase 4 sketch below. Phase 5 v0's dogfooding result determines what gets un-parked next.

**Pivot — 2026-05-28.** Phase 5 v0 dogfooding surfaced two issues: (a) `*_test.go` files were polluting retrieval (fixed: now excluded by default via `skip_file_patterns`, re-baseline at `baseline_v6.json` lifted hit@5 from 0.789 to 0.895), and (b) **navigation queries remain the weak type** (v6: navigation hit@5 = 0.75, MRR@5 = 0.47 — only type below 1.0 hit@5). Navigation answers are *structural*, not similarity-based, so reranking (Phase 3) was deprioritized below **GraphRAG** (new Phase 6, plan at `graphrag.md`). Reasoning: reranking only reorders within top-50 candidates; it cannot add a signal that is not already in the embedding/BM25 space. The product goal is *top-5 inclusion for LLM context*, and graph edges (calls / defines / imports) attack that directly.

**Honest framing of the GraphRAG-over-reranker bet.** The v6 recall gap (`recall@10 − recall@5 = 0.132`) actually *triggers* the reranker rule in `retrieval_quality_decisions.md` §2.5 (≥0.10 → reranker headroom). This pivot is **not** *"reranker can't help"* — it is a deliberate trade-off: *"structural signal pays more on navigation than reordering pays on the recall gap."* Recording the framing this way so future-us doesn't refight the decision under different evidence. Phase 3 reranking stays parked, not cancelled — revisit if it becomes the binding constraint after GraphRAG ships.

**Cheaper lever evaluated — `--answer-limit`.** The hypothesis (raise `--answer-limit` 5 → 8 to capture the recall gap's RAG value without a reranker or graph) did **not** validate on automated metrics — shape flat, p50 latency up ~55%, content benefit invisible to Tier B (2026-05-28). **Canonical A/B data: `retrieval_quality_decisions.md` §2.5.** Consequence for Phase 6: dogfooding judgment on 3–5 multi-chunk questions at AL=5 vs AL=8 is a prerequisite before any "narrow scope" verdict (see `graphrag.md` "Prerequisites before Step 1").

### Current product path toward full RAG

1. **Retrieval measurable and reliable.** ✅ Eval harness + hybrid search + stemming + test-file hygiene shipped (Phases 1–2). `baseline_v6` is the current canonical baseline.
2. **Answer generation deliberately.** ✅ Phase 5 v0 shipped: `--answer` flag, frozen prompt, auto-warm, greedy decoding, `--answer-limit`, and the Tier B reference-free `eval --answer` harness. See [`phase5_v0_answer_mode.md`](phase5_v0_answer_mode.md).
3. **Structural retrieval for top-5 inclusion.** ▶ Phase 6 — GraphRAG (`graphrag.md`). Adds a graph layer over hybrid so structural queries (navigation, "what calls X") land the right chunk in the top-5 sent to the LLM. Gated on ✅ ≥15-query structural subset (done, `baseline_v7_structural.json`) + ⏳ dogfooding judgment on AL=5 vs AL=8 (the eval-side A/B from 2026-05-28 was inconclusive on Tier B — see 2026-05-28 pivot above).
4. **Grounding safeguards after dogfooding.** Citation validation (Tier C faithfulness judge), low-confidence refusal guardrail, and streaming are v1 candidates in `phase5_v0_answer_mode.md`. The model currently refuses on its own (negative pass = 1.00), so these are justified-but-not-urgent.

Rust AST chunking (Phase 3.5) and output UX (Phase 4) remain supporting improvements. Reranking is parked behind Phase 6 (see 2026-05-28 pivot above).

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

**Why now:** Pure vector search misses exact-symbol queries. BM25 sparse vectors catch identifier matches. RRF is a well-understood combiner. Vision review's #3 P1 weakness.

**Algorithm history:** Original plan said BM25. Initial implementation in 2026-05-13 shipped as TF-IDF after the `hybrid_search.md` plan review argued BM25's length normalization adds little value on short, uniform-length code chunks. A 2026-05-15 spike re-ran the eval with BM25 (`k1=0.5`, `b=0.75`) — hit@1 jumped +21.1pp, MRR@5 +9.9pp, but one concept query (`hasher_concept`) regressed due to a plural/singular tokenizer mismatch. A same-day follow-up added additive Snowball stemming (`baseline_v4.json`), which recovered the regression while keeping hit@1 at +15.8pp and MRR@5 at +9.2pp vs the TF-IDF baseline — Pareto-better on every metric. See `hybrid_search.md` §3 for the full history and eval matrix.

**Exit criterion:** Eval shows ≥10pp `hit@5` improvement on exact-symbol queries (a tag in the golden set), with no regression on concept queries.

### Checklist

**Schema:**

- [x] Research Qdrant's named sparse vector API; confirm exact request shape.
- [x] Update collection schema: named dense vector (`"dense"`) + sparse vector slot (`"sparse"`).
- [x] Migration path: when an old (unnamed-vector) collection is detected, return a clear error with fix instructions.
- [x] Validate existing collections have both dense and sparse slots.

**Sparse vector generation:**

- [x] Implement TF-IDF tokenizer in `internal/embedding/sparse.go`. Code-aware tokenization (split camelCase, snake_case, digit runs; remove stop words + Go keywords).
- [x] Compute IDF globally over the full corpus per indexing run (not batch-local).
- [x] Generate sparse vector alongside dense in the ingest pipeline.
- [x] Tests for tokenization edge cases (CamelCase, snake_case, mixed, numbers, stop words, empty input, determinism, index/query parity).

**Hybrid search:**

- [x] Unify into one `qdrant.Client.Search` method that handles all three modes (dense, sparse, hybrid) via request shape.
- [x] Server-side RRF fusion via Qdrant `PrefetchQuery` + `NewQueryRRF(k=60)`. No client-side `rrf.go` needed.
- [x] Filters placed on each prefetch stage (not top-level) for correct hybrid ranking.
- [x] Add `--mode dense|sparse|hybrid` flag to both `search` and `eval`. Default: `hybrid`.
- [x] Unit tests for hybrid request shape (prefetch count, Using, limits, filter placement, RRF K).

**Evaluation:**

- [x] Tag golden queries by type (`navigation`, `concept`, `behavior`, `negative`).
- [x] Run eval in all three modes. Compare `hit@5` per tag.
- [x] Commit `docs/eval/baseline_v2.json` showing the delta.
- [x] Dense regression check: P2 dense `hit@5` matches P1 `baseline_v1.json` within 1pp.

**Documentation:**

- [x] Write `docs/plan/hybrid_search.md` documenting the schema change, RRF parameters, and the eval results.

### Files touched / created

- `internal/embedding/sparse.go` *(new — TF-IDF tokenizer, SparseVector type, IDF computation)*
- `internal/embedding/sparse_test.go` *(new — 15+ test cases)*
- `internal/qdrant/client.go` *(named vectors, sparse slot, unified Search with hybrid)*
- `internal/qdrant/client_test.go` *(named vector schema + hybrid request-shape assertions)*
- `internal/search/searcher.go` *(search mode switch: dense/sparse/hybrid)*
- `internal/ingest/pipeline.go` *(global IDF + sparse vector generation)*
- `cmd/ragcodepilot/main.go` *(`--mode` flag on search + eval)*
- `docs/plan/hybrid_search.md` *(new — design + eval results)*
- `docs/eval/baseline_v2.json` *(new — hybrid eval results)*
- `docs/eval/baseline_v2_dense.json` *(new — dense reference for Phase 3)*
- ~~`internal/embedding/bm25.go`~~ *(kept as `sparse.go`; the file name was set during the TF-IDF phase and stayed when the algorithm switched back to BM25 on 2026-05-15 — see `hybrid_search.md` §3)*
- ~~`internal/search/rrf.go`~~ *(not needed — Qdrant handles RRF server-side)*

### Out of scope for Phase 2 (kept as designed)

- No learned sparse models (SPLADE, etc.) — BM25 only.
- No reranking yet — that's Phase 3.
- No tuning beyond a single RRF `k` value (60 is the standard).
- No persisted IDF file — recompute in memory each indexing run.

---

## Phase 3 — Reranking + chunker upgrades [M]

> **⏸ Deprioritized 2026-05-28.** The reranking sub-phase is parked behind
> **Phase 6 — GraphRAG** (`docs/plan/graphrag.md`). Reasoning: a cross-encoder
> only reorders within top-50, so it cannot add the structural signal that
> navigation queries (the weakest type in `baseline_v6.json`) actually need.
> The product gate is *top-5 inclusion for LLM context*, and GraphRAG attacks
> that directly. The **Rust AST chunker** sub-phase is independent and remains
> a candidate after GraphRAG — see Phase 3.5 row in the summary table. The
> rest of this section is preserved as the original design for if and when
> reranking is reopened.

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
- [x] Faster cold-start (pre-warm Ollama; pin via `OLLAMA_KEEP_ALIVE`) — **✅ shipped in Phase 5 v0** (`answer.Warmer` auto-warm before the timed `Generate` call; `OLLAMA_KEEP_ALIVE=-1` documented in README setup)
- [ ] Update README with new flags (incremental as new flags land)

**Do not start until Phase 6 (GraphRAG) ships, or sooner if dogfooding surfaces a UX/output need.**

---

## Phase 5 — `--answer` mode (v0 shipped, v1 deferred)

**v0 status:** ✅ Shipped (commit `8597391`, PR #35). See [`phase5_v0_answer_mode.md`](phase5_v0_answer_mode.md) for the canonical design and what landed:

- `Generator` interface + `OllamaGenerator` + `FakeGenerator`
- Frozen v0 system prompt (golden-tested for exact wording)
- `--answer` CLI flag (opt-in; default retrieval path is byte-identical to pre-v0)
- Auto-warm via the `answer.Warmer` optional interface
- Greedy decoding (temperature 0 + fixed seed)
- `--answer-limit` (decouples answer context from the retrieval `--limit`)
- Tier B reference-free `eval --answer` harness (citation validity, refusal-on-negative, well-formedness, `recall@5`/`recall@10` gap diagnostic) — report-only

**v1 deferred items** (gated on real-use dogfooding signal):

- [ ] Multi-provider support (OpenAI-compatible HTTP first, then native Anthropic) — see §"v1: multi-provider support" in the v0 plan.
- [ ] Faithfulness eval (Tier C, LLM-as-judge). Tier B reference-free metrics already ship.
- [ ] Low-confidence refusal guardrail (refuse to generate when retrieval top-1 score is below threshold). *Justified-but-not-urgent — `baseline_v6` negative pass = 1.00, model already refuses on its own.*
- [ ] Streaming responses (currently synchronous; would lift the time-to-first-token UX, doesn't reduce total time).
- [ ] Semantic citation precision (Tier B only range-checks references; v1 verifies that the cited chunk actually contains the claimed fact).

---

## Deferred decisions (revisit triggers)

Explicit out-of-scope list. Each item has a trigger that should cause us to reopen it.

| Item | Status | Revisit trigger |
|---|---|---|
| **Explore Mode** (call graph + clustering + drill-down) | **Promoted into Phase 6 — GraphRAG.** The call-graph + drill-down idea is now a retrieval-quality lever, not a separate UX mode. See `graphrag.md`. | n/a — superseded. The UX presentation (TUI / drill-down navigation) remains deferred per the TUI row below. |
| **TUI implementation choice** (stdin-loop vs bubbletea) | Deferred per user request | After GraphRAG (Phase 6) ships and there is a connected-subgraph result shape worth navigating. Restart from §2.5 of `codemaps_review.md`. |
| **Cross-encoder reranking** | **Deprioritized 2026-05-28** behind GraphRAG. Only reorders within top-50; can't add structural signal. | After GraphRAG ships, if the **recall gap** (`recall@10 − recall@5`, currently 0.132 ≥ 0.10 on v6, 0.150 on v7_structural) remains the binding constraint on multi-chunk answer completeness — the standing trigger in `retrieval_quality_decisions.md` §2.5. (Concept `hit@5 = 1.00` on v6 means the LLM already gets a relevant chunk in top-5, so concept *precision* is a weaker trigger than the recall gap.) Note: the `--answer-limit 8` eval-side A/B (2026-05-28) was inconclusive on Tier B and added +55% latency — *not* a cheap alternative that pre-empts the reranker. A reranker would land rank-6–10 chunks in top-5 without the latency penalty, so this case strengthens after the AL=8 finding. |
| **Tree-sitter for non-Go languages** | Deferred | After GraphRAG ships and the first non-Go AST chunker (Rust) proves the multi-language pattern. |
| **Phase C** (custom vector DB in Go per `docs/plan/vecdb/`) | Deferred indefinitely | After Phase 5 ships AND explicit decision that learning goals outweigh continued product investment. |
| **Watch mode / incremental re-indexing** | Deferred | After Phase 4 (UX polish). |
| **Multi-modal embeddings** (separate code / prose / docstring vectors) | Deferred | Only if eval shows the single-vector approach has a clear ceiling we can't lift with GraphRAG or reranking. |
| **IDE plugin** | Deferred | After Phase 5; never in MVP. |

---

## Related docs

- `docs/review_feedback/system_vision_review.md` — source of the phase numbering and overall strategy.
- `docs/review_feedback/codemaps_review.md` — why Explore Mode is deferred.
- `docs/brainstorm/codemaps_analysis.md` — the original Explore Mode proposal (to reopen later).
- `docs/plan/graphrag.md` — Phase 6 (GraphRAG) design doc.
- `docs/knowledge/code_graph_retrieval_landscape.md` — industry context & prior art for Phase 6 (GraphRAG).
- `docs/plan/rag_evaluation_metrics.md` — eval harness spec (input for Phase 1).
- `docs/plan/checklist.md` — historical record of the original phase plan.
