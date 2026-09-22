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

Reviewed against `docs_update` at `d3775cf` and local `main` at `a3ac8ff` on
2026-09-22. The branch is one commit ahead and 26 behind main; this documentation
update does not merge code or regenerate benchmarks.

| Saved report | Queries / positives | hit@5 | Navigation hit@5 | Negative pass | Interpretation |
|---|---|---|---|---|---|
| `baseline_v6.json` on this branch | 23 / 19 | 17/19 = 0.8947 | 6/8 = 0.7500 | 4/4 at 0.55 | Historical; hybrid negative cutoff was ineffective |
| `baseline_v7.json` on this branch | 39 / 35 | 31/35 = 0.8857 | 20/24 = 0.8333 | 4/4 at 0.55 | Historical full set after structural queries were added |
| `main:docs/eval/baseline_v8.json` | 39 / 35 | 34/35 = 0.9714 | 23/24 = 0.9583 | 2/4 at 0.02 | Latest saved hybrid baseline on reviewed main; not a fresh run |

Main includes additive identifier tokens, named Go type/interface chunks, and
mode-calibrated negative checks absent from this branch. The v8 report predates
some later main changes; it is not proof of current HEAD performance. Reconcile
with main and capture a fresh baseline before choosing new retrieval work.

The old 0.55 cosine threshold cannot fail for two-list RRF with k=60 (maximum
about 0.0333). Replaying v6's saved scores at 0.02 fails the same two negatives
as v8: `oauth_middleware_negative` and `grpc_gateway_router_negative`. The 100%
to 50% change is not evidence of a new retrieval regression. Track those failures;
do not adjust thresholds to make the report green. RRF agreement is a retrieval
diagnostic, not a calibrated probability or a measure of answer faithfulness.

See [evaluation guidance](../eval/README.md) for snapshot and comparison rules.

## Milestones and dependencies

IDs from the July proposal are retained to avoid reusing M2 for unrelated work.
The intended order is **M0 -> M3**, with M4 local diagnostics/hygiene possible
alongside that work. M1 follows demonstrated answer-mode demand. M5 opens only
for failures established by M0/M3; it does not depend on completing every UX item.

| ID | Scope | Size | Exit criterion | Status |
|---|---|---|---|---|
| M0 | Refresh evidence and classify failures | S–M | Comparable baseline, external-repo smoke report, named failure inventory | Next |
| M3 | Indexing cost/recovery and external evaluation | M–L | Dense reuse + retry cases verified; at least two external sets with pinned inputs; meaningful regression policy | After M0 |
| M4 | Local onboarding and corpus hygiene | M | Actionable, mode-aware errors; documented .gitignore/exclusion behavior | Can proceed alongside M0/M3 |
| M1 | Usable terminal answers | S–M | Sources shown first; streamed output and failure handling verified; TTFT measured | Optional, demand-driven |
| M2 | Agent integration | — | Removed from active scope; see deferred decisions | Retired |
| M5 | Targeted retrieval experiments | Per lever S–L | Named failures improve on fresh paired evaluations without accepted-case regressions | Gated on M0/M3 |

## M0 — Refresh evidence and classify failures [S–M]

- [ ] Reconcile this branch's evidence and documentation with main before a
  baseline run. Record the tested code revision and representation version.
- [ ] Freeze source snapshots, query sets, config/filters, model artifacts and
  preprocessing, runtime versions, and candidate limits; record chunk counts.
- [ ] Capture fresh full and structural reports with the same retrieval path
  and negative-score semantics that later candidates will use.
- [ ] Run one external Go repository smoke evaluation using its own isolated
  collection. This bounded run does not wait for a large-scale indexing fix.
- [ ] Classify misses as definition lookup, missing candidate, ranking,
  incomplete multi-file evidence, negative-query false match, or stale data.
- [ ] Record which existing failures justify M3 or an M5 experiment. Keep
  GraphRAG deferred unless a reachability study of current misses warrants it.

**Exit:** evidence manifests and reports retained, named failures recorded, and
next experiment chosen or explicitly deferred. No new retrieval layer is built.

## M3 — Indexing reliability and external evidence [M–L]

Detailed contracts: [production readiness](../improvement/production_readiness_and_features.md).

- [ ] Reuse dense vectors only when enriched input, model artifact, and
  preprocessing match. Ordinary one-file edits embed that file's changed inputs;
  model/representation changes must invalidate reuse when required.
- [ ] Specify completed-generation state and retry behavior for partial sparse
  refreshes, deletions, and chunk-shape changes. Verify interrupted refresh + retry.
- [ ] Measure dense calls, sparse writes, and elapsed time independently. Corpus
  hashing/chunking/statistics/sparse refresh can remain proportional to scope size.
- [ ] Extend to at least two external Go repos of different sizes/styles, each
  with 15–20 curated queries and frozen revisions, isolated collections, and
  positive/negative labels. Keep a held-out set separate from tuning.
- [ ] Implement nightly/on-demand retrieval checks against comparable pinned
  baselines: zero query errors, no new negative failures under fixed score-aware
  thresholds, and no lost accepted hits/coverage without explicit investigation.
  Preserve known failures as visible follow-up work, not successful acceptance.
