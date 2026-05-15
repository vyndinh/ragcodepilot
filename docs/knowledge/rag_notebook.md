# RAG Notebook — Learning RAG Through ragcodepilot

> A beginner-friendly walkthrough that ties the **theory of RAG** to the **actual code in this repo**.
>
> Read this top-to-bottom the first time. After that, treat it as a reference: each section answers *what is X*, *why does it matter*, and *where does ragcodepilot do it*.

**Companion docs:**

- [`rag_parts.md`](rag_parts.md) — full enterprise RAG architecture (5 components)
- [`embeddings_explained.md`](embeddings_explained.md) — deep dive on embeddings
- [`compare.md`](compare.md) — Qdrant vs Milvus vs OpenSearch vs Weaviate
- [`../plan/system_design.md`](../plan/system_design.md) — this project's design
- [`../plan/hybrid_search.md`](../plan/hybrid_search.md) — hybrid (dense+sparse) search plan

---

## Table of contents

1. [What is RAG, in one paragraph](#1-what-is-rag-in-one-paragraph)
2. [The two halves of RAG: Indexing and Retrieval](#2-the-two-halves-of-rag-indexing-and-retrieval)
3. [Glossary — terms you'll see everywhere](#3-glossary--terms-youll-see-everywhere)
4. [How "full RAG" maps to ragcodepilot](#4-how-full-rag-maps-to-ragcodepilot)
5. [The Indexing pipeline, step by step](#5-the-indexing-pipeline-step-by-step)
6. [The Search pipeline, step by step](#6-the-search-pipeline-step-by-step)
7. [Key concept — embeddings and vector space](#7-key-concept--embeddings-and-vector-space)
8. [Key concept — cosine similarity](#8-key-concept--cosine-similarity)
9. [Key concept — sparse vectors and BM25](#9-key-concept--sparse-vectors-and-bm25)
10. [Key concept — hybrid search and RRF](#10-key-concept--hybrid-search-and-rrf)
11. [Key concept — payload filtering](#11-key-concept--payload-filtering)
12. [Re-indexing and change detection](#12-re-indexing-and-change-detection)
13. [Mental model recap](#13-mental-model-recap)
14. [Current system performance — what it can and can't do](#14-current-system-performance--what-it-can-and-cant-do)
15. [Comparison with Windsurf Codemap and similar tools](#15-comparison-with-windsurf-codemap-and-similar-tools)
16. [Where to go next](#16-where-to-go-next)

---

## 1. What is RAG, in one paragraph

**RAG** stands for **Retrieval-Augmented Generation**. The idea: a language model alone has no specific knowledge of *your* documents (your codebase, your wiki, your contracts). So instead of asking the model directly, you:

1. **Retrieve** the most relevant passages from your private data.
2. **Augment** the model's prompt with those passages.
3. **Generate** an answer grounded in real, current context.

The current ragcodepilot implementation focuses on the retrieval half: it returns the top-K most relevant code chunks for a natural-language query. That is the first stage of the intended RAG pipeline, not the final product boundary. The direction is to evolve toward full local RAG by adding an optional answer-generation layer that uses retrieved chunks as grounded context and cites the source chunks it used.

Until that answer mode ships, there is no LLM in the search path — you, the developer, are the "generator."

> 💡 **Why no LLM yet?** For code search, raw source code is often more useful than a paraphrased summary. The default system returns *exact code chunks*, not generated prose. A future answer mode should be opt-in and citation-heavy so generated text never hides the underlying evidence. See [`system_design.md`](../plan/system_design.md) and [`../plan/phase5_v0_answer_mode.md`](../plan/phase5_v0_answer_mode.md) for the reasoning.

---

## 2. The two halves of RAG: Indexing and Retrieval

Every RAG system has two flows running through the same infrastructure:

### Indexing (offline, slow, run once per doc change)

```
Source documents
    → Split into chunks
    → Convert each chunk to a vector (embedding)
    → Store vectors + metadata in a vector database
```

### Retrieval (online, fast, runs on every query)

```
User query
    → Convert query to a vector (same embedding model!)
    → Ask the vector database "which stored vectors are closest to this one?"
    → Return the top-K matching chunks (with their metadata)
```

Full RAG continues one more step:

```
Retrieved chunks
    → Insert into the LLM prompt as context
    → Generate an answer grounded in those chunks
    → Cite the chunks so the user can verify the answer
```

ragcodepilot currently stops after returning chunks. Future `--answer` mode would add the final generation step.

The *same embedding model configuration* should be used on both indexing and search — otherwise the two vectors can live in different "spaces" and distance comparisons become meaningless. ragcodepilot validates the collection's vector dimension at search time (`internal/qdrant/client.go` — `ValidateCollectionVectorSize`). This catches swaps to models with different dimensions, but it does **not** prove model identity: a different model with the same dimension would still pass. A stricter system would store model name/version metadata in the collection or in a pipeline fingerprint.

---

## 3. Glossary — terms you'll see everywhere

| Term | Plain-English definition | Where in ragcodepilot |
|---|---|---|
| **Chunk** | A small piece of a document — usually small enough to fit in a single embedding (200–500 tokens). Splitting matters because embedding a whole 1000-line file gives a vector that means "this file" — too vague to find a specific function. | `CodeChunk` struct in `internal/model/chunk.go` |
| **Token** | The unit a model uses internally. Roughly ¾ of a word for English; for code it's smaller. You don't usually count tokens by hand — just know "smaller chunk = fewer tokens = more precise embedding." | Conceptual; not computed directly |
| **Embedding** | A list of floating-point numbers (a vector) that represents the *meaning* of a piece of text. Texts with similar meanings → vectors close together. | Produced by `OllamaEmbedder` in `internal/embedding/ollama.go` |
| **Vector** | Just an array of numbers. In RAG it always means the output of an embedding model. | `[]float32` everywhere in the code |
| **Dimension** | How long the vector is. `nomic-embed-text` produces 768-dimensional vectors. Different dimensions are never interchangeable; same dimension still does not guarantee two models share the same vector space. | Auto-detected and validated in `internal/embedding/validate.go` |
| **Dense vector** | A "normal" embedding where every dimension has a meaningful float value. Captures *meaning*. | `dense` named vector in the Qdrant collection |
| **Sparse vector** | A vector where most entries are zero. Used for *keyword* matching (which words appear, weighted by importance). Captures *exact terms*. | `SparseVector` struct in `internal/embedding/sparse.go` |
| **Cosine similarity** | A math trick that asks "do these two vectors point in the same direction?" — the standard distance metric for embeddings. Output: 1.0 = identical direction, 0.0 = unrelated, −1.0 = opposite. | Configured via `Distance_Cosine` in `internal/qdrant/client.go` |
| **Vector database** | A database that's optimized for one query: "given this vector, find the closest stored vectors *fast*." Examples: Qdrant, Milvus, Weaviate. | Qdrant runs in Docker (`docker-compose.yml`) |
| **Collection** | A namespace inside a vector DB. Like a table in SQL. ragcodepilot uses one collection (default name `code_chunks`). | Created by `EnsureCollection` in `internal/qdrant/client.go` |
| **Point** | One stored record in a collection. Has an ID, one or more vectors, and a payload (metadata). | Each `CodeChunk` becomes one point |
| **Payload** | The metadata attached to a point — file path, language, repo, line numbers, the raw code text. Searchable as filters, not as semantic content. | `CodeChunk` fields other than the vector |
| **Payload index** | A regular database index on a payload field, so filters like `language = "go"` are fast. Without it, Qdrant scans every point. | Created in `ensurePayloadIndexes` in `internal/qdrant/client.go` |
| **Nearest-neighbor search (kNN / ANN)** | "Find the K closest vectors." Exact kNN is slow at scale; **Approximate** kNN (ANN) trades a tiny bit of recall for huge speedups. Qdrant uses HNSW internally. | `Query` method in `internal/qdrant/client.go` |
| **HNSW** | "Hierarchical Navigable Small World" — the graph algorithm Qdrant uses to do ANN. Treat it as a black box for now; just know "it's how Qdrant finds nearest vectors quickly." | Internal to Qdrant |
| **RRF (Reciprocal Rank Fusion)** | A simple, robust way to combine two ranked lists into one. Used here to merge dense and sparse search results. | Server-side in Qdrant; see [`hybrid_search.md`](../plan/hybrid_search.md) |
| **BM25** | The standard formula for weighting words in lexical retrieval: rare tokens count more than common ones (IDF), repeated tokens saturate so they don't dominate (`k1`), and long documents are penalized for shared tokens (`b`). The basis of the sparse vector. ragcodepilot uses `k1=0.5`, `b=0.75`. | `ComputeCorpusStats`, `BuildSparseVectors` in `internal/embedding/sparse.go` |
| **Enrichment** | Prepending metadata (file path, language, function name) to the chunk text *before* embedding, so the embedding captures context, not just code. | `enrichForEmbedding` in `internal/ingest/enrichment.go` |
| **Re-indexing** | Re-running ingestion after files change. ragcodepilot uses file hashes to skip unchanged files. | `Run` in `internal/ingest/pipeline.go` (steps 3–6) |

---

## 4. How "full RAG" maps to ragcodepilot

[`rag_parts.md`](rag_parts.md) describes a full enterprise RAG system with five components. ragcodepilot is a stripped-down learning version:

| Full RAG component | Role | In ragcodepilot |
|---|---|---|
| **RAG Server** | Orchestrates everything | The Go CLI (`cmd/ragcodepilot/main.go`) + the `ingest` and `search` packages |
| **Embedding Server** | Turns text into vectors | The `Embedder` interface (`internal/embedding/`) — calls local Ollama |
| **Vector DB (Qdrant)** | Stores vectors, runs nearest-neighbor search | Qdrant in Docker, wrapped by `internal/qdrant/client.go` |
| **MongoDB + MinIO** | Stores raw files and structured metadata | **Skipped.** We read files directly from the local filesystem. |
| **LLM Server** | Generates the final answer | **Skipped today.** The current system returns raw code chunks; future `--answer` mode would add this layer. |

Two skips, two reasons:

- **No MongoDB/MinIO**: the input is a local Git repo — files are already on disk.
- **No LLM in the current search path**: for code search, the raw source chunks are the evidence. Generation should come later as an optional layer that cites those chunks.

That makes the current implementation the **retrieval subsystem** of a larger RAG pipeline. This is a good learning order: chunking, embeddings, vector search, hybrid search, filtering, and evaluation must work before generation can be trusted. Once retrieval is strong enough, answer mode can add the "G" in RAG.

---

## 5. The Indexing pipeline, step by step

> 📍 Code: `internal/ingest/pipeline.go` — function `Pipeline.Run`

When you run `ragcodepilot index .`, this is what happens:

```
Step 1.  Walk the directory tree → list of source files
Step 2.  Ensure payload indexes exist in Qdrant
Step 3.  Hash every file on disk
Step 4.  Read existing file hashes from Qdrant
Step 5.  Classify each file as: unchanged / changed / new / stale
Step 6.  Delete stale points (files no longer on disk)
Step 7.  Chunk all current files
Step 7.5 Compute global IDF over every chunk (for sparse vectors)
Step 8.  For each batch of 32 chunks:
           a. Enrich text (prepend metadata)
           b. Call Ollama → dense vectors
           c. Build sparse vectors using global IDF
           d. Upsert (dense + sparse + payload) to Qdrant
Step 9.  Delete orphaned chunks from changed files (lines shifted)
```

Let's walk through the conceptually interesting steps.

### 5.1 Walking files

📍 `internal/ingest/walker.go` — `WalkFiles`

Recursively traverses the directory. Skips:

- Hidden directories (`.git`, `.github`, etc.) and configured skip-dirs (`node_modules`, `vendor`, …) via `config.yaml`
- Files that don't match a known language extension
- **Binary files** — detected by scanning the first 8KB for null bytes (`isBinaryFile`)

Result: a flat slice of absolute paths to source files.

### 5.2 Chunking — splitting a file into searchable pieces

📍 `internal/ingest/chunker.go` (router) + `internal/ingest/chunker_go.go` (Go AST) + the generic fallback in `chunker.go`

**Why chunk at all?** Embedding models have a maximum input length, and a 1000-line file produces a vector that means "general Go code" — too vague. Smaller chunks → more specific meanings → better retrieval.

Two strategies live side-by-side:

```
ChunkFile(path)
  if language == "go":
    → chunkGoFile()              uses go/parser to get one chunk per function
                                  + "block" chunks for gaps (imports, types, vars)
                                  + sliding-window split for functions over 80 lines
                                  + falls back to chunkGeneric() on syntax errors
  else:
    → chunkGeneric()             line-based sliding window
                                  default: 40 lines per chunk, 5 lines overlap
                                  uses regex (`namePatterns`) to guess function names
```

**Overlap** matters: without it, a function that crosses a chunk boundary gets cut in half and both halves embed poorly. 5 lines of overlap means the boundary code appears in both adjacent chunks, so at least one captures it cleanly.

Each chunk gets:

- A **deterministic ID** — UUID-v5 derived from `repo + filepath + symbol + index`. Stable across re-indexes so unchanged code keeps its ID and you don't get duplicates.
- A **payload**: repo, file path, language, chunk_type (`function` or `block`), name, content, line range, file hash, timestamp.

### 5.3 Enrichment — giving the embedder context

📍 `internal/ingest/enrichment.go` — `enrichForEmbedding`

This is one of the most underappreciated tricks in RAG. The raw chunk content is:

```
function WalkFiles(root, config):
  ...
```

A user might ask *"how do we list source files in a repo?"* — but the raw code never says "list" or "source files." Pure semantic match might miss it.

So before embedding, we prepend metadata in plain English:

```
File: internal/ingest/walker.go
Language: go
Function: WalkFiles

function WalkFiles(...) ...
```

Now the embedding captures *both* the code's meaning *and* its context (file location, language, name). The raw `Content` stored in Qdrant is **unchanged** — only the text fed to the embedder is enriched.

> 💡 **Why this works:** embedding models are trained on natural language. A pure function body is a poor input. Adding human-readable framing brings the chunk closer (in vector space) to the kinds of natural-language queries users actually type.

### 5.4 Dense embedding — text becomes a vector

📍 `internal/embedding/ollama.go` — `OllamaEmbedder.Embed`

Each enriched text gets sent to a local **Ollama** server running the `nomic-embed-text` model. Ollama returns a 768-dimensional vector of floats.

```
Embed(["File: walker.go\nLanguage: go\nFunction: WalkFiles\n\nfunction WalkFiles..."])
  → POST http://localhost:11434/api/embed
  → returns [[0.12, -0.31, 0.88, ..., 0.04]]    // 768 floats
```

The dimension (768) is **auto-detected on the first call** and locked. Every subsequent batch must produce vectors of the same dimension, or `ValidateVectorBatch` errors out. This prevents one class of silent failure: swapping to a model with a different vector size mid-pipeline.

> ⚠️ **The same model configuration should be used for both indexing and search.** Embedding "WAL recovery" with model A and the stored chunks with model B can put them in different coordinate systems → distances are meaningless. ragcodepilot enforces dimensional compatibility by storing the vector size on the collection and refusing to operate if it doesn't match. It does not yet store a model fingerprint, so same-dimension model swaps remain a possible operator mistake.

### 5.5 Sparse vector generation — keyword matching at scale

📍 `internal/embedding/sparse.go`

A dense vector is great for *meaning* ("WAL recovery" ≈ "log replay") but bad for *exact symbol names* ("`ChunkFile`"). For that, you want classic keyword search.

ragcodepilot computes a **sparse vector** for every chunk — a BM25 score per token. See [§9](#9-key-concept--sparse-vectors-and-bm25) for the math; for now, the pipeline shape:

```
Step 7.5  ComputeCorpusStats(allChunkTexts) → { idfMap, avgDocLen }
            // single pass:
            //   - IDF per token (BM25-smoothed: rare → high)
            //   - average document length (used by BM25's length norm)

Step 8     For each batch:
             BuildSparseVectors(batchTexts, corpusStats)
               → for each chunk: { token → BM25 weight }
               → represented as parallel arrays (Indices, Values)
```

The stats are computed **once per indexing run** over the entire corpus (not per batch), so the same token has a consistent weight everywhere. See [`hybrid_search.md`](../plan/hybrid_search.md) for why this matters.

### 5.6 Upsert to Qdrant

📍 `internal/qdrant/client.go` — `Upsert`

Send a batch of points (chunk + dense vector + sparse vector + payload) to Qdrant over gRPC. "Upsert" = insert-if-new-or-update-if-id-exists. Because IDs are deterministic, re-indexing the same file overwrites the same points rather than duplicating them.

A Qdrant point ends up looking like:

```
{
  id: "a1b2c3d4-..." (UUID-v5),
  vector: {
    dense:  [0.12, -0.31, ..., 0.04],    // 768 floats
    sparse: { indices: [...], values: [...] }
  },
  payload: {
    repo: "ragcodepilot",
    file_path: "internal/ingest/walker.go",
    language: "go",
    chunk_type: "function",
    name: "WalkFiles",
    content: "function WalkFiles(root, config) ...",
    start_line: 15,
    end_line: 47,
    file_hash: "sha256:abc...",
    indexed_at: "2026-05-13T10:00:00Z"
  }
}
```

---

## 6. The Search pipeline, step by step

> 📍 Code: `internal/search/searcher.go` — `SearchWithTimings`

When you run `ragcodepilot search "how does file walking work"`, here's what happens:

```
Step 1.  Determine search mode (default: hybrid)
Step 2.  If mode ∈ {dense, hybrid}:
           a. Embed the query with the SAME Ollama model
           b. Validate vector dimension matches the collection
Step 3.  If mode ∈ {sparse, hybrid}:
           a. Tokenize the query (same Tokenize() used at index time)
           b. Build a SparseVector (uniform weight 1.0 per token)
Step 4.  Call Qdrant Search() with the appropriate request shape:
           dense  → top-level Query = dense vector
           sparse → top-level Query = sparse vector
           hybrid → two Prefetch stages (dense + sparse) + RRF fusion
Step 5.  Return the top-K results, each {chunk, score}
Step 6.  CLI formats them for the terminal
```

### 6.1 Three search modes

| Mode | What it does | When it shines | When it fails |
|---|---|---|---|
| **`dense`** | Pure semantic search using only the embedding | Conceptual queries: *"how does crash recovery work"*, *"where do we validate user input"* | Exact-symbol queries (a function named `Foo` may not match the query `Foo`) |
| **`sparse`** | Pure keyword search using BM25 sparse vectors | Exact-symbol/navigation queries: *"where is `ChunkFile` defined?"* | Concept queries (no exact word overlap) |
| **`hybrid`** | Runs both, fuses ranks via RRF | Most queries — combines strengths | Slightly slower than either alone |

Default mode is **hybrid**, because evaluation showed +25pp `hit@5` on navigation queries with no regression on concept queries (see [`hybrid_search.md`](../plan/hybrid_search.md)).

### 6.2 Filtering — narrowing the search by metadata

Two CLI flags (`--language`, `--repo`) become **payload filters** on the Qdrant request. In hybrid mode the filter is applied to **each prefetch stage**, not after fusion — otherwise Qdrant would fuse unfiltered results and then throw most of them away, leaving you with too few. See [§11](#11-key-concept--payload-filtering).

---

## 7. Key concept — embeddings and vector space

> 📍 Deep dive: [`embeddings_explained.md`](embeddings_explained.md)

An embedding model is a neural network. You give it text, it gives you a fixed-length vector. The model has been trained so that **texts with similar meaning produce vectors that point in similar directions**, regardless of the exact words.

```
"function recover_from_wal()"    →  [0.82, -0.31, 0.44, ..., 0.15]
"procedure recover_wal()"        →  [0.80, -0.29, 0.42, ..., 0.17]    ← close
"function transcode_video()"     →  [0.11,  0.67, -0.23, ..., 0.55]   ← far
```

This is the magic that makes RAG work: the database doesn't need to understand Rust, Go, or English. It just compares numbers.

**Three things follow from this:**

1. **The pipeline is domain-agnostic.** The same code that indexes Rust can index Markdown.
2. **The model choice is the most consequential decision.** Better embeddings = better retrieval. ragcodepilot uses `nomic-embed-text` (768d) for a balance of quality and local-CPU speed.
3. **You should not mix model configurations.** Vectors from model A can live in a different coordinate system from model B's vectors, so distance comparisons may become meaningless. That's why ragcodepilot validates vector dimensions on the collection — though same-dimension model swaps remain an operator concern.

---

## 8. Key concept — cosine similarity

You don't need to memorize the formula, but the intuition is useful.

Given two vectors `A` and `B`, **cosine similarity** measures the **angle** between them:

```
cosine(A, B) =  1.0    same direction       (identical meaning)
                0.0    perpendicular        (unrelated)
               -1.0    opposite direction   (very rare with embeddings)
```

Why the angle and not the distance? Because embedding magnitudes can vary, but *direction* captures meaning consistently. Two vectors can have very different lengths but still point the same way.

In ragcodepilot, the collection is configured with `Distance: Cosine` when it's created (`internal/qdrant/client.go` — `EnsureCollection`). Qdrant takes care of the math.

---

## 9. Key concept — sparse vectors and BM25

A **dense vector** has a meaningful value at every position. A **sparse vector** has values at only a few positions (most are zero). They solve different problems.

### Why both?

Imagine searching for the exact identifier `ChunkFile`. Dense embedding works in *meaning space* — it might return chunks about "splitting files," "file processing," "iterating over directories," and so on. All semantically related, but the exact function `ChunkFile` could rank fifth or tenth.

A **sparse vector** built on the words actually present (with rare-word weighting) boosts chunks that contain the same identifier parts. In ragcodepilot, `ChunkFile` is split into `chunk` and `file`, so sparse search helps exact-symbol queries by matching those parts. It improves the odds, but it does not guarantee the definition ranks first — the eval shows a strong top-1/top-5 lift, not perfect top-1 accuracy.

### How ragcodepilot builds them

📍 `internal/embedding/sparse.go`

```
Step 1.  Tokenize(text) — single source of truth
           - Split on whitespace and punctuation
           - Sub-split camelCase: ChunkFile → ["chunk", "file"]
           - Sub-split snake_case: chunk_file → ["chunk", "file"]
           - Keep digit runs:      sha256Hash → ["sha256", "hash"]
           - Lowercase, drop Go keywords + English stop words

Step 2.  Per chunk: count term frequencies (TF — raw counts)
Step 3.  Corpus-wide: count how many chunks contain each token (DF),
         and track average document length (avgdl).
Step 4.  IDF[t]    = log((N - DF[t] + 0.5) / (DF[t] + 0.5) + 1)
                     (BM25's smoothed IDF — strictly positive even when
                     a token appears in every document)
Step 5.  lenNorm   = 1 - b + b * (|d| / avgdl)              with b = 0.75
         Weight[t] = IDF[t] * TF[t] * (k1 + 1)              with k1 = 0.5
                     / (TF[t] + k1 * lenNorm)
Step 6.  Represent as parallel arrays (CRC32 hash, weight)
```

The two new BM25 ideas on top of plain TF-IDF are:

- **`k1` (saturation)**: a token repeated 10 times no longer counts 10× more than appearing once. The score asymptotes. ragcodepilot uses `k1 = 0.5` (softer than the Elasticsearch default of `1.2`) because code chunks rarely have pathological repetition.
- **`b` (length normalization)**: a 200-line file with a few shared tokens shouldn't outrank a focused 20-line function. `lenNorm` rises when a doc is longer than the corpus average, shrinking the per-token weight.

Tiny worked example:

Assume one chunk tokenizes to five tokens, and `avgdl` across the corpus is 4 tokens:

```
["chunk", "file", "file", "hash", "qdrant"]   docLen = 5
```

`lenNorm = 1 − 0.75 + 0.75 × (5 / 4) = 1.1875`

With BM25 IDF values from the corpus pass and `k1 = 0.5`:

| Token | tf | IDF | numerator: tf·(k1+1) | denom: tf + k1·lenNorm | BM25 weight |
|---|---:|---:|---:|---:|---:|
| chunk | 1 | 1.20 | 1.5 | 1.594 | **1.13** |
| file | 2 | 0.70 | 3.0 | 2.594 | **0.81** |
| hash | 1 | 1.60 | 1.5 | 1.594 | **1.51** |
| qdrant | 1 | 2.30 | 1.5 | 1.594 | **2.16** |

Compare with plain TF-IDF, where `weight = (count / docLen) × IDF`:

| Token | TF-IDF weight |
|---|---:|
| chunk | 0.24 |
| file | 0.28 |
| hash | 0.32 |
| qdrant | 0.46 |

Same shape, different magnitudes. The headline difference: TF-IDF's `count/docLen` ties `file` (2 occurrences) to `chunk`/`hash`/`qdrant` (1 occurrence each) via per-doc normalization. BM25 separates them more cleanly — `file` has the highest raw tf but the lowest weight because saturation damps it and the rarer tokens win.

The stored sparse vector is not saved as token strings. It becomes parallel arrays:

```
indices = [CRC32("chunk"), CRC32("file"), CRC32("hash"), CRC32("qdrant")]
values  = [1.13, 0.81, 1.51, 2.16]
```

Qdrant sees integer dimensions and weights, while the tokenizer remains the source of truth for mapping text to those dimensions.

### Why BM25 (and why `k1 = 0.5`)?

ragcodepilot originally shipped TF-IDF in 2026-05-13. A 2026-05-15 spike re-ran the eval with BM25, and a same-day follow-up added additive Snowball stemming:

| Mode | hit@1 | hit@3 | hit@5 | MRR@5 | con h@5 | p95 ms |
|---|---|---|---|---|---|---|
| baseline_v2 (TF-IDF hybrid) | 0.421 | 0.737 | 0.895 | 0.607 | 1.000 | 173 |
| baseline_v3 (BM25 `k1=0.5` hybrid) | 0.632 | 0.789 | 0.842 | 0.706 | 0.857 | 119 |
| **baseline_v4 (BM25 + stemming hybrid)** | **0.579** | **0.789** | **0.895** | **0.699** | **1.000** | **141** |

`baseline_v3` lifted hit@1 by 21pp but lost one concept query (`hasher_concept`) — a tokenizer plural/singular issue (`hashes` vs `hash`). `baseline_v4` added additive Snowball stemming (emitting both `hashes` and `hash`), recovering full hit@5 and concept score while keeping hit@1 substantially above the original TF-IDF baseline. **`baseline_v4` is Pareto-better than `baseline_v2` on every metric.**

`k1 = 0.5` was chosen over Elasticsearch's default `1.2` because code chunks are short and uniform. Aggressive saturation (`k1 = 1.2`) lifted hit@1 by +10.5pp; softer saturation (`k1 = 0.5`) lifted it by +21.1pp. Going lower (`k1 = 0`) would disable saturation entirely — equivalent to IDF-only ranking. See `docs/plan/hybrid_search.md` §3 for the full history and trade-off analysis.

### Why CRC32 hashes as indices?

Sparse vectors need integer indices. ragcodepilot uses CRC32 of the token as the index — no vocabulary table to persist. The tradeoff: ~120 collisions across 1M unique tokens (birthday paradox). For a local dev tool, fine. Upgrade to xxhash if it ever matters.

### Critical invariant

The **same `Tokenize()` function** must be called at index time *and* query time. If they diverge (one lowercases, the other doesn't), exact matches fail silently. ragcodepilot enforces this with a single public `Tokenize()` and a parity test.

---

## 10. Key concept — hybrid search and RRF

> 📍 Plan + eval results: [`hybrid_search.md`](../plan/hybrid_search.md)

**Hybrid search** = run dense and sparse search separately, then fuse the two ranked lists into one. The challenge: dense scores (cosine similarity, 0–1) and sparse scores (BM25, unbounded) are on completely different scales. You can't just add them.

**Reciprocal Rank Fusion (RRF)** sidesteps this entirely by ignoring the raw scores and using only the *rank position* of each result in each list:

```
For each document d that appears in any list:
  RRF_score(d) = sum over all ranked lists L of:  1 / (k + rank_of_d_in_L)

  where k = 60 (a smoothing constant)

  Documents not in a list don't contribute from that list.
  Final ranking: sort by RRF_score descending.
```

Why it works:

- Scale-free — works no matter what scoring scheme each list used.
- Top results in any list dominate (small rank ⇒ big `1 / (k + rank)`).
- Documents that show up in *both* lists get a boost from each — a natural way to reward agreement.

ragcodepilot doesn't implement RRF itself. Qdrant has it built in:

```
QueryPoints {
  Prefetch: [
    { Query: denseVector,  Using: "dense",  Limit: 2*K, Filter: <filters> },
    { Query: sparseVector, Using: "sparse", Limit: 2*K, Filter: <filters> },
  ],
  Query: RRF(k=60),
  Limit: K,
}
```

One gRPC call, two retrieval paths, fused server-side. Simpler than two calls + client-side fusion.

**Filter placement matters**: filters live on each prefetch stage, *not* at the top level. If we filtered after fusion, both prefetches would return unfiltered top-K, fusion would happen on the wrong universe of documents, and we'd be left with very few matches.

---

## 11. Key concept — payload filtering

> 📍 Code: `internal/qdrant/client.go` — `ensurePayloadIndexes`, and the `Search` method

Each Qdrant point has a **payload** — arbitrary JSON-ish metadata. Common filters in ragcodepilot:

| Flag | Filter | Use case |
|---|---|---|
| `--language go` | `language == "go"` | Limit to a single language |
| `--language go,rust` | `language IN {"go", "rust"}` | Multi-language search |
| `--repo qdrant` | `repo == "qdrant"` | Limit to a specific repo |
| (both) | AND'd together | Narrow further |

**Payload indexes** make this fast. Without an index, Qdrant has to read the payload of every point to check the filter. With a keyword index on `language`, it's a constant-time lookup. ragcodepilot creates payload indexes on `repo`, `language`, and `file_path` whenever a collection is created or re-opened (`ensurePayloadIndexes`).

> 💡 **The big idea:** vector search and payload filtering are different tools that compose well. The vector search answers *"what's similar?"*; the payload filter answers *"within which subset?"*. Most production RAG systems lean heavily on both.

---

## 12. Re-indexing and change detection

> 📍 Code: `internal/ingest/pipeline.go` — `Run`, steps 3–6 and step 9

Naive re-indexing would re-embed every file every time — expensive (each embedding is a network call to Ollama). ragcodepilot uses **file hashes** to do incremental work:

```
For every source file on disk:
  hash_disk[file] = SHA-256 of file contents

Read every point's file_hash from Qdrant payload (scoped to the repo and language filter):
  hash_qdrant[file] = stored hash

Classify each file:
  unchanged  hash_disk[file] == hash_qdrant[file]   → can be skipped if nothing else changed
  changed    hash_disk[file] != hash_qdrant[file]   → corpus changed
  new        on disk, not in Qdrant                 → corpus changed
  stale      in Qdrant, not on disk                 → corpus changed

If no changed/new/stale files:
  Stop early. Nothing is embedded or upserted.

If any changed/new/stale file exists:
  Recompute global IDF over all current chunks.
  Re-upsert all current chunks, including unchanged files,
  so every sparse vector uses the same IDF snapshot.
```

**Why re-upsert unchanged chunks on a corpus-changing run?** Because IDF is corpus-wide. If you only re-embed changed files, the sparse weights for unchanged files become stale relative to the new IDF — exact-match quality degrades over time. This is the *one* place ragcodepilot accepts extra work for correctness.

**Caveat — language-scoped IDF**: if you re-index with `--language go`, the IDF is computed over all current Go chunks in that run. Other languages in the same collection retain the IDF weights from whichever run last wrote them. Fine for single-language search; less ideal for cross-language hybrid search. Documented inline at `internal/ingest/pipeline.go` (the comment around step 7.5).

---

## 13. Mental model recap

```
INDEX TIME                                    SEARCH TIME

  source repo                                    user query
       │                                             │
       ▼                                             ▼
  walk files                                  parse mode flag
       │                                             │
       ▼                                  ┌──────────┴──────────┐
  chunk into pieces                       ▼                     ▼
       │                              embed query          tokenize query
       ▼                              (Ollama)             (Tokenize → sparse)
  enrich with metadata                     │                     │
       │                                   └──────────┬──────────┘
       ▼                                              ▼
  embed (Ollama → dense)                     Qdrant Query:
  + tokenize (BM25 → sparse)                   dense   → vector kNN
       │                                       sparse  → BM25 dot product
       ▼                                       hybrid  → both + RRF
  upsert to Qdrant                                      │
  (dense + sparse + payload)                            ▼
                                              top-K {chunk, score}
                                                       │
                                                       ▼
                                              format + print to terminal
```

Two halves, same dense vector space, same sparse tokenizer, compatible embedding configuration. That symmetry is the *only* reason retrieval works well enough for RAG.

---

## 14. Current system performance — what it can and can't do

This section is an honest, beginner-friendly assessment. The goal is not to oversell the system; it's to help you understand *which jobs RAG-style retrieval is the right tool for*, and which ones need a different category of tool entirely.

### 14.1 What the numbers actually say

The most recent eval ([`baseline_v4.json`](../eval/baseline_v4.json), 2026-05-15 — see [`hybrid_search.md`](../plan/hybrid_search.md) for the full matrix) ran the system against **its own Go source code**: 350 chunks, 32 files, 23 hand-crafted queries (19 positive, 4 negative). Results in **hybrid mode** (the default, BM25 + additive Snowball stemming + dense + RRF):

These numbers are tied to that exact indexed corpus. For algorithm comparisons, compare runs against the same corpus; if chunking changes or new files are added, re-capture the baseline first. See [`../eval/README.md`](../eval/README.md) for the corpus-stability rule.

Future mixed-language baselines, such as a Phase 3 Go+Rust corpus, should be read as a new corpus generation rather than a direct replacement for these Go-only numbers.

| Metric | Value | Plain-English meaning |
|---|---|---|
| **hit@5** | **0.895** | For 9 out of 10 queries, the correct chunk appears in the top 5 results |
| hit@3 | 0.789 | ~4 out of 5 times, it's in the top 3 |
| hit@1 | 0.579 | The very first result is right about 58% of the time |
| MRR@5 | 0.699 | Average position of the correct answer is ~1.4 in the top 5 |
| Navigation hit@5 (exact-symbol queries) | 0.750 | Hybrid mode finds named symbols in top 5 for 3 of 4 such queries |
| Concept hit@5 | 1.000 | Conceptual queries always have a correct hit in top 5 |
| Negative queries (should return nothing on-topic) | 1.000 pass | No false matches on out-of-scope questions |
| p50 / p95 latency | 28ms / 141ms | Fast enough for interactive search at the terminal |

**What these numbers mean for you as a user:**

- **The first result is usually right; if it isn't, the right one is in the top 5.** Hit@1 = 0.579 and hit@5 = 0.895. You'll see the answer in the very first result the majority of the time, and almost always within the first five.
- **Conceptual questions work very well.** "Find the code that does X" is the sweet spot — concept hit@5 is 100% on this corpus.
- **Exact-symbol questions improved a lot from hybrid.** Going from dense-only (50% hit@5) to hybrid (75%) is one of the biggest wins in the project.
- **Latency is comfortable.** ~30ms p50 means searches feel instant in the terminal.

> ⚠️ **Scale caveat.** These numbers are from a **350-chunk** corpus. The design target is **200K chunks** ([`system_design.md`](../plan/system_design.md)). Quality and latency at that scale are *projected*, not measured. Expect surprises when you index 5–10 large repos.

### 14.2 What ragcodepilot supports well today

| Task | How well | Why |
|---|---|---|
| **"Where is `X` defined?"** | ✅ Strong (especially in hybrid mode) | Sparse vectors weight exact symbol tokens heavily; RRF surfaces them. |
| **"Find code that does Y"** (conceptual) | ✅ Very strong | Dense embeddings + enrichment is exactly designed for this. |
| **"Show all `Z` in language `L`"** | ✅ Strong | Payload filters on `language` / `repo` are precise and fast. |
| **Quick local exploration** of an unfamiliar repo | ✅ Solid | Hybrid + filters + sub-100ms latency = pleasant iteration loop. |
| **Negative / out-of-scope queries** | ✅ Solid | Hybrid mode doesn't hallucinate matches when there aren't any. |
| **Re-indexing after edits** | ✅ Solid | Hash-based change detection skips unchanged files automatically. |
| **Cross-language semantic links** ("find Rust code similar to this Go function") | ✅ Possible | All languages live in the same vector space. (Not specifically eval'd, but architecturally supported.) |

### 14.3 What ragcodepilot cannot do — and why it matters

These are the **structural limits** of the current system. Most of them aren't bugs; they're consequences of being a chunk-retrieval system without an LLM or full static-analysis layer.

#### AST chunking is not code understanding

ragcodepilot *does* use Go's AST in `internal/ingest/chunker_go.go`, but only to slice files into function-level chunks and extract function names. The body of each function is treated as plain text after that. There is no symbol table, no call graph, and no type-analysis pass.

In short: **AST chunking ≠ AST understanding**. The parser tells us *where* a function starts and ends, not *what it calls* or *what implements what*.

| Task | Supported? | Why not |
|---|---|---|
| **Hierarchical view of the codebase** (folder/file/function tree) | ❌ No | The system stores flat chunks. There's no notion of "this file contains these functions" as a navigable structure. The walker discards the tree as it goes. |
| **Call graph / execution flow** ("what calls `WalkFiles`?") | ❌ No | Requires identifier resolution. ragcodepilot parses Go files but only uses the AST to find function/method *boundaries* (line ranges + names) — it never walks call expressions or resolves identifiers to declarations. |
| **Component relationships** ("which structs implement this interface?") | ❌ No | Requires type analysis. The Go chunker stops at function-declaration boundaries; it does not keep method receiver context, so a method on `Server` named `Handle` becomes a chunk named just `Handle`. |
| **Cross-file dependency graph** | ❌ No | No import-graph extraction. Imports land in "block" chunks like any other top-level code. |
| **Code execution flow visualization** | ❌ No | No diagrams, no UI — the CLI prints text only. |
| **Multi-hop reasoning** ("explain how indexing handles a syntax error") | ❌ No | Each query is a single retrieval. There's no agent loop that follows references. |
| **Generated answers / summaries** | ❌ Not yet | No LLM is currently in the search path. A future `--answer` mode would add this layer on top of retrieved chunks. |
| **Reranking** for higher top-1 quality | ❌ Not yet | Planned as a retrieval-quality improvement. It becomes especially important if answer mode needs more precise top-3 context. |
| **Query rewriting / expansion** ("WAL" → "write-ahead log") | ❌ No | The query is embedded literally. |
| **AST chunking for non-Go languages** | ❌ Go only | Python/Rust/JS fall back to sliding window, which can split functions awkwardly. |
| **Symbol-aware deduplication** in results | ❌ No | If three chunks all mention `WalkFiles`, they may all rank in the top 5 even though one would be enough. |
| **Cross-language consistent sparse weights** when re-indexing one language at a time | ⚠️ Known limit | Documented in `internal/ingest/pipeline.go` step 7.5: per-language `--language go` re-index updates IDF only for Go chunks. |

### 14.4 Why hit@1 isn't higher — and what could improve it

Hit@1 is 0.579 today (up from 0.421 under TF-IDF). It's a respectable number for a system without reranking, but it's the metric with the most remaining headroom. The structural reasons:

1. **No reranking layer.** RRF merges dense + sparse ranks but never re-evaluates the top candidates with a stronger model. A cross-encoder reranker over the top 20 candidates typically lifts hit@1 by 10–20pp in published benchmarks.
2. **Code naturally repeats itself.** Test code shares tokens with the code it tests. A query like *"where is `ChunkFile` defined?"* may rank `TestChunkFile_RoutesGoFiles` very close to the actual `ChunkFile` declaration. The eval doc calls this out explicitly.
3. **No query understanding.** Natural-language queries are embedded as-is; there's no expansion or rewriting.

**Practical implication:** the first result is the right answer the majority of the time; when it isn't, the answer is almost always within the top 5. Hit@5 = 0.895. Under the RAG-readiness framework in [`retrieval_quality_decisions.md`](retrieval_quality_decisions.md), hit@5 matters more than hit@1 anyway — it's the size of the context window the LLM gets to work with.

### 14.5 Honest summary

**Strengths**

- Fast, local, free (no API cost).
- Strong on conceptual queries; meaningfully improved on exact-symbol queries thanks to hybrid mode.
- Solid engineering foundation: deterministic IDs, change detection, dimension validation, eval harness, payload indexes.
- Domain-agnostic by construction — same pipeline works for any language with an extension mapping.

**Weaknesses**

- Returns flat, isolated chunks. No structural understanding of the codebase.
- No call graph, no cross-references, no component map.
- No LLM yet → no current synthesis, summaries, or guided multi-hop exploration.
- No reranking yet → top-1 accuracy is 58% (up from 42% under TF-IDF); reranking could lift this to 70–80%.
- AST-quality chunking is Go-only.
- Untested at the 200K-chunk design target.
- No visualization layer.

**Honest verdict:** ragcodepilot today is an effective **semantic-search CLI** for code, not yet a **full codebase understanding platform**. For "find the chunk that does X" it works well. For "show me how this system fits together" it needs additional layers: static analysis or structural views for program shape, and an LLM answer layer for synthesis.

---

## 15. Comparison with Windsurf Codemap and similar tools

Tools in this space fall into two broad categories that solve **fundamentally different problems**. ragcodepilot and Windsurf's Codemap sit in opposite categories — they're complementary, not competitors.

### 15.1 Two categories of code-understanding tools

| Category | Built on | Answers questions about | Examples |
|---|---|---|---|
| **A. Structural / navigation tools** | Static analysis: AST, language servers (LSP), tree-sitter, type info | The **shape** of the code: what exists, what references what, what the hierarchy looks like | Windsurf **Codemap**, Sourcegraph code intel, ctags/LSP IDE outlines, GitHub code navigation |
| **B. Semantic retrieval tools** | Embeddings + vector search (often + LLM) | The **intent** behind a question: "find code that does X" where X is described in natural language | **ragcodepilot**, Cursor's `@codebase`, Sourcegraph Cody, Continue, Aider's repo-map |

Some products combine both (Cursor, Cody) — but the underlying *techniques* are distinct, and it's important to understand which one you have.

### 15.2 What Windsurf Codemap is, in plain terms

Codemap is Windsurf's structural overview feature: an outline / hierarchical view of the codebase produced by static analysis. It shows folders, files, declarations (functions, classes, types), and typically supports jumping to definitions and seeing references. It updates as you edit. It lives inside the IDE.

It does **not** answer natural-language questions like "where do we handle session expiry?" — it answers structural questions like "what does this file contain?" and "what calls this function?".

> ℹ️ Note: feature details for any vendor product change over time. The comparison below is by **category** (structural navigation vs. semantic retrieval), which is the stable axis. If a Windsurf release adds semantic search, that's a *different* feature stacked on top of Codemap — Codemap itself remains a structural view.

### 15.3 Side-by-side: who wins which question?

| Question you ask | Codemap (structural) | ragcodepilot (semantic) |
|---|---|---|
| "What's the structure of this project?" | ✅ Native — that *is* the feature | ❌ Returns chunks, not a tree |
| "List every function in `parser.go`" | ✅ Exact, instant | ⚠️ Possible via filter + chunked results, but messy |
| "Jump to the definition of `validateToken`" | ✅ Single click | ⚠️ Hybrid mode usually finds it in top 5 (75% on the eval corpus) |
| "Who calls `validateToken`?" | ✅ Cross-references from static analysis | ❌ No call graph; you'd grep |
| "What does this class inherit from?" | ✅ Trivial | ❌ Not modeled |
| "Show me the import graph" | ✅ Possible (depending on tool) | ❌ No |
| "How does session expiry work in this codebase?" | ⚠️ You have to navigate manually | ✅ Returns the relevant chunks directly |
| "Find auth code that times out idle connections" | ❌ No semantic understanding | ✅ Concept queries are the strongest mode |
| "Where do we retry failed requests?" | ⚠️ Only if you know the function name | ✅ Concept hit@5 = 100% on this repo |
| Cross-language semantic match ("find Rust code similar to this Go function") | ❌ Language-by-language only | ✅ Same vector space for all languages |
| Diagram / visual hierarchy | ✅ Built-in | ❌ Not in this system |
| Generated explanation in natural language | ❌ Not its job | ❌ Not today; planned direction is an optional answer mode on top of retrieval |
| Works offline / no API cost | Depends on product | ✅ Fully local (Ollama + Qdrant) |
| Works on unfamiliar codebases | ⚠️ Helps with structure but you still have to know where to look | ✅ Explicitly designed for this case |

### 15.4 Pros and cons in plain English

**Codemap-style tools (pros)**

- Precise and deterministic — answers come from the actual program structure, not a probability distribution.
- Excellent for *navigation* and *getting oriented* in a new codebase.
- Cheap to query (no embeddings, no model calls).
- Stays accurate as code changes if static analysis is rerun.

**Codemap-style tools (cons)**

- Requires language support (parser, LSP, type info). New or niche languages get worse coverage.
- Can't answer natural-language questions — you must already know the symbol name or file.
- Doesn't capture *intent*. "I want code that handles retries" doesn't translate to anything Codemap can search.
- Typically tied to an IDE; less useful from the command line or in CI.

**ragcodepilot-style semantic retrieval (pros)**

- Handles natural-language queries — you describe what you want, not where to look.
- Domain-agnostic — same pipeline works for code, docs, prose.
- Cheap to run locally (no GPU, no API), genuinely fast (<100ms p50).
- Hybrid search captures both meaning and exact-symbol matches.
- Easy to extend: swap the embedding model, add a reranker, plug into a larger RAG system.

**ragcodepilot-style semantic retrieval (cons)**

- Probabilistic — top-1 is unreliable; you read 3–5 candidates per query.
- No structural understanding: can't show hierarchy, call graphs, or component relationships.
- Quality depends heavily on chunk boundaries and the embedding model.
- Doesn't generalize well to "show me everything about X" — it ranks candidates but won't enumerate.
- Without an LLM on top, results are raw code, not explanations.

### 15.5 The real takeaway

If you're trying to **understand a codebase end-to-end**, you want **both**:

```
Structural view (Codemap-style)        Semantic retrieval (ragcodepilot-style)
  ↳ "What's in here?"                    ↳ "Find the part that does X"
  ↳ "What calls this?"                   ↳ "Where do we handle retries?"
  ↳ Hierarchical drill-down              ↳ Natural-language discovery

  + (often) LLM on top to synthesize across both sources of evidence
```

A production "AI code copilot" typically stacks all three:

1. Static analysis for navigation and precise lookups.
2. Vector retrieval for natural-language discovery.
3. An LLM that orchestrates the two and writes the explanation.

**ragcodepilot currently implements layer 2.** That is the right foundation for the full-RAG direction because generation is only useful when the retrieval context is good enough to trust.

Current path toward full RAG:

- **Keep retrieval measurable.** The eval harness stays the scoreboard for any change that affects search quality.
- **Add answer generation deliberately.** `docs/plan/phase5_v0_answer_mode.md` describes the minimal `--answer` mode: retrieve chunks, build a prompt, generate an answer, and print source chunks so the user can verify it.
- **Add grounding safeguards after v0.** Citation parsing, faithfulness checks, refusal on weak retrieval, and token-budget management belong after the first answer-mode dogfood pass.

Supporting retrieval-quality improvements:

- **Reranking** — improves top-1/top-3 precision, especially important if answer mode needs cleaner context.
- **Rust / non-Go AST chunking** — improves chunk boundaries for languages where sliding windows are too crude.
- **Structural extraction** — call graphs, imports, and symbol tables move the system toward codebase-understanding territory. This is larger than RAG alone and should be treated as a separate product layer.

---

## 16. Where to go next

Now that you understand the concepts, here are good next paths:

- **Run it locally**, watch the output, then read the code along the flow described in [§5](#5-the-indexing-pipeline-step-by-step) and [§6](#6-the-search-pipeline-step-by-step):

  ```
  docker compose up -d
  ollama pull nomic-embed-text
  go run ./cmd/ragcodepilot index --language go .
  go run ./cmd/ragcodepilot search "where do we walk files"
  go run ./cmd/ragcodepilot search --mode sparse "ChunkFile"
  go run ./cmd/ragcodepilot search --mode hybrid "ChunkFile"
  ```

- **Look at the eval harness** ([`../eval/README.md`](../eval/README.md), [`../plan/rag_evaluation_metrics.md`](../plan/rag_evaluation_metrics.md), `go run ./cmd/ragcodepilot eval`) — how do we *measure* whether retrieval got better or worse? Metrics like `hit@K` and `MRR@5` are the answer. The per-query results are especially useful for learning because they show exactly which queries improved, regressed, or missed.

- **Read [`hybrid_search.md`](../plan/hybrid_search.md)** end-to-end. It's the single best worked example in this repo of going from a design decision to an eval result.

- **Read [`../plan/phase3_rust_chunker.md`](../plan/phase3_rust_chunker.md)** to see how the system plans to improve non-Go chunk quality. Once that phase ships, index a Rust corpus and compare AST chunking against the generic fallback.

- **Read [`../plan/phase5_v0_answer_mode.md`](../plan/phase5_v0_answer_mode.md)** to see how the system can evolve from retrieval-only search into a full RAG loop with local answer generation and source chunks.

- **Separate product learning from database-internals learning.** The product path is retrieval quality → answer mode → grounding safeguards. The vector-DB learning path is Qdrant internals: HNSW graphs, segments, WAL, payload storage, and the eventual Rust→Go refactor mentioned in [`system_design.md`](../plan/system_design.md).

- **Compare vector DBs** in [`compare.md`](compare.md) — when *wouldn't* you choose Qdrant?
