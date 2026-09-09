# Cheaper Levers — Code Embeddings + Reranker (the conventional ladder before GraphRAG)

**Status:** Draft. Not started. Proposes running the two **conventional**
retrieval-quality levers — a code-specialized embedding model and a
cross-encoder reranker — *before* committing to the L-sized GraphRAG build
(`graphrag.md`). This is the path
[`../knowledge/code_graph_retrieval_landscape.md`](../knowledge/code_graph_retrieval_landscape.md)
§3 recommends: exhaust the cheaper, better-documented steps on the standard
ladder first, then decide what (if anything) GraphRAG still needs to solve.

This doc fills the `reranking.md` placeholder referenced by `mvp_roadmap.md`'s
Phase 3 row and adds the embedding-model lever that
`retrieval_quality_decisions.md` lists as untried (S–M, "+2–5pp likely").

---

## Why now

The standard ladder (landscape doc §3): structure-aware chunking ✅ → hybrid ✅
→ **reranking** → **code embeddings** → query transforms → bespoke graph. We are
parked at steps 3–4 and the GraphRAG plan jumps to step 6. Three reasons to run
3–4 first:

1. **Cost order** (the project's own rule, `building_ragcodepilot.md`). Both
   levers are smaller than GraphRAG (S–M vs L) and far more conventional.
2. **The eval evidence points here.** The recall gap is **wide** — 0.132 on
   `baseline_v6`, 0.150 on `baseline_v7_structural` — which trips the
   "reranking has headroom" rule in `retrieval_quality_decisions.md` §2.5. The
   Bucket B split (7 of 16 structural queries: chunk in top-10 but not top-5)
   is **reranker-shaped** by our own analysis.
3. **Embeddings are upstream of everything.** A code-tuned embedder targets the
   exact weak spot — identifier / navigation queries — and lifts the *whole*
   distribution, including the Bucket A cases (chunk absent from top-10) that a
   reranker can never recover.

**The bet vs GraphRAG.** GraphRAG is a *recall* mechanism for structural,
multi-hop questions; reranking is *precision*; better embeddings raise the
*floor*. These are not mutually exclusive (see "Relationship to GraphRAG"
below). The claim here is only about **order**: run the two cheap, standard
levers first, re-measure, and let what remains define GraphRAG's scope — which
may shrink to just the genuine multi-hop / trace class (Bucket A).

---

## Goal

Measure how far the two conventional levers move the binding metrics, on the
**same corpus and golden set** used for `baseline_v6` / `baseline_v7_structural`,
behind opt-in flags so the default path is unchanged until the eval proves a win.

**Exit framing (top-5, same as GraphRAG):** the question is still "is the right
chunk in the top-5 sent to the LLM." Each lever is judged per-query on the full
golden set *and* the structural subset, with no regression on
`concept` / `behavior` / `negative`.

---

## Lever 1 — Code-specialized embedding model

### Why it's the cheapest experiment

The codebase already supports a model swap with **no code changes** (confirmed
by exploration):

- The model is the `--ollama-model` flag (default `nomic-embed-text`), read
  identically by `index`, `search`, and `eval`
  (`cmd/ragcodepilot/main.go`, `resolveEmbedder`). Index and search must use the
  same value or vectors are incompatible — the only operational footgun.
- Vector **dimension is auto-detected** from the first embedding batch and
  validated thereafter (`internal/embedding/validate.go`,
  `internal/embedding/ollama.go`); there is **no hardcoded 768** in production
  code. A model of any dimension works.
- Collection dimension is **derived from the embedder** at creation
  (`internal/qdrant/client.go` `EnsureCollection`); a mismatch against an
  existing collection produces a clear "delete and re-index, or use a different
  collection name" error.
- `--collection` (default `code_chunks`) lets us index the new model into a
  **second collection** (e.g. `code_chunks_codeembed`) and A/B *without
  destroying the current index*.
- **Enrichment is inherited** — `enrichForEmbedding` runs before the embedder
  (`internal/ingest/enrichment.go`), so the new model gets the same metadata
  context.

Net: this lever is **config + re-index + eval**, no new package, no query-time
latency. That makes it the first experiment to run.

### The one real decision — model availability under local-first

The project is local-first via Ollama. Truly *code-specialized* embedders that
are cleanly `ollama pull`-able are limited, so **step 0 of this lever is to pick
a model and confirm how it runs locally**:

- **`nomic-embed-code`** — Nomic's code embedding model; verify Ollama
  availability and size (it is large; check RAM/latency).
- **`jina-embeddings-v2-base-code`** (768d) — strong code retriever; may require
  a GGUF import into Ollama rather than an official `pull`.
- **General-but-stronger** fallbacks already on Ollama (`mxbai-embed-large`
  1024d, `bge-m3`, `snowflake-arctic-embed2`) — not code-specialized, but a
  cheap sanity comparison.
- **`voyage-code-3` / cloud code embedders** — best-in-class but **API-only**;
  adopting one **breaks the local-first constraint** and is out of scope unless
  the user explicitly relaxes that.

If no code-specialized model is cleanly local, this lever's cost rises (GGUF
import or a sidecar embedder), which weakens its "cheapest" status — record the
finding and reconsider order (do the reranker first).

