# RAG Glossary

Terms used throughout ragcodepilot, organized from fundamentals to advanced.

---

## Core RAG Concepts

**RAG (Retrieval-Augmented Generation)** — Instead of asking an LLM to answer from memory, first *retrieve* relevant documents, then feed them as context to the LLM for answer *generation*. Reduces hallucination by grounding answers in real data.

**Retrieval** — Finding the most relevant pieces of information (chunks) from a large corpus given a query.

**Generation** — Using an LLM to synthesize a natural-language answer from the retrieved chunks.

**Grounding** — Ensuring the LLM's answer is based on the retrieved evidence, not made up.

**Context window** — The maximum amount of text an LLM can process at once (measured in tokens). If the input exceeds this, the model truncates or errors.

**Prompt template** — The structured text sent to the LLM: a system message (instructions) + user message (question + retrieved chunks).

**Hallucination** — When the LLM generates information that sounds correct but isn't supported by the source data. The #1 risk of RAG systems.

**Faithfulness** — How well the generated answer sticks to what the retrieved chunks actually say. High faithfulness = low hallucination.

**Dogfooding** — Using your own product in real daily work to test it before investing further.

---

## Embedding & Vectors

**Embedding** — Converting text into a fixed-length array of numbers (a "vector") using a neural network. Texts with similar meaning produce vectors that are close together.

**Embedding model** — The neural network that produces embeddings. Example: `nomic-embed-text` (768 dimensions).

**Vector** — An array of floating-point numbers representing the "meaning" of a text. Example: `[0.82, -0.31, 0.44, ...]` with hundreds of values.

**Dimension** — The length of a vector. Higher dimensions capture more nuance but cost more storage and compute.

**Vector space** — The mathematical space where all embeddings live. Distance between points = difference in meaning.

**Cosine similarity** — A measure of how similar two vectors are based on the angle between them. Score from -1 to 1; closer to 1 = more similar.

**Dense vector** — A regular embedding where every dimension has a value. Good at capturing *meaning* and *concepts*.

**Sparse vector** — A vector where most values are zero; only non-zero entries are stored as index-value pairs. Good at capturing *exact keyword matches*.

---

## Search & Retrieval Methods

**Semantic search** — Finding results by *meaning*, not exact words. "how does crash recovery work" finds `recover_from_wal()` even though no words match literally.

**Keyword search** — Finding results by *exact token matches*. "ChunkFile" finds code containing that exact identifier.

**Hybrid search** — Running both semantic (dense) and keyword (sparse) search, then combining results. Best of both worlds.

**BM25 (Best Matching 25)** — A classic keyword-ranking algorithm. Scores documents based on how many query terms they contain, weighted by how rare each term is across the corpus.

**TF-IDF (Term Frequency × Inverse Document Frequency)** — An older ranking formula. Term frequency measures how often a word appears in a document; inverse document frequency measures how rare it is across all documents.

**IDF (Inverse Document Frequency)** — How rare a token is across all documents. Rare tokens (like `ChunkFile`) get high IDF; common tokens (like `func`) get low IDF.

**RRF (Reciprocal Rank Fusion)** — A method for combining ranked lists from different retrieval methods. Formula: `score = Σ 1/(k + rank)`. Items ranked highly in *either* list get boosted in the fused result.

**Prefetch** — A mechanism for running multiple sub-queries (e.g., dense + sparse) before fusing their results in a single operation.

**Reranking** — A second-pass model that re-scores the top-N retrieved results for better precision. More expensive but more accurate than the initial retrieval.

**Cross-encoder** — A type of reranker that takes (query, document) pairs and jointly scores their relevance. More accurate than separate encoding but too slow to run on the full corpus.

**Payload** — Metadata attached to each vector in the database (e.g., file path, language, repo name). Used for filtering search results.

**Payload index** — A database index on metadata fields for fast filtered search. Without it, filters require scanning every stored point.

---

## Evaluation Metrics

**Golden dataset** — A curated set of queries with known correct answers. The ground truth for measuring retrieval quality.

**hit@k** — "Did the correct result appear anywhere in the top-k results?" Binary yes/no per query. Aggregated as a rate across all queries.

**hit@1** — Did the correct result appear as the #1 result? The strictest retrieval measure.

**hit@5** — Did the correct result appear in the top 5? The primary "RAG readiness" gate — if the right chunk is in the top 5, the LLM can find and use it.

**MRR@k (Mean Reciprocal Rank at k)** — Average of `1/rank` of the first correct result across all queries. If the correct result is #1 → 1.0, #2 → 0.5, #3 → 0.33, not found in top-k → 0. Higher is better.

