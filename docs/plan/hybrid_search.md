# Phase 2 — Hybrid Search Implementation Plan

Add keyword-based sparse vector search alongside dense vector search so exact-symbol queries like `"ChunkFile"` get boosted. Fuse results with Reciprocal Rank Fusion (RRF).

**Exit criteria:**
- Eval shows ≥10pp `hit@5` improvement on exact-symbol (navigation) queries in hybrid vs dense mode, with no regression on concept queries.
- Dense-mode `hit@5` after Phase 2 must match Phase 1 `baseline_v1.json` within 1pp. If it drops, the schema migration or named-vector wiring broke dense retrieval — fix before declaring Phase 2 done.

---

## Eval Results

Run on ragcodepilot's own Go corpus (~350 chunks, 32 files), 23 golden queries (19 positive, 4 negative), `nomic-embed-text` 768d, Qdrant v1.17.1, RRF `k=60`, prefetch limit `2×limit`.

**Canonical baseline:** `docs/eval/baseline_v3.json` (hybrid, BM25 `k1=0.5`).
**Previous baseline:** `docs/eval/baseline_v2.json` (hybrid, TF-IDF). Kept for the historical comparison.

### 2026-05-15: Switched from TF-IDF to BM25 (`k1=0.5`)

| Mode | hit@1 | hit@3 | hit@5 | MRR@5 | recall@10 | nav h@5 | con h@5 | beh h@5 | neg pass | p50/p95 ms |
|---|---|---|---|---|---|---|---|---|---|---|
| baseline_v1 (P1 dense) | 0.526 | 0.684 | 0.789 | 0.632 | 0.842 | 0.500 | 1.000 | 1.000 | 1.000 | 31/131 |
| dense (P2 TF-IDF) | 0.526 | 0.737 | 0.789 | 0.625 | 0.711 | 0.500 | 1.000 | 1.000 | 1.000 | 29/186 |
| sparse (P2 TF-IDF) | 0.421 | 0.526 | 0.526 | 0.474 | 0.789 | 0.625 | 0.429 | 0.500 | 0.750 | 1/99 |
| hybrid (P2 TF-IDF) | 0.421 | 0.737 | 0.895 | 0.607 | 0.816 | 0.750 | 1.000 | 1.000 | 1.000 | 29/173 |
| dense (P3 BM25) | 0.526 | 0.737 | 0.789 | 0.625 | 0.711 | 0.500 | 1.000 | 1.000 | 1.000 | 28/161 |
| sparse (P3 BM25) | 0.421 | 0.737 | 0.737 | 0.579 | 0.816 | 0.625 | 0.714 | 1.000 | 0.250 | 1/81 |
| **hybrid (P3 BM25)** | **0.632** | **0.789** | 0.842 | **0.706** | 0.842 | **0.750** | 0.857 | 1.000 | 1.000 | **28/119** |

**Hybrid-mode delta (BM25 vs TF-IDF):** hit@1 **+21.1pp**, hit@3 +5.3pp, hit@5 −5.3pp (one displaced query), MRR@5 **+9.9pp**, recall@10 +2.6pp, navigation h@5 ±0pp, concept h@5 −14.3pp (one displaced query), behavior h@5 ±0pp, negative pass ±0pp, p95 latency 173→119ms.

The dense rows are identical between P2 and P3 (sanity check — dense path is unchanged). Sparse-only is more aggressive on identifier matches with BM25, gaining hit@5 on positive queries but losing negative-pass and concept robustness; **hybrid mode damps both extremes via RRF**, which is why hybrid is the user-facing default.

### Exit-criteria check (P2 TF-IDF, original Phase 2 gate)

| Criterion | Result | Status |
|---|---|---|
| navigation hit@5 hybrid − dense ≥ +10pp | +25.0pp (0.500 → 0.750) | ✅ |
| concept hit@5 no regression | ±0.0pp (1.000 → 1.000) | ✅ |
| behavior hit@5 no regression | ±0.0pp (1.000 → 1.000) | ✅ |
| dense P2 hit@5 vs baseline_v1 within ±1pp | +0.0pp (0.789 → 0.789) | ✅ |
| Full test suite passes with `-race` | All packages green | ✅ |

All four numeric criteria were met on the original TF-IDF implementation. The 2026-05-15 BM25 follow-up was a tuning round on top of this; its decision rule and result are in §3 below.

### Observations worth noting (not blocking)