- [ ] Agree corpus-specific quality floors after measuring each corpus. The old
  self-corpus 0.85 hit@5 floor is a historical guardrail, not an external quality
  guarantee. Report per-query deltas, MRR, recall, and latency alongside aggregates.

**Exit:** repeatable cost/recovery evidence and external reports support the
claims. Large-corpus capacity remains unverified until measured at that size.

## M4 — Local onboarding and corpus hygiene [M]

- [ ] `doctor` and shared error mapping check only dependencies needed by the
  requested operation: sparse search need not require an embedder, retrieval need
  not require a generator, and first indexing need not require an existing collection.
- [ ] Respect .gitignore semantics, including nested rules and negation; retain
  explicit config exclusions and document precedence. Re-index removes newly excluded data.
- [ ] Evaluate credentials inside included source/config files. Hidden .env files
  and default-excluded key extensions already have walker protection. Any future
  redaction policy must cover payloads and embeddings, preserve line provenance,
  invalidate old data, and measure false positives before default enablement.
- [ ] Keep refusal detection report-only. Validate proposed changes against
  labeled quoted phrases, cited refusals, and real answers rather than assuming
  that a citation rules out a refusal.

**Exit:** clean-machine first index/search succeeds with actionable failures
for relevant missing dependencies; corpus inclusion/exclusion behavior is verified.
No team/shared-service readiness claim follows from this milestone.

## M1 — Usable terminal answers [S–M]

The historical AL=5 structural run measured generation p50 23,861 ms and p95
46,434 ms. It did not measure first-token latency. Sources-first output is useful
without adding another retrieval component.

- [ ] Show the exact answer-context sources, with matching citation numbers,
  immediately after retrieval and before model warmup/generation.
- [ ] Stream tokens while accumulating final text for existing answer metrics.
- [ ] Verify cancellation, generation failure, partial streams, and non-streaming
  eval compatibility. Label incomplete answers clearly.
- [ ] Measure source-display latency, cold startup, warm TTFT, and total generation
  separately on recorded hardware/model/prompt budgets. Warm p50 TTFT below 2 s
  is a provisional target; benchmark it before adopting it as a release gate.

Streaming changes delivery; it does not guarantee faster prompt evaluation or
identical elapsed time. Keep the current model and answer-limit defaults unless
human content review plus latency measurements justify a separate change.

## M5 — Targeted retrieval experiments [S–L, conditional]

Select the least costly experiment that addresses the observed failure class;
this is not a required ladder of features to build.

| Candidate | Size | Revisit condition |
|---|---|---|
| Exact-symbol lookup | S–M | Definition misses remain after main's identifier/type changes; test existing name payload before adding a second store |
| Alternative embedding model | S–M | Relevant evidence is absent from candidates and a compatible local model fits latency/resource budgets |
| Cross-encoder reranker | M | Required evidence is present in a fixed candidate pool but missing from top-5; external paired runs confirm value |
| GraphRAG | L | Missing structural evidence is reachable via supported edges, cheaper candidates do not solve it, and named coverage gains justify the store/extractor |
| Non-Go AST chunking | M | A separate non-Go evaluation demonstrates chunk-boundary failures; Go-only external sets cannot establish this |

[Cheaper levers](cheaper_levers.md) owns model/reranker experiments.
[GraphRAG](graphrag.md) owns its reachability and required-evidence gates.
Historical structural hit@5 is already 14/16; use completeness metrics rather
than requiring binary hit improvement on queries that already pass.

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

## Deferred decisions

| Item | Decision | Revisit only when |
|---|---|---|
| MCP / agent-first pivot | Removed from active scope | Repeated real coding tasks need external indexed retrieval and a controlled comparison shows task-level benefit |
| Multi-repo workspace commands | Removed from active scope | Repeated cross-repo workflows justify stable repo/worktree identity, completed-index freshness, and conflict handling |
| Shared/team deployment | Out of scope | Explicit demand includes access control, isolation, lifecycle, and operational ownership |
| CLI --json / --context-lines | Deferred standalone UX | A concrete scripting or source-reading workflow needs it; no MCP dependency |
| REPL / TUI / IDE plugin / HTTP daemon | Deferred | Existing CLI workflow is a demonstrated limit |
| Multi-provider answers / model routing | Deferred | Local answer-mode usage establishes a quality or latency need |
| Tier C answer-content evaluation | Deferred tooling | Manual answer review needs automation; Tier B remains a shape diagnostic |
| Custom vector DB / multi-modal embeddings | Deferred | Explicit learning objective or measured capability gap justifies the investment |

The former [MCP design](mcp_server_mode.md) is retained only as a short deferred
record. It carries no implementation checklist or committed release scope.

## Related docs

- [System design](system_design.md) — architecture and branch/main boundary.
- [Production readiness](../improvement/production_readiness_and_features.md) — indexing and local reliability contracts.
- [Evaluation](../eval/README.md) — evidence, metrics, and comparison workflow.
- [Retrieval decisions](../knowledge/retrieval_quality_decisions.md) — historical tradeoffs and current evidence notice.
- [Architecture decisions](../knowledge/architecture_decisions.md) — CLI/process-shape decisions.
