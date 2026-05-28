# Sparse Vectors vs Dense Vectors

Two fundamentally different ways to represent text as numbers for search.

---

## The core difference in one picture

```
Dense vector (768 dimensions — EVERY slot has a value):

  index:  [  0  ] [  1  ] [  2  ] [  3  ] [ ... ] [ 767 ]
  value:  [ 0.82] [-0.31] [ 0.44] [ 0.15] [ ... ] [-0.09]
            ▲       ▲       ▲       ▲                ▲
            │       │       │       │                │
            all dimensions carry meaning
            (but no single dimension means "chunk" or "file")


Sparse vector (millions of possible slots — MOST are zero):

  index:  [  42 ] [ 197 ] [ 8841] [ ... rest are all zero ... ]
  value:  [ 3.15] [ 2.07] [ 5.16]
            ▲       ▲       ▲
            │       │       │
            "func"  "chunk" "file"
            (each non-zero slot = one specific token)
```

**Dense:** Every dimension has a value. No single dimension has a human-readable meaning. The *pattern* of all 768 numbers together encodes meaning.

**Sparse:** Almost every dimension is zero. Each non-zero dimension corresponds to a specific word/token. The value = how important that word is (BM25 weight).

---

## How each is created

### Dense vector

```
Input text              Neural network              Output
─────────────          ──────────────              ──────
"func ChunkFile(       nomic-embed-text            [0.82, -0.31, 0.44, 0.15,
 path string,          (137M parameters)            ..., -0.09]
 lines int)"                                        
                                                    768 floating-point numbers
                       Learned from millions         Every value is meaningful
                       of text examples              No value is zero
```

The neural network has seen millions of code/text examples during training. It learned to compress text meaning into a fixed-size array where similar meanings produce similar arrays.

**You cannot look at a single number and know what it means.** Dimension 42 doesn't mean "function" or "file" — it's an abstract learned feature. The meaning emerges from the *combination* of all 768 numbers.

### Sparse vector

```
Input text                  Tokenizer + BM25           Output
─────────────              ──────────────              ──────
"func ChunkFile(            1. Split tokens            {
 path string,               2. Remove stop words         42: 3.15,    ← "func"
 lines int)"                3. Compute BM25 weight       197: 2.07,   ← "chunk"
                             4. Hash token → index        8841: 5.16,  ← "file"
                                                          1203: 1.89,  ← "path"
                            No neural network!            9044: 1.45,  ← "string"
                            Pure math + rules             6721: 1.12   ← "line"
                                                       }
                                                       
                                                       6 non-zero entries
                                                       out of 4 billion possible
```

No AI involved. A tokenizer splits the text, removes stop words, and computes a BM25 weight for each token based on:
- How often the token appears in this document (term frequency)
- How rare the token is across all documents (inverse document frequency)

**Every non-zero dimension maps to exactly one token.** If you see a high value at index 8841, you know that token (e.g., "file") is important in this document.

---

## What each captures

```
Code:  func ChunkFile(path string, lines int) []Chunk { ... }

Dense vector captures:                  Sparse vector captures:
─────────────────────                   ──────────────────────

✅ "this is about splitting             ✅ "chunk" appears (weight: 2.07)
   files into pieces"                   ✅ "file" appears (weight: 5.16)
✅ "related to data ingestion"          ✅ "path" appears (weight: 1.89)
✅ "similar to Python's                 ✅ "string" appears (weight: 1.45)
   text_splitter()"                     
✅ conceptual similarity to             ❌ no idea what the code MEANS
   "how does chunking work?"            ❌ can't match "splitting" to "chunk"
                                        ❌ can't match across languages
❌ "ChunkFile" as an exact name         ✅ "ChunkFile" → ["chunk", "file"]
   is diluted into general concepts        exact token match
```

---

## The analogy

Think of searching a library:

**Dense search** = asking a librarian *"I need books about recovering from database crashes."* The librarian understands your meaning and finds a book titled "Write-Ahead Logging for Storage Systems" — even though none of your words appear in the title.

**Sparse search** = using the card catalog index. You look up "crash" and "recovery" and find all books with those exact words. Fast, precise, but misses the WAL book because it doesn't contain "crash" or "recovery."

**Hybrid search** = asking the librarian AND checking the card catalog, then combining both lists.

---

## Comparison table

