# Feedback Analysis

Review of suggestions from external feedback on `system_design_with_feedback.md` and `rag_evaluation_metrics_with_feedback.md`.

## Summary

Both documents push heavily toward building an **evaluation framework**. Some suggestions are valuable, but most are **premature** for the current project stage. The system has 139 indexed chunks from a single repo — building a 6-mode eval CLI with CI regression policies would be massive over-engineering.

~60% of the eval content is **duplicated** between the two docs (same eval CLI, same modes, same CI policies).

---

## Action Items — Do Now (Phase 2)

| # | Item | Source | Effort | Why |
|---|------|--------|--------|-----|
| 1 | **Payload indexes** | system_design #6 | Trivial | Add `repo`, `language`, `file_path` as indexed fields in `EnsureCollection`. Without this, filtered search does full scan on every query |
| 2 | **Fix point ID strategy** | system_design #4 | Medium | Current `repo+file+start_line` creates orphan vectors when lines shift. Switch to `repo+file+symbol+chunk_index` for function chunks, `repo+file+content_hash` for anonymous blocks |
| 3 | **Re-indexing design** | system_design #5 | Medium | File-hash based change detection + delete-by-filter for stale points. Depends on #2 |

---

## Action Items — Defer to Phase 3

| # | Item | Source | Why defer |
|---|------|--------|-----------|
| 4 | **Simple eval harness** | both docs | Build a simple `ragcodepilot eval` with `hit@k` and `MRR@k` **before** adding hybrid search. One command, one YAML dataset, terminal output. No CI, no compare mode, no regression policy |
| 5 | **Observability hooks** | system_design #7 | Add timing fields (`query_embedding_ms`, `qdrant_search_ms`) when performance becomes a question |
| 6 | **Baseline workflow** | system_design #8 | Save semantic-only results before hybrid search. Build when Phase 3 is imminent |

---

## Skip — Over-engineering or Redundant

| # | Item | Source | Reason to skip |
|---|------|--------|----------------|
| Chunking strategy table | system_design #6 | Already in `docs/plan/function_level_chunker.md`. Second table creates drift |
| Explicit non-goals | system_design #9 | Over-documenting. README + system_design already scope the project |
| Graded relevance labels (0-3) | eval_metrics #1 | Binary relevant/irrelevant is enough for 25 queries on 1 repo |
| Filter correctness metrics | eval_metrics #3 | Already tested by unit tests (`TestClient_SearchLanguageOnly`, etc) |
| Query categories (6 types) | eval_metrics #4 | Useful at scale, overkill for 20-30 queries |
| Result-shape validation | eval_metrics #6 | Guaranteed by Go type system (`model.CodeChunk` struct). Every result always has these fields |
| Reproducibility metadata | eval_metrics #7 | Correct for team projects. Unnecessary for solo local development |
| Top-result quality checks | eval_metrics #5 | Implied by `hit@1` and `MRR@5`. Not a separate metric |
| 6-mode eval CLI | eval_metrics #8 | retrieval, filters, latency, ingestion, compare, regression — way too many modes |
| CI regression policy | eval_metrics #9 | No CI pipeline, no team, no automated deployment |
| 10-step implementation order | system_design #10 | Would halt feature progress for 1-2 weeks to build tooling for 139 chunks |

---

## Overlap Between Documents

| Topic | system_design | eval_metrics | Redundant? |
|-------|--------------|--------------|------------|
| `ragcodepilot eval` command | ✅ defined | ✅ defined | Yes — same thing |
| Eval modes (retrieval, filters, compare) | ✅ | ✅ | Yes — nearly identical |
| CI regression policy | ✅ | ✅ | Yes — same thresholds |
| Baseline workflow | ✅ | implied | Partial |
| Score interpretation | ✅ | ✅ | Yes |
| Negative cases | mentioned | ✅ detailed | No — eval_metrics adds detail |

---

## Decision Log

| Date | Item | Decision | Notes |
|------|------|----------|-------|
| 2026-05-08 | Initial review | Categorized all feedback | 3 items to do now, 3 defer, 11 skip |
| | Payload indexes | **Approved for Phase 2** | Trivial change in `EnsureCollection` |
| | Point ID fix | **Approved for Phase 2** | Prerequisite for re-indexing |
| | Re-indexing | **Approved for Phase 2** | Depends on point ID fix |
