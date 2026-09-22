# Product Roadmap — Reliable Local Code Search

> Updated 2026-09-22 after the docs_update review. This document owns sequencing
> and next-up tasks. `checklist.md` records the original phase plan.
> Milestones are planned unless checked complete with linked evidence; this is not a release claim.

## Product direction

ragcodepilot remains a **single-user, local code-search CLI** built on Go,
Qdrant, and Ollama, with hybrid retrieval and opt-in grounded answers. The
priority is useful, repeatable search on real repositories and affordable,
recoverable re-indexing. Keep the current core architecture.

MCP, an agent-first product pivot, repository registration/refresh commands,
and shared team deployment are **removed from the active roadmap**. They add
integration, identity, freshness, and process-lifecycle requirements without
validated demand. Existing repo filters remain supported; they do not imply
that a multi-repo workspace product has been accepted.

Local-first remains a constraint. Retrieval changes start as opt-in experiments,
keep the default path stable, and require comparable evidence before promotion.
A local sidecar is compatible with local-first but adds deployment complexity.

## Evidence snapshot and branch boundary

The fresh self-repository run on 2026-09-22 used merged revision `fed7f22`, a
clean archived source snapshot, and a new isolated collection. The
[run record](../eval/runs/2026-09-22-self-fed7f22/README.md) pins all inputs and
records limitations. No runtime source or golden labels changed for this refresh.

| Saved report | Queries / positives | hit@5 | Navigation hit@5 | Negative pass | Interpretation |
|---|---|---|---|---|---|
| `baseline_v6.json` | 23 / 19 | 17/19 = 0.8947 | 6/8 = 0.7500 | 4/4 at 0.55 | Historical; hybrid negative cutoff was ineffective |
| `baseline_v7.json` | 39 / 35 | 31/35 = 0.8857 | 20/24 = 0.8333 | 4/4 at 0.55 | Historical full set after structural additions |
| `baseline_v8.json` | 39 / 35 | 34/35 = 0.9714 | 23/24 = 0.9583 | 2/4 at 0.02 | Historical; predates later chunker changes |
| `baseline_v9.json` | 39 / 35 | 34/35 = 0.9714 | 23/24 = 0.9583 | 2/4 at 0.02 | Fresh pinned self baseline; 248 chunks, zero query errors |
| `baseline_v9_structural.json` | 16 / 16 | 15/16 = 0.9375 | 15/16 = 0.9375 | N/A | Same index; expected-file recall@5 0.6917 |

The implementation includes identifier tokens, named Go type/interface chunks,
and calibrated negative checks. v9 records one positive hit miss, two negative
failures, and eleven positives with incomplete expected-file coverage at top 5.
Historical reports are not controlled A/Bs against v9. The [external chi evaluation](../eval/external/chi-v5.2.3/README.md) is complete:
20 queries, zero errors, file hit@5 16/16, negatives 1/4. Its index audit found
382 generated chunks but only 371 retained points because chunk IDs collide.
Required source ranges are complete at top 5 for only 9/16 full-run positives.
This is evidence of a correctness gap, not broad quality acceptance.

The old 0.55 cosine threshold cannot fail for two-list RRF with k=60 (maximum
about 0.0333). Replaying v6's saved scores at 0.02 fails the same two negatives
as v8: `oauth_middleware_negative` and `grpc_gateway_router_negative`. The 100%
to 50% change is not evidence of a new retrieval regression. Track those failures;
do not adjust thresholds to make the report green. RRF agreement is a retrieval
diagnostic, not a calibrated probability or a measure of answer faithfulness.

See [evaluation guidance](../eval/README.md) for snapshot and comparison rules.

## Milestones and dependencies

The near-term queue has **three deliverables**: a fresh baseline plus one external
repository, dense embedding reuse with reliable recovery, and actionable errors
plus .gitignore handling. M0 is complete. The chunk-ID collision fix and fresh
self/chi point-survival recheck are complete; M3 cache work is next. M4 can proceed
independently. Active milestone IDs retain
their original meanings; gaps refer to retired proposals.

