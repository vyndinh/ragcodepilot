# Hybrid Search — Review History

Consolidated audit trail covering plan review, code reviews, and final task review for Phase 2 hybrid search.
All issues have been resolved.

---

## Plan Review (10 FEEDBACK items)

Review of `docs/plan/hybrid_search.md` before implementation. Three P1 issues addressed before coding; seven P2/P3 items addressed during implementation.

### What the plan got right (kept untouched)

- Server-side RRF via `PrefetchQuery + NewQueryRRF`.
- Breaking schema with a clear error message; no in-place migration.
- `k=60` hardcoded.
- The Out of Scope list.
- The general implementation order (tokenizer → client → pipeline → searcher → CLI).

### ✅ P1 FEEDBACK 1 — Batch-local IDF gives inconsistent token weights

**Problem:** Plan recommended batch-local IDF (32 chunks per batch). The same token gets different weights depending on which batch wrote it — breaks exact-match retrieval.

**Resolution:** Plan updated to compute IDF globally over the full ingestion run, in memory. Implemented at `pipeline.go:207-224` as `ComputeIDF(allTexts)` over all current chunks.

---

### ✅ P1 FEEDBACK 2 — BM25 vs TF-IDF naming

**Problem:** Plan called itself BM25 but pseudocode described TF-IDF (no `k1`/`b` parameters).

**Resolution:** Renamed to TF-IDF. File is `internal/embedding/sparse.go` (not `bm25.go`). No `k1`/`b` parameters — length normalization adds complexity without quality gain on short, uniform-length code chunks.

---

### ✅ P1 FEEDBACK 3 — Filters under PrefetchQuery need verification

**Problem:** Plan placed filters at the top level of `QueryPoints`. In Qdrant's hybrid model, filters must be per-prefetch to avoid fusing unfiltered results then discarding.

**Resolution:** Plan updated. Implemented at `client.go:338-350` — filter applied on each prefetch stage, not at the top level.

---

### ✅ P2 FEEDBACK 4 — Unify Search and HybridSearch

**Problem:** Plan introduced separate `Search` and `HybridSearch` methods, creating parallel code paths.

**Resolution:** Unified into one `Search` method that handles all modes via request shape (`client.go:280`). Pass `nil` for unused vectors.

---

### ✅ P2 FEEDBACK 5 — Use SparseVector struct, not parallel arrays

**Problem:** Parallel `sparseIndices []uint32` + `sparseValues []float32` is a footgun.

**Resolution:** `SparseVector` struct with `NewSparseVector` constructor validation (`sparse.go:14-27`).

---

### ✅ P2 FEEDBACK 6 — Split client change into substeps 2a/2b/2c

**Problem:** Plan lumped schema + upsert + hybrid search into one step.

**Resolution:** Split into 2a (named-vector schema), 2b (sparse slot + upsert), 2c (hybrid query path). Each left the suite green.

---

### ✅ P2 FEEDBACK 7 — Explicit PrefetchQuery request-shape test

**Problem:** No specifics on what the hybrid mock test should assert.

**Resolution:** Tests record outgoing `pb.QueryPoints` and assert: two prefetches, correct `Using`, correct limits, filters on prefetches, RRF with `K=60`.

---

### ✅ P3 FEEDBACK 8 — Document CRC32 collision rate

**Problem:** CRC32 with ~1M tokens has ~120 expected collisions. Silent.

**Resolution:** Documented in code at `sparse.go:164-166`. Collisions are merged via `+=` at `sparse.go:248`.

---

### ✅ P3 FEEDBACK 9 — Dense-mode regression check

**Problem:** Exit criteria only checked hybrid > dense, not that dense itself wasn't broken.

**Resolution:** Added exit criterion: "Dense-mode `hit@5` after Phase 2 must match Phase 1 `baseline_v1.json` within 1pp." Result: +0.0pp (0.789 → 0.789). ✅

---

### ✅ P3 FEEDBACK 10 — Shared tokenizer invariant

**Problem:** Index-time and query-time tokenizers must use the same canonical function.

