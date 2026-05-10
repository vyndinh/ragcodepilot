# Re-indexing: File-Hash Change Detection

## Problem

Without change detection, every `ragcodepilot index` run re-processes **all** files:

1. **Wasted compute** — Ollama re-embeds every chunk even if the file hasn't changed
2. **Stale data** — deleted or renamed files leave orphaned points in Qdrant
3. **Slow re-indexing** — embedding is the bottleneck (~50ms per chunk); skipping unchanged files saves minutes on large repos

## Solution: SHA-256 File Hashing

Each chunk stored in Qdrant now carries a `file_hash` payload — the SHA-256 hex digest of its source file content at indexing time. On re-index, the pipeline compares on-disk hashes with stored hashes to classify every file.

## Architecture

### Data flow

```
┌─────────────────────────────────────────────────────────────────┐
│                        Pipeline.Run()                           │
│                                                                 │
│  1. Walk source files                                           │
│  2. Hash every file on disk (SHA-256)                           │
│  3. Scroll Qdrant for existing {file_path → file_hash} pairs    │
│  4. Classify each file:                                         │
│       ┌───────────┬──────────────────────────────────────────┐  │
│       │ Category  │ Condition                                │  │
│       ├───────────┼──────────────────────────────────────────┤  │
│       │ Unchanged │ file exists in Qdrant, hash matches      │  │
│       │ Changed   │ file exists in Qdrant, hash differs      │  │
│       │ New       │ file on disk, not in Qdrant              │  │
│       │ Stale     │ file in Qdrant, not on disk              │  │
│       └───────────┴──────────────────────────────────────────┘  │
│  5. Delete points for changed + stale files (filter delete)     │
│  6. Chunk + embed + upsert only new + changed files             │
│  7. Print summary                                               │
└─────────────────────────────────────────────────────────────────┘
```

### First index vs re-index

**First index** (no existing collection):

```
Walk → Hash → Scroll (empty map) → All files classified as "new"
→ Chunk all → Embed all → Upsert all
```

**Re-index** (collection exists):

```
Walk → Hash → Scroll (populated map) → Classify
→ Skip 42 unchanged
→ Delete 3 changed + 2 stale
→ Chunk + Embed + Upsert 3 changed + 5 new
```

## Implementation Details

### File hashing (`hasher.go`)

```
HashFile(path) → hex_sha256
HashFiles(paths[]) → { path → hex_sha256 }
```

- Uses SHA-256 — collision-resistant and fast relative to embedding cost
- Reads entire file into memory (source files are small, typically < 100KB)
- Returns hex-encoded digest (64 characters), stored as a string payload in Qdrant

### Qdrant payload: `file_hash` field

Every point upserted to Qdrant now includes `file_hash` in its payload alongside existing fields (`repo`, `file_path`, `language`, `content`, etc.).

This field is not indexed — it's only read during scroll, never used in search filters.

### Scrolling existing hashes (`ScrollFileHashes`)

```
ScrollFileHashes(collection, repo) → { file_path → file_hash }

  if collection does not exist → return empty map
  scroll all points WHERE repo = repoName
    requesting only file_path and file_hash fields
  deduplicate by file_path (multiple chunks share the same hash)
  return { file_path → file_hash }
```

### File classification in `Pipeline.Run()`

The pipeline converts absolute disk paths to relative paths (matching what's stored in Qdrant) and classifies:

```
for each file on disk:
    rel_path = relative_path(file)
    disk_hash = sha256(file)
    existing_hash = existing_hashes[rel_path]

    if existing_hash exists AND existing_hash == disk_hash:
        → UNCHANGED — skip (no embed, no upsert)
    else if existing_hash exists AND existing_hash != disk_hash:
        → CHANGED — mark for delete + re-index
    else:
        → NEW — mark for index

for each file in existing_hashes:
    if file not on disk:
        → STALE — mark for delete
```

### Deleting stale/changed points (`DeleteByFilePaths`)

```
DeleteByFilePaths(collection, repo, file_paths[]):

  delete all points WHERE
    repo = repoName
    AND file_path IN [file_paths]
```

This deletes **all chunks** for those files in a single gRPC call. The `file_path` field is already indexed (from `ensurePayloadIndexes`), so the filter is efficient.

### Pipeline output

```
Found 50 source files in ragcodepilot
Change detection: 42 unchanged (skip), 3 changed, 5 new, 2 stale
Deleted points for 5 files
Generated 24 chunks from 8 files
Detected vector dimension: 768
Indexed 24/24 chunks
Successfully indexed 24 chunks into collection "code_chunks"
```

When everything is up to date:

```
Found 50 source files in ragcodepilot
Change detection: 50 unchanged (skip), 0 changed, 0 new, 0 stale
Everything up to date — nothing to index
```

## Files

| File | Purpose |
|------|---------|
| `internal/model/chunk.go` | `FileHash` field added to `CodeChunk` |
| `internal/ingest/hasher.go` | `HashFile`, `HashFiles` — SHA-256 file hashing |
| `internal/ingest/pipeline.go` | `Run()` rewritten with change detection flow |
| `internal/qdrant/client.go` | `ScrollFileHashes`, `DeleteByFilePaths`, `file_hash` in payload |

## Design Decisions

### Why SHA-256 (not file mtime)?

- **mtime is unreliable**: `git checkout`, `cp -a`, and CI environments can change mtime without changing content (or vice versa)
- **SHA-256 is deterministic**: same content → same hash, regardless of filesystem metadata
- **Performance is negligible**: hashing 50 source files takes < 1ms; embedding takes seconds

### Why store hash per-chunk (not per-file in a separate index)?

- **Simpler**: no second data store to manage
- **Atomic**: if a file's chunks are upserted, they all carry the correct hash
- **Deduplication at scroll time**: scrolling returns multiple chunks per file but map dedup is trivial

### Why not track individual chunks for partial re-indexing?

A file change can add/remove/reorder functions, shifting every chunk. Deleting all chunks for a changed file and re-chunking is simpler, correct, and fast enough — the bottleneck is embedding, not chunking.
