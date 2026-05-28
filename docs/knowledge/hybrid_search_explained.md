# Hybrid Search — What It Is and Why It Matters

## The problem: two kinds of questions

When developers search code, they ask two fundamentally different types of questions:

**Type 1 — Meaning questions:**
> "how does crash recovery work?"
> "what happens when embedding dimensions are wrong?"

These are *concept* questions. The answer could be in a function called `recoverFromWAL()` or `handleDimensionMismatch()` — words that don't appear in the question at all. You need to match by *meaning*.

**Type 2 — Name questions:**
> "where is ChunkFile defined?"
> "find ValidateVectorBatch"

These are *navigation* questions. The answer must contain the exact identifier. Meaning doesn't help — you need to match by *exact words*.

No single search method handles both well. That's the problem hybrid search solves.

---

## Three search methods compared

### 1. Dense (semantic) search

```
Query: "how does crash recovery work?"
                │
                ▼
        Embedding Model
                │
                ▼
   Query vector: [0.81, -0.30, 0.43, ...]
                │
                ▼
   Qdrant: find nearest vectors (cosine similarity)
                │
                ▼
   Results ranked by MEANING similarity
```

**How it works:** Convert the query into a dense vector (768 numbers) using a neural network. Find stored vectors that are closest in the vector space. "Closest" means most similar in meaning.

**Strengths:**
- Understands synonyms: "crash recovery" matches `recoverFromWAL()`
- Works across languages: a Go query can find Rust code with the same concept
- Handles natural language: "what happens when the file is too large" works

**Weaknesses:**
- Loses exact identifiers: "ChunkFile" becomes a vague cloud of "chunk" + "file" concepts
- Can't distinguish `ChunkFile` from `FileChunk` or `splitFileIntoChunks`
- Short identifier queries ("ValidateVectorBatch") get poor results

**Best for:** Concept questions, behavior questions, understanding code you haven't seen before.

---

### 2. Sparse (keyword) search

```
Query: "ChunkFile"
         │
         ▼
   Code-aware tokenizer
         │
         ▼
   Tokens: ["chunk", "file"]
   Weights: [1.0,    1.0  ]
         │
         ▼
   Qdrant: match tokens by BM25 score
         │
         ▼
   Results ranked by KEYWORD overlap
```

**How it works:** Split the query into tokens (splitting camelCase, snake_case, removing stop words). Score each document by how many query tokens it contains, weighted by how rare each token is (BM25 algorithm). No neural network involved — pure text matching.

**Strengths:**
- Exact identifier matches: "ChunkFile" finds exactly `ChunkFile`
- Fast: no embedding model needed (1ms vs 30ms)
- Predictable: you can reason about why a result matched
- Handles code-specific patterns: splits `ValidateVectorBatch` → `["validate", "vector", "batch"]`

**Weaknesses:**
- No understanding of meaning: "crash recovery" won't find `recoverFromWAL()`
- Sensitive to word choice: "hashes" won't match "HashFile" (without stemming)
- Long natural-language queries produce noisy results (too many common tokens)

**Best for:** Navigation questions, exact symbol lookup, "go to definition" style queries.

---

### 3. Hybrid search (dense + sparse + fusion)

```
Query: "where is ChunkFile defined?"
         │
         ├──────────────────────────────────┐
         ▼                                  ▼
  Embedding Model                    Code-aware tokenizer
         │                                  │
         ▼                                  ▼
  Dense vector                        Sparse vector
  [0.72, -0.15, ...]                 {chunk: 1.0, file: 1.0}
         │                                  │
         ▼                                  ▼
  Qdrant: nearest vectors            Qdrant: BM25 match
  (top 50 by meaning)                (top 50 by keywords)
         │                                  │
         └──────────┬───────────────────────┘
                    ▼
         Reciprocal Rank Fusion (RRF)
                    │
                    ▼
         Combined ranking (top 10)
         best of BOTH methods
```

**How it works:** Run *both* dense and sparse search simultaneously. Each produces its own ranked list. Then combine them using Reciprocal Rank Fusion (RRF):

```
For each result, its fused score =  1/(k + rank_dense)  +  1/(k + rank_sparse)

where k = 60 (dampening constant)
```

A chunk ranked #1 in dense and #50 in sparse:
```
score = 1/(60+1) + 1/(60+50) = 0.0164 + 0.0091 = 0.0255
```

A chunk ranked #3 in both:
```
score = 1/(60+3) + 1/(60+3) = 0.0159 + 0.0159 = 0.0317  ← wins!
```

Results that rank well in *both* methods get boosted. Results that rank well in *only one* method still appear, just lower.

**Strengths:**
- Handles both meaning and exact keywords
- No single failure mode — dense covers what sparse misses and vice versa
- RRF is parameter-free (only `k`, which rarely needs tuning)
- Negative queries are safer: dense and sparse must *both* agree for high scores

**Weaknesses:**
- Slower: two searches + fusion (but Qdrant does it server-side in one gRPC call)
- hit@1 can drop: RRF may push a dense #1 to position #2-3 when sparse disagrees
- Requires maintaining two vector types during ingestion

