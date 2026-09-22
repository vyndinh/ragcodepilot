# Retrieval Quality Decisions

> Reference doc for trade-off analyses on retrieval-quality changes. Add to it as the system evolves; each section is a self-contained analysis you can revisit before making the next decision.
>
> Currently covers:
>
> 1. **Reranking** — pros/cons for *this* system
> 2. **Which retrieval metrics matter** — and how the answer changes as ragcodepilot moves from retrieval-CLI toward full RAG

**Companion docs:**

- [`../plan/mvp_roadmap.md`](../plan/mvp_roadmap.md) — active local CLI roadmap and product direction
- [`../plan/hybrid_search.md`](../plan/hybrid_search.md) — current BM25 + dense + RRF design with additive stemming (`baseline_v4`)
- [`../plan/rag_evaluation_metrics.md`](../plan/rag_evaluation_metrics.md) — eval harness spec
- [`code_graph_retrieval_landscape.md`](code_graph_retrieval_landscape.md) — code-graph concepts and historical research references
- [`rag_notebook.md`](rag_notebook.md) — beginner walkthrough; §14 has historical performance examples

---

## Current decision boundary (2026-09-22)

The analyses below retain historical observations and hypotheses. Active sequencing
is the [local CLI roadmap](../plan/mvp_roadmap.md); MCP, the agent-first pivot, and
multi-repo workspace features are deferred. New retrieval layers and dedicated prototypes stay deferred until recurring
failures justify investigation. Start with one external repository and manual
comparisons; automated retrieval CI, broader benchmark collections, and extra
product surfaces are deferred.

The [fresh v9 self baseline](../eval/runs/2026-09-22-self-fed7f22/README.md) pins
merged revision `fed7f22`, 248 chunks, source/query hashes, runtime, and model digest.
It reports hit@5 34/35, structural hit@5 15/16, and negatives 2/4 at RRF 0.02,
with zero query errors. Eleven positives still lack complete expected-file coverage
at top 5. The external portion of M0 remains open. v8 is historical context,
not a controlled comparison against v9.

Historical v6/v7 hybrid 1.00 negative pass used an ineffective 0.55 cosine cutoff.
It is not a faithfulness floor. Preserve passing negatives under valid fixed
score-aware calibration and track known failures explicitly; never loosen a cutoff
to restore 1.00. The same two v6 saved negatives fail when scored at main's 0.02.

Structural hit@5 already passes 14/16 historical queries. All seven recall-gap
queries already pass it too. Use required-evidence coverage for completeness,
retain hit@5 as a preservation check, and do not require binary improvement on
already-passing queries. The concepts below explain these measurements without
scheduling another retrieval component.

## Table of contents

