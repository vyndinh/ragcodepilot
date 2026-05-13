# Re-indexing Pipeline — Code Review & Fixes

Consolidated review of the re-indexing pipeline covering two rounds of feedback.
All issues have been resolved.

---

## Round 1

### ✅ P1: Scroll pagination used `Scroll` instead of `ScrollAndOffset`

**Problem:** `ScrollFileHashes` called `Scroll` which discards `next_page_offset`, then guessed the next offset from the last point's ID. This could silently drop pages on repos with thousands of chunks.

**Fix:** Switched to `ScrollAndOffset` and loop until the returned offset is `nil`. Added a multi-page fake and `TestClient_ScrollFileHashesMultiPage`.

**Files:** `internal/qdrant/client.go`, `internal/qdrant/client_test.go`

---

### ✅ P2: Payload indexes only ensured on indexing paths

**Problem:** `EnsurePayloadIndexes` was only called inside `EnsureCollection`, which runs after the first embedding batch. Stale-only or fully-unchanged runs on older collections would never receive `repo`/`file_path` indexes.

**Fix:** Exposed `EnsurePayloadIndexes` as a public method on `Client`. `Pipeline.Run()` now calls it early (after `ScrollFileHashes` confirms the collection exists), before any filtered scroll or delete.

**Files:** `internal/qdrant/client.go`, `internal/ingest/pipeline.go`

---

### ✅ P2: Partial upsert hides missing chunks (documented)

**Problem:** If a crash occurs between batch upserts for the same file, the next run sees the file's hash in Qdrant and skips it — leaving incomplete chunks permanently.

**Resolution:** Documented as a known edge case in [reindexing.md](file:///Users/dinhvy/code/aiproject/ragsearch/docs/improvement/reindexing.md). The failure mode is narrow (requires crash between batches of a large file) and self-heals when the file is next modified. A future `--force` flag can serve as a manual escape hatch.

---

## Round 2

### ✅ P1: `--language` re-index deletes other languages

**Problem:** `ScrollFileHashes` returned hashes for *all* languages in a repo, but the pipeline only had Go files on disk. Python files were classified as stale and deleted.

**Fix:** Added a `languages []string` parameter to `ScrollFileHashes`. When non-empty, a language `Should`-filter is appended to the scroll query. `Pipeline.Run()` passes `p.languageKeys()` so the scroll only sees the target languages.

Added `TestClient_ScrollFileHashesWithLanguageFilter` and `TestClient_ScrollFileHashesNoLanguageFilterWhenEmpty`.

**Files:** `internal/qdrant/client.go`, `internal/ingest/pipeline.go`, tests

---

### ✅ P1: Changed-file points deleted before replacement confirmed

**Problem:** Changed files were deleted (Step 6) before embedding and upsert (Steps 7-8). If embedding failed, old chunks were permanently gone until the next run.

**Fix:** Split `filesToDelete` into `staleFiles` (deleted immediately — no replacement coming) and `changedFiles` (deleted after all upserts succeed). Removed dead `countNew` helper and simplified stats to direct `len()` calls.

New pipeline flow:
```
Step 6:  Delete stale files (no replacement)
Step 7:  Chunk new + changed files
Step 8:  Embed and upsert
Step 9:  Delete changed-file old points (after upsert confirmed)
```

Added `TestPipeline_RunDeletesChangedFilesAfterUpsert` and `TestPipeline_RunDeletesStaleFilesBeforeUpsert` using an `orderingStore` fake that records operation sequence.

**Files:** `internal/ingest/pipeline.go`, `internal/ingest/pipeline_test.go`

---

### ✅ P2: `DeletePoints` missing `Wait: true`

**Problem:** `DeletePoints` returned as soon as the request was accepted, not when applied. A race window existed where a subsequent upsert could be caught by the pending delete (both use `file_path` filter).

**Fix:** Added `Wait: pb.PtrOf(true)` to the `DeletePoints` request.

**Files:** `internal/qdrant/client.go`

---

### ✅ P2: Sentinel point `__pipeline_meta__` unsafe ID

**Problem:** The incremental processing roadmap used `__pipeline_meta__` as a point ID, but Qdrant string IDs must be UUID-shaped and a single fixed ID collides across repos.

**Fix:** Updated the roadmap to use `UUIDv5(namespace, "pipeline_meta:" + repo_name)` with `chunk_type: "metadata"` for search exclusion.

**Files:** `docs/improvement/incremental_processing_roadmap.md`

---

### ✅ P3: `gofmt` misalignment in `client_test.go`

**Fix:** Aligned `scrollPages` comment with `scrollResult` per `gofmt` tab rules.

**Files:** `internal/qdrant/client_test.go`
