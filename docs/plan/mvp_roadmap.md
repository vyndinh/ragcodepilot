# Product Roadmap — MVP → real-world use

> **Restructured 2026-07-06.** The phase-numbered plan (P1–P6) reorganized into
> product **milestones (M0–M5)** after the external system review
> ([`../improvement/production_readiness_and_features.md`](../improvement/production_readiness_and_features.md))
> concluded the binding constraints are now product surface, scaling
> correctness, and evidence quality — not another ranking algorithm. Old phase
> numbers are kept as cross-references (P1, P2, …) because other docs cite
> them; the mapping is in "What changed vs the phase plan" below.
>
> `docs/plan/checklist.md` remains the historical record of the original phase
> plan. **This document is canonical for sequencing and next-up tasks.**

---

## Product thesis

ragcodepilot is a **local-first retrieval engine for codebases** — a
persistent, hybrid (semantic + keyword) index that outlives any session, with
opt-in grounded answers.

**Who it serves, in order:**

1. **Coding agents** (Claude Code, Cursor, any MCP client) — the realistic
   heavy user: many retrieval calls per task, needs structured results, values
   an index it doesn't have to rebuild by grepping. This is the wedge: agents
   bundle LLMs but lack a persistent local index.
2. **Developers at the terminal** — cross-repo semantic search and grounded
   answers; latency and onboarding decide whether this becomes a habit.

**Constraints that hold:** local-first (no cloud APIs by default), default
path stays byte-stable (new capability behind opt-in flags), every retrieval
change is measured against committed baselines.

Retrieval quality stays a first-class discipline (the eval harness is the
scoreboard), but quality *levers* are now **gated on evidence** (M0, M3)
instead of being the spine of the roadmap.

---

## Milestone summary

| # | Milestone | Goal | Size | Exit criterion | Status |
|---|---|---|---|---|---|
| M0 | **Decide the navigation bet** | Run the two GraphRAG prerequisites + a symbol-table spike; decide build / rescope / shelve | S | Decision recorded in `graphrag.md` + here | ▶ **Next** |
| M1 | **Usable answers** | Streaming `--answer`, sources shown immediately | S | Warm time-to-first-token p50 < 2 s; sources printed before the answer streams | Queued |
| M2 | **Agent integration** | `--json`, `--context-lines`, MCP server v0 | M | MCP `search_code` passes the CLI-parity check; a Claude Code session uses it unprompted (dogfooding transcript recorded) | Queued |
| M3 | **Real-repo trust** | Diff-proportional re-indexing; external-repo eval; CI floors | M–L | 1-file change re-embeds only that file's chunks; ≥2 external golden sets + baselines committed; CI fails on `hit@5 < 0.85` or `negative_pass < 1.0` | Queued |
| M4 | **Team-ready hardening** | `doctor`, secrets redaction, `.gitignore`, multi-repo UX | M | First-run failure modes produce actionable fixes; `repos list` shows staleness | Queued |
| M5 | **Retrieval levers, evidence-scoped** | Symbol fast path / code embedder / reranker / GraphRAG — whichever M0 + M3 evidence justifies | M–L | Per-lever gates in `cheaper_levers.md` / `graphrag.md` | Gated on M0, M3 |

Completed work (P1 eval foundation, P2 hybrid search, P5 v0 answer mode,
re-indexing + watch mode) is recorded in "Completed phases" below.

---

## M0 — Decide the navigation bet [S]

**Why first:** the biggest open bet (GraphRAG, sized L) has been blocked since
2026-05-28 on two tasks its own design doc sizes at under a day combined. They
are still the cheapest information-per-hour available, and their outcome
shapes M5. Do not let an L-sized plan idle behind S-sized questions again.

**Checklist:**

- [ ] **Reachability dry-run** (paper): for each of the 16 structural queries,
  is the missing chunk reachable via v0 edges from a hybrid top-50 chunk?
  Establishes GraphRAG's ceiling. (Spec: `graphrag.md` "Prerequisites".)
- [ ] **AL=5 vs AL=8 dogfooding**: read 3–5 multi-chunk answers side by side;
  judge content completeness. (Spec: `graphrag.md` "Prerequisites".)
- [ ] **Symbol fast-path spike**: count how many navigation/structural misses
  are plain "where is X defined" — answerable by an exact symbol-table lookup
  (the `defines` edge alone, sized S–M) without graph expansion. (Spec:
  `production_readiness_and_features.md` §2.2.)
- [ ] Record the decision: GraphRAG **build / rescope / shelve**, symbol fast
  path **go / no-go**, in `graphrag.md` and this doc's M5 section.