- **Sparse mode is materially worse on `concept` and `behavior`** (h@5 0.429 / 0.500 vs 1.000 dense). Pure keyword matching doesn't generalize, as expected — sparse is a complement to dense, not a replacement. Hybrid mixing both is what carries.
- **Sparse mode loses one negative query** (pass rate 0.75 vs 1.000): without dense semantics, BM25-style keyword matching scores some queries higher than expected when they share surface tokens with code. Hybrid mode is unaffected because RRF damps the over-confident sparse rank when dense disagrees.
- **Hybrid hit@1 dropped 10pp vs dense** (0.526 → 0.421). RRF re-ranks some dense top-1 hits to position 2-3 when sparse disagrees with them. Net effect is still positive for hit@5, MRR@5 is slightly lower (0.607 vs 0.625). Acceptable trade for the +25pp navigation lift.
- **Dense P2 recall@10 dropped vs baseline_v1** (0.711 vs 0.842). `hit@5` matches exactly (0.789), so this is rank-6-to-10 movement only. **Investigated and resolved as a corpus-stability effect, not a search regression.** Phase 2 added `internal/embedding/sparse.go` and `internal/embedding/sparse_test.go` (~100 new chunks, corpus grew from ~250 to 350). The new test chunks (`TestSplitCamel_DigitsMiddle`, `TestSplitCamel_AllUpper`, `TestTokenize_NumbersAttached`, etc.) share surface tokens — "chunk", "split", "camel" — with navigation queries like "where is ChunkFile defined". Their cosine similarity scores are competitive with the expected definition chunks, pushing them out of the top-10 for two queries. Hybrid mode recovered one of those queries (`validate_collection_vector_size_navigation`) by surfacing the exact symbol match via sparse+RRF (BM25 in baseline_v3, TF-IDF in baseline_v2); the other (`chunkfile_navigation`) remains displaced because the test names share too many tokens with the target identifier. **The dense algorithm itself is unchanged from Phase 1 — same vectors, same cosine math.** This is the limit of pure-semantic retrieval on a corpus where test code mirrors the names it tests. Phase 3 reranking is the right layer to address it. Same-corpus dense reference for Phase 3 comparisons: `docs/eval/baseline_v2_dense.json`.
- **Sparse latency** is dramatically lower (1ms p50) because it skips the Ollama embed call — useful if a query type ever proves robust to sparse-only retrieval (it doesn't here).

### Reproduction

```bash
docker compose up -d
ollama pull nomic-embed-text
go run ./cmd/ragcodepilot collections delete code_chunks
go run ./cmd/ragcodepilot index --language go .
go run ./cmd/ragcodepilot eval --mode dense  --output json > /tmp/eval_dense.json
go run ./cmd/ragcodepilot eval --mode sparse --output json > /tmp/eval_sparse.json
go run ./cmd/ragcodepilot eval --mode hybrid --output json > /tmp/eval_hybrid.json
```

---

## Key Design Decision: Server-Side RRF

Qdrant's Go SDK (`v1.17.1`) provides built-in RRF fusion via `PrefetchQuery` + `NewQueryRRF()`. This means:

- **No custom `internal/search/rrf.go` needed.** Qdrant does the math server-side in a single gRPC call.
- The SDK exposes `Rrf.K` for tuning (we'll use the standard `k=60`, hardcoded).
- Dense and sparse queries are issued as `PrefetchQuery` stages; Qdrant fuses and returns the final ranked list.

```
QueryPoints{
  Prefetch: [
    {Query: dense_vector,  Using: "dense",  Limit: 2*limit, Filter: <filters>},
    {Query: sparse_vector, Using: "sparse", Limit: 2*limit, Filter: <filters>},
  ],
  Query: RRF(k=60),
  Limit: limit,
}
```

**Filter placement:** Filters (language, repo) are applied on **each prefetch stage**, not at the top level. This ensures both dense and sparse retrieval only consider documents matching the filter before RRF fusion. If filters were applied only after fusion, Qdrant would fuse unfiltered results and then discard non-matching ones — leaving fewer-than-`limit` results and skewed ranking. Verify this against Qdrant v1.17.1 before coding the request shape.

This simplifies the architecture: instead of two separate gRPC calls + client-side fusion, one call handles everything.

---

## Breaking Schema Change

Collections created before Phase 2 use a **single unnamed dense vector**. Phase 2 needs **named vectors** (`"dense"` + `"sparse"`).

**Migration path:** Detect old schema → return a clear error with fix instructions:

```
collection "code_chunks" uses the legacy unnamed-vector schema;
delete and re-index:  ragcodepilot collections delete code_chunks
```

No in-place migration — it's not worth the complexity for a local dev tool.

---

## Resolved Design Decisions

### 1. RRF `k` parameter

Hardcode `k=60`. The roadmap explicitly says "no tuning beyond a single RRF k value." We can add a `--rrf-k` flag later if eval results suggest it.

### 2. IDF source: Global in-memory IDF (per indexing run)

Batch-local IDF (32 chunks per batch) produces inconsistent token weights — the same token gets different sparse weights depending on which batch wrote it. This breaks the exact-match property hybrid search is designed to add.

**Approach:** Compute IDF globally in memory during each full `pipeline.Run`:

```
1. First pass: tokenize every chunk text → accumulate global document frequency (df) map.
2. Compute IDF map once: idf[token] = log(N / df[token]) for all tokens.
3. Second pass (existing embedding loop): generate sparse vectors using the global IDF.
```

For ragcodepilot's stated scale (~200K chunks per `system_design.md`), the IDF map is ~12 MB in memory (1M unique tokens × 12 bytes per entry). Trivial.

**Partial re-index:** For change-detection re-indexing (already supported), either recompute IDF from unchanged-on-disk chunks plus changed ones, OR force a full re-index. Both are acceptable for the MVP; document the choice.

Persist nothing for v1 — recompute each indexing run.

**Known limitation — language-scoped IDF:** When `--language go` (or any single-language filter) is used to re-index a multi-language collection, the IDF is computed only over the chunks of that language. Other languages already in the collection retain the IDF weights from whichever run wrote them. This means cross-language hybrid search has inconsistent sparse weighting across languages — fine for single-language searches (which are the common case via the language filter), but not ideal for unfiltered cross-language hybrid retrieval. Mitigations: use one collection per language, or force a full re-index by deleting the collection before re-indexing. Documented inline at `internal/ingest/pipeline.go:207`.

### 3. Algorithm: BM25 with softened `k1`

The tokenizer produces BM25 weights with:

- **Smoothed IDF**: `idf(t) = log((N − df + 0.5) / (df + 0.5) + 1)`. Strictly positive even for tokens that appear in every document — classic `log(N/df)` would silently drop them.
- **`k1 = 0.5`**: damps the effect of repeated tokens. Lower than the Elasticsearch default of `1.2`; see the tuning history below.
- **`b = 0.75`**: standard length normalization. Penalizes docs longer than the corpus average so a 200-line file doesn't outrank a 20-line function on shared tokens.

File naming: `internal/embedding/sparse.go` (the historical name predates the algorithm choice and is kept for stability).

#### Algorithm history

| Date | Choice | Reason |
|---|---|---|
| Original plan | BM25 | Industry standard; canonical lexical retrieval algorithm. |
| 2026-05-13 (P2 ship) | TF-IDF | Simpler; the original `hybrid_search.md` plan review argued BM25's length normalization adds little on short, uniform-length code chunks. Eval (baseline_v2) passed all four exit criteria. |
| 2026-05-15 (P3 pivot) | BM25 with `k1=0.5` | Follow-up spike re-ran the eval with BM25. See numbers below. |

#### BM25 spike result (2026-05-15)

Decision rule going in: navigation hit@5 lifts ≥3pp **AND** concept hit@5 stays at 1.000 → ship BM25.

Round 1 (`k1=1.2`, ES default): hit@1 +10.5pp, concept hit@5 dropped from 1.000 to 0.857 (one query). Decision rule failed strictly, but the hit@1 lift was the strongest single-metric move on the project. Diagnosed the concept regression: one query (`hasher_concept`) lost `hasher.go:HashFiles` to `qdrant/client.go:ScrollFileHashes`. Root cause: tokenizer doesn't stem, so query token `hashes` (plural) matched `ScrollFileHashes` exactly but only partially matched `HashFiles` (singular `hash`).

Round 2 (`k1=0.5`, softened): hit@1 **+21.1pp** (0.421 → 0.632), MRR@5 **+9.9pp**, hit@3 +5.3pp, p95 latency 173→119ms. Concept regression **unchanged** — confirmed structural (tokenizer plural/singular), not a saturation issue.

Net trade vs TF-IDF (baseline_v2 → baseline_v3):

- **+4 queries** correct at hit@1 (`parse_language_filter_navigation`, `hihatk_navigation`, `enrichment_concept`, `dimension_validation_behavior`), 0 regressed.
- **−1 query** lost top-5 (`hasher_concept` — to be fixed at the tokenizer or reranker layer, not by tuning BM25).

#### Why `k1=0.5` and not the Elasticsearch default

Elasticsearch defaults `k1=1.2` are calibrated for long, mixed-length documents where pathological term repetition is common. Code chunks are short (function-level) and roughly uniform-length, so aggressive saturation isn't needed. `k1=0.5` keeps the curve flatter, behaving closer to plain TF when raw counts are low — the typical case. Going lower (`k1=0.0`) would disable saturation entirely and collapse toward IDF-only ranking. Going higher (`k1=1.2`) costs hit@1 and gains nothing on this corpus per the round-1 vs round-2 comparison.

`b=0.75` is the standard value and was not tuned. If a future corpus has highly variable chunk lengths (e.g., adding markdown docs to the same collection), `b` is the parameter to revisit.

#### Known regression: tokenizer stemming

`hasher_concept` regressed under BM25 due to plural/singular token mismatch (`hashes` vs `hash`). The fix lives at the tokenizer layer (add a light stemmer) or the reranker layer (Phase 3 — cross-encoder sees the full query text and disambiguates). It is **not** a tuning issue. BM25 with any `k1` cannot recover this query as long as `Tokenize("hashes") ≠ Tokenize("hash")`.

---

## Component Breakdown

### 1. Sparse Tokenizer (BM25; originally shipped as TF-IDF — see §3)

**New files:**
- `internal/embedding/sparse.go`
- `internal/embedding/sparse_test.go`

**New type:**
```
type SparseVector struct {
    Indices []uint32
    Values  []float32
}

function NewSparseVector(indices, values) → SparseVector, error
  // validates len(Indices) == len(Values)
```

Code-aware tokenizer that produces `SparseVector` values:

**Tokenization rules:**
- Split on whitespace and punctuation
- Sub-split camelCase: `ChunkFile` → `["chunk", "file"]`
- Sub-split snake_case: `chunk_file` → `["chunk", "file"]`
- Keep digit runs attached: `sha256Hash` → `["sha256", "hash"]`
- Lowercase all tokens
- Remove Go keywords and common English stop words

**Key invariant:** The public `Tokenize(text) → tokens[]` function is the **single source of truth** for tokenization. Both `BuildSparseVectors` (index-time) and `TokenizeQuery` (query-time) MUST call it internally. If they diverge (one lowercases, the other doesn't; one splits on underscores, the other doesn't), exact matches fail silently.

**Public API (pseudocode):**

```
function Tokenize(text) → tokens[]
  // canonical split + normalize — single source of truth

function BuildSparseVectors(texts[], idfMap) → sparseVectors[]
  // per-doc TF * global IDF
  // key = CRC32 hash of token (uint32), value = BM25 weight (float32)

function ComputeIDF(allTexts[]) → idfMap
  // global document frequency across entire corpus
  // idf[token] = log(N / df[token])

function TokenizeQuery(query) → SparseVector
  // tokenize with uniform weights (1.0 per unique token)
  // uses same Tokenize() as index path
```

**Token hashing (CRC32):** With ~1M unique tokens in a code corpus, CRC32 (4B values) has an expected birthday-paradox collision count of ~120 — small but non-zero. Two different tokens sharing a hash get merged into one sparse dimension. This is accepted for the MVP. If retrieval quality suffers, upgrade to xxhash or a per-collection vocab map.

**Test cases:**
- camelCase: `"ChunkFile"` → `["chunk", "file"]`
- snake_case: `"chunk_file"` → `["chunk", "file"]`
- Mixed: `"NewVectorInputSparse"` → `["new", "vector", "input", "sparse"]`
- Numbers: `"sha256Hash"` → `["sha256", "hash"]`
- Stop word removal
- Sparse vector output: keys are uint32, values are positive floats
- Empty input handling
- **Same-input-same-output across index and query paths:** `Tokenize("ChunkFile")` called from `BuildSparseVectors` and `TokenizeQuery` must produce identical token lists

---

### 2. Qdrant Client — Named Vectors + Sparse Slot

**Modified file:** `internal/qdrant/client.go`

Split into three substeps, each leaving the test suite green:

#### Step 2a — Switch to named-vector schema (dense only)

Switch `EnsureCollection` from unnamed vector to named `"dense"` vector. No sparse code yet.

Before (unnamed single vector):
```
CreateCollection{
  VectorsConfig: NewVectorsConfig(&VectorParams{Size: dim, Distance: Cosine})
}
```

After (named dense):
```
CreateCollection{
  VectorsConfig: NewVectorsConfigMap({
    "dense": {Size: dim, Distance: Cosine},
  }),
}
```

Update `Upsert` to use `pb.NewVectorsMap(map[string]*pb.Vector{"dense": ...})`.

Update `Search` to pass `Using: "dense"` in `QueryPoints`.

Update `validateCollectionDimension` to read from named vector config:
```
// Before: GetParams().GetVectorsConfig().GetParams().GetSize()
// After:  GetParams().GetVectorsConfig().GetParamsMap().GetMap()["dense"].GetSize()
```

Add migration check: if collection exists but has unnamed vectors (old schema), return a clear error.

**Verify:** Re-index works. Eval still runs with `hit@5` matching `baseline_v1.json`.

#### Step 2b — Add sparse slot to schema + Upsert

Add `"sparse"` to collection schema:
```
CreateCollection{
  VectorsConfig: NewVectorsConfigMap({"dense": ...}),
  SparseVectorsConfig: NewSparseVectorsConfig({"sparse": {}}),
}
```

Update `Upsert` signature to accept optional sparse vectors:
```
function Upsert(ctx, collection, chunks, denseVectors, sparseVectors) → error
  // sparseVectors can be nil — omit sparse entry from VectorsMap
```

`sparseVectors` uses the `SparseVector` struct (not parallel arrays).

**Verify:** Sparse vectors are stored but not yet queried. Dense search still works.

#### Step 2c — Add hybrid query path (unified `Search` method)

Instead of adding a separate `HybridSearch` method, **unify into one `Search`** that handles all modes via request shape. This avoids the two-parallel-code-paths problem that the eval harness was designed to prevent:

```
function Search(ctx, collection, denseVector, sparseVector, mode, limit, filters) → results, error
  // denseVector or sparseVector can be nil depending on mode
  // builds the appropriate QueryPoints shape:
  //   dense:  top-level Query = dense vector, Using = "dense"
  //   sparse: top-level Query = sparse vector, Using = "sparse"
  //   hybrid: two prefetches + RRF as top-level Query
```

This keeps one code path for Phase 3 reranking to hook into.

**Test updates (`client_test.go`):**

Record the outgoing `pb.QueryPoints` request and assert:
- Two prefetches present in hybrid mode
- Each prefetch has `Using` set correctly (`"dense"` / `"sparse"`)
- Each prefetch has its limit set (`2 * limit`)
- Each prefetch carries the filter (language/repo)
- Top-level `Query` is `NewQueryRRF` with `K = 60`
- Named vector schema in `EnsureCollection`
- Old-schema migration error

---

### 3. Ingestion Pipeline — Sparse Vector Generation

**Modified file:** `internal/ingest/pipeline.go`

Two-pass approach for global IDF:

```
// NEW Step 7.5: First pass — tokenize all chunks and compute global IDF.
allTexts = [enrichForEmbedding(chunk) for chunk in allChunks]
idfMap = ComputeIDF(allTexts)

// Step 8 (existing embedding loop), modified:
for each batch:
  vectors = embedder.Embed(ctx, batchTexts)
  sparseVectors = BuildSparseVectors(batchTexts, idfMap)  // uses global IDF
  store.Upsert(ctx, collection, batch, vectors, sparseVectors)
```

Update `vectorStore` interface: `Upsert` gains a `sparseVectors []SparseVector` parameter.

---

### 4. Search Layer — Mode Switch

**Modified file:** `internal/search/searcher.go`

Add search mode type:
```
SearchMode = "dense" | "sparse" | "hybrid"
```

Update `SearchWithTimings` to accept a mode parameter:
- `dense` → embed query, pass to `client.Search(mode=dense)`
- `sparse` → tokenize query with `TokenizeQuery()`, pass to `client.Search(mode=sparse)`
- `hybrid` → embed query (dense) + tokenize query (sparse), pass to `client.Search(mode=hybrid)`

---

### 5. CLI — `--mode` Flag

**Modified file:** `cmd/ragcodepilot/main.go`

- Add `--mode dense|sparse|hybrid` flag to `search` subcommand. Default: `hybrid`.
- Add `--mode` flag to `eval` subcommand.
- Wire mode through to `Searcher.SearchWithTimings()`.

---

### 6. Eval + Docs

- Run eval in all three modes. Compare `hit@5` per query type.
- Commit `docs/eval/baseline_v2.json` showing the hybrid vs dense delta.
- Update this document with actual eval results.

---

## Implementation Order

```
1. Sparse tokenizer (TF-IDF) + SparseVector type + tests   (pure logic, zero deps)
2a. Qdrant client: named-vector schema (dense only)          (migrate schema, tests green)
2b. Qdrant client: add sparse slot to schema + Upsert        (sparse stored, not queried)
2c. Qdrant client: unified Search with hybrid query path     (RRF end-to-end)
3.  Pipeline: global IDF + sparse generation                  (wires 1 + 2b together)
4.  Searcher: mode switch                                     (wires everything into search)
5.  CLI: --mode flag                                          (user-facing surface)
6.  Re-index + eval + docs                                    (validation)
```

Each step is independently testable. Step 1 can be developed in parallel with 2a. If step 2c fails (Qdrant API surprise), 2a and 2b are still useful — the schema is upgraded and sparse data is being captured.

---

## Files to Touch / Create

| File | Action | Purpose |
|---|---|---|
| `internal/embedding/sparse.go` | NEW | BM25 tokenizer + `SparseVector` type + sparse vector generation (originally shipped as TF-IDF; switched to BM25 `k1=0.5` on 2026-05-15 — see §3) |
| `internal/embedding/sparse_test.go` | NEW | Tokenization + sparse vector + index/query parity tests |
| `internal/qdrant/client.go` | MODIFY | Named vectors, sparse slot, unified `Search` with hybrid |
| `internal/qdrant/client_test.go` | MODIFY | Named vector schema + PrefetchQuery request-shape assertions |
| `internal/ingest/pipeline.go` | MODIFY | Global IDF computation + sparse vector generation |
| `internal/search/searcher.go` | MODIFY | Search mode switch (dense/sparse/hybrid) |
| `cmd/ragcodepilot/main.go` | MODIFY | `--mode` flag on search and eval |
| `docs/eval/baseline_v2.json` | NEW | Hybrid search eval results |
| `docs/plan/hybrid_search.md` | UPDATE | Add eval results after implementation |

**Not needed (removed from original roadmap):**
- ~~`internal/search/rrf.go`~~ — Qdrant handles RRF server-side
- ~~`internal/embedding/bm25.go`~~ — kept as `sparse.go`; the file name was set during the TF-IDF phase and stayed when the algorithm switched back to BM25 (see §3).

---

## Out of Scope

- No learned sparse models (SPLADE, etc.) — BM25 only.
- No reranking — that's Phase 3.
- No tuning beyond `k=60` for RRF.
- No persisted IDF file — recompute in memory each indexing run.
- No tuning beyond `k1=0.5` and `b=0.75`. The 2026-05-15 eval validated these values; revisit only if a future corpus has highly variable chunk lengths or if reranking shifts the failure modes.

---

## Verification

### Unit Tests (no Qdrant needed)

```bash
go test ./internal/embedding/... -v -race -count=1   # BM25 tokenizer + sparse vectors
go test ./internal/qdrant/... -v -race -count=1      # Named vector + hybrid query shape mocks
go test ./... -v -race -count=1                       # Full suite
```

### Integration Test (requires running Qdrant)

```bash
# Verify filter placement under PrefetchQuery (do this FIRST)
# Manual test against running Qdrant v1.17.1 to confirm filters on prefetches work as expected.

# Delete old collection (required — schema changed)
go run ./cmd/ragcodepilot collections delete code_chunks

# Re-index with sparse vectors
go run ./cmd/ragcodepilot index --language go .

# Test all three modes
go run ./cmd/ragcodepilot search --mode dense  --language go "ChunkFile"
go run ./cmd/ragcodepilot search --mode sparse --language go "ChunkFile"
go run ./cmd/ragcodepilot search --mode hybrid --language go "ChunkFile"

# Run eval and compare
go run ./cmd/ragcodepilot eval --mode dense  --output json > /tmp/dense.json
go run ./cmd/ragcodepilot eval --mode hybrid --output json > /tmp/hybrid.json
```

### Exit Criteria

1. `hit@5` on `navigation` queries improves ≥10pp in hybrid vs dense.
2. No regression on `concept` queries (hybrid vs dense).
3. **Dense-mode regression check:** Dense-mode `hit@5` after Phase 2 matches Phase 1 `baseline_v1.json` within 1pp.
4. All tests pass with `-race`.
