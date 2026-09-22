# Local Reliability and Practical Improvements

> Updated 2026-09-22 to resolve the docs_update review. This is a companion to
> the [roadmap](../plan/mvp_roadmap.md), which owns sequencing. Proposed behavior
> below is not implemented merely because it appears here. Effort uses S/M/L/XL.

## Scope

Keep the Go CLI, Ollama, Qdrant, hybrid retrieval, and optional answer generation.
Focus on trustworthy local operation and evidence from external repositories.
MCP, an agent-first pivot, multi-repo workspace commands, and shared deployment
are removed from active scope. Their brief revisit conditions are in the roadmap.

## 1. Dense embedding reuse and recoverable re-indexing [M–L]

### Current behavior

`Pipeline.Run` skips a run only when hashes and representation versions match
and there are no stale files. A change, addition, deletion, or version refresh
causes all remaining files in that run's scope to be chunked, embedded, and
upserted. The scope is the selected repository and languages, not every point
in a shared collection. [Re-indexing](reindexing.md) already documents this.

Only BM25 statistics and sparse weights depend on corpus composition. Dense
embeddings depend on the exact enriched text, model artifact, and preprocessing.
Reusing a dense vector is valid only when those inputs match. File content alone
is insufficient: path/name headers, chunk boundaries, or model changes matter.

### Proposed contract

Prefer a bounded first implementation: a content-addressed dense cache while
retaining full sparse refresh. Sparse-only vector updates are an alternative
optimization after payload/version consistency is specified. Both still require
recoverable completion state; adding a cache alone does not fix interrupted runs.

```text
on index(scope):
    freeze source manifest and representation fingerprint
    acquire exclusive writer ownership for scope
    persist an incomplete generation before changing points
    chunk and enrich all included files in the manifest
    compute BM25 statistics for the declared scope
    for each batch:
        key = hash(model artifact, preprocessing, exact enriched text)
        reuse validated cached dense vectors when key matches
        embed remaining inputs and validate dimensions
        build sparse vectors using this generation's statistics
        upsert vectors and matching payload provenance; await completion
    delete stale and orphaned points using the completed input manifest
    verify source manifest still matches the input snapshot
    mark generation complete only after all writes and cleanup succeed
    release writer ownership

on retry with an incomplete generation:
    replay the full affected scope from a fresh consistent manifest
    reuse valid dense cache entries
    do not skip work merely because individual file hashes now match
```

Persist generation/completion state durably and define its location, scope key,
and crash recovery before implementation. Mid-run source changes require retry;
a failed run must not advance the last-completed marker. In-place writes may be
visible during a failed run: this design claims eventual recovery, not atomic
snapshot reads. Gate a reader or use staged generations if atomic visibility
becomes a requirement. Prevent concurrent CLI/watch writers from interleaving.

The representation fingerprint must cover chunker/enrichment versions, model
artifact/digest, preprocessing, dimensions, tokenizer/statistics version, and
inclusion scope/config. A changed fingerprint triggers the appropriate refresh
even if file hashes match. Same-dimensional model swaps cannot be detected by
dimension validation alone. Bound cache storage and avoid persisting raw secret
inputs in cache keys/logs; redaction changes also invalidate affected entries.

If sparse-only updates are chosen, preserve dense vectors only for proven matching
inputs and existing point IDs; update payload provenance/version consistently.
New or reshaped chunks need complete vectors, and obsolete IDs need cleanup.
A missing point or partial batch remains incomplete and recoverable. Explicitly
validate the selected Qdrant client's update behavior during implementation.

### Acceptance

| Case | Required evidence |
|---|---|
| Ordinary one-file edit | Ollama receives only changed/new enriched inputs; unchanged dense inputs are reused |
| Delete/rename | Old IDs disappear; renames invalidate path-dependent enriched inputs; sparse weights refresh |
| Model/enrichment/chunker change | Required inputs are regenerated despite unchanged file hashes; old chunk IDs are removed |
| Interrupted refresh + retry | Completion does not advance on failure; replay produces the same final point set/weights as a clean rebuild |
| Concurrent writers / source changes | One writer per scope; changed snapshot cannot be marked complete |
| No-op after a complete run | No embedding/upsert work when manifest and representation match |

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

## 2. External-repository evaluation and regression checks [M]