| ID | Scope | Size | Exit criterion | Status |
|---|---|---|---|---|
| M0 | Fresh evidence on self + one external repo | S–M | Pinned inputs, saved reports, named failure inventory, manual comparison checklist | Complete as measurement; known failures retained |
| M3 | Dense reuse + retry/cleanup | M | Cache reuse and invalidation, interrupted-run replay, stale-ID cleanup, one writer per index | Next: recovery/cache implementation |
| M4 | Existing-command errors + .gitignore | S–M | Actionable operation-specific failures; nested ignore/exclusion behavior verified | Independent |
| M1 | Sources-first terminal output | S | Exact answer sources appear before warmup and generation | Optional small UX change |

## M0 — Fresh baseline and one external repository [S–M]

- [x] Record the reconciled code revision and freeze the source snapshot, query
  set, config/filters, model artifact/preprocessing, representation version, and limits.
- [x] Capture fresh full and structural self-corpus reports with current score
  semantics: [v9 evidence and failure inventory](../eval/runs/2026-09-22-self-fed7f22/README.md).
- [x] Evaluate **one representative external Go repository**, in its own collection,
  with a small carefully labeled query set (about 15–20 positive/negative cases).
  [chi v5.2.3 evidence](../eval/external/chi-v5.2.3/README.md): 20 frozen cases,
  seven multi-file positives, four negatives, and a complete failure inventory.
- [x] Complete the failure inventory across self and external repos; the self-run
  record already includes a manual regression checklist. Retain the checklist for
  changes to retrieval. Retain comparable control/candidate reports and inspect
  query errors, accepted hits/coverage, negatives, and latency.

**Exit:** reproducible reports and a named failure inventory. No new retrieval
component, model sweep, or graph prototype is required. Add another external repo
before making claims of generalization or promoting a retrieval change broadly;
it is not a dependency for the first indexing improvement. Automated retrieval CI
waits until this manual process is stable and worth automating.

## M3 — Dense reuse and reliable retry/cleanup [M]

**Chunk-identity correctness [S–M] — complete.** The original chi run found seven
same-file name collision groups and 11 overwritten chunks. The fix includes receiver
and declaration identity, bumps the representation version, removes old points on
same-hash refresh, and passes fresh self/chi point-survival checks:
[recheck record](../eval/runs/2026-09-23-idfix/README.md).

- [x] Fix receiver/declaration identity in deterministic IDs; cover same-name methods
  on different receivers and a package function sharing a method name.
- [x] Bump the representation version and define cleanup/rebuild for affected points.
- [x] Reindex the self and chi corpora; all generated IDs are unique and no points
  are lost. Unchanged evaluation labels preserve the original known failures.

Proceed to dense-cache recovery and reuse. The original [chi index audit](../eval/external/chi-v5.2.3/index-audit.json)
remains as the defect record.

Detailed contract: [local reliability](../improvement/production_readiness_and_features.md).

- [ ] Cache dense vectors by model artifact, preprocessing, and exact enriched
  input. Reuse unchanged inputs; correctly invalidate model/enrichment/chunker changes.
- [ ] Retain the existing full sparse refresh and complete-point upsert path.
  Do not add sparse-only updates, staged collections, or atomic index swapping.
- [ ] Use a durable incomplete-run marker with the input/representation fingerprint.
  A failed run must remain detectable and replay the affected scope rather than
  skipping work because some file hashes were already written.
- [ ] Verify deletions, renames, obsolete chunk cleanup, interrupted-run retries,
  source changes during indexing, and exclusion of overlapping writers.
- [ ] Measure dense calls, sparse writes, and total time independently; a dense
  cache does not make all re-index work proportional to the changed files.

**Exit:** a warm-cache one-file edit embeds only changed/new inputs, and retry
produces the same final index as a clean rebuild. Claim recovery after a complete
run, not atomic visibility during updates or verified large-corpus capacity.

## M4 — Actionable errors and .gitignore [S–M]

- [ ] Improve errors in existing `index`/`search` commands with an actionable fix
  for missing relevant services/models. No standalone `doctor` command yet.