**recall@k** — What fraction of *all* expected results appeared in the top k? Unlike hit@k (binary), recall measures coverage when there are multiple correct answers.

**Baseline** — A saved evaluation result used as the reference point for measuring improvement or regression from subsequent changes.

**pp (percentage points)** — Absolute difference between two percentages. "+10pp" means a metric went from, say, 0.75 to 0.85 (not a 10% relative change).

**Negative query** — A query that *should not* match well (e.g., "OAuth middleware" in a project that has none). Tests whether the system correctly returns low-confidence results.

---

## Ingestion & Chunking

**Ingestion pipeline** — The full process of preparing source code for search: walk files → chunk → enrich → embed → store vectors.

**Chunk** — A small piece of source code extracted from a file. The fundamental unit of retrieval — what gets embedded and searched.

**Chunking** — Splitting source files into chunks. Strategies include AST-based (parsing code structure) and sliding window (fixed-size with overlap).

**AST chunker** — Parses source code into an Abstract Syntax Tree and extracts one chunk per function/method. Produces cleaner, more semantically meaningful chunks.

**Sliding window** — A generic chunking strategy: take N lines, then slide forward by N−overlap lines. Produces overlapping chunks. Used when AST parsing isn't available.

**Overlap** — The number of lines shared between consecutive chunks. Prevents losing context at chunk boundaries.

**Enrichment** — Prepending metadata (file path, language, chunk type/name) to raw code before embedding. Helps the embedding model understand *what* the code is, improving search relevance.

**Upsert** — "Update or insert." Writes vectors + metadata to the database. If a record with the same ID exists, it's replaced; otherwise a new record is created.

**Change detection** — During re-indexing, comparing file hashes to skip unchanged files. Avoids the cost of re-embedding everything.

**Index version** — A fingerprint stored with each chunk so the system knows when tokenizer or weighting changes require re-embedding, even if the source file hasn't changed.

---

## Tokenization & Text Processing

**Tokenizer** — Splits text into individual tokens for sparse vector generation. Code-aware tokenizers split camelCase, snake_case, remove stop words, etc.

**Stemming** — Reducing words to their root form: `hashes` → `hash`, `running` → `run`. Helps match different grammatical forms of the same word.

**Stop words** — Common words with little search value (e.g., "the", "is", "func", "return"). Removed during tokenization to reduce noise.

**k1 parameter** — BM25 tuning knob controlling term-frequency saturation. Lower k1 = repeated terms matter less. Default in Elasticsearch is 1.2; code search often uses lower values because chunks are short.

**b parameter** — BM25 length normalization factor. `b=0.75` penalizes documents longer than the corpus average so long files don't unfairly outrank short functions on shared terms.

---

## Infrastructure

**Vector database** — A database specialized in storing and searching high-dimensional vectors. Examples: Qdrant, Milvus, Weaviate.

**Qdrant** — A vector database that stores embeddings and supports dense, sparse, and hybrid search with metadata filtering.

**Collection** — A namespace in a vector database that holds vectors + payloads. Analogous to a table in a relational database.

**Ollama** — A local LLM runner that serves models via HTTP API. Can host both embedding models and generative models.

**gRPC** — A high-performance RPC protocol. Faster than HTTP/JSON for batch operations like upserting thousands of vectors.

**HNSW (Hierarchical Navigable Small World)** — An approximate nearest-neighbor index algorithm used internally by vector databases for fast search. Trades a small amount of accuracy for dramatic speed improvement over brute-force search.

**Cold start** — The delay when an LLM runner loads a model into memory for the first time. Can take 30-120 seconds for large models. Subsequent calls are fast because the model stays loaded.

---

## Metrics Shorthand

| Shorthand | Meaning |
|-----------|---------|
| `hit@5 = 0.895` | 89.5% of queries had the correct chunk in the top 5 |
| `MRR@5 = 0.699` | Average reciprocal rank of first correct result is 0.699 |
| `+15.8pp` | Improved by 15.8 percentage points |
| `nav h@5` | hit@5 for navigation-type queries only |
| `con h@5` | hit@5 for concept-type queries only |
| `beh h@5` | hit@5 for behavior-type queries only |
| `neg pass` | Pass rate for negative queries |
| `p50 / p95` | Median / 95th percentile latency |
| `RRF k=60` | Reciprocal Rank Fusion with k parameter = 60 |
| `768d` | 768-dimensional vectors |
