# Core technique: embeddings make everything the same shape

This document explains the fundamental technique that makes the system domain-agnostic and reusable across different use cases.

## The problem

Different use cases have different data:

```
Use case 1: Refactor Rust vector DB to Go
  → Rust source code, Go source code

Use case 2: Refactor Scala media service to Go
  → Scala source code, Go source code

Use case 3: Search API documentation
  → Markdown files, plain English text
```

These look like completely different problems. The text is different, the languages are different, the vocabulary is different. How can one system handle all of them?

## The solution: embedding models

An embedding model is a neural network that converts any text into a fixed-length array of floating-point numbers (a "vector"). The key property is that **texts with similar meaning produce vectors that are close together**, regardless of the exact words used.

```
Input text                              Output vector (simplified to 4 dims)
─────────────────────────────────────   ────────────────────────────────────
"fn recover_from_wal()"                 → [0.82, -0.31, 0.44, 0.15]
"func recoverWAL()"                     → [0.80, -0.29, 0.42, 0.17]   ← close!
"def transcode_video(input, output)"    → [0.11, 0.67, -0.23, 0.55]   ← far away
```

The Rust function `recover_from_wal` and the Go function `recoverWAL` produce **nearby vectors** because they mean similar things. The Python function `transcode_video` produces a **distant vector** because it means something completely different.

In practice, vectors are much longer (384 to 1536 numbers), but the principle is the same.

## Why this works: the vector space

The embedding model maps all text into a mathematical space where distance = difference in meaning.

```
                    ▲ dimension 2
                    │
                    │        • "video encoding"
                    │        • "transcode video"
                    │        • "media pipeline"
                    │
                    │
  "WAL recovery" • │
  "recover log"  • │
  "replay ops"   • │
                    │
                    │                    • "user authentication"
                    │                    • "login handler"
                    │
                    └──────────────────────────► dimension 1
```

Points that are close together are semantically similar. Points that are far apart are semantically different. This is true regardless of programming language, spoken language, or domain.

## What the vector database sees

After embedding, the vector database only sees arrays of numbers. It has no concept of "Rust", "Scala", "video", or "WAL". It just calculates distances between float arrays.

```
Qdrant's perspective:

Point 1: [0.82, -0.31, 0.44, ..., 0.15]   payload: {language: "rust", ...}
Point 2: [0.80, -0.29, 0.42, ..., 0.17]   payload: {language: "go", ...}
Point 3: [0.11, 0.67, -0.23, ..., 0.55]   payload: {language: "python", ...}

Query:   [0.81, -0.30, 0.43, ..., 0.16]

Result:  Point 1 (distance: 0.02) ← closest
         Point 2 (distance: 0.04) ← second closest
         Point 3 (distance: 0.89) ← far away
```

This is why the system is domain-agnostic. The vector DB does not need to understand the data. It only needs to compare numbers.

## The uniform pipeline

Because of embeddings, every use case flows through the same pipeline:

```
Any text → Embedding model → Fixed-length vector → Store in Qdrant → Search by distance
```

Concretely:

```
Use case 1 (Rust code):
  "fn recover_from_wal()" → embedding model → [0.82, -0.31, ...] → Qdrant

Use case 2 (Scala code):
  "def transcodeVideo()" → embedding model → [0.11, 0.67, ...]  → Qdrant

Use case 3 (documentation):
  "WAL ensures crash safety" → embedding model → [0.79, -0.28, ...] → Qdrant

Search:
  "how does crash recovery work?" → embedding model → [0.81, -0.30, ...] → find nearest
```

The pipeline code is identical for all three. Only the input text and the metadata (payload) change.

## Three techniques that enable multi-use-case support

### Technique 1: Collections for project isolation

Each project or use case gets its own Qdrant collection. Collections are independent and share nothing.

```
ragcodepilot index ./qdrant-rust   --collection vectordb-refactor
ragcodepilot index ./scala-media   --collection media-refactor
ragcodepilot index ./api-docs      --collection documentation
```

Inside Qdrant:

```
Collection: "vectordb-refactor"     → 200K points of Rust + Go code
Collection: "media-refactor"        → 50K points of Scala + Go code
Collection: "documentation"         → 10K points of markdown text
```

Each collection can even use a different embedding model and vector dimension. They are completely independent databases sharing the same Qdrant instance.

### Technique 2: Payload filtering for scoping within a collection

Within a single collection, you can filter by metadata without changing any code:

```
Search all languages:
  ragcodepilot search "error handling" --collection vectordb-refactor

Search only Rust code:
  ragcodepilot search "error handling" --collection vectordb-refactor --language rust

Search only a specific repo:
  ragcodepilot search "error handling" --collection vectordb-refactor --repo qdrant/qdrant
```

The filter is applied **before or during** the vector search. Qdrant uses payload indexes to do this efficiently without scanning every point.

The search code stays the same. Only the filter values in the request change.

### Technique 3: Embedder interface for swappable models

Different use cases might benefit from different embedding models:

```go
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Dimension() int
}
```

Examples:

| Use case | Best embedding model | Dimension |
|---|---|---|
| Code search (any language) | `text-embedding-3-small` (OpenAI) | 1536 |
| Code search (open source) | `all-MiniLM-L6-v2` | 384 |
| Code-specific search | `CodeBERT` or `StarCoder` | 768 |
| Documentation search | `all-MiniLM-L6-v2` | 384 |
| Multilingual text search | `multilingual-e5-large` | 1024 |

The rest of the system (walker, chunker, Qdrant client, search service, CLI) does not change when you swap the embedding model. You just plug in a different implementation of the `Embedder` interface.

## What the system reuses vs what changes per use case

| Component | Reused across use cases? | What changes |
|---|---|---|
| File walker | Yes | Nothing (walks any directory) |
| Chunker | Yes | Nothing (splits any text) |
| Embedding interface | Yes (interface) | Implementation may change (different model) |
| Qdrant client | Yes | Nothing (upserts/searches any collection) |
| Search service | Yes | Nothing (embeds query, calls Qdrant, returns results) |
| CLI | Yes | Nothing (collection name is a flag) |
| Payload schema | Mostly | Could add domain-specific fields (e.g., `module`, `service`) |
| Collection name | No | Different name per project |

## Summary

The system can support different domains because:

1. **Embeddings normalize everything** — any text becomes the same shape (a float vector), so the pipeline code is identical regardless of input
2. **Collections isolate projects** — each use case gets its own namespace in Qdrant
3. **Payload filters scope queries** — search within a language, repo, or file type without code changes
4. **The Embedder interface is pluggable** — swap the model without touching the rest of the system

The only domain-specific part is the input data itself. The system does not need to understand Rust, Scala, or video transcoding. It converts text to numbers and finds similar numbers. That is all.