**Resolution:** `Tokenize()` is the single source of truth (`sparse.go:52-64`). Both `BuildSparseVectors` and `TokenizeQuery` call it internally. Test coverage includes same-input-same-output parity.

---

## Code Review: Steps 1 + 2a

Review of the initial implementation diff (sparse tokenizer + named-vector schema migration).

### Diff details

- **Determinism fix (sparse.go):** Uses intermediate `entries` slice with `slices.SortFunc`. Local-scoped `entry` struct keeps the public surface clean.
- **Upsert test (client_test.go):** The accessor chain `point.GetVectors().GetVectors().GetVectors()` is correct protobuf nested-oneof traversal.
- **Migration error formatting:** Multi-line with indented commands for easy paste.

### Cosmetic nits (all resolved)

| Nit | Status |
|---|---|
| `sort.Slice` → `slices.SortFunc` for consistency with Go 1.26 | ✅ Fixed — `sparse.go:260` uses `slices.SortFunc` |
| `TokenizeQuery` not sorted by index | ✅ Documented — comment at `sparse.go:286-287` explains first-appearance order is sufficient |
| No determinism regression test | Accepted — sort is deterministic by Go semantics; not worth the test |

**Verdict:** Steps 1 + 2a clean and reviewable. Named-vector foundation solid for 2b.

---

## Code Review: Tasks 1–5

Final review of all five implementation tasks before eval (Task 6).

### Accepted non-actions

- **IDF smoothing:** `idf = log(N / df)` produces zero when a token appears in every document. Sparse builder filters non-positive weights. Standard behavior, keep unless eval shows issues.
- **Renaming `Timings.Embed`:** Treat as "pre-Qdrant query prep" for now. Renaming would churn report output.

### Findings

#### ✅ P1: Existing named dense-only collections accepted as valid

**Problem:** `validateCollectionDimension` only checked for `"dense"` vector. Collections created after named-dense step but before sparse-slot support would pass validation, then fail at sparse upsert.

**Fix:** Added sparse slot validation at `client.go:159-171`. Returns clear delete/re-index error if `"sparse"` slot is missing.

---

#### ✅ P2: Language-scoped IDF inconsistency

**Problem:** `--language go` re-index computes IDF only over Go chunks. Other languages retain old weights.

**Resolution:** Documented as explicit design choice. Inline caveat at `pipeline.go:210-219` and plan §2 line 122. Mitigations: separate collections per language, or force full re-index.

---

#### ✅ P2: `--limit` accepts negative values

**Problem:** Negative `--limit` cast to `uint64` becomes a very large value.

**Fix:** Added `*limit <= 0` validation at `main.go:85-86` (search) and `main.go:161-162` (eval).

---

#### ✅ P3: CRC32 collisions not merged during sparse construction

**Problem:** Plan stated collisions merge, but code didn't combine duplicate hashed indices.

**Fix:** `sparse.go:248` — `hashWeights[tokenHash(tok)] += float32(weight)` merges via `+=` on the map key. Comment at `sparse.go:229-233` explains the structural guarantee.

---

### Task-by-task status (all implemented)

| Task | Status | Notes |
|---|---|---|
| 1. Sparse Tokenizer (TF-IDF) | ✅ | `Tokenize` is shared. Tests cover camelCase, snake_case, mixed, numbers, stop words, empty input, deterministic output, index/query parity |
| 2. Qdrant Client (named vectors + sparse) | ✅ | Named dense + sparse slot. Unified Search. Hybrid RRF with prefetch filters. Sparse slot validation on existing collections |
| 3. Pipeline (global IDF + sparse gen) | ✅ | Enriched texts built once. IDF computed once per run. Dense and sparse from same texts. Language-scoped IDF documented |
| 4. Search Layer (mode switch) | ✅ | Dense embeds + validates. Sparse tokenizes query. Hybrid builds both. Default: hybrid. Invalid mode fails early |
| 5. CLI (`--mode` flag) | ✅ | `search --mode`, `eval --mode`, default hybrid. Limit validation added |

### Integration verification

All unit tests pass with `-race`. Live Qdrant smoke test confirms:
- Named dense + sparse collection schema accepted
- Sparse vector upsert works
- Hybrid RRF query works with filters on prefetch stages