### Eval gate (Lever 1)

- Index the candidate model into a **new collection**; keep `code_chunks`
  (nomic) intact for comparison.
- Run `eval` on the **full** golden set and the **structural** subset against
  both collections; compare with `docs/eval/compare.py`.
- **Pass:** `hit@5` and/or `MRR@5` up on `navigation` (the weak type) with **no
  regression >2pp** on `concept` / `behavior`, and `negative_pass_rate` = 1.00.
  Report the `recall@10 − recall@5` gap — a code embedder should *narrow* it by
  retrieving more right chunks (raising the floor).
- If it wins, promote the new model + collection to default (update README /
  config) and re-baseline (`baseline_v8_codeembed.json`).

---

## Lever 2 — Cross-encoder reranker

### Architecture (mirror the `answer.Generator` pattern)

Retrieve hybrid **top-N (50)** → rerank → return **top-k**. New
`internal/rerank/` package modeled exactly on `internal/answer/`:

```
interface Reranker:
    Rerank(ctx, query, results []SearchResult, limit) → []SearchResult   # rescored + reordered, truncated to limit

interface Warmer:                       # optional, type-asserted like answer.Warmer
    Warmup(ctx) → error
```

- **Seam:** in `internal/search/searcher.go`, immediately *after* the Qdrant
  call and *before* return (the point where `[]model.SearchResult` exists). When
  no reranker is set, behavior is byte-identical to today.
- **Inputs/outputs:** the reranker reads `query` + each `result.Chunk.Content`,
  rewrites `result.Score`, sorts, truncates to `limit`. `model.SearchResult` =
  `{Chunk, Score}` (`internal/model/chunk.go`).
- **Impls:** `FakeReranker` (deterministic reorder, for wiring tests) +
  one real impl (see runtime decision). Follow
  `answer/{generator.go, ollama.go, fake.go}` structure.
- **CLI:** `--rerank` (bool, default **off**), `--reranker {…|fake}`,
  `--rerank-model`. Add to `search` and `eval`. A `resolveReranker` helper
  mirrors `resolveGenerator`.
- **Latency:** add a `Rerank` field to `search.Timings` (alongside
  `Embed`/`Qdrant`/`Total`) and surface p50/p95 in the eval report, to hold the
  ≤200ms warm budget.
- **Warm-up:** wire the optional `Warmer` exactly like `--answer` does today, so
  model-load cost is pulled out of the timed path.

### The runtime decision (the parked Phase 3 question — still open)

**Important correction:** Ollama has **no native cross-encoder / rerank
endpoint**. So "OllamaReranker calling `/api/rerank`" does not exist. Real
local-first options, behind the `Reranker` interface (which makes this
swappable):

| Option | What | Cost | Notes |
|---|---|---|---|
| **Local rerank server** (TEI / llama.cpp `--reranking`, Infinity) | small BERT cross-encoder (e.g. `bge-reranker-base`, `ms-marco-MiniLM`) over HTTP `/rerank` | M | True cross-encoder, fast (≤200ms warm), but a new local service alongside Qdrant/Ollama |
| **Python sidecar** (sentence-transformers) | same models, our own process | M | Simplest to prototype; adds a Python process + IPC |
| **Pure-Go ONNX** (onnxruntime-go) | run a cross-encoder in-process | M–L | No new process, but CGo/ONNX build complexity |
| **LLM-as-reranker via Ollama `/api/chat`** | prompt the existing generative model to score relevance | S | Fastest to get an *eval signal*; but slow per-query, and the vision review flagged "avoid LLM-as-reranker early." Use only to decide whether reranking helps at all, not as the shipped impl. |