**Exit:** decisions recorded. No retrieval code written in M0.

---

## M1 — Usable answers [S]

**Why:** answer mode works but p50 generation is ~24 s of silent synchronous
waiting (measured, `retrieval_quality_decisions.md` §2.5) — the largest felt
product gap. Streaming was a "v1 candidate"; it is now the next shippable item.

**Checklist:**

- [ ] Print retrieval results immediately, then stream the answer below them.
- [ ] `stream: true` generation in `OllamaGenerator`; tokens printed as they
  arrive. Determinism story unchanged (greedy decoding; the final text is the
  metric input, streaming only changes delivery).
- [ ] Timings gain a time-to-first-token measure; eval report includes it.
- [ ] README: answer-mode section updated (expectations, `OLLAMA_KEEP_ALIVE`).

**Exit:** warm TTFT p50 < 2 s on the reference query set; total generation
time unchanged (streaming is delivery, not speed).

**Deliberately not in M1:** model routing / smaller default generative model —
revisit only if streaming leaves the experience unacceptable.

---

## M2 — Agent integration [M]

**Why:** repositions the product as the retrieval backend agents call (the
product thesis), instead of a CLI competing with grep. Design is locked in
[`mcp_server_mode.md`](mcp_server_mode.md); `--json` is a shared prerequisite.

**Checklist:**

- [ ] Shared JSON serializer for `SearchResult` + content truncation guard
  (one serializer for both `--json` and MCP — build once).
- [ ] `search --json` flag (stable schema) and `--context-lines N`.
- [ ] `serve --mcp`: stdio server, `search_code` + `list_collections` tools,
  startup embedder warmup, actionable errors when Qdrant/Ollama are down.
- [ ] Parity check: for 5 golden queries, MCP results == CLI results.
- [ ] Dogfood in a real Claude Code session; record transcript findings and
  the v1 tool wishlist in `mcp_server_mode.md`.

**Exit:** parity passes; dogfooding shows the agent choosing `search_code`
unprompted for code-location questions.

---

## M3 — Real-repo trust [M–L]

**Why:** everything so far is validated on one 182-chunk self-indexed corpus,
and any change triggers a full-corpus re-embed. Neither survives contact with
a real repository. This milestone makes the system's claims true off this repo.

**Checklist:**

- [ ] **Diff-proportional re-indexing** (spec:
  `production_readiness_and_features.md` §1.1): embed only changed files;
  rebuild sparse vectors locally for unchanged chunks and update only the
  sparse named vector in Qdrant (or a content-addressed embedding cache).
  Measured exit: a 1-file change makes Ollama embedding calls for that file's
  chunks only.
- [ ] **External-repo eval** (spec: §1.2): 2–3 well-known OSS Go repos,
  15–20 golden queries each (`golden_external_<repo>.yaml`), baseline matrix
  committed. Any future retrieval change must not regress externally.
- [ ] **CI regression gate** (spec: §1.4): nightly/on-demand job (Qdrant +
  Ollama services) enforcing the agreed floors — `hit@5 ≥ 0.85`,
  `negative_pass_rate = 1.0`. Answer metrics stay report-only.
- [ ] Scale notes: record ingest time, chunk counts, IDF cost on the largest
  external repo — first real data against the 200K-chunk design envelope.

**Exit:** the three checklist exits above; `system_design.md`'s "known gap"
paragraph updated or removed.

---

## M4 — Team-ready hardening [M]

**Why:** three-service stack + raw gRPC errors = failed first runs; plaintext
secrets in payloads block any shared use. All specs in
`production_readiness_and_features.md` §§1.5–1.7, 2.4.

**Checklist:**

- [ ] `ragcodepilot doctor` — checks Qdrant, Ollama, models, collection
  dimension; prints the exact fix per failure. `index`/`search` reuse the
  checks for actionable errors.
- [ ] Secrets: default skip patterns (`.env*`, `*.pem`, key files) + optional
  redact-on-match scan, default on.
- [ ] Walker respects the repo's `.gitignore` as an additional skip source.
- [ ] Multi-repo UX: `repos add/list/refresh`, indexed-commit SHA stored per
  repo, staleness display ("api: 14 commits behind").
- [ ] Refusal heuristic tightened (refusal phrase AND no citations).

**Exit:** a new user on a clean machine reaches a successful first search with
no error message that lacks a fix command; `repos list` shows staleness.

---

## M5 — Retrieval levers, evidence-scoped [M–L, gated]

**Why last (as a build): the levers are already well-designed; what's missing
is evidence about which one pays.** M0 decides the navigation bet; M3's
external eval decides whether wins generalize. Enter M5 with both in hand.