**Best for:** Everything. It's the default search mode for a reason.

---

## Side-by-side example

Query: `"where is ChunkFile defined?"`

| Rank | Dense (semantic) only | Sparse (keyword) only | Hybrid (fused) |
|------|----------------------|----------------------|----------------|
| #1 | `chunker.go:ChunkFile` ✅ | `chunker.go:ChunkFile` ✅ | `chunker.go:ChunkFile` ✅ |
| #2 | `chunker.go:chunkGeneric` | `chunker_go.go` (has "chunk") | `chunker_go.go` |
| #3 | `pipeline.go` (calls ChunkFile) | `pipeline.go` (has "chunk", "file") | `pipeline.go` |

Both methods get #1 right here. But consider this query:

Query: `"how does change detection prevent redundant re-embedding?"`

| Rank | Dense only | Sparse only | Hybrid |
|------|-----------|------------|--------|
| #1 | `hasher.go:HashFiles` ✅ | `pipeline.go` (has "change", "re") | `hasher.go:HashFiles` ✅ |
| #2 | `pipeline.go` | `enrichment.go` (has "embedding") | `pipeline.go` |
| #3 | `enrichment.go` | `sparse.go` (has "embedding") | `enrichment.go` |

Dense finds the right answer by meaning. Sparse is confused because the query's words overlap with many unrelated files. Hybrid trusts dense's ranking here because sparse doesn't have a strong counter-signal.

Now this query:

Query: `"ValidateVectorBatch"`

| Rank | Dense only | Sparse only | Hybrid |
|------|-----------|------------|--------|
| #1 | `ollama.go` (embedding related) | `validate.go:ValidateVectorBatch` ✅ | `validate.go:ValidateVectorBatch` ✅ |
| #2 | `fake.go` (embedding related) | `ollama.go` (has "vector") | `ollama.go` |
| #3 | `validate.go` ✅ (ranked 3rd!) | `client.go` (has "vector", "batch") | `fake.go` |

Dense puts the exact match at #3 — it sees "validate vectors in batches" as a concept and finds related code first. Sparse nails it at #1 because the exact tokens match. Hybrid lifts the sparse winner to #1.

---

## How RRF fusion works (visual)

```
Dense results:          Sparse results:         After RRF:
                                                
#1  chunk_A  ─────────── not in top 50          chunk_A (dense #1)
#2  chunk_B  ─────────── #8 chunk_B             chunk_C (dense #3 + sparse #1)
#3  chunk_C  ─────────── #1 chunk_C  ←──────── chunk_B (dense #2 + sparse #8)
#4  chunk_D  ─────────── not in top 50          chunk_E (sparse #2)
#5  chunk_E  ─────────── #2 chunk_E             chunk_D (dense #4)
```

chunk_C was #3 in dense and #1 in sparse → **rises to #2 in hybrid** because it's good in both.
chunk_A was #1 in dense but absent in sparse → **stays #1** but only because its dense score was dominant.
chunk_E was #5 in dense and #2 in sparse → **rises to #4** — boosted by sparse agreement.

---

## When to use which mode

```
ragcodepilot search "how does chunking work?"              # hybrid (default)
ragcodepilot search --mode dense  "explain the RAG flow"   # concept-heavy → dense
ragcodepilot search --mode sparse "ValidateVectorBatch"    # exact name → sparse
ragcodepilot search --mode hybrid "ChunkFile"              # both → hybrid
```

| Question type | Best mode | Why |
|---|---|---|
| Concept / behavior | Hybrid (dense dominates) | Meaning matters most; sparse adds insurance |
| Exact identifier | Hybrid (sparse dominates) | Keyword match is primary; dense provides context |
| Natural language + identifiers | Hybrid | Both signals contribute equally |
| Out-of-scope / negative | Hybrid | Both methods must agree → fewer false positives |

**In practice, always use hybrid.** The `--mode` flag exists for debugging and evaluation, not for users to think about.

---

## ragcodepilot's actual numbers

From the eval on ragcodepilot's own codebase (23 queries):

| Mode | hit@1 | hit@5 | MRR@5 | Navigation hit@5 | Concept hit@5 |
|---|---|---|---|---|---|
| Dense only | 0.526 | 0.789 | 0.625 | 0.500 | 1.000 |
| Sparse only | 0.579 | 0.737 | 0.658 | 0.625 | 0.714 |
| **Hybrid** | **0.579** | **0.895** | **0.699** | **0.750** | **1.000** |

Key observations:

- **Navigation queries improved +25pp** (0.500 → 0.750) — hybrid's main value
- **Concept queries stayed at 1.000** — dense already perfect, hybrid didn't hurt
- **Sparse alone is worse on concepts** (0.714) — keyword matching can't understand meaning
- **Hybrid hit@5 is the highest** (0.895) — better than either method alone

This is the pattern you see across RAG systems: **hybrid ≥ max(dense, sparse)** on aggregate metrics. It's the "free lunch" of retrieval — two imperfect methods covering each other's blind spots.
