# ragcodepilot — Documentation Index

This folder is organized into tiers by **purpose**, not by feature. Use this
page to find the right entry point.

| Tier | Folder | What lives here | Start with |
|---|---|---|---|
| **Plan** | `plan/` | Design docs, the roadmap, phase implementation plans | [`plan/mvp_roadmap.md`](plan/mvp_roadmap.md) |
| **Knowledge** | `knowledge/` | Reference: trade-off decisions + learning material | [`knowledge/building_ragcodepilot.md`](knowledge/building_ragcodepilot.md) (story) / [`knowledge/rag_notebook.md`](knowledge/rag_notebook.md) (beginners) |
| **Review feedback** | `review_feedback/` | Review logs and decision records for past plans | [`review_feedback/system_vision_review.md`](review_feedback/system_vision_review.md) |
| **Brainstorm** | `brainstorm/` | Early exploration, not committed direction | — |
| **Improvement** | `improvement/` | Roadmaps for incremental indexing / re-indexing | [`improvement/incremental_processing_roadmap.md`](improvement/incremental_processing_roadmap.md) |
| **Eval** | `eval/` | Metrics spec, golden set, baselines | [`eval/README.md`](eval/README.md) |
| **Task tracker** | `task_tracker/` | Per-phase task tracking | — |

> **New here?** Read [`knowledge/building_ragcodepilot.md`](knowledge/building_ragcodepilot.md) for the narrative of how the system was built, then [`plan/mvp_roadmap.md`](plan/mvp_roadmap.md) for what's next.
>
> **Making a retrieval-quality or architecture decision?** See the two decision docs in `knowledge/`: [`retrieval_quality_decisions.md`](knowledge/retrieval_quality_decisions.md) (what to score) and [`architecture_decisions.md`](knowledge/architecture_decisions.md) (process shape). `retrieval_quality_decisions.md` §2.5 is the canonical record for current baseline numbers and the `--answer-limit` A/B.

---

## plan/ — design & roadmap

| Doc | Purpose |
|---|---|
| [`mvp_roadmap.md`](plan/mvp_roadmap.md) | **Canonical next-up tasks** and product direction. Start here. |
| [`graphrag.md`](plan/graphrag.md) | Phase 6 — GraphRAG structural retrieval layer (design doc). |
| [`phase5_v0_answer_mode.md`](plan/phase5_v0_answer_mode.md) | Phase 5 v0 — `--answer` mode (minimal RAG seam). |
| [`hybrid_search.md`](plan/hybrid_search.md) | Phase 2 — hybrid search (BM25 + dense + RRF) implementation + eval history. |
| [`phase3_rust_chunker.md`](plan/phase3_rust_chunker.md) | Phase 3 — Rust AST chunker plan (deferred). |
| [`function_level_chunker.md`](plan/function_level_chunker.md) | Go AST function-level chunker design. |
| [`chunk_enrichment.md`](plan/chunk_enrichment.md) | Metadata enrichment before embedding. |
| [`embedding_dimension_validation.md`](plan/embedding_dimension_validation.md) | Dimension auto-detection + batch validation. |
| [`rag_evaluation_metrics.md`](plan/rag_evaluation_metrics.md) | Eval harness spec (input for Phase 1). |
| [`system_design.md`](plan/system_design.md) | Full system design document. |
| [`plan_comparison.md`](plan/plan_comparison.md) | `vector_db_app.md` vs `system_design.md` comparison. |
| [`checklist.md`](plan/checklist.md) | Historical record of the original phase plan. |

## knowledge/ — reference & trade-offs

**Decision docs** (revisit before making the next call):

| Doc | Purpose |
|---|---|
| [`retrieval_quality_decisions.md`](knowledge/retrieval_quality_decisions.md) | Quality trade-offs (reranking, metrics, §2.5 = canonical baselines + AL=8 A/B). |
| [`architecture_decisions.md`](knowledge/architecture_decisions.md) | Process-shape trade-offs (CLI vs daemon, watch mode, cold start). |
| [`code_graph_retrieval_landscape.md`](knowledge/code_graph_retrieval_landscape.md) | Industry landscape & prior art for Phase 6 (GraphRAG). |

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

## review_feedback/ — review logs & decision records

| Doc | Purpose |
|---|---|
| [`system_vision_review.md`](review_feedback/system_vision_review.md) | Source of the phase numbering and overall strategy. |
| [`codemaps_review.md`](review_feedback/codemaps_review.md) | Why Explore Mode was deferred (now superseded by GraphRAG). |
| [`hybrid_search_review.md`](review_feedback/hybrid_search_review.md) | Hybrid search review history. |
| [`reindexing_review.md`](review_feedback/reindexing_review.md) | Re-indexing pipeline review history. |
| [`system_design_with_feedback.md`](review_feedback/system_design_with_feedback.md) | System design with inline feedback. |
| [`rag_evaluation_metrics_with_feedback.md`](review_feedback/rag_evaluation_metrics_with_feedback.md) | Eval metrics with inline feedback. |
| [`feedback_analysis.md`](review_feedback/feedback_analysis.md) | Feedback analysis. |

## brainstorm/ — exploratory (not committed)

| Doc | Purpose |
|---|---|
| [`codemaps_analysis.md`](brainstorm/codemaps_analysis.md) | Original Explore Mode proposal. |
| [`idea_end_to_end_rag_pipeline.md`](brainstorm/idea_end_to_end_rag_pipeline.md) | Ideal end-to-end RAG pipeline sketch. |
| [`vector_db_app.md`](brainstorm/vector_db_app.md) | Early "mini Qdrant for code search" idea. |
| [`vector_db_core.md`](brainstorm/vector_db_core.md) | Vector-DB internals notes. |

## improvement/ — incremental indexing roadmaps

| Doc | Purpose |
|---|---|
| [`incremental_processing_roadmap.md`](improvement/incremental_processing_roadmap.md) | Incremental processing roadmap. |
| [`reindexing.md`](improvement/reindexing.md) | Re-indexing via file-hash change detection. |

## eval/ — metrics, golden set, baselines

See [`eval/README.md`](eval/README.md). Holds the golden query set and the
`baseline_v*.json` files; `baseline_v6.json` is the current canonical baseline,
`baseline_v7_structural.json` is the Phase 6 comparison target.

## task_tracker/ — task tracking

| Doc | Purpose |
|---|---|
| [`phase3_rust_chunker.md`](task_tracker/phase3_rust_chunker.md) | Phase 3 Rust AST chunker task tracker. |

## Top-level

| Doc | Purpose |
|---|---|
| [`discussion_about_plan_own_vectorDB.md`](discussion_about_plan_own_vectorDB.md) | Discussion log on building a custom vector DB (Phase C). |
| [`questions.md`](questions.md) | Open questions. |