- [ ] Keep checks operation-specific: sparse search does not need an embedder,
  retrieval does not need a generator, and first indexing creates a collection.
- [ ] Respect nested .gitignore rules and negation while preserving explicit config
  exclusions; document precedence and remove newly excluded files on re-index.
- [ ] Preserve current hidden-file/extension exclusions. Automatic secret scanning
  and redaction are deferred; exclusions are not a guarantee of secret-free content.

**Exit:** common failures explain the remedy, and inclusion/exclusion behavior is
verified. Shared/team deployment and a broader diagnostics framework remain out of scope.

## M1 — Sources-first output [S, optional]

If answer-mode waiting is a frequent annoyance, show the exact selected sources
with matching citation numbers immediately after retrieval, before warmup/generation.
Verify that generation errors leave sources readable and the default search output
unchanged. This small change does not need token streaming or a new model.

Streaming, partial-stream handling, TTFT targets, model routing, and refusal-heuristic
changes remain deferred until sustained `--answer` usage warrants them. Keep current
answer defaults and report-only refusal diagnostics; review questionable answers manually.

## Completed work and history

P1 evaluation foundation, P2 hybrid retrieval, P5 v0 answer mode, file-hash/index-
version change detection, and watch mode remain implemented. See the linked
phase plans and [historical checklist](checklist.md); indexing still rebuilds
all dense vectors in the current scope when a run detects changes.

- **2026-05-14:** answer mode was pulled forward after retrieval cleared the
  historical self-corpus floor; this remains an opt-in CLI capability.
- **2026-05-28:** navigation misses motivated a GraphRAG-first hypothesis.
  The AL=8 experiment increased latency without demonstrating content benefit
  through shape metrics alone. Defaults remain unchanged.
- **2026-07-06:** the initial docs_update proposal introduced milestones and an
  agent-integration pivot. That proposed ordering is superseded here.
- **2026-09-22:** keep the local CLI scope, retire M2, prioritize evidence and
  indexing reliability, and correct the acceptance gates before further builds.
  Further scope reduction limits the active queue to M0/M3/M4; broader experiments
  and product surfaces stay outside the plan. Detailed deferred proposals were
  removed; evaluation lessons and historical reports remain available.

## Out of current scope

These items have no current delivery commitment or scheduled milestone. Removed proposals
remain in Git history. Reopen only when the stated need is demonstrated:

| Area | Why excluded / revisit condition |
|---|---|
| MCP, agent-first integration, multi-repo workspaces, shared deployment | No validated demand for the added identity, lifecycle, and operational work; revisit for repeated workflows with demonstrated benefit. |
| GraphRAG, rerankers, model sweeps, symbol databases, non-Go AST builds | No fresh evidence yet justifies another retrieval component; revisit the smallest change supported by repeated named misses. |
| Streaming, model routing, new CLI/UI surfaces, standalone doctor | Keep existing commands useful first; revisit when sustained usage exposes a concrete limitation. |
| Broader benchmark collections, retrieval CI, automated answer judging | Start with one external repo and manual comparisons; expand before generalization claims or when repeated checks warrant automation. |
| Secret scanners and new refusal heuristics | Preserve exclusions and manual review; revisit for demonstrated failures and a tested policy. |
| Sparse-only writes and atomic index publication | Keep recoverable in-place updates; revisit for measured write costs or a real atomic-read requirement. |
| Custom vector database and multimodal retrieval | Outside the local code-search goal; requires a separate explicit objective. |

## Related docs

- [System design](system_design.md) — reconciled architecture and saved-evidence limitations.
- [Production readiness](../improvement/production_readiness_and_features.md) — indexing and local reliability contracts.
- [Evaluation](../eval/README.md) — evidence, metrics, and comparison workflow.
- [Retrieval decisions](../knowledge/retrieval_quality_decisions.md) — historical tradeoffs and current evidence notice.
- [Architecture decisions](../knowledge/architecture_decisions.md) — CLI/process-shape decisions.