1. [Reranking — pros/cons for ragcodepilot](#1-reranking--proscons-for-ragcodepilot)
2. [Which retrieval metrics matter](#2-which-retrieval-metrics-matter)
3. [Measurement rules](#3-measurement-rules)

---

## 1. Reranking — pros/cons for ragcodepilot

A reranker scores query/chunk pairs and changes the order of retrieved candidates.
It can move relevant evidence into the displayed or generated context, but cannot
recover evidence absent from its input pool. Its quality and latency effects must
be measured on the actual corpus; there is no established local gain to promise.

The historical recall gap identifies additional relevant files below top-5. It
does not establish that a reranker will promote the right chunks, improve answers,
or fit the local resource budget. Candidate depth is also a separate variable:
compare ordering on the same pool to isolate it.

Stemming, identifier tokens, and named Go type/interface chunks are implemented.
Answer mode is also implemented and did not require a reranker. Detailed runtime
choices, predicted uplift tables, and feature sequencing have been removed.
Current comparison rules live in the
[evaluation guide](../eval/README.md#interpreting-coverage-and-isolating-changes).

---

## 2. Which retrieval metrics matter

### 2.1 What each metric measures

| Metric | Plain meaning | What it captures |
|---|---|---|
| **Hit@1** | Is the *very first* result correct? | Top-of-list precision; pure ranking quality. |
| **Hit@K** (K=3, 5) | Is the correct result somewhere in the top K? | Tolerance for slightly imperfect ranking. |
| **MRR@K** | Average position of the first correct hit (`1/rank`). | Hybrid of precision and top-of-list bias. |
| **Recall@K** | What fraction of *all* relevant chunks made it to top K? | Coverage — matters when there's more than one right answer. |
| **Negative pass** | Out-of-scope queries return nothing on-topic. | "We know when we don't know." Mode-calibrated: dense uses cosine 0.55; hybrid RRF fails only on dual-prefetch agreement (ceiling 0.02); sparse BM25 uses ceiling 10. A single 0.55 cutoff is vacuous under RRF. |

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

For the implemented CLI and optional answer mode, preserve accepted hits and
required-evidence coverage first. Use MRR@5 and hit@1 to diagnose ordering;
use recall@5 and recall@10 to locate incomplete context. No single metric or
historical threshold selects the next retrieval component.

Negative checks use fixed score-aware policies with named known failures.
Answer content requires manual review. Follow the
[evaluation comparison rules](../eval/README.md#interpreting-coverage-and-isolating-changes)
for new measurements.

### 2.4 Implemented answer diagnostics and measurement gaps

These are **generation-quality** metrics. Some now exist as a **reference-free** answer-eval tier (`eval --answer`, shipped with Phase 5 v0) — deterministic checks on real generation (greedy/temp 0), reported but never gated:

- **Citation validity** ✅ *(reference-free, shipped)*: do `[N]` references in the answer point at chunks that were actually provided? Catches dangling citations. This is the cheap, deterministic cousin of citation precision.
- **Refusal rate on weak retrieval** ✅ *(reference-free, shipped)*: on negative queries (no strong match), does the model say "not enough information" instead of hallucinating? Detected by a phrase heuristic — a diagnostic, not ground truth. This does not measure hallucination directly.
- **Well-formedness** ✅ *(shipped)*: non-empty answer produced.

The following content measurements are not automated; they describe gaps, not a scheduled feature list:

- **Faithfulness / groundedness**: does the generated answer's *content* match the retrieved chunks? Review manually; automated judging is outside the current scope.
- **Citation precision** (semantic): do the cited chunks actually *contain the claimed facts*? (Validity checks the reference resolves; precision checks the claim is supported — the latter needs content review.)
- **Per-query-type breakdown for answers**: navigation vs concept answers have different "what counts as success" definitions in RAG. (Retrieval already breaks down by type; answer metrics do not yet.)

### 2.5 Practical implications for current decisions

#### The BM25 commit (2026-05-15)

Result: hit@1 +21pp, hit@5 −5pp (one query), MRR@5 +10pp. p95 latency 173→119ms.

- **Was that a good trade for current state?** Yes — hit@1 +21pp is huge, the hit@5 loss is one tokenizer-bound query (a structural issue, not BM25). Users see better top-1 results immediately.
- **Was it a good trade for RAG state?** Yes — and the stemming follow-up (`baseline_v4`) closed the one hit@5 gap, making the BM25 switch a strict win in retrospect.
- **Current acceptance:** preserve accepted hits and required coverage on frozen inputs. Historical aggregate floors do not authorize losing cases in exchange for higher MRR.

#### Corpus re-baseline + test-file hygiene (2026-05-27, `baseline_v6`)

After Phase 5 v0 (`--answer` mode) landed, re-indexing the repo grew the corpus (new `internal/answer`, `internal/eval` code) and the golden set grew to 23 queries. Three re-baselines followed:

- `baseline_v5_pre` — the grown corpus *with* test files indexed.
- `baseline_v5` — after excluding `*_test.go` (and hidden dirs like `.claude/worktrees`) from indexing. **182 chunks.**
- `baseline_v6` — same corpus, first run carrying the new `recall@5` / recall-gap diagnostic. **Historical pre-identifier-token baseline; see the current decision boundary above.** (The tiny v5→v6 drift — hit@3 0.632→0.684, MRR@5 0.668→0.673 — is the recall-gap code itself getting indexed between runs; a reminder the signal-to-noise is low at this corpus size.)

| Metric | `baseline_v4` (Phase 2, 350 chunks) | `baseline_v5_pre` (grown, +tests) | `baseline_v6` (historical, −tests) |
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
3. **The recall gap was 0.132 (recall@10 0.921 − recall@5 0.789).** This corrected an earlier guess that the residual was purely an embedding/chunking floor. ~13pp of expected files are retrieved but ranked 6–10, outside the top-5 the answer sees — most relevant for multi-chunk concept/behavior answers.

The two residual `nav hit@5` misses split along that line:

- `run_eval_navigation` — `r@5=0, r@10=1`: retrieved but ranked 6–10 → **reranker-shaped** (ordering and context depth were hypotheses to test).
- `chunkfile_navigation` — `r@5=0, r@10=0`: absent from the top-10 → **embedding/chunking-shaped**, reranking can't surface it. (Now beaten by `internal/answer/fake.go` — code added this session competing, i.e. more self-inflicted drift.)

**Historical interpretation:** this gap motivated a reranking hypothesis and the
larger-context experiment below. Neither the small sample nor the gap proves a
reranker would help. The saved measurements remain useful; the proposed build
and runtime estimates have been removed.

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
3. **No retrieval-layer benefit was demonstrated.** The result does not prove a
   graph or reranker would provide complete evidence at lower total latency.
   Keep answer defaults unchanged without content-quality evidence.

This is the cleanest example so far of a hypothesis that looked right on retrieval logic but didn't show up on the shape metrics. The eval is a forecast tool, not a content judge.

#### Coverage and ordering are different measurements

Stemming recovered the historical `hasher_concept` hit@5 regression. Its measured
result is retained in the hybrid-search history. A possible ordering change must
be evaluated separately: it can change the final top-K, but only using evidence
already in its candidate pool. Neither mechanism implies a fixed next-feature order.

### 2.6 The intuitive reframing

> **Pure search:** *"Did the user find it on the first page?"* → hit@K with small K.
>
> **RAG:** *"Did we give the model enough material to write a correct answer — and refuse when we didn't?"* → hit@K + recall@K + negative pass + (later) faithfulness.

Track retrieval coverage and manually reviewed answer quality separately; answer mode is already implemented.

---

## 3. Measurement rules

- Preserve accepted hit@5 cases and required evidence; use ranking metrics to
  diagnose ordering rather than as a substitute for coverage.
- Keep negative checks meaningful with fixed score-aware policies on frozen
  inputs. Track existing failures and reject new regressions.
- Inspect answer content manually; citation shape and recall gaps do not prove
  faithfulness or task usefulness.
- Save reports with pinned manifests and unique revision/experiment names.
  Do not overwrite historical baselines or treat different snapshots as an A/B.

The [roadmap](../plan/mvp_roadmap.md) owns next work. This reference records
concepts and evidence, not a feature backlog.