The candidate ladder, in cost order (designs already written):

1. **Symbol fast path** [S–M] — if the M0 spike shows most navigation misses
   are exact-lookup-shaped (`production_readiness_and_features.md` §2.2).
2. **Code-specialized embedding model** [S–M] — config + re-index + eval;
   gated on local availability (`cheaper_levers.md` Lever 1).
3. **Cross-encoder reranker** [M] — if the recall gap (`recall@10 − recall@5 ≥
   0.10`) persists on the external corpora (`cheaper_levers.md` Lever 2).
4. **GraphRAG** [L, possibly rescoped smaller by M0/levers] — for whatever
   query classes the cheaper rungs provably cannot reach (`graphrag.md`).
5. **Rust AST chunker** [M] — when multi-language coverage becomes the binding
   gap (external eval will show this honestly for the first time).

**Rule:** each rung runs behind an opt-in flag, is judged per-query on the
full + structural + external sets with the standing no-regression gates, and
its result (win or loss) is committed to `docs/eval/` before the next rung.

---

## What changed vs the phase plan (2026-07-06 restructure)

| Old | Disposition |
|---|---|
| P1 eval foundation, P2 hybrid search, P5 v0 answer mode | ✅ Done — see "Completed phases" |
| P3 reranking | → M5 rung 3, unchanged gates; still parked, trigger now includes external-eval confirmation |
| P3.5 Rust AST chunker | → M5 rung 5 |
| P4 UX polish | Split: streaming → M1; `--json` / `--context-lines` → M2; grouping-by-file deferred |
| P5 v1 items | Streaming → M1; multi-provider, Tier C judge, refusal guardrail → deferred (unchanged triggers in `phase5_v0_answer_mode.md`) |
| P6 GraphRAG ("▶ Next") | → **M0 decides**, then M5 rung 4. The design doc is unchanged and remains the build spec if the evidence says build |
| (new) MCP server | → M2, design at `mcp_server_mode.md` |
| (new) diff-proportional re-index, external eval, CI gate, doctor/secrets/repos UX | → M3, M4, from the production-readiness review |

**What did *not* change:** the eval-first discipline, the no-regression gates,
the corpus-stability methodology, local-first, and every already-recorded
decision (BM25, stemming, test-file exclusion, answer-mode v0 freeze).

---

## Pivot history

**Pivot — 2026-05-14.** Phase 5 v0 pulled ahead of Phases 3 and 4 after
Phase 2's hit@5 of 0.895 cleared the vision review's "retrieval is strong
enough to feed an LLM" gate and the user confirmed the RAG product direction.

