# Local Reliability and Practical Improvements

> Updated 2026-09-22 to resolve the docs_update review. This is a companion to
> the [roadmap](../plan/mvp_roadmap.md), which owns sequencing. Proposed behavior
> below is not implemented merely because it appears here. Effort uses S/M/L/XL.

## Scope

Keep the Go CLI, Ollama, Qdrant, hybrid retrieval, and optional answer generation.
Focus on trustworthy local operation and evidence from external repositories.
MCP, an agent-first pivot, multi-repo workspace commands, and shared deployment
are removed from active scope. Their brief revisit conditions are in the roadmap.

## 1. Dense embedding reuse and recoverable re-indexing [M]

### Current behavior

`Pipeline.Run` skips a run only when hashes and representation versions match
and there are no stale files. A change, addition, deletion, or version refresh
causes all remaining files in that run's scope to be chunked and upserted with
fresh sparse weights. Valid cached dense vectors are reused; only cache misses
call the embedder. The scope is the selected repository and languages, not every
point in a shared collection. [Re-indexing](reindexing.md) documents classification
and cleanup; the cache and recovery contracts below extend that flow.

Only BM25 statistics and sparse weights depend on corpus composition. Dense
embeddings depend on the exact enriched text, model artifact, and preprocessing.
Reusing a dense vector is valid only when those inputs match. File content alone
is insufficient: path/name headers, chunk boundaries, or model changes matter.

### Consolidated incremental-indexing decisions

This contract replaces the earlier tiered incremental-processing proposal.
Representation fingerprinting, exact enriched-input dense reuse, and
`index --watch` belong to the existing pipeline; they do not require a new daemon
or a multi-source dependency graph. Watch events re-run the scoped pipeline, not
just the changed file. Sparse statistics and complete-point upserts remain
scope-wide even when most dense vectors are reusable.

The old sentinel-point proposal and content-only point IDs are not the selected
design: durable run state lives outside the source repository, cache keys are
separate from point identity, and declaration identity prevents same-name Go
methods from overwriting one another. Reuse must not discard distinct identical
declarations or leave obsolete points after a rename or boundary change.

