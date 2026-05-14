# Re-indexing Pipeline — Review History

Consolidated audit trail covering all review rounds for the re-indexing pipeline.
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

**Resolution:** Documented as a known edge case in [reindexing.md](../improvement/reindexing.md). The failure mode is narrow (requires crash between batches of a large file) and self-heals when the file is next modified. A future `--force` flag can serve as a manual escape hatch.

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

---

## Round 3: Correctness follow-up

Follow-up review after the Round 2 fixes. One P1 correctness bug was found and fixed; four lower-severity items also resolved.

### ✅ P1: Step 9 over-deletes freshly upserted chunks

**Problem:** `DeleteByFilePaths` filters by `repo + file_path` only. Step 9 calls it on `changedFiles` *after* the upsert — but the new chunks share the same `file_path` as the old orphans, so all of them are deleted.

| Chunk type | After upsert | After Step 9 delete |
|---|---|---|
| Named function (same name) | Overwritten in-place ✓ | **Deleted** ✗ |
| New unnamed block (shifted start_line) | New point added ✓ | **Deleted** ✗ |
| Old unnamed block (orphaned position) | Still present ← target | Deleted ✓ |

Result: after one pipeline run on a changed file, that file has zero searchable points. On the next run it looks "new" (no existing hash in Qdrant), re-indexes cleanly, and self-heals — but there is a one-cycle gap where the file is missing from search results.

**Root cause:** `generateChunkID` uses `startLine` as the fallback index for unnamed blocks (`internal/ingest/chunker.go:133`). When lines shift, old start-line IDs are orphaned and new IDs are created. The file_path delete catches both.

**Fix:** Added `Client.DeleteStaleChunksByFilePath(ctx, collection, repo, filePath, currentHash)`. It filters `Must[repo, file_path]` + `MustNot[file_hash = currentHash]`, so only old-hash orphans are removed while the freshly upserted new-hash chunks survive. Step 9 in `Pipeline.Run` now loops over `changedFiles` and calls this method with each file's current disk hash (`relHashes[rel]`), replacing the blanket `DeleteByFilePaths` call.

```
// new filter in DeleteStaleChunksByFilePath
Must:    [repo = X, file_path = Y]
MustNot: [file_hash = current_hash]   ← preserves freshly upserted chunks
```

**Tests added:**
- `TestClient_DeleteStaleChunksByFilePath` — verifies filter has 2 `Must` conditions (repo + file_path), 1 `MustNot` condition (file_hash exclusion), and `Wait: true`.
- `TestPipeline_RunDeletesChangedFileChunksWithCurrentHash` — verifies the current disk hash (not the old one) is passed to the delete call.

**Files:** `internal/qdrant/client.go`, `internal/ingest/pipeline.go`, `internal/qdrant/client_test.go`, `internal/ingest/pipeline_test.go`

---

### ✅ P2: `Upsert` missing `Wait: true`

**Problem:** Round 2 P2 of the previous review added `Wait: pb.PtrOf(true)` to `DeletePoints` to close a race window. `UpsertPoints` at `internal/qdrant/client.go:184` has no corresponding `Wait`. Qdrant's WAL is sequential so no observable race in practice, but the sequencing guarantee is asymmetric with the delete.

**Fix:** Added `Wait: pb.PtrOf(true)` to the `UpsertPoints` request. Also added `TestClient_UpsertBatchSplitting` which verifies `Wait` is set on all batch requests.

**Files:** `internal/qdrant/client.go`

---

### ✅ P3: `ensurePayloadIndexes` called twice on re-index

**Problem:** On any re-index run (collection already exists), `ensurePayloadIndexes` is triggered twice — 6 gRPC round-trips instead of 3:

1. **Step 2** — `Pipeline.Run` calls `p.store.EnsurePayloadIndexes()` early, before the scroll (`pipeline.go:96`).
2. **Step 8** — `EnsureCollection` is called for the first embedding batch; for existing collections it also calls `ensurePayloadIndexes` (`client.go:63`).

Both calls are idempotent so there is no correctness issue, but the Step 2 call exists specifically to cover stale-only or fully-unchanged runs where Step 8 never fires.

**Fix:** Removed `ensurePayloadIndexes` from `EnsureCollection`'s existing-collection branch. The new-collection path still creates indexes. `Pipeline.Run()` Step 2 covers existing collections. Updated `TestClient_EnsureCollectionAcceptsMatchingExistingDimension` to expect 0 field index calls.

**Files:** `internal/qdrant/client.go`, `internal/qdrant/client_test.go`

---

### ✅ P3: `Client.Upsert` 64-chunk batch logic untested

**Problem:** `fakeSDKClient.Upsert` panics on any call (`internal/qdrant/client_test.go:210`), so the 64-chunk batch slicing loop in `client.go:178-193` has no unit test coverage. An off-by-one in the batch boundary would be invisible.

**Fix:** Replaced panic with recording. Added `TestClient_UpsertBatchSplitting` (130 chunks → 3 batches of 64/64/2, total 130 points, Wait: true on all).

**Files:** `internal/qdrant/client_test.go`

---

### ✅ P3: Misleading test name for stale-file deletion

**Problem:** `TestPipeline_RunDeletesStaleFilesBeforeUpsert` (`internal/ingest/pipeline_test.go:362`) only asserts that `delete:removed.py` appears somewhere in `ops` — it does not verify the delete occurred before the upsert. The name implies an ordering assertion that is not present.

**Fix:** Renamed to `TestPipeline_RunDeletesStaleFiles`.

**Files:** `internal/ingest/pipeline_test.go`

---

## Appendix: Doc-level feedback

Findings from a separate review of `docs/improvement/reindexing.md` and `docs/improvement/incremental_processing_roadmap.md`. All were addressed in the rounds above.

| Finding | Severity | Resolution |
|---|---|---|
| `ScrollFileHashes` only reads one page | P1 | Fixed in Round 1 (scroll pagination) |
| Existing collections skip payload index creation | P2 | Fixed in Round 1 (exposed `EnsurePayloadIndexes`) |
| Stale-only cleanup reports "Everything up to date" | P2 | Fixed in pipeline — distinct message for stale-cleanup-only runs |
| Roadmap wording for unnamed block IDs inaccurate | P3 | Updated in `incremental_processing_roadmap.md` |
