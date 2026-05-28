# BM25 vs TF-IDF — How Keyword Scoring Works

Both BM25 and TF-IDF answer the same question: **"How relevant is this document to this query?"** They score documents by looking at which query words appear and how important those words are. The difference is in how they handle edge cases — and those edges matter in practice.

---

## Start with the intuition

Imagine searching for `"ChunkFile"` across 350 code chunks. The scoring needs to answer two sub-questions:

1. **How important is the word?** If `"ChunkFile"` appears in only 2 chunks out of 350, it's very specific — high signal. If `"func"` appears in 300 chunks, it's nearly useless for ranking.

2. **How much does this document use the word?** If a chunk mentions `"ChunkFile"` 5 times, it's probably more relevant than one that mentions it once. But is 5 times really 5× better than once?

TF-IDF and BM25 agree on question 1 but disagree on question 2.

---

## TF-IDF: the simple version

### Formula

```
score(query, document) = Σ  tf(t, d)  ×  idf(t)
                        t∈q
```

Where:
- **tf(t, d)** = how many times term `t` appears in document `d`
- **idf(t)** = how rare term `t` is across all documents

### Term Frequency (TF)

Raw count. If `"chunk"` appears 4 times in a document, `tf = 4`.

```
Document A: "chunk chunk chunk chunk file"     → tf("chunk") = 4
Document B: "chunk file path"                  → tf("chunk") = 1
```

TF-IDF says Document A is 4× more relevant for the term `"chunk"`. Is that true? Maybe, but probably not. After the first mention, each repetition adds less information. A function that says `chunk` 10 times isn't 10× more about chunking than one that says it twice — it might just have verbose variable names.

**This is TF-IDF's main weakness: no saturation.**

### Inverse Document Frequency (IDF)

Measures how rare a term is:

```
idf(t) = log(N / df(t))

Where:
  N    = total number of documents
  df(t) = number of documents containing term t
```

Example with 350 chunks:

| Term | df | idf = log(350 / df) |
|------|---:|----:|
| `ChunkFile` | 2 | 5.16 |
| `embedding` | 15 | 3.15 |
| `func` | 300 | 0.15 |
| `the` | 340 | 0.03 |

Rare terms (`ChunkFile`) get high weight. Common terms (`func`, `the`) get near-zero weight. This is the part both algorithms agree on.

### TF-IDF score example

Query: `"ChunkFile embedding"`

| | Document A | Document B |
|---|---|---|
| tf("ChunkFile") | 3 | 1 |
| tf("embedding") | 0 | 2 |
| idf("ChunkFile") | 5.16 | 5.16 |
| idf("embedding") | 3.15 | 3.15 |
| **Score** | 3×5.16 + 0×3.15 = **15.48** | 1×5.16 + 2×3.15 = **11.46** |

Document A wins because it repeats `"ChunkFile"` 3 times, even though Document B matches *both* query terms. That's the saturation problem in action.

---

## BM25: TF-IDF with two fixes

BM25 (Best Matching 25, from the Okapi research project in the 1990s) makes two improvements:

1. **Term frequency saturation** — diminishing returns for repeated terms
2. **Document length normalization** — penalizes long documents that match by volume rather than relevance

### Formula

```
score(query, document) = Σ  idf(t)  ×  tf(t,d) × (k1 + 1)
                        t∈q          ─────────────────────────
                                      tf(t,d) + k1 × (1 - b + b × |d|/avgdl)
```

Where:
- **k1** = term frequency saturation parameter (typically 0.5 – 1.5)
- **b** = length normalization parameter (typically 0.75)
- **|d|** = length of document d (in tokens)
- **avgdl** = average document length across the corpus

### IDF (smoothed)

BM25 uses a slightly different IDF that stays positive even for terms appearing in every document:

```
idf(t) = log( (N - df + 0.5) / (df + 0.5) + 1 )
```

With classic `log(N/df)`, a term appearing in all N documents gets `idf = log(1) = 0` — it completely disappears from scoring. BM25's smoothed version keeps a small positive weight, which is safer.

### Fix 1: Term frequency saturation

The key difference. Look at how TF vs BM25 scores grow as a term repeats:

```
tf:    1     2     3     4     5    10    20    50
────────────────────────────────────────────────────
TF-IDF: 1.00  2.00  3.00  4.00  5.00 10.00 20.00 50.00   (linear, no limit)
BM25:   1.00  1.33  1.50  1.60  1.67  1.82  1.90  1.96   (k1=0.5, approaches 2.0)
BM25:   1.00  1.55  1.83  2.00  2.11  2.35  2.49  2.58   (k1=1.2, approaches 3.4)
```

```
Score
  │
  │                                         ╱ TF-IDF (linear — no ceiling)
  │                                       ╱
  │                                     ╱
  │                                   ╱
  │                                 ╱
  │              ╭──────────────── BM25 k1=1.2 (saturates ~3.4)
  │          ╭───╯
  │      ╭───╯──────────────────── BM25 k1=0.5 (saturates ~2.0)
  │  ╭───╯
  │ ╱
  │╱
  └───────────────────────────────────────── Term frequency
  0    1    2    3    5   10   20   50
```

