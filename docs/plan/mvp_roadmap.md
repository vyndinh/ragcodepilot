# Product Roadmap — Reliable Local Code Search

> Updated 2026-09-22 after the docs_update review. This document owns sequencing
> and next-up tasks. `checklist.md` records the original phase plan.
> These are planned changes, not implementation or release claims.

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

Reconciled with fetched `origin/main` at `a3ac8ff` on 2026-09-22. This branch
now includes that implementation and its saved reports. The PR adds documentation
only relative to main; no retrieval benchmark was rerun for this update.

| Saved report | Queries / positives | hit@5 | Navigation hit@5 | Negative pass | Interpretation |
|---|---|---|---|---|---|
| `baseline_v6.json` on this branch | 23 / 19 | 17/19 = 0.8947 | 6/8 = 0.7500 | 4/4 at 0.55 | Historical; hybrid negative cutoff was ineffective |
| `baseline_v7.json` on this branch | 39 / 35 | 31/35 = 0.8857 | 20/24 = 0.8333 | 4/4 at 0.55 | Historical full set after structural queries were added |
| `baseline_v8.json` | 39 / 35 | 34/35 = 0.9714 | 23/24 = 0.9583 | 2/4 at 0.02 | Latest saved hybrid baseline on reviewed main; not a fresh run |

The reconciled implementation includes additive identifier tokens, named Go
type/interface chunks, and mode-calibrated negative checks. The v8 report predates
some later chunker changes; it is not proof of current HEAD performance. Capture
a fresh baseline before choosing new retrieval work.

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
plus .gitignore handling. M0 precedes M3; M4 can proceed independently. IDs from
the July proposal remain so historical references do not change meaning.

| ID | Scope | Size | Exit criterion | Status |
|---|---|---|---|---|
| M0 | Fresh evidence on self + one external repo | S–M | Pinned inputs, saved reports, named failure inventory, manual comparison checklist | Next |
| M3 | Dense reuse + retry/cleanup | M | Cache reuse and invalidation, interrupted-run replay, stale-ID cleanup, one writer per index | After M0 |
| M4 | Existing-command errors + .gitignore | S–M | Actionable operation-specific failures; nested ignore/exclusion behavior verified | Independent |
| M1 | Sources-first terminal output | S | Exact answer sources appear before warmup and generation | Optional small UX change |
| M2 | Agent integration | — | Removed from active scope | Retired |
| M5 | Retrieval layers and dedicated prototypes | S–L if reopened | Repeated named failures establish a concrete need | Deferred; no scheduled research/build |

## M0 — Fresh baseline and one external repository [S–M]

- [ ] Record the reconciled code revision and freeze the source snapshot, query
  set, config/filters, model artifact/preprocessing, representation version, and limits.
- [ ] Capture fresh full and structural self-corpus reports with current score
  semantics. Saved v8 evidence is useful history, not a substitute for this run.
- [ ] Evaluate **one representative external Go repository**, in its own collection,
  with a small carefully labeled query set (about 15–20 positive/negative cases).
  Include required multi-file evidence; retain per-query failures and source revision.
- [ ] Classify recurring misses, then document a manual regression checklist for
  changes to retrieval. Retain comparable control/candidate reports and inspect
  query errors, accepted hits/coverage, negatives, and latency.

**Exit:** reproducible reports and a named failure inventory. No new retrieval
component, model sweep, or graph prototype is required. Add another external repo
before making claims of generalization or promoting a retrieval change broadly;
it is not a dependency for the first indexing improvement. Automated retrieval CI
waits until this manual process is stable and worth automating.

## M3 — Dense reuse and reliable retry/cleanup [M]

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

## M5 — Deferred retrieval work

No dedicated graph study, symbol database, embedding sweep, reranker prototype, or
non-Go chunker build is in the near-term queue. Record actual misses during normal
use and M0. Reopen only the smallest experiment addressing a repeated failure:

| Candidate | Revisit condition |
|---|---|
| Exact-symbol lookup | Repeated definition misses after existing identifier/type support; try current name payload first |
| Alternative embeddings | Required evidence repeatedly absent from candidates |
| Reranker | Required evidence repeatedly retrieved but below the useful context window |
| GraphRAG | Missing evidence requires supported structural relationships and simpler fixes have not helped |
| Non-Go AST chunking | Active use of that language exposes concrete chunk-boundary failures |

[Cheaper levers](cheaper_levers.md) and [GraphRAG](graphrag.md) retain conditional
design notes only. If reopened, use fresh paired evidence and required-evidence
coverage; historical structural hit@5 already passes 14/16 queries.

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
  and product surfaces stay deferred.

## Deferred decisions

| Item | Decision | Revisit only when |
|---|---|---|
| MCP / agent-first pivot | Removed from active scope | Repeated real coding tasks need external indexed retrieval and a controlled comparison shows task-level benefit |
| Multi-repo workspace commands | Removed from active scope | Repeated cross-repo workflows justify stable repo/worktree identity, completed-index freshness, and conflict handling |
| Shared/team deployment | Out of scope | Explicit demand includes access control, isolation, lifecycle, and operational ownership |
| Additional external benchmark repos | Deferred expansion | Needed before generalization claims or broader retrieval promotion |
| Nightly/on-demand retrieval CI | Deferred automation | Manual pinned-input evaluation is stable and recurring checks justify automation |
| Standalone doctor command | Deferred | Existing-command error guidance proves insufficient |
| Token streaming / TTFT targets | Deferred | Sustained answer-mode use makes sources-first output insufficient |
| Secret scanning/redaction | Deferred | Included-content exposure and a tested policy justify false-positive/invalidation costs |
| Refusal-heuristic changes | Deferred | Labeled recurring mistakes justify changing the report-only diagnostic |
| Sparse-only updates / atomic index publication | Deferred | Measured sparse-write cost or a real atomic-read requirement justifies added complexity |
| CLI --json / --context-lines | Deferred standalone UX | A concrete scripting or source-reading workflow needs it; no MCP dependency |
| REPL / TUI / IDE plugin / HTTP daemon | Deferred | Existing CLI workflow is a demonstrated limit |
| Multi-provider answers / model routing | Deferred | Local answer-mode usage establishes a quality or latency need |
| Tier C answer-content evaluation | Deferred tooling | Manual answer review needs automation; Tier B remains a shape diagnostic |
| Custom vector DB / multi-modal embeddings | Deferred | Explicit learning objective or measured capability gap justifies the investment |

The former [MCP design](mcp_server_mode.md) is retained only as a short deferred
record. It carries no implementation checklist or committed release scope.

## Related docs

- [System design](system_design.md) — reconciled architecture and saved-evidence limitations.
- [Production readiness](../improvement/production_readiness_and_features.md) — indexing and local reliability contracts.
- [Evaluation](../eval/README.md) — evidence, metrics, and comparison workflow.
- [Retrieval decisions](../knowledge/retrieval_quality_decisions.md) — historical tradeoffs and current evidence notice.
- [Architecture decisions](../knowledge/architecture_decisions.md) — CLI/process-shape decisions.
