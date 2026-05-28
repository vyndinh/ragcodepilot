# GraphRAG — Structural Retrieval Layer (Design Doc)

**Status:** Draft. Not started. **Promoted ahead of Phase 3 reranking on
2026-05-28** — see "Why this before reranking" below. Still gated on extending
the golden eval with multi-hop / structural queries before A/B begins.

This document proposes adding a **graph layer** over the existing hybrid
retrieval, so that structural relationships between code entities (calls,
defines, implements, imports) become a first-class retrieval signal. It is the
canonical design for what `mvp_roadmap.md` calls "Explore Mode (call graph +
clustering + drill-down)," promoted from a UX feature into a retrieval-quality
feature.

GraphRAG is **additive**: a `--graph` flag layered onto the hybrid retrieve
path, with a fallback to today's flat results. The default `search` and
`--answer` behavior is unchanged until the eval proves it helps.

**Framing — the real goal.** The user-visible objective is **"the right chunk
is in the top-5 sent to the LLM."** That is what `--answer` mode depends on,
and it is what the eval should measure. Anything that does not move top-5
inclusion on the queries that matter is not worth doing right now.

---

## Why now (and why not yet)

**Signal from the v6 baseline** (`docs/eval/baseline_v6.json`, 23 queries,
collection re-indexed with `*_test.go` excluded):

| Type | count | hit@5 | MRR@5 |
|---|---|---|---|
| behavior | 4 | 1.00 | 1.00 |
| concept | 7 | 1.00 | 0.71 |
| **navigation** | **8** | **0.75** | **0.47** |
| negative | 4 | n/a | n/a |

Navigation is the only query type below 1.0 on hit@5 and the lowest on MRR@5.
Navigation queries ("where is X defined", "what calls Y", "trace from A to B")
are **structural questions**, not semantic-similarity questions. Pure vector +
BM25 hybrid is the wrong instrument for them — the answer is in the *edges*
between chunks, not in the chunks themselves. A graph layer attacks this
directly.

### Why this before reranking (2026-05-28)

Phase 3 reranking was the originally-planned next lever. It is now
**deprioritized below GraphRAG**, on the following reasoning:

