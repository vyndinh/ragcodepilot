# Chunk Enrichment for Search Quality

## Current problem

`ragcodepilot` embeds raw source code with no surrounding context. When a user searches an unfamiliar repository with a natural-language query like "how does indexing work?", the embedding model must match the query against raw code tokens alone. This produces low relevance scores and unpredictable results.

Current behavior:

- `internal/ingest/pipeline.go` extracts `chunk.Content` (raw code) and passes it directly to the embedder:

  ```
  texts[i] = chunk.Content
  ```

- The embedding model sees only code syntax. It does not know the file path, function name, language, or repo — even though all of this metadata is stored in the `CodeChunk` struct and indexed as Qdrant payload.

- A user searching "how does file walking work" must guess that the code uses `WalkFiles` in `walker.go`. On an unknown repo, this is impossible.

Concrete example with current behavior:

```
Query:  "how does indexing work?"
Result: qdrant/client.go:getStringValue (score: 0.57)  ← wrong file, wrong function
```

After manually rewriting the query to match code vocabulary:

```
Query:  "walk files and chunk code for ingestion pipeline"
Result: ingest/walker.go:WalkFiles (score: 0.70)  ← correct
```

The primary bottleneck here is the lack of searchable context in the embedded text. Model quality also matters — a stronger embedding model would produce better results across the board — but even a good model cannot match a natural-language query to raw code that contains no human-readable context about its purpose.

## Target behavior

Before embedding, prepend structured metadata to the raw code content. The embedding model receives a richer text that includes human-readable context, making natural-language queries match more reliably.

Target rules:

- Build an enriched text string from `CodeChunk` metadata before embedding.
- Store the original `chunk.Content` unchanged in Qdrant payload (so search results display clean code).
- Only the text sent to the embedder changes; no other pipeline behavior changes.
- The enrichment format should be simple, readable, and consistent.

Example enriched text:

```
File: internal/ingest/walker.go
Language: go
Function: WalkFiles

func WalkFiles(root string, cfg *config.Config) ([]string, error) {
    var files []string
    ...
```

Expected improvement:

```
Query:  "how does indexing work?"
Result: ingest/pipeline.go:Run (higher score)  ← metadata words "ingest", "pipeline" boost match
```

## Implementation plan

### 1. Add an enrichment function

Add a function in `internal/ingest/pipeline.go` (or a new file `internal/ingest/enrichment.go`):

```
enrichForEmbedding(chunk) → enriched_text:

  text = ""
  text += "File: {chunk.FilePath}"
  text += "Language: {chunk.Language}"

  label = chunkTypeLabel(chunk.ChunkType)
  if chunk has a name:
    text += "{label}: {chunk.Name}"
  else:
    text += "Type: {label}"

  text += blank line
  text += chunk.Content

  return text


chunkTypeLabel(type) → label:
  "function" → "Function"
  "block"    → "Block"
  otherwise  → "Chunk"
```

Design decisions:

- Use `File:` not `FilePath:` — natural language reads better for the embedding model.
- Include `Language:` — queries like "show me the Go code that handles X" benefit from this.
- Include function/struct name with its type label — "Function: WalkFiles" adds semantic signal.
- Separate metadata from code with a blank line — keeps the embedding clean.
- Do not include `Repo:` in the default enrichment — for single-repo indexing it adds no signal. Revisit this when multi-repo indexing or `--repo` filtering is implemented.
- Always include chunk type context. When the chunk has a name, use `Function: WalkFiles`. When no name is detected, use `Type: block` so unnamed chunks still carry type context for the embedding model.

### 2. Use enriched text in the pipeline

Change `internal/ingest/pipeline.go` in the embed-and-upsert loop:

Current:

```
texts[i] = chunk.Content
```

New:

```
texts[i] = enrichForEmbedding(chunk)
```

No other pipeline changes are needed. The `chunk.Content` stored in Qdrant payload remains the original raw code.

**Operational note**: existing collections must be deleted and re-indexed after this change. Old vectors were embedded from raw code and will not benefit from enrichment. Mixing enriched and non-enriched vectors in the same collection will degrade search quality. The CLI should log a reminder when indexing into a non-empty collection.

### 3. Consider search query enrichment (optional, later)

The search query is a natural-language string. Enrichment of the query is not needed now — the embedding model handles natural language well. However, if filters are provided via CLI flags, they could optionally be prepended to improve matching:

```
Original query: "error handling"
With --language go: "Language: go\n\nerror handling"
```

This is a minor improvement and can be deferred.

## What this does NOT solve

- **Function-level chunking**: enrichment improves matching for existing chunks, but a 40-line sliding window still splits functions mid-body. Function-level chunking (Phase 2 checklist) is a separate, complementary improvement.
- **Dimension mismatches**: enrichment does not affect vector shape. See `embedding_dimension_validation.md` for that problem.
- **Keyword search**: enrichment helps the semantic model, but exact keyword matches (e.g., searching for a function name `WalkFiles`) still need BM25/hybrid search (Phase 3).

## Test scenarios

### Enrichment function

- Chunk with all fields populated: output includes file path, language, chunk type label, name, and content.
- Chunk with empty name: output includes `Type: block` (or `Type: function`) instead of omitting type info.
- Chunk with unknown `chunk_type`: output uses `Type: Chunk` as fallback.
- Content is never duplicated or modified.

### Search quality (manual verification)

After re-indexing with enrichment, verify these queries return relevant results on the ragcodepilot repo itself:

| Query | Expected top result | Why |
|-------|-------------------|-----|
| "how does indexing work?" | `pipeline.go:Run` or `walker.go:WalkFiles` | "ingest" and "pipeline" in file path metadata |
| "embedding interface" | `embedder.go:Embedder` | "embedding" in file path, "Embedder" in name |
| "Qdrant collection management" | `client.go:EnsureCollection` | "qdrant" in file path, "Collection" in name |
| "parse code into chunks" | `chunker.go:ChunkFile` | "chunk" in file path and name |
| "configuration loading" | `config.go:Load` | "config" in file path, "Load" in name |

### Regression

- Existing unit tests in `pipeline_test.go` and `main_test.go` continue to pass.
- Search with `--embedder fake` still works (enrichment is applied regardless of embedder).

## Acceptance criteria

- Embedding input includes file path, language, and function name metadata.
- Raw code content in Qdrant payload is unchanged.
- Enrichment measurably improves search relevance for natural-language queries compared to raw-code embedding. Full relevance also depends on model quality, chunking strategy, hybrid search, and reranking — enrichment is one layer in that stack.
- The enrichment function is unit-tested.
- The ragcodepilot repo can be re-indexed and the manual verification queries above produce expected results.
