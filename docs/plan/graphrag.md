# GraphRAG — Structural Retrieval Layer (Deferred Design)

**Status: deferred, not started. Updated 2026-09-22.** This design is an M5
candidate under the [roadmap](mvp_roadmap.md), not the next committed build.
M0/M3 focus on evidence and indexing reliability; no dedicated graph prototype
or reachability study is scheduled unless recurring structural failures justify it. Existing implementation sketches below remain
proposals and require validation against the selected code/runtime versions.

The proposed graph adds `defines`, concrete `calls`, and `imports` relationships
to hybrid candidates. Default retrieval and answer behavior stay unchanged until
fresh paired evidence justifies promotion. A local graph is not required for
ordinary definition lookup, and navigation questions are not all graph problems.

## Why now (and why not yet)

The May v6 baseline motivated a GraphRAG-first hypothesis: navigation hit@5 was
6/8. That priority is superseded. Reviewed main already contains identifier-token
and type/interface improvements; its saved v8 navigation hit@5 is 23/24. Those
snapshots differ in source/query sets, so the change is not a controlled A/B.
Use the roadmap's branch/evidence boundary and refresh the residual failure list.

Historical `baseline_v7_structural.json` contains 16 queries, of which 14 already
pass hit@5. All seven queries with recall@10 greater than recall@5 also pass hit@5.
These queries lack additional evidence, not every relevant result. The old gate
requiring binary hit@5 improvement on 60% of all structural queries was impossible.

A graph may recover a missing candidate reachable from hybrid seeds. A reranker
may promote evidence already in the candidate pool. Neither is proven useful by
the old recall gap alone. See [cheaper levers](cheaper_levers.md) for controlled
experiments. No graph-first or mandatory graph-plus-reranker sequence is accepted.

### Prerequisites before Step 1 (do these first)

These studies are S-sized and only enter the queue when M0/M3 expose persistent
structural failures. Completing them does not imply a build decision.

1. **Freeze current baselines.** Retain full, structural, and external reports
   with identical source/query/config/model manifests for later A/Bs. The existing
   16 structural labels are a starting point, not proof they cover current misses.
2. **Define required evidence.** For each targeted query, label required files,
   symbols, or concrete relationships and what constitutes complete top-5 context.
   Use file recall as the automated proxy; inspect chunk/relationship coverage
   separately because a file hit need not contain the required evidence.
3. **Reachability dry-run.** For each named incomplete query, determine whether
   supported v0 edges can reach the missing evidence from a fixed hybrid top-50
   pool. Record unreachable/interface-dispatch cases explicitly. Establish a
   numerical minimum of improved target queries justified by that ceiling before
   implementation; defer if achievable benefit is too small for an L-sized build.
4. **Answer-context review, when relevant.** Compare AL=5 versus AL=8 content
   on representative multi-chunk questions. The historical A/B only measured shape
   and additional latency; it did not prove a content benefit. Do not raise the
   default without quality and latency evidence. This does not block indexing work.
5. **Record build/rescope/defer.** Include cheaper alternatives, the fixed target
   list, coverage definitions, acceptance count, and performance budget in the roadmap.

## Goal

Expand and order candidates so required structural evidence fits within the
bounded context sent to answer generation or shown to the user.

**Authoritative acceptance contract:**

- Before building, freeze target query IDs and a concrete minimum improvement
  count based on the reachability study. That count is currently undecided, so
  implementation remains gated. A post-hoc choice cannot turn a failed run into a win.
- Improve required-evidence coverage in top-5 on at least that many named targets;
  report file recall@5 and manually verified symbol/relationship coverage. Preserve
  all previously accepted hit@5 cases and required coverage on full/external sets.
- Compare fresh on/off runs against one fixed candidate/seed configuration and
  frozen source. Historical v6/v7 reports are context only. Report named historic
  misses such as `chunkfile_navigation` if still relevant, without assuming they fail.
- Preserve passing negatives under a predeclared score-aware policy; investigate
  existing failures separately. Keep raw retrieval and graph scores distinct.
  Graph blend scores cannot use cosine or RRF thresholds unchanged. Confidence
  filtering is a hypothesis, not a guarantee of perfect negative pass.
- Report MRR@5, per-query regressions, extraction coverage, cold/warm latency, and
  storage/ingest cost. Meet the performance budgets chosen before the experiment.