**Recommendation:** prototype the wiring with `FakeReranker`, get an early
quality signal cheaply (LLM-as-reranker is acceptable *for the signal only*),
then ship a real cross-encoder via a local rerank server or Python sidecar.
Record the decision in this doc once made.

### Eval gate (Lever 2)

- Same corpus/collection as the chosen embedder; run `eval` with and without
  `--rerank` on the full set + structural subset; compare with `compare.py`.
- **Pass:** `MRR@5` up (reranking's natural metric) and **`hit@5` up on Bucket B
  queries** (rank-6–10 chunks promoted into top-5), **no regression >2pp** on
  `concept`/`behavior`/non-structural `navigation`, `negative_pass_rate` = 1.00,
  and added latency **≤200ms warm p95**. Document the tradeoff if it exceeds.

---

## Sequencing & the GraphRAG go/no-go

Recommended order (cost-first, and embeddings are upstream of reranking):

1. **Lever 1 (embeddings) first** — cheapest (no code), raises the floor,
   changes what the reranker has to work with. *Contingent on a local
   code-embedder existing* (see decision above); if it stalls, swap order.
2. **Lever 2 (reranker) second** — on top of the winning embedder, targeting the
   residual Bucket B.
3. **Re-measure, then decide GraphRAG.** Re-run the structural subset. Whatever
   the two levers do **not** fix is GraphRAG's real scope:
   - If Bucket B is closed and only the multi-hop / trace cases (Bucket A)
     remain → GraphRAG scope **narrows** to those, and the L may shrink.
   - If the structural per-query gate (≥60%, per `graphrag.md`) is already met →
     GraphRAG may be **deferred** entirely.
   - If little moves → the GraphRAG bet is **strengthened**, now with evidence
     that the cheap levers were genuinely insufficient.

This makes the GraphRAG decision evidence-based instead of speculative — the
same discipline as the rest of the project.

---

## Implementation order

| Step | Description | Size |
|---|---|---|
| 1 | **Lever 1 step 0:** pick + confirm a locally-runnable code embedder (Ollama pull / GGUF import); record availability + dimension + latency | S |
| 2 | Index candidate model into a new collection; `eval` full + structural; `compare.py` vs `baseline_v6` / `_v7_structural`; write `baseline_v8_codeembed.json` | S |
| 3 | If win: promote model+collection to default (README/config), re-baseline | S |
| 4 | `internal/rerank/` skeleton: `Reranker` + `Warmer` interfaces, `FakeReranker`, wiring tests (mirror `internal/answer/`) | S |
| 5 | Reranker seam in `internal/search/searcher.go` + `Rerank` timing field; `--rerank`/`--reranker`/`--rerank-model` flags + `resolveReranker` in CLI; default off | M |
| 6 | Decide reranker runtime (table above); implement one real `Reranker` impl + `Warmup` | M |
| 7 | `eval` with/without `--rerank` on full + structural; `compare.py`; write `baseline_v9_rerank.json`; check latency budget | S |
| 8 | Re-run structural subset; record the GraphRAG go/no-go decision in `graphrag.md` + `mvp_roadmap.md` | S |

Total: **M** (smaller than GraphRAG's L). Steps 1–3 are independent of 4–7 and
can ship alone if the reranker runtime decision stalls.

---

## Files to touch / create

**New:**
- `internal/rerank/reranker.go` — `Reranker` + `Warmer` interfaces.
- `internal/rerank/<impl>.go` — the chosen real reranker (server client / sidecar / onnx).
- `internal/rerank/fake.go` — deterministic `FakeReranker`.
- `internal/rerank/rerank_test.go` — wiring + ordering tests.
- `docs/eval/baseline_v8_codeembed.json`, `docs/eval/baseline_v9_rerank.json` — A/B results.

**Touch:**
- `internal/search/searcher.go` — rerank seam + `Timings.Rerank`.
- `internal/eval/runner.go` — capture/aggregate `RerankMS` (p50/p95).
- `cmd/ragcodepilot/main.go` — `--rerank` family of flags, `resolveReranker`, pass reranker into `runSearch`/`runEval`.
- `config.yaml` / README — document the chosen embedder + reranker model and the local rerank service, if any.
- `mvp_roadmap.md` — point Phase 3 (reranking) + the embedding-lever row at this doc; record the go/no-go outcome.
- `graphrag.md` — record the narrowed/deferred scope after step 8.

No changes needed for the embedding swap itself — it is config + re-index only.

---

## Verification

**Embedding A/B (Lever 1):**

```
# index candidate into an isolated collection (current index untouched)
ragcodepilot index --language go --collection code_chunks_codeembed --ollama-model <code-model> .

# eval both, full set + structural subset, JSON for comparison
ragcodepilot eval --collection code_chunks            --ollama-model nomic-embed-text --output json > /tmp/base_nomic.json
ragcodepilot eval --collection code_chunks_codeembed  --ollama-model <code-model>     --output json > /tmp/cand_codeembed.json
ragcodepilot eval --collection code_chunks_codeembed  --ollama-model <code-model> --subtype structural --output json > /tmp/cand_codeembed_structural.json

python3 docs/eval/compare.py docs/eval/baseline_v6.json /tmp/cand_codeembed.json
python3 docs/eval/compare.py docs/eval/baseline_v7_structural.json /tmp/cand_codeembed_structural.json
```
Check: `hit@5` / `MRR@5` up on navigation, recall gap narrows, no >2pp regression on concept/behavior, `negative_pass_rate` = 1.00.

**Reranker A/B (Lever 2):** same collection, toggle `--rerank`:

```
ragcodepilot eval --subtype structural --output json > /tmp/rr_off.json
ragcodepilot eval --subtype structural --rerank --reranker <impl> --output json > /tmp/rr_on.json
python3 docs/eval/compare.py /tmp/rr_off.json /tmp/rr_on.json
```
Check: `MRR@5` up, Bucket B `hit@5` up, latency `Rerank` p95 ≤ 200ms warm, no regressions, negatives hold.

**Unit/wiring:** `go test ./... -race -count=1` — `FakeReranker` reorders
deterministically; rerank stage is skipped (byte-identical output) when
`--rerank` is off; `Warmup` is type-asserted and skipped for impls that don't
implement `Warmer`.

**Corpus discipline:** every comparison must be on the *same indexed corpus*
(the `building_ragcodepilot.md` corpus-drift lesson). The embedding A/B uses two
collections of the *same source tree*; the reranker A/B uses one collection.

---

## Risks & tradeoffs

- **Local code-embedder availability.** The biggest unknown. If none is cleanly
  `ollama pull`-able, Lever 1's "free" status evaporates (GGUF import / sidecar).
  Gate the whole lever on step 1's finding.
- **Reranker runtime is a new dependency.** Every real option adds a service
  (rerank server / sidecar) or build complexity (ONNX). The `Reranker` interface
  isolates the choice, but the operational footprint grows beyond "Qdrant +
  Ollama." Keep it behind `--rerank` (off by default).
- **Latency.** Cross-encoders add per-query cost; hold the ≤200ms warm budget,
  document if exceeded. (Note: this is *retrieval* latency, separate from
  `--answer` generation latency.)
- **Re-baseline churn.** New embedder = new corpus fingerprint; re-baseline and
  update the "current canonical baseline" pointer in
  `retrieval_quality_decisions.md` if promoted.
- **Negative-query safety.** A stronger embedder could raise a negative query's
  top-1 score above its golden threshold; the eval gate checks
  `negative_pass_rate` explicitly.

---

## Relationship to GraphRAG

These levers and GraphRAG are **complements, not competitors** (see
`graphrag.md` → "Composition with reranking", and landscape doc §5):

- **Better embeddings** raise the recall floor → fewer Bucket A misses for
  GraphRAG to rescue.
- **Reranker** is the *precision* stage that the eventual `expand → merge →
  rerank` pipeline wants anyway — building it now is not wasted if GraphRAG
  later ships; the reranker slots on top of graph candidates.
- Running these first **scopes** GraphRAG honestly: it earns the L only for the
  query classes the cheap levers provably cannot reach.

---

## Decisions needed (before step 1)

1. **Is local-first firm?** If a cloud code embedder (voyage-code-3) or a
   non-Ollama local rerank server is acceptable, the option set widens. Default
   assumption here: local-first holds; cloud is out of scope.
2. **Which code embedder** to try first (nomic-embed-code vs
   jina-embeddings-v2-base-code vs a stronger general model) — decided by step 1
   availability check.
3. **Reranker runtime** (local rerank server vs Python sidecar vs pure-Go ONNX
   vs LLM-as-reranker-for-signal) — decided at step 6, prototyped at step 4.
