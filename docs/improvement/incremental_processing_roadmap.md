# Incremental Processing Roadmap

Inspired by [CocoIndex](https://github.com/cocoindex-io/cocoindex)'s incremental engine, this roadmap outlines improvements to make ragcodepilot's indexing smarter — moving from file-level change detection toward fine-grained, pipeline-aware incremental processing.

## Current state (implemented)

File-hash change detection (see [reindexing.md](reindexing.md)):

- SHA-256 hash per file stored in Qdrant payload
- On re-index: skip unchanged files, delete stale files, re-embed changed files
- Granularity: **file-level** — if any byte in a file changes, all its chunks are re-embedded

This handles the 80% case well, but has blind spots:

- Changing enrichment logic or embedding model doesn't trigger re-indexing (hash is based on raw file content, not pipeline output)
- A 1-line change in a 500-line file re-embeds all chunks from that file
- No way to detect pipeline version drift (old vectors embedded with different settings coexist with new ones)

---

## Tier 1: Pipeline-aware hashing (next)

### Problem

The current `file_hash` is computed from raw file content. But the text sent to the embedder is **enriched** (metadata header + code). If you change the enrichment format, the embedding model, or the chunking strategy, the `file_hash` stays the same — and the pipeline **incorrectly skips** files whose embeddings are now stale.

### Idea: pipeline version fingerprint

Compute a "pipeline fingerprint" from the settings that affect embedding output:

```
pipeline_fingerprint = hash(
  enrichment_format_version,
  embedding_model_name,
  chunk_size,
  chunk_overlap,
  chunker_version         // e.g. "go-ast-v1" vs "go-ast-v2"
)
```

Store this fingerprint as a **sentinel point** in the collection — a deterministic UUID derived from the repo name (e.g. `UUIDv5(namespace, "pipeline_meta:" + repo_name)`) with the fingerprint in its payload. This keeps repo-scoped metadata inside the same collection without requiring external storage or Qdrant collection-level metadata APIs.

> **Design constraints:**
> - Qdrant string point IDs must be UUID-shaped — raw strings like `__pipeline_meta__` are rejected by the SDK.
> - One sentinel per repo avoids collisions when multiple repos share a collection.
> - Sentinel points should carry `chunk_type: "metadata"` so search queries can exclude them with a `chunk_type != "metadata"` filter (or the search query already filters by `chunk_type` values like "function"/"block").

On re-index:

```
if stored_fingerprint != current_fingerprint:
    log "Pipeline config changed — full re-index required"
    delete all points for this repo
    re-index everything
else:
    use normal file-hash change detection
```

### Value

- Prevents silent embedding staleness when you upgrade models or change enrichment
- Zero cost when nothing changed — just a single string comparison
- Users get a clear message explaining why a full re-index is happening

### Effort: small

One new function to compute the fingerprint, one metadata read/write, one comparison in `Pipeline.Run()`.

---

## Tier 2: Chunk-level change detection

### Problem

When a file changes, we currently delete **all** chunks and re-embed the entire file. But often only 1 function in a 500-line file changed — the other 9 chunks are identical.

### Idea: hash enriched chunks, not files

After chunking and enrichment, hash each chunk's enriched text:

```
for each chunk in changed_file:
    chunk_hash = hash(enriched_text)

    existing_point = find point in Qdrant WHERE file_path AND chunk_hash match

    if existing_point found AND point ID is unchanged:
        skip (reuse existing embedding and point)
    else if existing_point found BUT point ID changed:
        copy existing vector to new point ID, update payload
        (avoids re-embedding, but requires a vector read + write)
    else:
        embed and upsert as new point
```

Also delete chunks that exist in Qdrant but no longer appear after re-chunking (function was removed or renamed).

The key constraint: **skipping embedding is only safe if the old vector can be reused under the same or a new point ID**. If the point ID scheme changes (e.g. chunk index shifts), the old vector must either be copied to the new ID or the chunk must be re-embedded. Making point IDs content-stable (derived from `chunk_hash`) would eliminate this problem entirely.

### Value

- Saves embedding cost proportional to actual code change, not file size
- Especially impactful for large files with many functions
- Embedding is the bottleneck (~50ms/chunk with Ollama) — skipping 9 of 10 chunks saves ~450ms per file

### Effort: medium

Requires storing `chunk_hash` in payload, scrolling at chunk granularity, and matching chunks by content hash rather than position.

### Open question: point ID stability

Chunk boundaries can shift when code is inserted or removed (e.g. a new function pushes all line numbers down). Need to match by content hash, not by `start_line`.

The current point ID strategy has a split (see `generateChunkID` in `chunker.go`):

- **Named chunks** (functions, methods): ID = `hash(repo + file + symbol_name + chunk_index)` — stable across line shifts as long as the function name doesn't change
- **Unnamed blocks** (top-level code, imports): ID = `hash(repo + file + "" + start_line)` — the name is empty and the index falls back to `start_line`, making IDs fragile since inserting code above shifts all line numbers

For chunk-level reuse to work reliably, point IDs should be **content-derived**: `hash(repo + file + chunk_hash)`. This makes IDs stable as long as the chunk content is unchanged, regardless of position in the file. The trade-off is that truly identical code appearing twice in the same file would collide — but this is rare in practice and could be handled with a disambiguating counter.

---

## Tier 3: Watch mode (live sync)

### Problem

Users must manually run `ragcodepilot index` after code changes. The search index drifts stale during active development.

### Idea: filesystem watcher

```
ragcodepilot watch --language go .
```

- Use `fsnotify` to watch the repo directory for file create/modify/delete events
- Debounce rapid changes (e.g. 500ms quiet period after last event)
- On change: re-index only the affected files using the existing change detection pipeline
- Keep the search index continuously fresh while you code

### Value

- Search results are always up-to-date without manual re-indexing
- Natural fit for IDE integration (run watch in the background)
- Transforms ragcodepilot from a batch tool to a live tool

### Effort: medium

`fsnotify` integration, event debouncing, graceful shutdown, and handling of rename/move events.

---

## Tier 4: Multi-source and dependency graph (future vision)

### Problem

ragcodepilot currently indexes one source (local git repo) into one target (Qdrant). Future use cases may involve:

- Multiple repos in one collection
- Non-code sources (markdown docs, API specs, meeting notes)
- Multiple targets (Qdrant + local SQLite summary + knowledge graph)

### Idea: declarative pipeline DAG

Define transformations as a directed acyclic graph. When a source changes, trace through the graph to find affected outputs and recompute only those.

```
source: ./repo-a/*.go ──→ chunk ──→ enrich ──→ embed ──→ Qdrant
source: ./repo-b/*.rs ──→ chunk ──→ enrich ──→ embed ──┘
source: ./docs/*.md ────→ split ──→ enrich ──→ embed ──┘
```

This is essentially what CocoIndex does — "React for data engineering." A change in one source propagates through the graph, updating only the affected downstream records.

### Value

- Enables multi-source, multi-target architectures
- Automatic stale data reconciliation across all targets
- Foundation for a production-grade indexing system

### Effort: large (architecture shift)

This would be a major refactor — likely introducing a state store (PostgreSQL or SQLite) to track the dependency graph and change propagation. Worth considering only when the use cases demand it.

---

## Summary

| Tier | Improvement | Status | Effort | Trigger |
|------|------------|--------|--------|---------|
| Current | File-hash change detection | ✅ Done | — | — |
| 1 | Pipeline version fingerprint | 📋 Planned | Small | Next |
| 2 | Chunk-level change detection | 📋 Planned | Medium | After Tier 1 |
| 3 | Watch mode (fsnotify) | 💭 Idea | Medium | After Phase 3 |
| 4 | DAG pipeline / multi-source | 💭 Vision | Large | When multi-source needed |