1. **Reranking only reorders within the top-50; it cannot add a signal that
   is not in the embedding/BM25 space.** Navigation answers are structural —
   "what calls X" is a *graph* question, not a similarity question. If the
   correct chunk's text does not lexically/semantically resemble the query
   (e.g. the caller does not mention the callee's purpose), reranking will
   not lift it.
2. **The product goal is top-5 inclusion for LLM context, not MRR
   refinement.** Phase 3's success metric is `MRR@5` improvement — useful,
   but the binding constraint for `--answer` mode is whether the correct
   chunk *appears* in top-5 at all. GraphRAG attacks this directly by
   pulling in neighbors of seed nodes.
3. **Cost asymmetry.** A cross-encoder reranker adds ≤200ms warm latency,
   model-load complexity, and a deferred decision (Ollama? sidecar?
   pure-Go?) for a precision-only lift. The graph store adds local SQLite
   and a per-language extractor — comparable cost, but adds *new* signal
   instead of refining the existing one.
4. **Eval-set risk.** Reranking is hard to evaluate honestly with the
   current golden set, because the ambiguous-query subset Phase 3 needed
   was never tagged. GraphRAG forces the eval to grow in the direction that
   matters anyway (structural / multi-hop queries).

### Honest caveats to this argument

- **The v6 recall-gap data triggered the reranker rule.** `baseline_v6` shows
  `recall@10 − recall@5 = 0.132` — above the standing 0.10 "reranker has
  headroom" threshold in `retrieval_quality_decisions.md` §2.5. This pivot is
  therefore **not** *"reranker can't help"* — it is a deliberate bet:
  *"structural signal pays more on navigation than reordering pays on the
  recall gap."* Recording the framing this way so future-us doesn't refight
  the decision under different evidence.
- **GraphRAG also depends on hybrid for the seeds.** Expansion only finds the
  right chunk if it is connected by ≤N hops to *something* in the top-50.
  The v6 navigation misses split unevenly: `run_eval_navigation` (r@5=0,
  r@10=1) is already retrieved but ranked 6–10 — **reranker-shaped**, not
  graph-shaped. `chunkfile_navigation` (r@10=0) is invisible to hybrid and
  therefore invisible to GraphRAG too, unless `ChunkFile` happens to be
  connected by `defines` / `called_by` to a hybrid seed. **The eval must
  report the v6→v6+graph delta on `chunkfile_navigation` explicitly** — it is
  the case GraphRAG is most exposed on.
- **Cheaper lever evaluated — `--answer-limit`.** Since `recall@10 ≫
  recall@5`, raising `--answer-limit` from 5 → ~8 *mechanically* puts the
  rank-6–10 chunks into the answer prompt. The hypothesis was "free win."
  Eval-side A/B (2026-05-28) showed the cost wasn't free: **Tier B shape
  metrics are flat, p50 generation latency rises ~55%** (23.9s → 37.1s).
  Tier B can't measure answer *content* — whether the extra chunks
  improved coverage — so the cheap lever is **inconclusive on automated
  metrics** and requires dogfooding to decide. Implications: (a) GraphRAG's
  scope does *not* automatically narrow; (b) a true retrieval-side fix
  (reranker or graph) would land Bucket B chunks in top-5 *without* the
  latency penalty, which strengthens, not weakens, the case for Phase 6.
  See gating criteria below for the updated prerequisite.

Reranking is **parked, not cancelled.** Revisit it if GraphRAG ships and
top-5 inclusion on `concept` queries (currently MRR@5 = 0.71) remains the
binding constraint, or if reranking can be drop-in via a model already in
Ollama for trivial cost.

**But not yet, because:**

1. **The current golden set under-measures graph value.** Of 8 navigation
   queries, most are single-hop "where is X defined." Multi-hop ("what would
   break if I change the `Embedder` interface") is barely represented. Without
   those queries, this work cannot be evaluated honestly.
2. **Avoid stacking unproven layers.** Extend golden set → build graph →
   A/B against the v6 hybrid baseline.

**Gating criteria to start:**

- Golden set extended with **≥15** multi-hop / structural queries tagged
  `structural` (sub-type of navigation), committed to `golden.yaml`. ✅ Done
  — 16 queries landed; current count 39. Comparison files
  `baseline_v7.json` (full) and `baseline_v7_structural.json` (16-query
  subset).
- v7 baseline (`baseline_v7_structural.json`) is the comparison point.
- Decision recorded in `mvp_roadmap.md` that Phase 3 stays parked.
- **`--answer-limit 8` evaluated** (2026-05-28). Eval-side A/B
  (`baseline_v7_structural_answer_al5.json` vs `_al8.json`) shows Tier B
  shape metrics are **flat** (CitedRate, AllCitationsValidRate, dangling, all
  unchanged) and **generation latency rises ~55% at p50** (23.9s → 37.1s).
  Tier B cannot measure whether the *content* improved — that requires
  dogfooding or Tier C. **The "free win" hypothesis did not validate
  automatically.** Updated gate:
  - Before starting GraphRAG, **dogfood 3–5 multi-chunk concept/behavior
    questions side-by-side at AL=5 and AL=8** and judge whether content is
    more complete at AL=8.
  - If the dogfooding judgment says AL=8 materially helps → consider raising
    the default and re-evaluating GraphRAG's narrowed scope.
  - If the dogfooding judgment says it doesn't (or is mixed) → leave the
    default at 5; GraphRAG keeps its full scope, since the cheap lever did
    not subsume Bucket B.

---

## Goal

Add a graph store of code entities and relationships, populated during
ingestion. Use it at query time to expand and re-score hybrid results so that
structural queries return connected subgraphs instead of disjoint chunks.

**Exit criterion (top-5 framing, evidence-honest):**

- Structural subset has **≥15** multi-hop / structural queries (see "Gating
  criteria to start"). Aggregate-percentage gates on a smaller subset are
  noise-dominated on this corpus.
- `ragcodepilot search --graph "..."` lifts **`hit@5` on ≥60%** of the
  structural queries (per-query gating, named list in `golden.yaml`), so a
  single flaky case cannot flip the verdict.
- The v6 → v6+graph delta is **reported explicitly for `chunkfile_navigation`**
  — the case GraphRAG is most exposed on (see "Honest caveats" above).
- No regression >2pp on any `behavior` / `concept` / non-structural
  `navigation` query (per-query, not aggregate).
- **`negative_pass_rate` stays at 1.00.**
- `MRR@5` reported as a secondary diagnostic; **not** a gate. Movement *into*
  top-5 is what matters for `--answer` grounding.

---

## Non-goals

- **Replacing hybrid search.** Vector + BM25 remains the seed-finding stage.
- **A separate graph database.** No Neo4j, no JanusGraph. Local CLI, local
  store.
- **Cross-repo graphs.** v0 graph is per-repo, same scope as today's
  collection.
- **Runtime / dynamic call graphs.** Static AST edges only.
- **LLM-driven graph construction** (e.g. Microsoft GraphRAG's
  community-summarization pipeline). That's expensive, non-deterministic, and
  not how source code is best modeled. We have an AST — use it.

---

## Conceptual model

### Node types

- `chunk` — existing `CodeChunk` (the unit returned by retrieval).
- `symbol` — a named, addressable entity: function, method, type, interface,
  package. A symbol belongs to a chunk (its definition site) but is queried
  separately because callers reference *names*, not chunks.
- `file` — coarse anchor for file-level queries.

A chunk can contain multiple symbols (a file-level Go chunk with several
function definitions). A symbol has exactly one defining chunk.

### Edge types (v0)

| Edge | From | To | Source |
|---|---|---|---|
| `defines` | chunk | symbol | AST: function/method/type declarations |
| `calls` | symbol | symbol | AST: call expressions, resolved to package-qualified name where possible |
| `imports` | file | package | AST: import declarations |
| `contains` | file | chunk | already implicit in payloads |

**Deferred to v1:**

- `implements` (symbol(type) → symbol(interface)) — Go's interface type-set
  resolution is non-trivial; ~25% of the extraction work. v0 ships
  `defines + calls + imports` only, which already covers the most-asked
  navigation questions ("where is X defined" / "what calls X"). Add when
  the eval shows interface queries are the binding constraint.
- `references` (non-call name use).
- `co-changes-with` (git log).
- `mentions` (docstring / comment cross-references).

### Why these edges

Each edge type maps to a query pattern users actually ask:

- "where is `ChunkFile` defined" → `defines` reverse-lookup on symbol name.
- "what calls `ChunkFile`" → `calls` reverse traversal.
- "trace from CLI search to Qdrant" → shortest `calls` path between two
  symbols.
- "what implements `Embedder`" / "what would break if I change
  `Embedder.Embed`" → v1, when `implements` ships.

---

## Architecture

```
ingest pipeline (existing)
  walk → chunk → enrich → embed → upsert
                   |
                   └── NEW: extract_edges(chunk, ast) → []Edge
                                                  |
                                                  ↓
                                          graph store (NEW)

search path
  query → embed → hybrid retrieve top-50 (existing)
                         |
                         ├── (default) → rerank → top-10  [flat results]
                         |
                         └── (--graph) → graph expansion → connected subgraph
                                                                 |
                                                                 ↓
                                                       formatter (new shape)
```

**Where the graph lives.** v0 stores the graph in **SQLite** in
`~/.ragcodepilot/<collection>.graph.db`. Rationale:

- Already a Go-friendly local dependency (`modernc.org/sqlite`, pure Go).
- Edge queries are trivially expressible as joins; no graph-query DSL needed
  at this scale (a repo of 10⁴ symbols and 10⁵ edges fits easily).
- Survives across runs without standing up another container alongside
  Qdrant.
- Schema migrations are familiar.

Not in Qdrant payloads: a flat payload can't express 1-many edges cleanly,
and reverse lookups would force per-query scans.

---

## Schema (SQLite, v0)

```text
table symbols
  id              integer pk
  name            text          -- short name, e.g. "ChunkFile"
  qualified_name  text          -- package-qualified, e.g. "ingest.ChunkFile"
  kind            text          -- "func" | "method" | "type" | "interface" | "package"
  language        text
  file_path       text
  chunk_id        text          -- FK to Qdrant point id (the defining chunk)
  start_line      int
  end_line        int
  index (qualified_name), index (name), index (chunk_id)

table edges
  id              integer pk
  edge_type       text          -- "calls" | "implements" | "defines" | "imports"
  from_symbol_id  int           -- nullable for file-rooted edges
  to_symbol_id    int           -- nullable for unresolved external calls
  from_file       text          -- used when from_symbol_id is null
  to_package      text          -- used for imports / unresolved calls
  index (from_symbol_id, edge_type), index (to_symbol_id, edge_type)

table graph_meta
  collection      text pk
  built_at        text
  edge_extractor_version  int
```

**Unresolved edges are kept.** A call to `someExternalPkg.DoThing` whose
target we cannot resolve to an indexed symbol still records the edge with
`to_symbol_id = null` and `to_package = "someExternalPkg"`. This is how
"calls into Qdrant" stays useful even though Qdrant's source isn't indexed.

---

## Ingest changes

Pseudocode for the new stage, slotted after chunking and before embedding:

```
function ExtractEdges(chunk, ast, packageContext) → []Edge:
    edges = []
    for each declaration D in ast:
        if D is FunctionDecl or MethodDecl or TypeDecl:
            symbol = upsertSymbol(name=D.name,
                                  qualified_name=qualify(D, packageContext),
                                  kind=kindOf(D),
                                  chunk_id=chunk.id,
                                  file=chunk.file)
            edges.append(Edge(defines, chunk, symbol))
            for call C in callExpressionsInside(D):
                target = resolveCallTarget(C, packageContext)
                edges.append(Edge(calls,
                                  from=symbol,
                                  to=target.symbol_or_null,
                                  to_package=target.package))
    for import I in ast.imports:
        edges.append(Edge(imports, from_file=chunk.file, to_package=I.path))
    return edges
    # NOTE: `implements` edges (interface satisfaction) are deferred to v1.
```

`resolveCallTarget` is best-effort. Same-package and same-file resolutions
are reliable from the AST alone; cross-package calls resolve when the
target's qualified name matches an indexed symbol. Unresolved → record
package only.

**Two-pass ingest.** The first pass discovers all symbols across all files.
The second pass extracts edges, using the symbol table built in pass 1 to
resolve cross-file calls. This avoids "symbol not yet seen" misses.

**Incremental re-index.** When a file changes, three things must happen:

1. **Delete** the file's symbols and the edges originating from them.
2. **Re-extract** symbols and edges from the new AST.
3. **Re-resolve cross-file edges** whose targets pointed at deleted or
   renamed symbols. This is the step naive "delete + re-extract" misses —
   when `ChunkFile` is renamed in `chunker.go`, every caller's `calls` edge
   in *other files* becomes stale.

Step 3 implementation: the symbol table lives in SQLite. On every change,
query `edges WHERE to_symbol_id IN (deleted_ids)` and either re-resolve them
to the new symbol (rename case — match by qualified name) or unset
`to_symbol_id` (move / delete case — leave `to_package` intact so the edge
is still useful for "calls into <package>" queries). Cost is one indexed
lookup per deletion; trivial at this scale.

This policy explicitly avoids the "best-effort, may go stale until full
re-index" failure mode — silent staleness is the trust-eroding outcome we
do not want for a graph store.

**Language scope for v0.** Go only — leverages the existing
`internal/ingest/chunker_go.go` AST pass. Other languages get a no-op
extractor and continue to work in hybrid-only mode. The Rust AST chunker
(deferred Phase 3 item) is the natural second language.

---

## Search-time expansion

```
function GraphSearch(query, k):
    seeds = HybridSearch(query, k=50)                # existing path
    seedSymbols = symbolsDefinedIn(seeds.chunks)

    candidates = seeds.chunks
    relations = {}                                   # chunk_id → []Relation

    for s in seedSymbols:
        for edge in edgesTouching(s, types={calls, called_by, implements, defines}):
            neighbor = chunkOf(edge.other_end)
            if neighbor and neighbor not in candidates:
                candidates.append(neighbor with graph_boost)
            relations[neighbor.id].append(
                Relation(edge_type, direction, label=s.name))

    rescored = rescore(candidates,
                       baseScore = hybrid_score,
                       graphScore = f(edge_count, edge_types, distance))
    return rescored.topK(k), relations
```

**Scoring blend.** Final score combines the existing hybrid (RRF) score with
a small graph boost — capped, so seed nodes from hybrid keep priority. v0
formula:

```
final_score(c) = hybrid_score(c)
               + α · log(1 + structural_edges_to_seeds(c))
               + β · (1 if c contains a defining symbol of a query token else 0)
```

with α and β tuned on the structural eval subset; defaults `α = 0.15`,
`β = 0.30`. Both are flags so they can be swept without recompiling.

**Latency budget.** Graph expansion runs on top-50 seeds → ≤200 1-hop SQLite
lookups per query. Target: ≤30ms added p95 on the existing eval set. If
exceeded, narrow expansion to top-20 seeds.

---

## Output shape

Today's `model.SearchResult` is flat. Extend it with an **optional**
`Relations` field. The CLI keeps today's terse format by default and shows
relations only with `--graph`.

```text
struct Relation:
    edge_type:   string          -- "calls" | "called_by" | "implements" | "defines"
    other_chunk: ChunkRef        -- nullable for unresolved targets
    label:       string          -- symbol name on the seed side
    distance:    int             -- 1 for direct, 2 for second hop

struct SearchResult:
    chunk:     CodeChunk
    score:     float
    relations: []Relation        -- empty unless --graph
```

**Three output presentations** behind one flag:

1. **`--graph`** — flat result list with a relations sub-tree per hit (terminal default):

   ```
   1. ChunkFile  internal/ingest/chunker.go:42        score 0.84
        calls →      extractName       chunker.go:88
        calls →      chunkGoAST        chunker_go.go:31
        called-by ←  Pipeline.Run      pipeline.go:120
   ```

2. **`--graph --trace <symbolA> <symbolB>`** — shortest-path mode for the
   "trace from X to Y" question; returns the ordered call path:

   ```
   main.runSearch
     → Searcher.Search
     → Embedder.Embed              (interface; 1 implementor)
     → ollama.Client.Embed
   3 hops, 4 chunks, total score 0.71
   ```

3. **`--graph --output json`** — full subgraph (`nodes`, `edges`) for
   programmatic consumers (future TUI, future answer-mode grounding).

**Answer-mode coupling.** When both `--graph` and `--answer` are set, the
prompt builder receives the connected subgraph instead of K disjoint chunks
and adds an "Edges:" section to the context block. The Phase 5 v0 prompt
template is frozen — the v1 prompt is the right place to consume this.

**Refactor shape (locked).** `answer.ChunkContext` already exists as the
prompt's per-chunk type. v1 prompt unfreeze ships as: extend `ChunkContext`
with `Relations []Relation` (default-empty); `renderChunk` in `prompt.go`
learns to render an `Edges:` sub-section when `Relations` is non-empty;
the system prompt is updated once to teach the LLM what `Edges:` means; the
prompt golden tests are extended to lock the new format.
`internal/search/searcher.go` populates `Relations` **only** in `--graph`
mode. When `--graph` is off the rendered prompt is byte-identical to today's
v0 — regression-safe. This is step 9 in the implementation order (sized **M**,
not S).

---

## CLI surface

```text
ragcodepilot search --graph "where is ChunkFile defined"
ragcodepilot search --graph --hops 2 "what calls ChunkFile"
ragcodepilot search --graph --trace cmd.runSearch qdrant.Client.Search
ragcodepilot search --graph --output json "..."
```

Flags (all default to off / hybrid):

- `--graph` — turn on graph expansion.
- `--hops N` — expansion depth, default 1, cap 3.
- `--trace A B` — shortest-path mode (overrides `--hops`).
- `--graph-alpha`, `--graph-beta` — scoring weights, hidden in `--help` but
  honored, for eval sweeps.

---

## Implementation order

| Step | Description | Size |
|---|---|---|
| 1 | `internal/graph/` package skeleton: `Store` interface, SQLite impl, schema migration | S |
| 2 | Go AST edge extractor: `defines` + `imports` only, no calls | S |
| 3 | Two-pass ingest wiring: build symbol table, run extractor, write to store | M |
| 4 | Extend extractor to `calls` (same-package, then cross-package). `implements` is v1. | M |
| 5 | Graph expansion + rescoring in `internal/search/`; `--graph` flag in CLI | M |
| 6 | Output formatter for relations + `--trace` shortest-path | S |
| 7 | Eval: add `structural` query subtype, write ≥15 multi-hop queries (see Gating), A/B harness | S |
| 8 | Incremental-reindex fixup: delete + cross-file edge re-resolution on file change (see "Incremental re-index" below) | M |
| 9 | (Optional) `--graph --answer` prompt-builder integration — extend `ChunkContext` with `Relations`; update prompt golden tests | M |

Total v0: **L** (steps 1–7). Step 8 promotes it from prototype to durable. Step 9
gates on Phase 5 v1 (the unfrozen prompt).

---

## Files to touch / create

**New:**

- `internal/graph/store.go` — `Store` interface (`UpsertSymbol`,
  `UpsertEdge`, `EdgesFromSymbol`, `EdgesToSymbol`, `ShortestPath`,
  `DeleteFileSymbols`).
- `internal/graph/sqlite.go` — SQLite implementation.
- `internal/graph/sqlite_test.go` — round-trip + traversal tests.
- `internal/graph/extract_go.go` — Go AST edge extractor.
- `internal/graph/extract_go_test.go` — fixture-based extraction tests.
- `internal/graph/expand.go` — query-time expansion + rescoring.
- `internal/graph/expand_test.go` — fixture graph + golden expansion.
- `docs/eval/golden_structural.yaml` — additive query set, or new `type:
  structural` entries appended to `golden.yaml`.

**Touch:**

- `internal/ingest/pipeline.go` — two-pass ingest, edge extraction stage.
- `internal/search/searcher.go` — `--graph` path that calls
  `graph.Expand(...)`.
- `internal/model/result.go` — add `Relations []Relation`.
- `cmd/ragcodepilot/main.go` — `--graph`, `--hops`, `--trace` flags.
- `docs/plan/mvp_roadmap.md` — promote "Explore Mode" from deferred to a
  numbered phase that references this doc.
- `README.md` — Configuration / Architecture sections describe graph store
  location and the new flag.
- `config.yaml` — optional `graph.enabled: true/false` to disable extraction
  entirely on large repos where users only want flat hybrid.

---

## Verification

**Unit tests (no Qdrant, no Ollama):**

- Edge extractor on small Go fixtures: assert exact edge sets for known
  inputs (table-driven, same style as `chunker_go_test.go`).
- SQLite store round-trips and traversal correctness.
- Expansion blends scores deterministically given a fake graph.
- `--trace` returns the known shortest path on a constructed graph; returns
  empty on disconnected pairs.

**Integration (requires Qdrant; Ollama optional with fake embedder):**

- Re-index ragcodepilot's own repo with graph extraction on.
- Spot-check edge counts (`select count(*) from edges group by edge_type`)
  against an obvious ground truth: e.g. `Pipeline.Run` must have an outgoing
  `calls` edge to `ChunkFile`.

**Eval gate (the one that matters):**

- Run `ragcodepilot eval --dataset docs/eval/golden.yaml` twice: once with
  `--graph=false` (hybrid baseline = `baseline_v6.json`), once with
  `--graph=true`. Compare on the `structural` subset.
- **Pass criteria (top-5 framing):** ≥10pp `hit@5` lift on `structural`
  — i.e. the LLM gets the right chunk in its context more often — with no
  regression >2pp on `behavior` / `concept` / non-structural `navigation`,
  and `negative_pass_rate` unchanged at 1.0. `MRR@5` reported as a
  secondary diagnostic; not a gate.
- **Fail handling:** if the lift is below the gate, keep the code behind the
  flag, document the negative result in `docs/eval/`, and revisit only if
  the eval set grows or reranking changes the landscape.

---

## Risks & tradeoffs

- **Edge resolution accuracy.** Cross-package calls in dynamically-dispatched
  languages (and even Go interfaces) are not statically resolvable in all
  cases. v0 records unresolved edges by package; this is honest but means
  some "what calls X" answers will be incomplete. Document this clearly in
  the output.
- **Ingest latency.** Edge extraction adds AST work we already pay for during
  chunking, but the symbol-resolution pass is extra. Budget: ≤20% ingest-time
  increase on the ragcodepilot repo. Measure before enabling by default.
- **Storage growth.** SQLite graph file expected at ~5–10× the size of the
  flat chunk metadata. Still small in absolute terms (single-digit MB for
  this repo).
- **Maintenance surface.** Two extractors per language eventually (chunker +
  edge extractor). Worth it if eval shows the lift; otherwise it's
  ceremony.
- **Confidence inflation in `--answer`.** Connected context makes LLM
  answers *sound* more authoritative even when wrong. Citation validation
  (deferred in `phase5_v0_answer_mode.md`) becomes more important, not less,
  once GraphRAG feeds answer mode.

---

## Dependencies

- Golden eval extended with ≥6 structural multi-hop queries (the binding
  prerequisite — without these the eval gate is meaningless).
- v6 baseline (`docs/eval/baseline_v6.json`) is the comparison point.
- Decision on SQLite driver: `modernc.org/sqlite` (pure Go, no CGo) is the
  preferred choice — keeps the cross-compile story simple.
- **Not a dependency:** Phase 3 reranking. Deprioritized per the
  "Why this before reranking" section.

---

## Out of scope (revisit later)

- LLM-built community summaries over the graph (Microsoft-style GraphRAG).
- Cross-repo graphs / multi-collection traversal.
- Runtime / dynamic call graphs from tracing.
- Graph visualization in a TUI — separate UX phase, not a retrieval
  concern.
- Co-change edges from git history — v1 candidate, gated on eval value.
- A query DSL (Cypher-like). 1- to 2-hop expansion does not justify it; if
  v1 needs deeper queries, revisit.

---

## Decisions (locked before implementation)

These were promoted from "open questions" once the scope and CLI contract
became binding:

- **`--limit` counts seeds, not final chunks.** Under `--graph` the connected
  subgraph (5 seeds + 1-hop neighbors) easily reaches 12–15 chunks. The
  output header reports both — e.g. *"5 seeds, 11 chunks total (graph
  expansion +6)"* — so users are never surprised that `--limit 5` returns
  12 things in the result block.
- **`implements` is deferred to v1.** v0 ships `defines + calls + imports`
  only. Go interface type-set resolution is non-trivial (~25% of the
  extraction work) and the dominant navigation questions ("where is X
  defined / what calls X") do not need it. Add `implements` when the eval
  shows interface queries are the binding constraint.

## Open questions

1. **Do we keep symbol-level scoring inside Qdrant or in SQLite?** Current
   plan: hybrid scoring stays in Qdrant (today's path); graph rescoring
   happens in Go after the SQLite lookups. Revisit if latency suffers.
2. **Negative-query interaction.** Does graph expansion change
   `negative_pass_rate`? An expanded neighborhood might surface a chunk that
   *looks* like the negative target. Eval gate explicitly tests this; this
   open Q is *"do we need a tighter floor than `pass_rate ≥ 1.00`"* (e.g.
   "no negative query's top-1 score rises above its golden threshold even
   with graph boost applied"). Decide once we have data.
