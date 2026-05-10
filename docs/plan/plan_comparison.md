# Plan comparison: vector_DB_app.md vs system_design.md

This document compares the two implementation plans for the semantic code search project.

## Summary

| | vector_DB_app.md (old plan) | system_design.md (new plan) |
|---|---|---|
| **Approach** | Bottom-up: build engine first, then application | Top-down: build application first, then study engine |
| **What you code** | The vector DB engine (storage, indexes, segments, WAL) | The search application (CLI, ingestion, search, embedding) |
| **Vector DB used** | Your own, built from scratch in Go | Qdrant (existing, running in Docker) |
| **Time to first search result** | Weeks/months (engine must work first) | Days (Qdrant handles the engine) |
| **Embedding approach** | Start with fake/generated vectors | Real embeddings from day 1 |

---

## Architecture comparison

### Old plan (vector_DB_app.md)

```
Source code repo
       │
       ▼
Chunker / parser
       │
       ▼
Embedding generator
       │
       ▼
┌─────────────────────────┐
│  YOUR GO VECTOR DB      │  ← You build this from scratch
│                         │
│  - Collections          │
│  - Segments             │
│  - Vector storage       │
│  - Payload storage      │
│  - Flat index → HNSW    │
│  - WAL + snapshots      │
│  - Payload filters      │
└─────────────────────────┘
       │
       ▼
REST / gRPC / CLI API
```

### New plan (system_design.md)

```
┌───────────────────────┐
│  CLI / TUI            │  ← You build this
└───────────┬───────────┘
            │
  ┌─────────┴─────────┐
  │                    │
  ▼                    ▼
┌──────────┐    ┌───────────┐
│ Ingestion│    │  Search   │  ← You build these
│ Pipeline │    │  Service  │
└────┬─────┘    └─────┬─────┘
     │                │
     ▼                ▼
┌─────────────────────────┐
│  Embedding Service      │  ← You build this (thin wrapper)
└─────────────────────────┘
            │
            ▼
┌─────────────────────────┐
│  QDRANT (Docker)        │  ← Existing, you don't build this
│                         │
│  Handles: storage,      │
│  HNSW, segments, WAL,   │
│  filtering, hybrid      │
└─────────────────────────┘
```

---

## Phase-by-phase comparison

### Phase 1

| Old plan | New plan |
|---|---|
| Build `Point`, `Collection`, `Vector` structs | Set up Go project + Qdrant Docker |
| Implement cosine, dot, L2 distance functions | Implement file walker + text chunker |
| Implement brute-force search (scan all points) | Implement embedder (API or local model) |
| Use fake/generated vectors | Use real embeddings from day 1 |
| **Result**: can insert and search fake vectors | **Result**: can index a real repo and search it |

### Phase 2

| Old plan | New plan |
|---|---|
| Build payload filtering from scratch | Add language detection + Qdrant payload filters |
| Build payload indexes (inverted index) | Improve chunker (function-level parsing) |
| Implement filter intersection logic | Add re-indexing (detect changes) |
| **Result**: filtered brute-force search works | **Result**: filtered semantic search on real code |

### Phase 3

| Old plan | New plan |
|---|---|
| Build segments (mutable/sealed) | Add sparse vectors for BM25 keyword matching |
| Build segment interface | Implement hybrid search with RRF fusion |
| Build deleted bitmaps | Add exact function name search |
| **Result**: data is split into segments | **Result**: hybrid search (keyword + semantic) works |

### Phase 4

| Old plan | New plan |
|---|---|
| Build persistence: WAL, snapshots, disk storage | Polish CLI, result formatting |
| Implement crash recovery | Study Qdrant internals (map to Vector_DB_core.md) |
| **Result**: data survives restarts | **Result**: ready for Phase C (Rust→Go refactor) |

### Phase 5-8 (old plan only)

| Old plan | New plan |
|---|---|
| Build HNSW index from scratch | (Not needed — Qdrant handles this) |
| Build compaction, optimized segments | (Not needed — study how Qdrant does it instead) |
| Connect real embeddings | (Already using real embeddings since Phase 1) |
| Build benchmarks | (Not needed at this stage) |

---

## What each plan teaches

### Old plan teaches (bottom-up)

- How distance functions work mathematically
- How brute-force search works
- How payload indexes are structured (inverted indexes)
- How segments organize data
- How WAL provides crash safety
- How HNSW builds a navigable graph
- How compaction merges segments

### New plan teaches (top-down)

- How to model data for a vector DB (points, payloads, collections)
- How to chunk and embed real data
- How to design search queries (semantic, filtered, hybrid)
- How to use payload filtering in practice
- How hybrid search (dense + sparse) works from the user side
- How to interact with a vector DB API (upsert, search, delete, manage collections)

### Both plans eventually teach everything

```
New plan path:
  Build app on Qdrant → Study Qdrant internals → Refactor Rust→Go
  (learn application first)    (learn engine second)   (build engine third)

Old plan path:
  Build engine from scratch → Connect real app → Refactor Rust→Go
  (learn engine first)        (learn app second)  (already have engine)
```

---

## Why we chose the new plan

1. **Faster to working result**: real semantic search in days, not months
2. **Motivation**: see useful results early instead of debugging internal data structures
3. **Better Phase C preparation**: by using Qdrant as a user, you know what a good vector DB API should look like before building one
4. **Theory still covered**: `Vector_DB_core.md` remains the reference for Phase B (studying internals)
5. **The old plan is not wasted**: when you reach Phase C (Rust→Go refactor), `vector_DB_app.md` becomes a useful reference for what the engine needs to implement internally

---

## Relationship between documents

```
compare.md           → Which existing vector DB to use? (Answer: Qdrant)
system_design.md     → How to build the application on top of Qdrant (Phase A)
Vector_DB_core.md    → Theory of vector DB internals (Phase B study material)
vector_DB_app.md     → Reference for building engine internals (Phase C guide)
plan_comparison.md   → This document (explains why we chose top-down)
```
