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
│  5. Delete stale files (no replacement coming)                  │
│  6. If nothing to index AND no stale deletions → stop early      │
│  7. Chunk ALL current files (IDF must be corpus-wide)            │
│  8. Embed + upsert in batches (dense + sparse)                  │
│  9. Delete orphaned chunks for changed files (late deletion)     │
│ 10. Print summary                                               │
└─────────────────────────────────────────────────────────────────┘
```

### First index vs re-index

**First index** (no existing collection):

```
Walk → Hash → Scroll (empty map) → All files classified as "new"
→ Chunk all → Embed all → Upsert all
```

**Re-index** (collection exists, corpus changed):

```
Walk → Hash → Scroll (populated map) → Classify
→ 42 unchanged, 3 changed, 5 new, 2 stale
→ Delete 2 stale files immediately (no replacement)
→ Chunk ALL 50 current files (IDF must be consistent)
→ Embed + Upsert all chunks with fresh IDF weights
→ Delete orphaned chunks for 3 changed files (late deletion)
```

**Re-index** (collection exists, nothing changed):

```
Walk → Hash → Scroll (populated map) → Classify
→ 50 unchanged, 0 changed, 0 new, 0 stale
→ Stop early: "Everything up to date — nothing to index"
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

This field is not payload-indexed (not in `ensurePayloadIndexes`) — it's only read during scroll for change detection and used as a `MustNot` filter in `DeleteStaleChunksByFilePath`.

### Scrolling existing hashes (`ScrollFileHashes`)

```
ScrollFileHashes(collection, repo, languages[]) → { file_path → file_hash }

  if collection does not exist → return empty map
  scroll all points WHERE repo = repoName (AND language IN languages, if provided)
    requesting only file_path and file_hash fields
  deduplicate by file_path (multiple chunks share the same hash)
  return { file_path → file_hash }
```

The `languages` parameter is critical: without it, a `--language go` re-index would see Python points, classify them as stale, and delete them.

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

### Deleting stale files (`DeleteByFilePaths`)

Stale files (in Qdrant but not on disk) are deleted immediately — no replacement is coming:

```
DeleteByFilePaths(collection, repo, file_paths[]):

  delete all points WHERE
    repo = repoName
    AND file_path IN [file_paths]
```

This deletes **all chunks** for those files in a single gRPC call. The `file_path` field is already indexed (from `ensurePayloadIndexes`), so the filter is efficient.

### Deleting orphaned chunks for changed files (`DeleteStaleChunksByFilePath`)

Changed files use **late deletion**: upsert new chunks first, then remove only the orphaned old-hash chunks. This avoids a data-loss window if embedding fails mid-run.

```
DeleteStaleChunksByFilePath(collection, repo, file_path, current_hash):

  delete all points WHERE
    repo = repoName
    AND file_path = file_path
    AND file_hash != current_hash    ← preserves freshly upserted chunks
```

### Pipeline output

```
Found 50 source files in ragcodepilot
Change detection: 42 unchanged, 3 changed, 5 new, 2 stale
Deleted points for 2 stale files
Generated 300 chunks from 50 files
Detected vector dimension: 768
Indexed 32/300 chunks
Indexed 64/300 chunks
...
Indexed 300/300 chunks
Cleaned up stale chunks for 3 changed files
Successfully indexed 300 chunks into collection "code_chunks"
```

Note: even though only 8 files changed, all 50 are re-chunked and re-embedded because IDF must be corpus-wide.

When everything is up to date:

```
Found 50 source files in ragcodepilot
Change detection: 50 unchanged, 0 changed, 0 new, 0 stale
Everything up to date — nothing to index
```

When only stale files are cleaned up (no current files to index):

```
Found 0 source files in ragcodepilot
Change detection: 0 unchanged, 0 changed, 0 new, 2 stale
Deleted points for 2 stale files
Cleaned up stale files — no current source files to index
```

## Files

| File | Purpose |
|------|---------|
| `internal/model/chunk.go` | `FileHash` field added to `CodeChunk` |
| `internal/ingest/hasher.go` | `HashFile`, `HashFiles` — SHA-256 file hashing |
| `internal/ingest/pipeline.go` | `Run()` with change detection, corpus-wide IDF, and late deletion |
| `internal/qdrant/client.go` | `ScrollFileHashes`, `DeleteByFilePaths`, `DeleteStaleChunksByFilePath`, `file_hash` in payload |

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