The old 0.895 hit@5 result came from 19 positive queries in a 23-query v6 report,
not all 39 queries now in the golden set. The latest reviewed main report is v8;
[the roadmap](../plan/mvp_roadmap.md#evidence-snapshot-and-branch-boundary) records
its scope and the changed negative-score semantics. None proves external quality.

Start with one bounded external Go-repo smoke run, then at least two repos with
different sizes/styles and 15–20 curated queries each. Use separate collections,
pin source revisions, and include definition, concept, behavior, required multi-file
evidence, and negative cases. Keep tuning queries separate from held-out checks.
Go-only evidence does not establish Rust or other language quality.

Every paired comparison records source revision/file hashes, query IDs and dataset
hash, config/language/repo filters, chunk IDs/counts, representation version, model
artifact/preprocessing, runtime versions, retrieval mode, and candidate/output limits.
Record hardware and warm/cold state for latency. Use the same frozen source for both
arms, even if implementation changes add files to this repository.

`compare.py` prints deltas; it does not enforce input compatibility or quality gates.
The planned CI wrapper must reject mismatched inputs, query errors, missing required
query classes, or thresholds applied to the wrong score family before judging quality.

Nightly/on-demand checks should preserve accepted hits/coverage and passing negatives
against a fixed comparable baseline. Track named existing failures separately. Agree
per-corpus floors from measured evidence; a historical self-corpus hit@5 floor of
0.85 is not an external-repo guarantee. Do not require the historical hybrid negative
pass rate of 1.00: its 0.55 threshold could not fail. Do not tune cutoffs on held-out
queries to manufacture success. Answer shape metrics stay report-only.

## 3. Sources-first and streaming answers [S–M, demand-driven]

The saved structural AL=5 run measured generation p50 23,861 ms / p95 46,434 ms.
Those are historical complete-generation measurements, not TTFT or a hardware-
independent promise. Keep source provenance and citation numbering intact:

1. Print the exact answer-context sources after retrieval, before warmup.
2. Stream tokens while collecting final text for existing answer metrics.
3. Measure source-display latency, warmup, first token, and complete generation
   separately on recorded hardware/model/prompt settings.
4. Verify cancellation, transport failure, incomplete-answer labeling, and eval
   behavior. Do not count a partial answer as successful generation.

Warm TTFT p50 below 2 s is a provisional target pending a reference benchmark.
Streaming does not remove prompt processing cost. Smaller-model routing is a
separate experiment requiring human content review as well as Tier B checks.

## 4. Local onboarding and corpus hygiene [M]

**Mode-aware diagnostics.** A planned `doctor` command and shared error mapping
should explain missing Qdrant/models with actionable remedies. Do not require a
generator for retrieval, Ollama for sparse search, or an existing collection for
first indexing. Answer generation already provides some Ollama recovery hints;
extend the uncovered paths instead of claiming every current error is raw.

**.gitignore support.** The walker already skips hidden files/directories,
configured directories (including `bin`), unsupported extensions, and configured
filename patterns. Add nested Git ignore/negation behavior with explicit precedence
against config exclusions. Verify newly excluded files are removed on re-index.

**Sensitive included content.** Hidden .env files and default-unsupported .pem/key
extensions are already excluded, but included source/YAML/JSON can contain credentials.
A redaction experiment must sanitize payloads and embedding inputs consistently,
preserve line mapping, invalidate previously indexed content, and assess false positives.
Do not declare the tool team-safe or enable an unmeasured default scanner simply by
adding filename patterns.

**Refusal diagnostics.** A phrase heuristic can mistake quoted text for refusal,
while a valid refusal can cite evidence. Keep it report-only and assess any new rule
on labeled cited refusals, uncited refusals, quoted phrases, and supported answers.
Phrase AND no-citations is not an accepted improvement without that evidence.

## 5. Conditional symbol lookup and CLI output [S–M]

Only revisit exact-symbol lookup if fresh results after main's identifier/type changes
still show definition misses. First test the existing name payload and richer naming
before introducing SQLite plus a second index lifecycle. The AST chunker does not
retain every declaration (for example package variables/constants).

```text
if query expresses an unambiguous definition lookup:
    matches = lookup with exact case and repo/package/receiver scope
    if exactly one valid match:
        merge that match with hybrid results and deduplicate
    else:
        return disambiguated candidates or fall back to hybrid
else:
    return hybrid results
```

Go names are case-sensitive and receiver/package context matters. Never pin an
arbitrary case-normalized match. Verify the ordinary positive/negative sets before
promotion. [Cheaper levers](../plan/cheaper_levers.md) covers other experiments.

`search --json` and `--context-lines` remain deferred standalone conveniences.
Reopen for a concrete scripting/source-reading need; neither is required for the
current reliability work. A future JSON shape needs a versioned schema/provenance;
context expansion must distinguish live file contents from the indexed snapshot.

## Deferred product expansion

The agent-first claim, MCP tools, `repos add/list/refresh`, automatic staleness UX,
shared services, and index-at-ref product commands have no active milestone.
[MCP's short decision record](../plan/mcp_server_mode.md) preserves its revisit
conditions. Pinned clean checkouts can support evaluation without building a new
index-at-ref command. Future workspace support needs stable repository/worktree IDs
and dirty/partial-index provenance; a HEAD SHA or latest timestamp alone is insufficient.