| Aspect | Dense vector | Sparse vector |
|--------|-------------|---------------|
| **Created by** | Neural network (nomic-embed-text) | Tokenizer + BM25 math |
| **Size** | Fixed (768 numbers, all non-zero) | Variable (5-50 non-zero out of billions) |
| **Storage** | ~3 KB per vector | ~0.1 KB per vector (only store non-zero) |
| **Dimensions** | 768 (model-dependent) | ~4 billion possible (CRC32 hash space) |
| **What each dimension means** | Nothing interpretable | One specific token |
| **Matches by** | Semantic meaning | Exact keyword overlap |
| **Strengths** | Synonyms, concepts, cross-language | Exact names, identifiers, predictable |
| **Weaknesses** | Loses exact identifiers | No concept understanding |
| **Query speed** | ~30ms (includes embedding call) | ~1ms (no neural network) |
| **Training required** | Yes (pre-trained model) | No (pure algorithm) |

---

## Real example from ragcodepilot

Query: `"ValidateVectorBatch"`

```
Dense search thinks:
  "validate" → concepts about checking, verification
  "vector"   → concepts about embeddings, arrays, math
  "batch"    → concepts about groups, processing

  Best matches (by meaning):
    #1  ollama.go (embedding + batching concepts)     ← wrong
    #2  fake.go (vector generation concepts)           ← wrong
    #3  validate.go:ValidateVectorBatch               ← right but ranked 3rd!

Sparse search thinks:
  "validate" → token hash 4421, weight 1.0
  "vector"   → token hash 8812, weight 1.0
  "batch"    → token hash 3097, weight 1.0

  Best matches (by token overlap):
    #1  validate.go:ValidateVectorBatch  ← right! all 3 tokens match
    #2  ollama.go (has "vector")
    #3  client.go (has "vector", "batch")
```

Dense understood the *concepts* but couldn't pinpoint the *exact function*. Sparse found the exact match instantly because the tokens aligned perfectly.

Now flip it — Query: `"how does the system prevent embedding dimension mismatches?"`

```
Dense search thinks:
  This is about dimension validation, vector size checking,
  preventing errors when models change...

  Best matches:
    #1  validate.go:ValidateVectorBatch    ← right!
    #2  ollama.go (dimension auto-detect)
    #3  client.go (collection dimension)

Sparse search thinks:
  "system" → too common (low weight)
  "prevent" → not in most code
  "embedding" → in many files
  "dimension" → in a few files
  "mismatch" → rare (high weight, but only if present)

  Best matches:
    #1  sparse.go (has "embedding")             ← wrong
    #2  enrichment.go (has "embedding")         ← wrong
    #3  validate.go (has "dimension")           ← right but ranked 3rd
```

Dense understood the *intent* and found the right code. Sparse matched surface words and got distracted by files that happen to mention "embedding."

---

## How they're stored in Qdrant

```
Qdrant point for one code chunk:

{
  "id": "a1b2c3d4-...",
  
  "vectors": {
    "dense":  [0.82, -0.31, 0.44, ... 768 values ...],    ← named vector
    "sparse": {                                             ← named vector
      "indices": [42, 197, 8841, 1203, 9044, 6721],
      "values":  [3.15, 2.07, 5.16, 1.89, 1.45, 1.12]
    }
  },
  
  "payload": {
    "file_path": "internal/ingest/chunker.go",
    "content": "func ChunkFile(path string, lines int) ...",
    ...
  }
}
```

Both vectors live in the same Qdrant point. Dense is stored as a flat array. Sparse stores only the non-zero index-value pairs (much smaller).

---

## Why you need both

Neither is sufficient alone:

```
                         Dense        Sparse       Hybrid
                        (meaning)    (keywords)   (both)
                        ─────────    ──────────   ──────
Concept questions        ✅ great     ❌ poor      ✅ great
Exact identifier         ❌ weak      ✅ great     ✅ great
Natural language         ✅ great     ⚠️ okay      ✅ great
Cross-language           ✅ yes       ❌ no        ✅ yes
Negative queries         ✅ good      ❌ risky     ✅ best
Speed                    ⚠️ 30ms      ✅ 1ms       ⚠️ 30ms

Overall hit@5            0.789        0.737        0.895
```

Hybrid takes the best of both. Dense provides the semantic foundation; sparse adds the keyword precision. Together they cover each other's blind spots — which is why hybrid is ragcodepilot's default.
