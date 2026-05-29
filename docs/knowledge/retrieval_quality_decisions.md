# Retrieval Quality Decisions

> Reference doc for trade-off analyses on retrieval-quality changes. Add to it as the system evolves; each section is a self-contained analysis you can revisit before making the next decision.
>
> Currently covers:
>
> 1. **Reranking** — pros/cons for *this* system
> 2. **Which retrieval metrics matter** — and how the answer changes as ragcodepilot moves from retrieval-CLI toward full RAG

**Companion docs:**

- [`../plan/mvp_roadmap.md`](../plan/mvp_roadmap.md) — phase plan and product direction (full RAG)
- [`../plan/hybrid_search.md`](../plan/hybrid_search.md) — current BM25 + dense + RRF design with additive stemming (`baseline_v4`)
- [`../plan/rag_evaluation_metrics.md`](../plan/rag_evaluation_metrics.md) — eval harness spec
- [`code_graph_retrieval_landscape.md`](code_graph_retrieval_landscape.md) — industry landscape / prior-art companion to the GraphRAG (Phase 6) decision
- [`rag_notebook.md`](rag_notebook.md) — beginner walkthrough; §14 has current performance numbers

---

## Table of contents

1. [Reranking — pros/cons for ragcodepilot](#1-reranking--proscons-for-ragcodepilot)
2. [Which retrieval metrics matter](#2-which-retrieval-metrics-matter)
3. [Bottom line](#3-bottom-line)

---

## 1. Reranking — pros/cons for ragcodepilot

### 1.1 What it is

Two-stage retrieval. Today is one stage (hybrid search returns top-K). Reranking adds a second:

```
Stage 1 (current): hybrid search → top-50 candidates           (~30ms p50 today)
Stage 2 (new):     cross-encoder scores each (query, chunk) pair
                   → re-sort → return top-K                    (+200–500ms)
```

A **cross-encoder** processes query and document **together** through attention. Much richer signal than the bi-encoder (embedding) similarity used in stage 1, but it runs per-pair instead of as a single query embedding — so it's slower.

### 1.2 Pros

1. **Biggest available hit@1 lever.** Published cross-encoder benchmarks consistently add **+10–20pp on hit@1** over bi-encoder retrieval. Current state (`baseline_v6`, unchanged from `baseline_v4`): hit@1 = 0.579 → reranking plausibly pushes to **0.70–0.80**.
2. **Stabilizes top-K ordering and may recover navigation queries that stemming displaced.** Stemming (shipped 2026-05-15) gave back full hit@5 but cost two navigation hit@1 results (`run_eval_navigation`, `hihatk_navigation` — both lost top-1 due to the expanded token space adding competition for exact-identifier queries). A cross-encoder reading the full query text could re-promote these. The `hasher_concept` regression that BM25 originally introduced is **already fixed** by stemming; reranking is no longer needed for that specific failure.
3. **Improves all query categories**, not just navigation. Concept, behavior, and navigation queries all benefit when retrieval surfaces 5–10 plausible candidates that need re-ordering.
4. **No re-indexing.** Drops in on top of existing hybrid retrieval. Unlike the BM25 switch, you can A/B reranking without touching the Qdrant collection.
5. **Composable with everything else.** Orthogonal to chunking, embedding model, tokenizer, sparse algorithm. Adds a step, doesn't replace one.
6. **Precondition for Phase 5 (answer mode).** If you feed chunks to an LLM, you want top-3 to be the actually-best 3. Reranking is the realistic path to "top-K I'd trust an LLM to summarize without hallucinating."

### 1.3 Cons

1. **Latency: 5–10× the current p50.** Cross-encoders run per-pair. Reranking top-20 with a small cross-encoder on CPU adds 200–500ms. p50 goes from 28ms → ~250–500ms; p95 likely 800ms+. **This is the dominant cost** and the single biggest reason not to do it. Whether it matters depends on how "interactive" the CLI should feel.
2. **Deployment complexity.** No clean local option:
   - **Ollama**: primarily a generation runtime. Most rerankers (BGE-reranker, jina-reranker, mxbai-rerank) are encoder-only and don't have first-class Ollama support. Workable but custom.
   - **Python sidecar**: easiest model access (sentence-transformers, FlagEmbedding), but introduces Python to a Go-only repo, plus IPC + startup overhead.
   - **Pure Go**: porting cross-encoder inference is unrealistic for a learning project.
3. **Model choice matters — a bad reranker is *worse* than no reranker.** Safe default (BGE-reranker-base) is decent; larger ones (BGE-reranker-large, jina-reranker-v2) cost 2–3× more latency. Tuning required.
4. **Hit@1 lift not guaranteed at our scale.** Published numbers are mostly MS MARCO (large general corpus). 350-chunk code corpus is a different regime — the lift could be smaller.
5. **Doesn't fix everything.** Reranker can only re-order what retrieval surfaces. If hybrid drops the right chunk out of the top-20 entirely (the `chunkfile_navigation` case in `hybrid_search.md`), reranking can't recover it. **Retrieval quality is the floor.**
6. **Eval harness work.** Need to extend the harness to capture pre- vs post-rerank scores so you can isolate where the lift comes from.
7. **Adds a second model to maintain.** The current "single Ollama embedder + Qdrant" simplicity goes away. Versioning, model files, integration tests all grow.

### 1.4 Compared to alternatives

| Next step | Effort | Expected hit@1 lift | Latency impact | State |
|---|---|---|---|---|
| **Reranking** | M–L | **+10–20pp** | **−5× to −10× (slower)** | Not yet attempted |
| **Tokenizer stemming** (`hashes` → `hash`) via `kljensen/snowball` | S | small (recovered the one regression) | minor (+18% p95 on hybrid) | ✅ **Shipped 2026-05-15 in `baseline_v4`** |
| **Better embedding model** (e.g. `jina-embeddings-v2-base-code`) | S–M | unknown; +2–5pp likely | similar | Not yet attempted |
| **Phase 5 v0 answer mode** (per current roadmap) | M | n/a (new capability, not retrieval) | adds LLM call | Not yet attempted |
| **Rust AST chunker** | M | small (Rust-only) | none | Not yet attempted |

### 1.5 Decision criteria

Pick reranking **if** all of:

- Top-1 quality is the metric you actually care about *(see §2 — under RAG, this is less true than it seems)*.
- You'll accept p50 latency rising from 28ms to ~250–500ms.
- You're OK with a Python sidecar OR finding/customizing a cross-encoder for Ollama.

~~Pick stemming instead if...~~ **Stemming already shipped (2026-05-15)**. Result: hit@5 recovered to 0.895, concept hit@5 back to 1.000, hit@1 dropped 5.3pp vs `baseline_v3` (small navigation cost). See [`../plan/hybrid_search.md`](../plan/hybrid_search.md) for the full P3 → P4 comparison.

Pick **answer mode (Phase 5 v0) instead if** you want to start exercising the full-RAG product surface before retrieval is "perfect." The mvp_roadmap.md pivoted in this direction on 2026-05-14; the argument was that `baseline_v3` hit@5 of 0.842 was strong enough to feed an LLM. **With `baseline_v4` at hit@5 = 0.895 the argument is even stronger.** Reranking would make those answers visibly sharper at top-1, but you don't strictly need it before starting answer mode.

### 1.6 Risks specific to this codebase

- **Two retrieval-system "personality" effects**: cross-encoders calibrated on natural-language QA may misrank code-specific tokens (identifiers, snake_case symbols). The same risk affected stemming. Plan eval comparison carefully.
- **CPU-only inference budget**: most users run ragcodepilot on a laptop without GPU. Latency claims here assume CPU; with a Metal/CUDA backend the picture changes meaningfully.
- **Eval corpus size is small (350 chunks).** A 20-chunk top-K means reranker sees ~6% of the corpus. The signal-to-noise ratio in eval results will be limited until the corpus grows.

---

## 2. Which retrieval metrics matter

### 2.1 What each metric measures

| Metric | Plain meaning | What it captures |
|---|---|---|
| **Hit@1** | Is the *very first* result correct? | Top-of-list precision; pure ranking quality. |
| **Hit@K** (K=3, 5) | Is the correct result somewhere in the top K? | Tolerance for slightly imperfect ranking. |
| **MRR@K** | Average position of the first correct hit (`1/rank`). | Hybrid of precision and top-of-list bias. |
| **Recall@K** | What fraction of *all* relevant chunks made it to top K? | Coverage — matters when there's more than one right answer. |
| **Negative pass** | Out-of-scope queries return nothing on-topic. | "We know when we don't know." |

Not currently measured but worth adding eventually:

- **nDCG@K** — quality-graded ranking metric (better than hit@K when relevance has degrees).
- **Faithfulness / Groundedness** — for answer mode: does the generated answer match the retrieved chunks?

### 2.2 Regime change: retrieval-CLI vs RAG

The metric that matters depends on **what the retrieval feeds into.**

#### Pure retrieval (current CLI mode)

A human reads the results.

- They scan top 3–5 anyway, so **hit@5 is a fair proxy** for "did we help the user?"
- Top-1 matters because it's the first thing they see, but if it's wrong they can keep scrolling.
- Negative pass is nice-to-have — wrong results just waste a click.

#### RAG (Phase 5 v0 onwards)

An LLM reads top-K and synthesizes an answer.

- The LLM gets **all top-K chunks** in its prompt. So "did we put enough information in the prompt for the LLM to answer?" — that's **hit@K and recall@K**, not hit@1.
- **Hit@1 still matters indirectly**: LLMs anchor on the *first* chunk in the prompt (primacy bias). A wrong chunk @1 can poison the answer even when chunks @2–5 are right.
- **Negative pass becomes critical.** If retrieval surfaces confidently-wrong chunks for an unanswerable question, the LLM will confidently hallucinate using them. Negative pass is your hallucination floor.
- **Per-query-type recall matters more.** Navigation queries usually have one right answer. **Concept and behavior queries often need 2–4 chunks combined** (e.g., "how does indexing handle errors" lives across `pipeline.go`, `walker.go`, `chunker.go`). RAG can only synthesize across what retrieval surfaces.

#### So is Hit@1 less important under RAG?

**Yes, but not "unimportant."**

- **Less important as a final-answer metric** — the LLM does the choosing, not the user.
- **Still important as an "answer-shape" predictor** — top-1 quality correlates with answer tone confidence and how much the LLM anchors on the correct framing.
- **Underweight it when comparing search-algorithm changes** — a `−5pp` hit@5 regression that buys `+20pp` hit@1 (the BM25 trade) is a clear win for retrieval-CLI but a more nuanced trade for RAG.

### 2.3 Recommended priority order for ragcodepilot today

Given you're between retrieval-CLI (now) and answer mode (next phase):

1. **MRR@5 — headline metric.** Single number that captures both "is top-1 right" and "if not, is the right answer near the top." Best for tracking improvements and catching regressions in one number.
2. **Hit@5 — RAG-readiness gate.** Once answer mode ships, `hit@5 < 0.70` means the LLM gets the right info less than 70% of the time. Set a floor (e.g., *"no future change drops hit@5 below 0.85"*). Current value (`baseline_v6`, current corpus, 182 chunks): **0.895** — above the floor. It briefly dipped to 0.789 when Phase 5 grew the corpus (`baseline_v5_pre`), but excluding `*_test.go` from indexing recovered it; see §2.5 "Corpus re-baseline + test-file hygiene".
3. **Negative pass — faithfulness floor.** If this ever drops below 1.0, your hallucination risk in answer mode goes up. Treat as a hard exit-criterion gate.
4. **Recall@10 vs Recall@5 ratio — diagnostic, not a target.** If recall@10 is much higher than recall@5, *"we know it but can't rank it"* → **reranking** is the right next step. If they're equal, embedding/chunking is the floor → upgrade the embedding model.
5. **Hit@1 — secondary.** Tracks retrieval-CLI quality. Useful as a leading indicator of "did the algorithm get sharper?" but no longer the final-answer metric.

### 2.4 New metrics needed when answer mode ships

These are **generation-quality** metrics. Some now exist as a **reference-free** answer-eval tier (`eval --answer`, shipped with Phase 5 v0) — deterministic checks on real generation (greedy/temp 0), reported but never gated:

- **Citation validity** ✅ *(reference-free, shipped)*: do `[N]` references in the answer point at chunks that were actually provided? Catches dangling citations. This is the cheap, deterministic cousin of citation precision.
- **Refusal rate on weak retrieval** ✅ *(reference-free, shipped)*: on negative queries (no strong match), does the model say "not enough information" instead of hallucinating? Detected by a phrase heuristic — a diagnostic, not ground truth. The hallucination floor.
- **Well-formedness** ✅ *(shipped)*: non-empty answer produced.

Still **not** implemented (the reference-*based* / judge tier — deferred to v1, "Tier C"):

- **Faithfulness / groundedness**: does the generated answer's *content* match the retrieved chunks? Needs LLM-as-judge — non-deterministic, requires a judge model, must not gate CI.
- **Citation precision** (semantic): do the cited chunks actually *contain the claimed facts*? (Validity checks the reference resolves; precision checks the claim is supported — the latter needs a judge.)
- **Per-query-type breakdown for answers**: navigation vs concept answers have different "what counts as success" definitions in RAG. (Retrieval already breaks down by type; answer metrics do not yet.)

### 2.5 Practical implications for current decisions

#### The BM25 commit (2026-05-15)

Result: hit@1 +21pp, hit@5 −5pp (one query), MRR@5 +10pp. p95 latency 173→119ms.

- **Was that a good trade for current state?** Yes — hit@1 +21pp is huge, the hit@5 loss is one tokenizer-bound query (a structural issue, not BM25). Users see better top-1 results immediately.
- **Was it a good trade for RAG state?** Yes — and the stemming follow-up (`baseline_v4`) closed the one hit@5 gap, making the BM25 switch a strict win in retrospect.
- **For the *next* trade, raise the hit@5 bar.** A future change should not drop hit@5 below 0.85 without an equally large or larger MRR@5 gain — measured on a *fixed* corpus (see "Corpus re-baseline" below; the Phase 2 number was 0.895).

#### Corpus re-baseline + test-file hygiene (2026-05-27, `baseline_v6`)

After Phase 5 v0 (`--answer` mode) landed, re-indexing the repo grew the corpus (new `internal/answer`, `internal/eval` code) and the golden set grew to 23 queries. Three re-baselines followed:

- `baseline_v5_pre` — the grown corpus *with* test files indexed.
- `baseline_v5` — after excluding `*_test.go` (and hidden dirs like `.claude/worktrees`) from indexing. **182 chunks.**
- `baseline_v6` — same corpus, first run carrying the new `recall@5` / recall-gap diagnostic. **This is the current canonical baseline.** (The tiny v5→v6 drift — hit@3 0.632→0.684, MRR@5 0.668→0.673 — is the recall-gap code itself getting indexed between runs; a reminder the signal-to-noise is low at this corpus size.)

| Metric | `baseline_v4` (Phase 2, 350 chunks) | `baseline_v5_pre` (grown, +tests) | `baseline_v6` (current, −tests) |
|---|---|---|---|
| hit@1 | 0.579 | 0.579 | 0.579 |
| hit@3 | 0.737 | 0.737 | 0.684 |
| hit@5 | 0.895 | 0.789 | **0.895** |
| MRR@5 | 0.699 | 0.660 | 0.673 |
| recall@5 | — | — | 0.789 |
| recall@10 | — | 0.789 | **0.921** |
| concept hit@5 | 1.000 | 0.714 | **1.000** |
| neg pass | 1.00 | 1.00 | 1.00 |

**Findings:**

1. **The v5_pre dip was corpus drift, not a regression** — hit@1 identical, run stable; the bulk of it was **test files crowding the top-K** (concept queries hurt most).
2. **Excluding test files recovered hit@5 to 0.895** (+10.5pp) and concept hit@5 to 1.000 (+28.6pp), clearing the 0.85 RAG-readiness floor — an S-effort fix, no reranker required. (hit@3 dipped from reshuffling, but that's below the top-5 the answer prompt uses.)
3. **The recall gap came back 0.132 (recall@10 0.921 − recall@5 0.789) — *above* the 0.10 threshold → reranking has headroom.** This corrected an earlier guess that the residual was purely an embedding/chunking floor. ~13pp of expected files are retrieved but ranked 6–10, outside the top-5 the answer sees — most relevant for multi-chunk concept/behavior answers.

The two residual `nav hit@5` misses split along that line:

- `run_eval_navigation` — `r@5=0, r@10=1`: retrieved but ranked 6–10 → **reranker-shaped** (a reranker, or a larger answer window, would fix it).
- `chunkfile_navigation` — `r@5=0, r@10=0`: absent from the top-10 → **embedding/chunking-shaped**, reranking can't surface it. (Now beaten by `internal/answer/fake.go` — code added this session competing, i.e. more self-inflicted drift.)

**Decision: reranker is justified-but-deferred.** The gap legitimately trips the "reranking has headroom" rule, but three things argue against building it now: (a) small sample — 19 positives, a 0.132 mean gap ≈ 2–3 queries; (b) hit@5 already clears the floor, so this is about recall@5 *completeness*, not the headline rate; (c) the reranker's cost is high (Python sidecar + 200–500 ms, the dominant con in §1.3). **Cheaper lever evaluated:** see `--answer-limit 8` A/B below.

#### `--answer-limit 8` A/B (2026-05-28)

**Hypothesis:** since `recall@10 ≫ recall@5`, raising `--answer-limit` from 5 → 8 puts the rank-6–10 chunks straight into the answer prompt for free, capturing most of the recall-gap's RAG value without building a reranker.

**Setup:** eval-side A/B on the 16-query structural subset. Two runs, only difference is `--answer-limit`:

- `baseline_v7_structural_answer_al5.json` — current default
- `baseline_v7_structural_answer_al8.json` — experiment

**Result:**

| Metric | AL=5 | AL=8 | Δ |
|---|---|---|---|
| hit@5 (retrieval, sanity) | 0.875 | 0.875 | 0 ✅ identical |
| WellFormedRate | 1.00 | 1.00 | 0 |
| CitedRate (positive) | 0.938 | 0.938 | 0 |
| AllCitationsValidRate | 1.00 | 1.00 | 0 |
| DanglingCitations | 0 | 0 | 0 |
| **GenerateP50MS** | **23,861** | **37,082** | **+13.2 s (+55%)** |
| **GenerateP95MS** | 46,434 | 58,805 | +12.4 s (+27%) |
| Total wall-clock | 455 s | 598 s | +142 s (+31%) |

**Interpretation:**

- The retrieval part is identical (sanity passes — `--answer-limit` doesn't touch the retrieval path).
- All Tier B *shape* metrics are flat. Cited rate is 0.938 in both runs (one uncited query each, just different queries); citations are 100% valid; no dangling refs.
- Per-query citation counts shift in *both* directions with more context (some queries cite more, some fewer) — non-systematic.
- **Generation latency rises ~55% at p50, ~27% at p95.** Real product cost.

**What Tier B can't measure:** whether the *content* of the answer improved — i.e., whether the additional chunks in the prompt caused the LLM to cover material it missed at AL=5. That's a correctness question; Tier B only sees shape. Decision needs **dogfooding** (or Tier C faithfulness judge).

**Implications:**

1. **Don't change the default `--answer-limit`** based on this data alone — shape is flat, latency cost is real.
2. **The Bucket B "free win" claim doesn't validate on automated metrics.** Whether AL=8 actually helps answers requires human judgment.
3. **GraphRAG's scope is *not* automatically narrowed.** A reranker or graph layer that lands Bucket B chunks in top-5 would deliver the same prompt without the latency penalty — this *strengthens* the case for a retrieval-side fix, not weakens it.
4. **The gating prerequisite for Phase 6 was updated** to require dogfooding judgment (not an eval-side number) before any "narrow scope" verdict — see `graphrag.md` gating section.

This is the cleanest example so far of a hypothesis that looked right on retrieval logic but didn't show up on the shape metrics. The eval is a forecast tool, not a content judge.

#### Stemming vs reranking, reframed under the RAG lens

- **Stemming** ✅ **shipped** — recovered the missing hit@5 result (`hasher_concept`) and the concept hit@5 score. Hybrid p95 latency +18% (119→141 ms), still well under the interactive threshold.
- **Reranking** lifts hit@1 and MRR@5 further. **Does not add new chunks to top-K.** If the right chunk is already there but ranked #4, reranking promotes it to #1 — the LLM may anchor on it (good), but doesn't change *what's in the prompt*. If the right chunk is missing from top-K entirely, reranking can't surface it.

Both help; **stemming is the more RAG-aligned cheap win**, **reranking is the bigger CLI-perception win**. For pure RAG quality, sequence: stemming → embedding model upgrade → reranking. For pure CLI feel, sequence: reranking → stemming.

### 2.6 The intuitive reframing

> **Pure search:** *"Did the user find it on the first page?"* → hit@K with small K.
>
> **RAG:** *"Did we give the model enough material to write a correct answer — and refuse when we didn't?"* → hit@K + recall@K + negative pass + (later) faithfulness.

Same retrieval substrate, different success criterion. Track both axes; emphasize the RAG axis as Phase 5 approaches.

---

## 3. Bottom line

- **Track MRR@5 as the single headline metric.** It blends "did we put the right answer near the top" (CLI relevance) and "is the LLM likely to anchor on something correct" (RAG relevance) — the same blend the product cares about during this transition phase.
- **Use Hit@5 as a hard floor.** Once answer mode ships, hit@5 below ~0.80 means the LLM is missing the right context too often. No retrieval-algorithm change should violate this floor without an exceptional MRR@5 lift to compensate.
- **Treat Hit@1 as a precision indicator, not the final-answer metric.** Useful for tracking algorithm sharpness; don't optimize it at the expense of hit@5.
- **Don't let Negative pass slip from 1.0.** It's a faithfulness floor for answer mode.
- **For the next quality investment:**
  - If you want the cheapest fix to the one known regression: **stemming** (S, no latency cost, RAG-aligned).
  - If you want the biggest hit@1 lift and have latency budget: **reranking** (M–L, large latency cost, CLI-aligned, also helps RAG by stabilizing top-K ordering).
  - If you want the new capability before more quality work: **Phase 5 v0 answer mode** (M, doesn't lift retrieval but proves the RAG product surface).

Whatever choice gets made next, **commit the eval result alongside the code change** (as `baseline_v4*.json`) so this doc and `hybrid_search.md` stay grounded in measured behavior, not predicted behavior.
