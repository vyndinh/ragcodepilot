# ragcodepilot — Documentation Index

This folder is organized into tiers by **purpose**, not by feature. Use this
page to find the right entry point. The roadmap owns active work; historical
plans and reviews are not a second backlog.

| Tier | Folder | What lives here | Start with |
|---|---|---|---|
| **Plan** | `plan/` | Design docs, the roadmap, phase implementation plans | [`plan/mvp_roadmap.md`](plan/mvp_roadmap.md) |
| **Knowledge** | `knowledge/` | Reference: trade-off decisions + learning material | [`knowledge/building_ragcodepilot.md`](knowledge/building_ragcodepilot.md) (story) / [`knowledge/rag_notebook.md`](knowledge/rag_notebook.md) (beginners) |
| **Review feedback** | `review_feedback/` | Historical reviews; not current implementation guidance | [Historical records](#historical-records) |
| **Brainstorm** | `brainstorm/` | Early exploration, not committed direction | — |
| **Improvement** | `improvement/` | Indexing and local reliability contracts | [`improvement/production_readiness_and_features.md`](improvement/production_readiness_and_features.md) |
| **Eval** | `eval/` | Metrics spec, golden set, baselines | [`eval/README.md`](eval/README.md) |
| **Task tracker** | `task_tracker/` | Per-phase task tracking | — |

> **New here?** Read [`knowledge/building_ragcodepilot.md`](knowledge/building_ragcodepilot.md) for the narrative of how the system was built, then [`plan/mvp_roadmap.md`](plan/mvp_roadmap.md) for what's next.
>
> **Making a retrieval-quality or architecture decision?** See the two decision docs in `knowledge/`: [`retrieval_quality_decisions.md`](knowledge/retrieval_quality_decisions.md) (what to score) and [`architecture_decisions.md`](knowledge/architecture_decisions.md) (process shape). The roadmap records the reconciled implementation and saved-evidence boundary; `retrieval_quality_decisions.md` §2.5 retains the historical `--answer-limit` A/B.

---

## plan/ — design & roadmap

| Doc | Purpose |
|---|---|
| [`mvp_roadmap.md`](plan/mvp_roadmap.md) | **Canonical next-up tasks**: fresh evidence on one external repo, dense reuse/recovery, and local errors/.gitignore. Start here. |
| [`phase5_v0_answer_mode.md`](plan/phase5_v0_answer_mode.md) | Phase 5 v0 — `--answer` mode (minimal RAG seam). |
| [`hybrid_search.md`](plan/hybrid_search.md) | Phase 2 — hybrid search (BM25 + dense + RRF) implementation + eval history. |
| [`phase3_rust_chunker.md`](plan/phase3_rust_chunker.md) | Phase 3 — Rust AST chunker plan (deferred). |
| [`function_level_chunker.md`](plan/function_level_chunker.md) | Go AST function-level chunker design. |
| [`chunk_enrichment.md`](plan/chunk_enrichment.md) | Metadata enrichment before embedding. |
| [`embedding_dimension_validation.md`](plan/embedding_dimension_validation.md) | Dimension auto-detection + batch validation. |
| [Evaluation guide](eval/README.md) | Canonical metrics, dataset schema, comparison workflow, and measurement gaps. |
| [`system_design.md`](plan/system_design.md) | Full system design document. |
| [Application-first decision](knowledge/architecture_decisions.md#application-first-not-a-custom-vector-database) | Why the CLI uses Qdrant instead of building a storage engine. |

## knowledge/ — reference & trade-offs

**Reference docs** (current work is defined by the roadmap):

| Doc | Purpose |
|---|---|
| [`retrieval_quality_decisions.md`](knowledge/retrieval_quality_decisions.md) | Retrieval concepts and historical evaluation findings, including the AL=8 A/B. |
| [`architecture_decisions.md`](knowledge/architecture_decisions.md) | Process-shape trade-offs (CLI vs daemon, watch mode, cold start). |
| [`code_graph_retrieval_landscape.md`](knowledge/code_graph_retrieval_landscape.md) | Code-graph concepts and historical prior-art references; no implementation plan. |

**Learning material:**

| Doc | Purpose |
|---|---|
| [`building_ragcodepilot.md`](knowledge/building_ragcodepilot.md) | The story — how the system evolved, one decision at a time. |
| [`rag_notebook.md`](knowledge/rag_notebook.md) | Beginner walkthrough of RAG via ragcodepilot. |
| [`rag_glossary.md`](knowledge/rag_glossary.md) | RAG terminology. |
| [`rag_parts.md`](knowledge/rag_parts.md) | The component parts of a RAG system. |
| [`embeddings_explained.md`](knowledge/embeddings_explained.md) | How embeddings work. |
| [`sparse_vs_dense.md`](knowledge/sparse_vs_dense.md) | Sparse vs dense vectors. |
| [`hybrid_search_explained.md`](knowledge/hybrid_search_explained.md) | What hybrid search is and why it matters. |
| [`bm25_vs_tfidf.md`](knowledge/bm25_vs_tfidf.md) | How keyword scoring works. |
| [`compare.md`](knowledge/compare.md) | Search & vector-database comparison. |

## Historical records

These records are archived in place so existing links and decision history remain
available. Their feature statuses, priorities, and measurements describe the time
of writing, not current behavior. Use the [roadmap](plan/mvp_roadmap.md) for active work.

| Doc | Purpose |
|---|---|
| [`checklist.md`](plan/checklist.md) | Original phase checklist; no longer the current progress tracker. |
| [`system_vision_review.md`](review_feedback/system_vision_review.md) | Historical strategy review, including the original ten product questions. |
| [`codemaps_review.md`](review_feedback/codemaps_review.md) | Historical review of Explore Mode; not an active delivery plan. |
| [`hybrid_search_review.md`](review_feedback/hybrid_search_review.md) | Hybrid search review history. |
| [`reindexing_review.md`](review_feedback/reindexing_review.md) | Re-indexing pipeline review history. |
| [`feedback_analysis.md`](review_feedback/feedback_analysis.md) | Historical disposition of the original reviews; later decisions supersede it. |

Duplicated specification copies have been consolidated into the
[system design review disposition](plan/system_design.md#historical-review-disposition)
and [evaluation review disposition](eval/README.md#historical-review-disposition).
The full original documents remain in Git history at revision `8644cc7`.

## brainstorm/ — exploratory (not committed)

| Doc | Purpose |
|---|---|
| [`codemaps_analysis.md`](brainstorm/codemaps_analysis.md) | Original Explore Mode proposal. |
| [`idea_end_to_end_rag_pipeline.md`](brainstorm/idea_end_to_end_rag_pipeline.md) | Ideal end-to-end RAG pipeline sketch. |
| [`vector_DB_app.md`](brainstorm/vector_DB_app.md) | Historical custom-engine proposal; outside the active product roadmap. |
| [`Vector_DB_core.md`](brainstorm/Vector_DB_core.md) | Vector-DB internals learning notes, not an implementation commitment. |
| [Flat-search exercise](plan/vecdb/vecdb_phase1_flat_search.md) | Separate custom-engine learning proposal; not active product work. |
| [Custom-vector-DB discussion](discussion_about_plan_own_vectorDB.md) | Archived conversation, not a delivery plan. |

## improvement/ — indexing contracts

| Doc | Purpose |
|---|---|
| [`reindexing.md`](improvement/reindexing.md) | Re-indexing via file-hash + index-version change detection. |
| [`production_readiness_and_features.md`](improvement/production_readiness_and_features.md) | Local indexing recovery/cost, external eval, and onboarding contracts; product expansion deferred. |

## eval/ — metrics, golden set, baselines

See [`eval/README.md`](eval/README.md). Holds the golden query set and the
`baseline_v*.json` files. v6/v7 on this branch are historical snapshots.
The latest self-corpus evidence is [v9 full](eval/baseline_v9.json) plus
[v9 structural](eval/baseline_v9_structural.json), run at merged revision `fed7f22`.
The [run record](eval/runs/2026-09-22-self-fed7f22/README.md) includes pinned inputs,
zero-error validation, and named misses. The [external chi run](eval/external/chi-v5.2.3/README.md)
completes M0 measurements and retains the original collision record plus the
[post-fix recheck](eval/runs/2026-09-23-idfix/README.md).
Use the [comparison rules](eval/README.md#baselines-and-the-corpus-stability-assumption).
Fresh same-input control/candidate runs are required for acceptance.

## task_tracker/ — task tracking

| Doc | Purpose |
|---|---|
| [`phase3_rust_chunker.md`](task_tracker/phase3_rust_chunker.md) | Phase 3 Rust AST chunker task tracker. |