- Preserve the default path. If acceptance fails, retain the result and keep the
  experiment disabled or shelved; do not widen scope automatically.

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

  **Consequence — v0 `calls` is concrete-dispatch only.** Without `implements`,
  a call to an interface method (e.g. `Embedder.Embed`, `Generator.Generate`,
  `graph.Store.UpsertEdge`) resolves only to the *interface* symbol; v0 cannot
  follow it to the concrete implementor. Interface-mediated calls are common in
  this codebase, so v0 "what calls X" is incomplete wherever X is reached
  through an interface. The reachability dry-run (Prerequisites, above) must
  count how many of the 16 structural queries actually require interface
  resolution — if most do, v0's addressable set is much smaller than "what
  calls X" implies, and the scope must be reconsidered before step 1.
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

```text
ingest pipeline (existing)
  walk -> chunk -> enrich -> embed -> upsert
                   |
                    +-- NEW: extract_edges(chunk, ast) -> []Edge
                                                  |
                                                  v
                                          graph store (NEW)

search path
  query -> embed -> hybrid retrieve top-50 (existing)
                         |
                          +-- (default) -> existing hybrid results
                         |
                          +-- (--graph) -> graph expansion -> connected subgraph
                                                                 |
                                                                 v
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
  collection           text pk
  built_at             text
  edge_extractor_version  int
  collection_build_id  text          -- identity of the Qdrant collection snapshot this graph was built against
```

**Consistency model (the graph is a derived cache, never authoritative).**
`symbols.chunk_id` is an FK into Qdrant, but the two stores share no
transaction — a crash between the Qdrant upsert and the SQLite write, or a
re-index that reassigns point IDs, would orphan edges or dangle `chunk_id`s.
We handle this by treating the graph as a rebuildable cache:

- The graph is **rebuilt whenever the collection is rebuilt** (full re-index).
  It is never the source of truth for chunk content — Qdrant is.
- `graph_meta.collection_build_id` records the identity of the Qdrant
  collection snapshot the graph was built against (e.g. completed
  indexing generation and source/representation manifest; creation time or point
  count alone cannot detect same-count edits).
- At query time, `--graph` **fails closed**: if `collection_build_id` does not
  match the live collection's identity, the search silently falls back to flat
  hybrid rather than serving stale edges. This fallback requires a trustworthy completion marker. It does not prove
  that the underlying hybrid index matches the live working tree.

**Unresolved edges are kept.** A call to `someExternalPkg.DoThing` whose
target we cannot resolve to an indexed symbol still records the edge with
`to_symbol_id = null` and `to_package = "someExternalPkg"`. This is how
"calls into Qdrant" stays useful even though Qdrant's source isn't indexed.

---

## Ingest changes

Pseudocode for the new stage, slotted after chunking and before embedding:

```text
function ExtractEdges(chunk, ast, packageContext) -> []Edge:
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

**Interaction with `index --watch` (already shipped).** `index --watch`
(`architecture_decisions.md` §3.2, `internal/ingest/watcher.go`) calls
`Pipeline.Run` on every change, but the cross-file edge re-resolution above is
**step 8 — deferred past the eval gate**. Until step 8 lands, wiring graph
extraction into the pipeline under `--watch` would reproduce exactly the silent
staleness this section forbids (rename `ChunkFile` → every caller's `calls`
edge in other files goes stale). **Therefore, until step 8 ships, graph
extraction is hard-gated OFF under `--watch`**: `config.yaml graph.enabled` is
not honored in watch mode, and a one-time `--watch` startup warning states that
graph edges require a manual full `index` and are not maintained incrementally
yet. Steps 1–7 ship graph extraction only on full `index` runs. Step 8 removes
the gate.

**Language scope for v0.** Go only — leverages the existing
`internal/ingest/chunker_go.go` AST pass. Other languages get a no-op
extractor and continue to work in hybrid-only mode. The Rust AST chunker
(deferred Phase 3 item) is the natural second language.

---

## Search-time expansion

```text
function GraphSearch(query, k):
    seeds = HybridSearch(query, k=50)                # existing path
    seedSymbols = symbolsDefinedIn(seeds.chunks)

    candidates = seeds.chunks
    relations = {}                                   # chunk_id -> []Relation

    for s in seedSymbols:
        for edge in edgesTouching(s, types={calls, called_by, defines}):   # implements is v1
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