FEEDBACK: Keep invalidation, deletion/rename cleanup, interrupted-run replay,
cache-storage bounds, and source consistency as explicit acceptance concerns.
Dense-cache hits alone do not prove recovery or fully incremental indexing.
Multi-source DAGs, sparse-only writes, and atomic publication remain deferred
under the [roadmap's scope boundary](../plan/mvp_roadmap.md#out-of-current-scope).

### Correctness prerequisite discovered by M0

The [external chi audit](../eval/external/chi-v5.2.3/index-audit.json) found seven
same-file name collision groups: 382 generated chunks became 371 stored points.
The fix now includes receiver/declaration identity, bumps the representation
version, deletes same-hash old points during migration, and retains 382/382 points
in the [post-fix recheck](../eval/runs/2026-09-23-idfix/README.md). This stays within
single-repo indexing; it does not reopen workspace expansion.

### M3-A: Durable run state and writer ownership (complete)

The CLI stores state under the platform user cache directory at
`<user-cache>/ragcodepilot/index-state/`. Each collection gets a SHA-256-derived
state and lock filename, so indexing does not write control files into the source
repository. The state JSON records the collection, repository path, representation,
input fingerprint, file count, process, timestamps, and one of `in_progress`,
`completed`, or `failed`.

The input fingerprint covers sorted relative file hashes, collection/repository,
language scope, chunk size/overlap, and the current representation version. It is
published atomically after walking and hashing the source but before Qdrant schema
or point changes. A failed run keeps the marker with its error; an interrupted run
leaves `in_progress` visible for the next operator.

The collection lock is created with an atomic exclusive file create. A live owner
causes the next index command to fail with the PID, host, and lock path. If the
recorded PID is no longer alive, the lock is archived and ownership is retried.
The lock is released after the marker is completed or failed. This prevents
overlapping CLI/watch writers while keeping in-place updates and their recovery
limits explicit.

### M3-B — Dense cache [core complete; recovery/cleanup open]

The implementation is a content-addressed dense cache with the existing
full sparse refresh and complete-point upserts. Sparse-only vector updates, staged
collections, and atomic swaps are deferred. The M3-A run-state and writer contract
above handles interrupted ownership; the cache alone does not make a failed run
safe to resume.

The implementation stores one JSON vector entry per key under
`<user-cache>/ragcodepilot/index-state/dense-cache/<collection-hash>/`. Each key
includes the exact enriched text, representation version, and embedder identity.
The Ollama identity includes the model artifact digest returned by `/api/tags`;
other embedders fall back to their concrete type and dimension until they expose
a stronger identity. Cache entries are written through a temporary file and rename,
and corrupt or dimension-mismatched entries are treated as misses.

The existing sparse-statistics and complete-point upsert path is unchanged. A run
reports dense cache hits and misses; cache misses call the embedder only for the
missing texts in each batch. Cache writes occur only after vector validation, so a
failed embedding cannot publish a partial vector entry.

```text
on index(scope):
    acquire exclusive writer ownership for the collection
    freeze source manifest and representation fingerprint
    persist an incomplete-run marker before changing points
    chunk and enrich all included files in the manifest
    compute BM25 statistics for the declared scope
    for each batch:
        key = hash(model artifact, preprocessing, exact enriched text)
        reuse validated cached dense vectors when key matches
        embed remaining inputs and validate dimensions
        build sparse vectors using this run's statistics
        upsert vectors and matching payload provenance; await completion
    delete stale and orphaned points using the completed input manifest
    verify source manifest still matches the input snapshot
    clear incomplete marker and record completion only after writes and cleanup succeed
    release writer ownership

on retry with an incomplete-run marker:
    replay the full affected scope from a fresh consistent manifest
    reuse valid dense cache entries
    do not skip work merely because individual file hashes now match
```

M3-A now persists the marker, input/representation fingerprint, and last
successful run state durably. M3-B must keep using that contract: a source change
during a run or a failed write must leave the run incomplete.
Retry can replay the entire affected scope, reusing valid cached dense inputs.
This is a single recoverable in-place index, not multiple publication generations.

Existing indexes can also contain mixed file hashes or representation versions
after an interrupted refresh. The scroll state marks a file as mixed whenever its
points disagree; the next run refreshes and replaces the whole file so stale IDs
cannot be accepted from an arbitrary point's metadata.

### M3-C — Retry, cleanup, and source consistency [in progress]

After batch writes and stale-point cleanup, the pipeline re-hashes the filtered
source manifest. If any file was added, removed, or changed during the run, the
run fails and remains retryable rather than publishing a completed state. The
existing mixed-state detection and changed-file cleanup make the next retry
replace obsolete points. Acceptance still requires interruption/retry results to
match a clean rebuild and explicit deletion/rename coverage.

Prevent overlapping CLI/watch writers with a collection-level lock. In-place writes
can be visible while incomplete; acceptance covers recovery after successful retry,
not atomic snapshot reads. Defer staging/reader gating unless a real requirement
emerges. Release writer ownership on all exits without clearing a failed run marker.

The representation fingerprint must cover chunker/enrichment versions, model
artifact/digest, preprocessing, dimensions, tokenizer/statistics version, and
inclusion scope/config. A changed fingerprint triggers the appropriate refresh
even if file hashes match. Same-dimensional model swaps cannot be detected by
dimension validation alone. Bound cache storage and avoid persisting raw secret
inputs in cache keys/logs; redaction changes also invalidate affected entries.

### Acceptance

| Case | Required evidence |
|---|---|
| Ordinary one-file edit | Ollama receives only changed/new enriched inputs; unchanged dense inputs are reused |
| Delete/rename | Old IDs disappear; renames invalidate path-dependent enriched inputs; sparse weights refresh |
| Model/enrichment/chunker change | Required inputs are regenerated despite unchanged file hashes; old chunk IDs are removed |
| Interrupted refresh + retry | Completion does not advance on failure; replay produces the same final point set/weights as a clean rebuild |
| Concurrent writers / source changes | One writer per collection; changed snapshot cannot be marked complete |
| No-op after a complete run | No embedding/upsert work when manifest and representation match |

Each CLI index run emits one machine-readable-friendly `Index metrics:` line with
file/chunk counts, dense calls and inputs, cache hits/misses, sparse-vector writes,
upsert batches, stage durations, source-verification duration, and total duration.
Use those records for clean-build versus warm-cache comparisons; latency from a
single local run is descriptive and not a capacity claim.

Measure dense calls, sparse writes, hashing/chunking/statistics time, and total
elapsed time separately. **Dense work can become proportional to changed inputs;
total index work remains proportional to scope size.** A cache miss or eviction
can cause additional embeddings without violating correctness. The old 200K-chunk
and 30-minute figures are estimates, not verified capacity.

For now, use separate explicitly named collections for different repo/worktree or
language evaluation scopes. This avoids claiming globally consistent IDF across
independently indexed scopes. Existing repo identity is the directory basename;
same-named checkouts must not share a collection because point/deletion scopes
can collide. Stable workspace identity remains a prerequisite to future expansion.

## 2. One external repository and manual regression checks [S–M]

**Measurement complete:** [chi v5.2.3](../eval/external/chi-v5.2.3/README.md),
20 pre-labeled queries, zero query errors. File hit@5 is 16/16, negatives 1/4;
known chunk loss and source-range gaps prevent an index-integrity or broad-quality
claim. The rules below describe the retained workflow for future comparisons.

The old 0.895 hit@5 result came from 19 positives in a 23-query v6 report, not
all 39 queries in the current golden set. The reconciled branch includes v8;
[the roadmap](../plan/mvp_roadmap.md#evidence-snapshot-and-branch-boundary) records
its scope and negative-score semantics. None establishes external quality.

Start with **one representative external Go repository** and about 15–20 carefully
labeled cases covering definitions, concepts, behavior, multi-file evidence, and
negatives. Pin its revision and use an isolated collection. Record errors and
named misses. A second repository is required before generalization claims or
broad retrieval promotion; it is deferred from this first delivery. Go-only
results do not establish Rust or other language quality.

Use the [manual comparison checklist](../eval/README.md#baselines-and-the-corpus-stability-assumption)
for each retrieval change. Retain source/query manifests, config/filters, chunk
counts, representation/model/preprocessing versions, mode/limits, score kinds,
and paired reports. Record hardware and warm/cold state for latency. Implementation
files added during an experiment must not change one arm's indexed source.

`compare.py` prints deltas but does not check compatibility or enforce gates.
Manually reject mismatched inputs, query errors, omitted required query classes,
lost accepted hits/coverage, and new negative failures under fixed score-aware
rules. Preserve named known failures separately. Historical hybrid 1.00 at an
unreachable 0.55 threshold is not a gate; do not tune thresholds to turn it green.
Answer shape/refusal metrics remain report-only with manual content review.

Nightly/on-demand retrieval CI is deferred until these pinned manual checks are
stable and frequent enough to warrant automation. Existing unit/site CI remains
unchanged. If automated later, enforce this same policy rather than inventing
universal quality floors from one small corpus.

## 3. Sources-first output [S, optional]

If `--answer` waiting is a recurring problem, print its selected context sources
with matching citation numbers after retrieval and before warmup/generation.
Keep them readable when generation fails and preserve ordinary search output.
This does not require streaming, new models, or new evaluation machinery.

The saved AL=5 run's generation p50 23,861 ms / p95 46,434 ms explains the UX
motivation; it does not establish TTFT. Token streaming, TTFT targets, partial-stream
handling, model routing, and refusal-rule changes are deferred until repeated CLI
usage justifies them. Continue manual review of questionable answers.

## 4. Existing-command errors and .gitignore [S–M]

**Mode-aware errors — implemented.** Existing `index`/`search` failures now include
operation-specific remedies for missing Qdrant or Ollama/model services, active
writer locks, source changes during indexing, and collection-before-indexing
mistakes. Defer the separate `doctor` command. Do not require a
generator for retrieval, Ollama for sparse search, or an existing collection for
first indexing. Answer generation already provides some Ollama recovery hints;
extend the uncovered paths instead of claiming every current error is raw.

**.gitignore support — implemented.** The walker applies Git's nested patterns and
negation through `git check-ignore --no-index --stdin`, after hidden-file and
configured-directory/file exclusions. Configuration exclusions therefore take
precedence over negation. Newly ignored files disappear from the next manifest and
the existing stale-file cleanup removes their points on re-index.

The walker still skips hidden files/directories, configured directories (including
`bin`), unsupported extensions, and configured filename patterns.

**Sensitive included content — scanning deferred.** Preserve explicit exclusions;
hidden .env files and unsupported key extensions already have walker protection.
Included source/YAML/JSON can still contain credentials, so make no secret-free or
team-safe claim. Revisit automatic scanning/redaction only for a demonstrated need
with a measured false-positive policy and consistent payload/embedding invalidation.

**Refusal diagnostics — changes deferred.** Keep the current heuristic report-only
and inspect questionable answers manually. A cited refusal can be valid; a quoted
phrase can trigger a false positive. New rules require repeated labeled failures,
not a scheduled heuristic-improvement feature.

Scope exclusions and revisit conditions are recorded once in the
[roadmap](../plan/mvp_roadmap.md#out-of-current-scope).
