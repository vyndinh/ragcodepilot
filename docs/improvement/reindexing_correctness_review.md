# Re-indexing Pipeline — Correctness Review

Follow-up review of the re-indexing pipeline after the consolidated `docs/review_feedback/reindexing_review.md`.
One P1 correctness bug was found and fixed; four lower-severity items remain open.

---

## Summary

| # | Severity | Issue | Status |
|---|---|---|---|
| 1 | P1 | Step 9 over-deletes freshly upserted chunks for changed files | ✅ Fixed |
| 2 | P2 | `Upsert` missing `Wait: true` | ✅ Fixed |
| 3 | P3 | `ensurePayloadIndexes` called twice on re-index | ✅ Fixed |
| 4 | P3 | `Client.Upsert` 64-chunk batch logic untested | ✅ Fixed |
| 5 | P3 | Misleading test name for stale-file deletion | ✅ Fixed |

---

## Fixed

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

## Resolved

### ✅ P2: `Upsert` missing `Wait: true`

**Problem:** Round 2 P2 of the previous review added `Wait: pb.PtrOf(true)` to `DeletePoints` to close a race window. `UpsertPoints` at `internal/qdrant/client.go:184` has no corresponding `Wait`. Qdrant's WAL is sequential so no observable race in practice, but the sequencing guarantee is asymmetric with the delete.

**Recommendation:** Add `Wait: pb.PtrOf(true)` to the `UpsertPoints` request for symmetry and to make the guarantee explicit.

**Resolution:** Added `Wait: pb.PtrOf(true)` to `UpsertPoints`. Also added `TestClient_UpsertBatchSplitting` which verifies `Wait` is set on all batch requests.

**Files:** `internal/qdrant/client.go`

---

### ✅ P3: `ensurePayloadIndexes` called twice on re-index

**Problem:** On any re-index run (collection already exists), `ensurePayloadIndexes` is triggered twice — 6 gRPC round-trips instead of 3:

1. **Step 2** — `Pipeline.Run` calls `p.store.EnsurePayloadIndexes()` early, before the scroll (`pipeline.go:96`).
2. **Step 8** — `EnsureCollection` is called for the first embedding batch; for existing collections it also calls `ensurePayloadIndexes` (`client.go:63`).

Both calls are idempotent so there is no correctness issue, but the Step 2 call exists specifically to cover stale-only or fully-unchanged runs where Step 8 never fires. Eliminating the redundancy would require restructuring the call sites.

**Recommendation:** Accept the overhead as documented, or remove `ensurePayloadIndexes` from `EnsureCollection`'s existing-collection branch and rely solely on the Step 2 call in `Pipeline.Run`. Either is safe; the latter requires care to ensure first-time indexing (collection does not yet exist) still creates indexes via `EnsureCollection`'s new-collection path.

**Resolution:** Removed `ensurePayloadIndexes` from `EnsureCollection`'s existing-collection branch. The new-collection path still creates indexes. `Pipeline.Run()` Step 2 covers existing collections. Updated `TestClient_EnsureCollectionAcceptsMatchingExistingDimension` to expect 0 field index calls.

**Files:** `internal/qdrant/client.go`, `internal/qdrant/client_test.go`

---

### ✅ P3: `Client.Upsert` 64-chunk batch logic untested

**Problem:** `fakeSDKClient.Upsert` panics on any call (`internal/qdrant/client_test.go:210`), so the 64-chunk batch slicing loop in `client.go:178-193` has no unit test coverage. An off-by-one in the batch boundary would be invisible.

**Recommendation:** Replace the panic in `fakeSDKClient.Upsert` with call and request recording (similar to `Delete`), then add a test that feeds ~130 chunks and verifies: (a) the SDK `Upsert` is called 3 times, (b) batch sizes are 64 / 64 / 2, and (c) all chunks are covered.

**Resolution:** Replaced panic with recording. Added `TestClient_UpsertBatchSplitting` (130 chunks → 3 batches of 64/64/2, total 130 points, Wait: true on all).

**Files:** `internal/qdrant/client_test.go`

---

### ✅ P3: Misleading test name for stale-file deletion

**Problem:** `TestPipeline_RunDeletesStaleFilesBeforeUpsert` (`internal/ingest/pipeline_test.go:362`) only asserts that `delete:removed.py` appears somewhere in `ops` — it does not verify the delete occurred before the upsert. The name implies an ordering assertion that is not present.

**Recommendation:** Rename to `TestPipeline_RunDeletesStaleFiles`. Stale-file deletion order relative to upsert does not need to be enforced (the stale file has no replacement coming), so adding an ordering assertion would over-specify the contract.

**Resolution:** Renamed to `TestPipeline_RunDeletesStaleFiles`.

**Files:** `internal/ingest/pipeline_test.go`
