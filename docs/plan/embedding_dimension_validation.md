# Embedding Dimension Auto-Detection and Validation

## Current problem

`ragcodepilot` now supports real semantic embeddings through Ollama, but vector dimensions are still treated as a fixed configuration value in code.

Current behavior:

- `cmd/ragcodepilot/main.go` creates the Ollama embedder with a hard-coded dimension:

  ```go
  embedding.NewOllamaEmbedder(ollamaURL, ollamaModel, 768)
  ```

- `internal/embedding/ollama.go` returns that stored value from `Dimension()`.
- `internal/ingest/pipeline.go` calls `embedder.Dimension()` before embedding any code chunks.
- `internal/qdrant/client.go` creates a collection with the requested vector size, but if the collection already exists, it returns without checking whether the existing collection has the same vector size.

This is risky because Ollama embedding vector length depends on the model. `nomic-embed-text` may be 768 dimensions, but another model may be 384, 1024, or another size. Qdrant collections require a consistent vector size, so indexing and search must use vectors with the same dimensionality as the collection.

The current implementation can fail late or produce confusing errors when:

- a collection was created with fake vectors and later searched with Ollama vectors
- a collection was created with one Ollama model and later searched with another model
- a model returns a dimension different from the hard-coded `768`
- an embedding API returns malformed or inconsistent vectors

## Target behavior

The embedding response should be the source of truth for vector dimension.

Target rules:

- Infer vector dimension from `len(firstEmbedding)`.
- Create new Qdrant collections using the inferred dimension.
- Validate an existing Qdrant collection before indexing or searching.
- Validate every embedding batch before upsert or query.
- Fail early with clear errors when model output and collection dimensions do not match.

Example error:

```text
collection "code_chunks" uses 768-dimensional vectors, but model "all-minilm" returned 384-dimensional vectors; re-index with the same model or use a different collection
```

## Implementation plan

### 1. Remove hard-coded Ollama dimension

Change the Ollama constructor from:

```go
func NewOllamaEmbedder(baseURL, model string, dim int) *OllamaEmbedder
```

to:

```go
func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder
```

Update CLI embedder resolution:

```go
return embedding.NewOllamaEmbedder(ollamaURL, ollamaModel), nil
```

Keep `NewFakeEmbedder(dim int)` unchanged because fake embeddings are explicitly synthetic and test-oriented.

### 2. Cache detected Ollama dimension

Change `OllamaEmbedder` so `dim` starts at `0`.

After each successful embed response:

- validate the returned batch
- set `dim` from the first vector if `dim == 0`
- reject future responses where vectors do not match the cached `dim`

`Dimension()` should return:

- `0` before the first successful embedding
- the detected dimension after the first successful embedding

This makes `Dimension()` descriptive, not authoritative before embedding.

### 3. Add vector batch validation

Add a helper in the embedding package or ingestion package:

```go
func ValidateVectorBatch(vectors [][]float32, expectedDim int) (int, error)
```

Behavior:

- return an error if `vectors` is empty
- return an error if any vector is empty
- return an error if vectors in the same batch have different lengths
- if `expectedDim > 0`, return an error when a vector length differs from `expectedDim`
- return the detected dimension when valid

Use this helper for both indexing and search.

### 4. Change ingestion order

Current ingestion order:

```text
walk files -> chunk files -> ensure collection from embedder.Dimension() -> embed batches -> upsert
```

New ingestion order:

```text
walk files -> chunk files -> embed first batch -> infer dimension -> ensure/validate collection -> upsert first batch -> embed/validate/upsert remaining batches
```

Important detail: do not call `EnsureCollection` before a real vector is available.

### 5. Validate existing Qdrant collection dimension

Update `internal/qdrant/client.go`.

Current behavior:

```text
if collection exists:
    return nil
```

New behavior:

```text
if collection does not exist:
    create collection with inferred vector size

if collection exists:
    fetch collection info with GetCollectionInfo
    read collection vector size
    compare with inferred vector size
    return clear error if mismatch
```

For the current unnamed dense vector collection, read:

```go
info.GetConfig().GetParams().GetVectorsConfig().GetParams().GetSize()
```

If support for named vectors is added later, this logic must also support `GetParamsMap()`.

Suggested method shape:

```go
func (c *Client) EnsureCollection(ctx context.Context, name string, vectorSize uint64) error
```

can keep the same public signature, but its behavior should become "ensure or validate".

### 6. Validate search query dimension

Search flow should validate before sending a query to Qdrant:

```text
embed query -> validate query vector -> validate collection dimension -> query Qdrant
```

Add a Qdrant client method if useful:

```go
func (c *Client) ValidateCollectionVectorSize(ctx context.Context, name string, vectorSize uint64) error
```

Then `Search` can call it before `Query`.

This makes mismatched models fail before Qdrant returns a lower-level vector size error.

## Future improvement: model metadata

Dimension validation catches shape mismatches, but it does not catch semantic model mismatches when two models return the same dimension.

Eventually store embedding metadata:

- embedder type: `ollama`
- model name: `nomic-embed-text`
- vector dimension: `768`
- created/indexed timestamp

Possible storage locations:

- Qdrant collection metadata, if supported cleanly through the Go client
- a sentinel point payload in the collection
- a local project metadata file

Target behavior:

- fail or warn when searching with a different model from the model used for indexing
- include model metadata in collection inspection output
- document how to intentionally re-index with a new model

## Test scenarios

### Embedding validation

- Ollama returns one 768-dimensional vector: validation returns dimension `768`.
- Ollama returns one 384-dimensional vector: validation returns dimension `384`.
- Ollama returns an empty embedding list: validation fails.
- Ollama returns an empty vector: validation fails.
- Ollama returns vectors with different lengths in one batch: validation fails.
- Ollama returns a later batch with a dimension different from the cached dimension: validation fails.

### Indexing

- New collection plus 768-dimensional first batch: collection is created with vector size `768`.
- New collection plus 384-dimensional first batch: collection is created with vector size `384`.
- Existing collection has matching dimension: indexing continues.
- Existing collection has mismatched dimension: indexing fails before upsert.

### Search

- Query vector matches collection dimension: search proceeds.
- Query vector differs from collection dimension: search fails before Qdrant query.
- Search uses a different model with the same dimension: dimension validation passes, but metadata validation should catch this in a later phase.

## Acceptance criteria

- No production code path for Ollama relies on a hard-coded vector dimension.
- A new collection always uses the vector dimension returned by the active embedder.
- Existing collection dimension mismatch produces a clear actionable error.
- Indexing validates all embedding batches before upsert.
- Search validates query vector dimension before querying Qdrant.
- Unit tests cover valid dimensions, empty vectors, inconsistent batches, and collection mismatch behavior.