**The k1 parameter controls how fast the curve flattens:**
- `k1 = 0` → only cares whether the term is present (binary), count doesn't matter
- `k1 = 0.5` → mild saturation, good for short code chunks (ragcodepilot's choice)
- `k1 = 1.2` → Elasticsearch default, moderate saturation for general text
- `k1 = ∞` → no saturation, behaves like raw TF-IDF

**Why k1=0.5 for code search?** Code chunks are short (typically one function, 10-80 lines) and roughly uniform length. A token repeated 3 times in a 20-line function is already strong evidence. The Elasticsearch default of 1.2 is calibrated for long, varied-length documents (web pages, articles) where repetition across paragraphs is more common.

### Fix 2: Document length normalization

Without normalization, longer documents score higher simply because they contain more words. A 200-line utility file mentioning `"hash"` 8 times isn't more relevant than a 20-line `HashFile` function — it's just longer.

The normalization factor:

```
1 - b + b × (|d| / avgdl)
```

- If `|d| = avgdl` (average length) → factor = 1.0 (no effect)
- If `|d| > avgdl` (longer than average) → factor > 1.0 (penalty, score decreases)
- If `|d| < avgdl` (shorter than average) → factor < 1.0 (boost, score increases)

**The b parameter controls how much length matters:**
- `b = 0` → no length normalization (all documents treated equally regardless of length)
- `b = 0.75` → standard value (used by most systems including ragcodepilot)
- `b = 1.0` → full normalization (maximum length penalty)

### BM25 score example (same query, same documents)

Query: `"ChunkFile embedding"`, corpus average length = 40 tokens, k1 = 0.5, b = 0.75

| | Document A (60 tokens) | Document B (30 tokens) |
|---|---|---|
| tf("ChunkFile") | 3 | 1 |
| tf("embedding") | 0 | 2 |
| length norm | 1 - 0.75 + 0.75×(60/40) = **1.375** | 1 - 0.75 + 0.75×(30/40) = **0.8125** |
| BM25 tf("ChunkFile") | 3×1.5 / (3 + 0.5×1.375) = **1.22** | 1×1.5 / (1 + 0.5×0.8125) = **1.06** |
| BM25 tf("embedding") | 0 | 2×1.5 / (2 + 0.5×0.8125) = **1.24** |
| idf("ChunkFile") | 5.16 | 5.16 |
| idf("embedding") | 3.15 | 3.15 |
| **Score** | 1.22×5.16 = **6.30** | 1.06×5.16 + 1.24×3.15 = **9.37** |

**Document B now wins.** BM25 flipped the ranking because:
1. Document A's 3 repetitions of "ChunkFile" only counted ~1.2× (not 3×) due to saturation
2. Document A was penalized for being longer than average
3. Document B matched *both* query terms, which matters more than repeating one term

---

## Side-by-side comparison

| Aspect | TF-IDF | BM25 |
|--------|--------|------|
| **Term frequency** | Linear — 10 mentions = 10× score | Saturating — 10 mentions ≈ 1.8× score (k1=0.5) |
| **Length normalization** | None | Yes, via b parameter |
| **Tuning parameters** | None | k1 (saturation) and b (length) |
| **IDF formula** | `log(N/df)` — can be zero | `log((N−df+0.5)/(df+0.5)+1)` — always positive |
| **Best for** | Simple/uniform corpora | Most real-world corpora |
| **Failure mode** | Long documents with repeated terms dominate | Over-tuned k1/b can suppress valid signals |
| **Industry usage** | Academic baselines, simple systems | Elasticsearch, Lucene, Qdrant, most production search |

---

## How ragcodepilot uses BM25

### The journey: TF-IDF → BM25

ragcodepilot started with TF-IDF (2026-05-13) because code chunks are short and uniform, so the plan review argued BM25's length normalization adds little. The initial eval passed all exit criteria.

Two days later, a spike re-tested with BM25 (`k1=0.5`). Results:

| Metric | TF-IDF | BM25 k1=0.5 | Change |
|--------|--------|-------------|--------|
| hit@1 | 0.421 | 0.579 | **+15.8pp** |
| MRR@5 | 0.607 | 0.699 | **+9.2pp** |
| hit@5 | 0.895 | 0.895 | ±0pp |

BM25 was Pareto-better on every metric. The saturation mattered even on short chunks.

### Where BM25 runs in the code

```
Indexing:
  source code → Tokenize() → BM25 weights (k1=0.5, b=0.75) → sparse vector → Qdrant

Searching:
  query → Tokenize() → uniform weights (1.0 per token) → sparse vector → Qdrant
```

At index time, each token gets a BM25-weighted value. At query time, tokens get uniform weight — the corpus statistics (IDF, document length) are baked into the indexed vectors, not the query.

### Why the query uses uniform weights

The query is a single short phrase, not a document. There's no meaningful "term frequency" or "document length" to normalize. Each query term is equally important, so weight = 1.0. The ranking comes entirely from the BM25 weights pre-computed at index time.

---

## When to use which

**Use TF-IDF when:**
- You want simplicity and no tuning parameters
- Your documents are all roughly the same length
- Term repetition is rare or always meaningful
- You're building a baseline to measure improvements against

**Use BM25 when:**
- Documents vary in length (some are 10 lines, others are 200)
- Term repetition doesn't always mean higher relevance
- You want industry-standard ranking quality
- You need tunable parameters for your specific domain

**For code search specifically:** BM25 with a low k1 (0.5) is the practical winner. Code chunks are short but not uniform — a gap chunk (imports + type declarations) is structurally different from a focused function body. Length normalization helps rank the focused function higher, and mild saturation prevents verbose variable names from dominating.
