# Phase 1: Vector Math + Flat Search

> Build the simplest possible vector database — in-memory, brute-force search, no persistence.

## What you will learn

This phase teaches the **core mechanic** of every vector database: given a query vector, find the K most similar vectors from a stored collection. Everything else (HNSW, segments, WAL, filtering) is built on top of this foundation.

---

## Project location

The vector DB lives as a new package inside ragsearch:

```
ragsearch/internal/vecdb/    ← new package, zero dependencies on ragsearch
```

This means it can later implement the existing `vectorStore` interface to replace Qdrant — no separate repo needed.

---

## How it works

### 1. Data model

A vector database stores **points**. Each point has three parts:

```
Point
  ├── ID       →  unique identifier (string)
  ├── Vector   →  a list of float32 numbers (the embedding)
  └── Payload  →  arbitrary metadata (key-value pairs)
```

Points are grouped into **collections**. A collection has a fixed vector dimension and a distance metric (cosine, dot product, or L2).

### 2. Distance functions

To find "similar" vectors, we need a way to measure how close two vectors are. Three standard metrics:

**Dot product** — multiply corresponding elements and sum them. Higher = more similar.

```
dot([1,2,3], [4,5,6]) = 1×4 + 2×5 + 3×6 = 32
```

**Cosine similarity** — measures the angle between two vectors, ignoring their magnitude. Range: [-1, 1]. Higher = more similar.

```
cosine(a, b) = dot(a, b) / (‖a‖ × ‖b‖)
```

Key insight: if vectors are **normalized** (length = 1), then cosine similarity equals dot product. Many vector DBs pre-normalize vectors to make search faster — one less division per comparison.

**L2 squared distance** — sum of squared differences. Lower = more similar. We skip the square root because `sqrt` is monotonic — it doesn't change which vector is closest, only the absolute number.

```
L2²([1,2,3], [4,5,6]) = (1-4)² + (2-5)² + (3-6)² = 27
```

### 3. Top-K selection

The search returns the best K results, not all results. Two approaches:

**Naive**: compute all scores → sort → take first K. Cost: O(n log n).

**Min-heap**: maintain a heap of size K. For each new candidate, compare against the worst item in the heap. If better, swap. Cost: O(n log K). This is significantly faster when n is large (100K vectors) and K is small (10).

Go's `container/heap` provides the heap interface. The heap keeps the **worst** score at the root, so checking "is this candidate better than my worst?" is O(1).

### 4. Brute-force search

The simplest possible search — scan every vector in the collection:

```
Search(query, topK):
    collector = new min-heap of size topK

    for each point in collection:
        score = distance(query, point.vector)
        collector.push(score, point)

    return collector.results sorted best-first
```

This is called **flat search** (or linear scan, exact search). It always returns the true nearest neighbors — no approximation.

Why start here? Because flat search is the **ground truth**. Later, when you build HNSW (Phase 5), you measure its quality by comparing against flat search:

```
Recall@10 = overlap(HNSW top10, Flat top10) / 10
```

If flat search doesn't work correctly, you can't evaluate anything else.

### 5. Concurrency

Use `sync.RWMutex` on the collection:
- Writes (upsert, delete) take an exclusive lock
- Reads (search, get) take a shared lock
- Multiple concurrent searches don't block each other

---

## API surface

The collection should support these operations:

| Operation | Description |
|-----------|-------------|
| `CreateCollection(name, dim, metric)` | Create a new empty collection |
| `Upsert(point)` | Insert or update a point |
| `Delete(id)` | Remove a point by ID |
| `Get(id)` | Retrieve a point by ID |
| `Search(query, topK)` | Brute-force nearest-neighbor search |
| `Size()` | Number of stored points |

Plus a thin `DB` layer to manage multiple collections (create, get, delete, list).

---

## Files to create

| File | Purpose |
|------|---------|
| `model.go` | Core types: Point, Vector, Payload, MetricType, ScoredPoint |
| `distance.go` | Dot, CosineSimilarity, L2Squared, Normalize |
| `topk.go` | Min-heap collector for top-K selection |
| `collection.go` | Collection struct with CRUD + brute-force search |
| `db.go` | Multi-collection manager |
| `*_test.go` | Tests for each file, run with `-race` |

Estimated total: ~730 lines.

---

## Verification

- All tests pass with `go test ./internal/vecdb/ -v -race`
- Benchmarks at 768 dimensions (matching `nomic-embed-text`):
  - Distance functions: measure μs per call
  - Search at 1K, 10K, 100K vectors: measure latency scaling
- Manual smoke test: insert 3 known vectors, search, verify ranking

---

## What this phase does NOT include

| Deferred to | Feature |
|-------------|---------|
| Phase 2 | Payload filtering (`WHERE language = 'rust'`) |
| Phase 3 | Segments (mutable → sealed) |
| Phase 4 | WAL + persistence |
| Phase 5 | HNSW index |
| Phase 6 | Filtered HNSW + query planner |