**Scoring blend.** The graph boost is added to the hybrid score, but the two
must be on the **same scale first**. Qdrant's server-side RRF score is
`Σ 1/(k+rank)` (k=60) — roughly 0.01–0.05 for top hits — so a raw additive
`α·log(1+edges)` term (0.1–0.5 at α=0.15) would swamp it and turn ordering into
"most-connected wins," risking the accepted-hit and required-coverage gates
on behavior/concept.
v0 therefore **normalizes the hybrid score into [0,1] across the candidate set
before blending**:

```text
norm_hybrid(c) = (hybrid_score(c) − min_hybrid) / (max_hybrid − min_hybrid)

final_score(c) = norm_hybrid(c)                                      -- in [0,1]
               + α · log(1 + structural_edges_to_seeds(c))
               + β · (1 if c contains a defining symbol of a query token else 0)
```

Now α and β operate on a comparable [0,1] base, so the defaults `α = 0.15`,
`β = 0.30` represent meaningful fractions of the hybrid signal rather than
50× multiples of it.

**Seed vs. neighbor tension is explicit, not hand-waved.** A pure neighbor
(a chunk hybrid did not retrieve — the Bucket A case) has `norm_hybrid = 0` and
scores only on the graph terms; it can reach ~0.45 (β + α·log) and so *can*
cross into top-5 past a weakly-ranked seed. This is intended: the whole point
for Bucket A is to promote a non-seed neighbor into top-5. For Bucket B and
non-structural queries, the [0,1] base keeps strong seeds ahead unless a
neighbor has substantial structural support. The exact α/β must be validated,
not asserted:

> **Validation requirement (blocking for step 5).** Write a worked numeric
> example for one Bucket A query and one Bucket B query showing the target
> chunk actually crossing the top-5 boundary under these weights. If the
> arithmetic does not cross, the scoring design fails the exit criterion and
> must change before the eval A/B is run.

`α` and `β` are exposed as flags (`--graph-alpha`, `--graph-beta`) and **swept**
on the structural subset. Because the subset is only 16 queries with no
train/test split, **the swept values are descriptive, not predictive**: the
per-query gate passed with a tuned α/β is **provisional** until the structural
set grows enough to hold out a validation slice. Record the swept values and
the date alongside the baseline so a later set-growth can re-confirm them.

**Latency budget.** Graph expansion runs on top-50 seeds → ≤200 1-hop SQLite
lookups per query. Target: ≤30ms added p95 on the existing eval set. If
exceeded, narrow expansion to top-20 seeds.

---

## Composition with reranking (graph for recall, reranker for precision)

Graph expansion and reranking can compose if each has independently earned its
cost. A graph adds candidates; a reranker orders a fixed candidate set. A
cross-encoder is not assumed to outperform a measured blend without an A/B.
Neither is a mandatory final architecture.

```text
function GraphRerankSearch(query, output_limit):
    seeds = hybrid_search(query, fixed_candidate_limit)
    neighbors = graph_expand(seeds, bounded_hops, supported_edges)
    candidates = deduplicate(seeds + neighbors)
    if validated_reranker_enabled:
        ordered = rerank(query, candidates)
    else:
        ordered = graph_blend(candidates, fixed_parameters)
    return top(ordered, output_limit), relations_of(candidates)
```

Keep candidate generation and ordering separate. Record raw retrieval scores,
graph scores, and optional reranker scores with their kinds. Compare against the
same candidate-depth controls described in `cheaper_levers.md`; added candidates
and a new ordering function are separate treatment effects. Reopen either layer
only for named residual failures, not because the other has shipped.

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
   "trace from X to Y" question; returns the ordered call path. **v0 traces
   concrete-dispatch `calls` edges only** — a path that must cross an interface
   method (e.g. `Embedder.Embed → ollama.Client.Embed`) cannot be resolved
   until `implements` ships in v1, and `--trace` reports "no concrete path
   found" rather than inventing one. A v0-resolvable example:

   ```
   main.runSearch
     → Searcher.Search
     → qdrant.Client.Search
   2 hops, 3 chunks
   ```

   **`--trace` is diagnostic / exploratory and is NOT part of the eval gate**
   — no gating criterion measures path correctness. It is unit-tested for
   shortest-path correctness on a constructed graph (see Verification), but its
   end-to-end quality is not gated in v0. Promote it to a gated feature (with
   golden trace queries + expected ordered paths) only if it proves to be a
   primary use mode after dogfooding.

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
| 7 | Eval: refresh existing structural labels, implement required-evidence scoring and frozen-input A/B harness | S |
| 8 | Incremental-reindex fixup: delete + cross-file edge re-resolution on file change (see "Incremental re-index" below). **Also removes the `--watch` graph-extraction gate** (steps 1–7 keep it off under `--watch`). | M |
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
- `docs/eval/golden.yaml` — refresh the existing navigation queries with
  `subtype: structural`; retain versioned required-evidence labels for the experiment.