**Pivot — 2026-05-28.** Phase 5 v0 dogfooding surfaced test-file pollution
(fixed via `skip_file_patterns`; re-baseline `baseline_v6` lifted hit@5
0.789 → 0.895) and navigation as the weak query type (v6 navigation
hit@5 = 0.75). Reranking deprioritized below GraphRAG on the reasoning that
navigation answers are structural. **Honest framing kept from the original:**
the v6 recall gap (0.132) actually *trips* the reranker-headroom rule — the
pivot was a bet ("structural signal pays more on navigation than reordering
pays on the recall gap"), not a claim that reranking can't help. The
`--answer-limit 8` cheap lever was evaluated and did not validate (canonical
A/B: `retrieval_quality_decisions.md` §2.5).

**Restructure — 2026-07-06.** External review found the GraphRAG build blocked
five weeks on two sub-day prerequisites, answer-mode latency (p50 ~24 s
synchronous) unaddressed on the roadmap, all evidence self-referential (own
repo, own queries), and re-indexing not diff-proportional. Roadmap reorganized
into product milestones M0–M5: decide the navigation bet cheaply (M0), fix the
felt product gaps (M1–M2), make the claims true on real repos (M3–M4), then
spend on retrieval levers with evidence (M5). GraphRAG demoted from "next" to
"M0 decides"; MCP server mode added as the agent-integration wedge.

---

## Completed phases (record)

Full checklists live in git history and the linked design docs; this is the
durable summary.

### P1 — Evaluation foundation ✅

`internal/eval/`: YAML golden set, `hit@1/3/5`, `MRR@5`, `recall@5/10` +
recall-gap diagnostic, `negative_pass_rate`, per-stage latency percentiles,
per-type breakdown, `--output json`, type/subtype filters. Golden set grown to
39 queries (16-query `structural` subtype). Baselines committed under
`docs/eval/` (`baseline_v6.json` canonical; `baseline_v7_structural.json` the
structural comparison point). Spec: `rag_evaluation_metrics.md`; usage:
[`../eval/README.md`](../eval/README.md).

### P2 — Hybrid search ✅

Named dense + sparse vectors in Qdrant, code-aware tokenizer, BM25
(`k1=0.5, b=0.75`) with additive Snowball stemming, corpus-wide IDF,
server-side RRF (k=60) via prefetch, `--mode dense|sparse|hybrid` (default
hybrid), filters per prefetch stage. Algorithm history (TF-IDF → BM25 →
+stemming, with the eval matrix): `hybrid_search.md` §3. Result vs dense
baseline: hit@1 +15.8pp, MRR@5 +9.2pp, no hit@5 loss (`baseline_v4`,
reconfirmed at `baseline_v6`: hit@5 = 0.895, MRR@5 = 0.673).

### P5 v0 — `--answer` mode ✅

`Generator` interface + Ollama (`qwen2.5-coder:7b`) / Fake implementations,
frozen golden-tested v0 prompt, greedy decoding, `--answer-limit`, auto-warm
via optional `Warmer`, Tier B reference-free answer eval (citation validity,
refusal-on-negative, well-formedness) — report-only. Default retrieval path
byte-identical with the flag off. Design: `phase5_v0_answer_mode.md`.

### Re-indexing + watch mode ✅

SHA-256 file-hash + `index_version` change detection, stale-file deletion,
late deletion of orphaned chunks, `index --watch` (fsnotify + debounce).
Known limitation — re-index cost is not diff-proportional (fix = M3):
[`../improvement/reindexing.md`](../improvement/reindexing.md).

---

## Deferred decisions (revisit triggers)

| Item | Status | Revisit trigger |
|---|---|---|
| **GraphRAG (P6)** | Gated on M0 | M0 reachability ceiling + symbol-spike results; enters M5 as rung 4 if justified |
| **Cross-encoder reranking (P3)** | Parked | After M3: recall gap ≥ 0.10 confirmed on *external* corpora (standing rule, `retrieval_quality_decisions.md` §2.5) |
| **Explore Mode** | Superseded | Promoted into GraphRAG (retrieval lever); TUI presentation still deferred per TUI row |
| **TUI / drill-down UX** | Deferred | A connected-subgraph result shape worth navigating exists (post-GraphRAG) |
| **REPL / chat mode** | Deferred | Triggers unchanged (`phase5_v0_answer_mode.md` §v2+, `architecture_decisions.md` §5.2); note MCP (M2) may absorb the agent-side need |
| **Multi-provider answers (OpenAI-compatible, Anthropic)** | Deferred | Real dogfooding demand; local-first default holds |
| **Tier C faithfulness judge** | Deferred | After M2 dogfooding shows answer-content quality is the binding question |
| **Tree-sitter for non-Go languages** | Deferred | External eval (M3) shows non-Go retrieval is the binding gap |
| **Rust AST chunker (P3.5)** | Deferred → M5 rung 5 | Same trigger as above |
| **Multi-modal embeddings** | Deferred | Single-vector ceiling demonstrated after M5 levers |
| **IDE plugin** | Reframed | MCP (M2) is the editor/agent integration path; a native plugin only if MCP proves insufficient |
| **True daemon / HTTP server** | Deferred | Triggers unchanged (`architecture_decisions.md` §5.3); MCP stdio deliberately avoids it |
| **Phase C (custom vector DB in Go)** | Deferred indefinitely | Explicit decision that learning goals outweigh product investment |

---

## Related docs

- [`../improvement/production_readiness_and_features.md`](../improvement/production_readiness_and_features.md) — the review this restructure implements; detailed specs for M1–M4 items.
- [`mcp_server_mode.md`](mcp_server_mode.md) — M2 design doc.
- [`graphrag.md`](graphrag.md) — M0 prerequisites + M5 rung 4 build spec.
- [`cheaper_levers.md`](cheaper_levers.md) — M5 rungs 2–3.
- [`phase5_v0_answer_mode.md`](phase5_v0_answer_mode.md) — shipped answer mode + deferred v1 items.
- [`../knowledge/retrieval_quality_decisions.md`](../knowledge/retrieval_quality_decisions.md) — metric priorities, §2.5 canonical baselines.
- [`../knowledge/architecture_decisions.md`](../knowledge/architecture_decisions.md) — process-shape decisions (CLI vs daemon, watch, cold start).
- [`checklist.md`](checklist.md) — historical record of the original phase plan.
- [`../review_feedback/system_vision_review.md`](../review_feedback/system_vision_review.md) — source of the original phase numbering.
