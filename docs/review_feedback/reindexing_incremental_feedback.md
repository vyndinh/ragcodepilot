# Re-indexing and Incremental Processing Feedback

## Reviewed Documents

- `docs/improvement/reindexing.md`
- `docs/improvement/incremental_processing_roadmap.md`

## Summary

The updated docs are directionally good. They now cover failure semantics, choose sentinel-point metadata for pipeline fingerprints, and clarify chunk-level vector reuse constraints.

## Findings

### P1: `ScrollFileHashes` still only reads one page

- File: `internal/qdrant/client.go`
- Lines: around `282-289`
- Problem: docs say it scrolls all points, but code only calls `Scroll` once with `Limit: 10000`.
- Risk: repos with more than 10k chunks can miss existing hashes and misclassify files.
- Recommendation: paginate until Qdrant returns no next offset.

### P2: Existing collections still skip payload index creation

- File: `internal/qdrant/client.go`
- Lines: around `58-60`
- Problem: `EnsureCollection` validates dimension and returns for existing collections.
- Risk: older collections may not have `repo`, `language`, and `file_path` indexes.
- Recommendation: call `ensurePayloadIndexes` for existing collections too, or document that old collections must be recreated.

### P2: Stale-only cleanup still reports "Everything up to date"

- File: `internal/ingest/pipeline.go`
- Lines: around `155-165`
- Problem: if stale files are deleted but no files need indexing, CLI prints `Everything up to date — nothing to index`.
- Risk: misleading CLI output.
- Recommendation: print a distinct stale-cleanup message.

### P3: Roadmap wording for unnamed block IDs is slightly inaccurate

- File: `docs/improvement/incremental_processing_roadmap.md`
- Lines: around `107-111`
- Problem: doc says unnamed blocks use `hash(repo + file + "block" + chunk_index)`, but code uses empty name plus start/index fallback.
- Recommendation: adjust wording to match `generateChunkID`.

## Mechanical Checks

- `gofmt -l` still reports:
  - `internal/ingest/pipeline.go`
  - `internal/qdrant/client.go`
- `go test ./internal/ingest ./internal/qdrant` could not run because local Go is inconsistent:
  - `GOVERSION=go1.26.3`
  - `GOROOT/GOTOOLDIR` point to `go1.26.1`
  - error: `compile: version "go1.26.1" does not match go tool version "go1.26.3"`