**Touch:**

- `internal/ingest/pipeline.go` — two-pass ingest, edge extraction stage.
- `internal/search/searcher.go` — `--graph` path that calls
  `graph.Expand(...)`.
- `internal/model/result.go` — add `Relations []Relation`.
- `cmd/ragcodepilot/main.go` — `--graph`, `--hops`, `--trace` flags.
- `docs/plan/mvp_roadmap.md` — record the M5 build/rescope/defer decision and
  the predeclared coverage target before implementation.
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
- Expansion blends scores deterministically given a fake graph, **after
  normalizing the hybrid score into [0,1]** (see Scoring blend).
- **Worked numeric example (blocking for step 5):** one Bucket A and one
  Bucket B query, asserting the target chunk crosses the top-5 boundary under
  the chosen α/β. If it doesn't, the scoring design changes before the A/B.
- `--trace` returns the known shortest path on a constructed graph; returns
  "no concrete path found" on disconnected pairs (and on paths that would
  require a v1 `implements` hop).

**Integration (requires Qdrant; Ollama optional with fake embedder):**

- Re-index ragcodepilot's own repo with graph extraction on.
- Spot-check edge counts (`select count(*) from edges group by edge_type`)
  against an obvious ground truth: e.g. `Pipeline.Run` must have an outgoing
  `calls` edge to `ChunkFile`.

**Eval gate (the one that matters):**

- Capture fresh graph-off/on reports on the same frozen full, structural, and
  external sets. Pin candidate/seed depths and output limits. Proposed graph
  flags must first be wired through eval; historical v6 is not the control arm.
- Apply the Goal section's frozen target list and minimum required-evidence
  improvement count, preserving accepted hits/coverage and passing negatives.
  Include a full set with negatives; the structural-only set has none.
- Compare final graph scores only under their own fixed calibration. Report the
  unchanged raw retrieval diagnostic separately and inspect changed false matches.
- Record every failed gate and keep the feature disabled/shelved if it fails.
  No current graph result or acceptance count has been established.

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
  answers *sound* more authoritative even when wrong. Existing Tier B citation validity only checks references. Human content
  review (and later Tier C) must assess whether connected evidence supports claims.

---

## Dependencies

- M0/M3 evidence and the prerequisite studies above, including a frozen target
  coverage count and supported-edge ceiling.
- Fresh paired full/structural/external baselines and score-aware negative rules.
- Validate the proposed SQLite driver and persistence/recovery behavior before
  the experiment; a design preference is not a tested runtime choice.
- Reranking is optional; assess simpler remedies first when they address the same
  observed miss. No mandatory graph-first order remains.

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

## Proposed contracts to confirm before implementation

These inherited draft choices need confirmation against the coverage gate and
selected experiment scope. They do not authorize implementation:

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
- **Low-confidence expansion policy requires calibration.** Skipping expansion
  below a seed-score floor may limit false matches, but cannot guarantee negative
  pass: known negative queries can have high RRF agreement. Freeze the policy on
  a calibration split, preserve raw scores, and evaluate graph-score negatives
  independently. Tuning on held-out queries to reach 1.00 is not allowed.

## Open questions

1. **Do we keep symbol-level scoring inside Qdrant or in SQLite?** Current
   plan: hybrid scoring stays in Qdrant (today's path); graph rescoring
   happens in Go after the SQLite lookups. Revisit if latency suffers.
2. **Does a seed-score floor help?** Compare it with no expansion on labeled
   positive/negative calibration cases, then verify the fixed policy on held-out
   queries. It must not suppress legitimate weak structural seeds or hide the
   existing false matches.
