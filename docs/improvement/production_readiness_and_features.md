# Production Readiness & Practical Feature Proposals

> External system review, 2026-07-06. Covers: gaps that block real-world / production
> use, and feature proposals that would make ragcodepilot more practical, with the
> reasoning for each. Sizes are t-shirt (S/M/L/XL). Companion to
> [`../plan/mvp_roadmap.md`](../plan/mvp_roadmap.md) — nothing here overrides the
> roadmap; it feeds the next re-prioritization.

---

## Part 1 — Production gaps (fix before real users)

### 1.1 Full-corpus re-embedding on any change ⚠️ scaling blocker [M]

**Problem.** Change detection (`reindexing.md`) skips work only when *nothing*
changed. The moment one file changes, `Pipeline.Run` re-chunks and re-embeds
**every current file** (the chunking loop iterates all files, not `filesToIndex`),
because BM25 IDF is corpus-wide and all sparse vectors must be rewritten with the
new weights. At today's 182 chunks this is invisible. At the system-design target
(~200K chunks, ~30 min ingest), **a one-line edit costs a full 30-minute re-embed**.
Watch mode makes this worse: every debounced save triggers it.

**Key insight: only the *sparse* vectors depend on IDF. Dense embeddings depend
only on chunk content** — re-embedding an unchanged chunk through Ollama produces
the same vector and is pure waste (and embedding is the documented bottleneck).

**Fix (pseudocode):**

```
on re-index with changes:
    embed only chunks of changed/new files          # the expensive Ollama calls
    recompute IDF over full corpus                  # cheap, local, already done
    for unchanged chunks:
        rebuild sparse vector locally                # cheap, no Ollama
        update ONLY the sparse named vector in Qdrant  # partial vector update API
    for changed chunks:
        upsert dense + sparse as today
```

Qdrant supports per-point named-vector updates, so dense vectors of unchanged
points never move. Fallback if partial updates prove awkward: a local
content-addressed embedding cache `hash(model, enriched_text) → vector`, so
"re-embedding" unchanged chunks becomes a cache hit (this cache also pays for
itself in Lever 1 model A/Bs — see `../plan/cheaper_levers.md`).

**Value:** re-index cost becomes proportional to the diff, which is the property
watch mode and any multi-repo setup silently assume already exists.

### 1.2 Eval is self-referential — validate on external repos [M]

**Problem.** Every baseline ever committed measures ragcodepilot searching
**its own source**, with golden queries written by the same person who wrote the
code. Three compounding risks:

- **Overfitting:** decisions (BM25 k1=0.5, stemming, test-file exclusion,
  GraphRAG-vs-reranker) are tuned to one small Go repo's vocabulary.
- **Statistical fragility:** 39 queries, 182 chunks. hit@5 = 0.895 means ~2
  misses; every pivot is riding on 2–3 queries. The docs admit v5→v6 drift came
  from *the eval code itself being indexed* — the instrument measures itself.
- **No transfer evidence:** zero data that quality holds on a repo the embedder's
  enrichment format and the tokenizer weren't hand-fit to.

**Fix:** pick 2–3 well-known OSS Go repos of different sizes/styles (e.g. a CLI
tool, an HTTP library, something with heavy interfaces), write 15–20 golden
queries each (same YAML schema), commit as `golden_external_<repo>.yaml`.
Run the full baseline matrix once. Any future retrieval change must not regress
externally even if it wins on the self-corpus.

**Value:** converts "works on my repo" into evidence; also stress-tests scale
assumptions (IDF over 10K+ chunks, ingest time, recall at realistic corpus size)
before a real user does.

### 1.3 Answer-mode latency is not a viable interactive product yet [S–M]

**Problem.** The AL=5/AL=8 A/B recorded **GenerateP50 ≈ 24s, P95 ≈ 46s** per
answer (qwen2.5-coder:7b, CPU). A 24-second synchronous wait with no output is
outside what users tolerate for an interactive CLI; this is the single biggest
product-experience gap, and it's barely visible in the roadmap (streaming is a
"v1 candidate").

**Fix ladder (cheapest first):**

1. **Streaming** (`stream: true`, print tokens as they arrive) — makes
   time-to-first-token the felt latency. Already sketched in
   `phase5_v0_answer_mode.md`; promote from v1-candidate to next-up. [S]
2. Print retrieval results immediately, then stream the answer below them — the
   user reads sources while the model writes. [S]
3. Model routing: default to a smaller/faster generative model; `--answer-model`
   for quality. Measure the quality delta with the existing Tier B harness. [M]

### 1.4 No CI regression gate on retrieval metrics [M]

**Problem.** The eval harness exists precisely to catch regressions, but gating
is manual ("the harness reports; you decide"). A refactor that silently drops
hit@5 will only be noticed if someone remembers to run eval.

**Fix:** a CI job (nightly or on-demand label, not per-PR — it needs Qdrant +
Ollama services) that indexes the repo, runs eval, and fails on:
`hit@5 < 0.85` (the documented RAG-readiness floor) or `negative_pass_rate < 1.0`.
Keep answer metrics report-only as designed. The floor values are already agreed
in `retrieval_quality_decisions.md` §3 — this just enforces them.

### 1.5 Onboarding friction & failure UX [S]

**Problem.** The stack is three moving parts (Go binary, Qdrant in Docker,
Ollama + pulled models). Every real-user failure in the first five minutes will
be "connection refused" from one of them, and today the user gets a raw gRPC/HTTP
error.

**Fix:** `ragcodepilot doctor` — checks Qdrant reachability, Ollama reachability,
embedding model present, generative model present, collection existence/dimension
match, and prints the exact fix command for each failure
(`docker compose up -d`, `ollama pull nomic-embed-text`, ...). Reuse the same
checks to make `index`/`search` fail with actionable messages instead of raw
transport errors.

**Value:** first-run success rate is the top-of-funnel for any adoption; this is
the cheapest lever on it.

### 1.6 Sensitive content goes into Qdrant payloads verbatim [S–M]

**Problem.** Chunk `content` is stored plaintext in Qdrant. Indexing a repo that
contains `.env`-style files, hardcoded keys, or credentials copies the secrets
into a second, less-audited store (and, with `--answer`, into LLM prompts). Fine
for a single-user local tool; a real problem the day a team shares a Qdrant
instance.

**Fix:** (a) default `skip_file_patterns` additions for common secret carriers
(`.env*`, `*.pem`, `id_rsa*`, ...); (b) optional lightweight secret-pattern scan
at chunk time (`AWS_`, `-----BEGIN ... PRIVATE KEY-----`, high-entropy strings)
that skips or redacts matching chunks with a warning. Behind
`config.yaml security.redact_secrets: true`, default on.

### 1.7 Small hygiene items [S each]

- **Doc drift:** `reindexing.md` still describes the pre-`IndexVersion` pipeline
  ("all 50 files are re-chunked and re-embedded") — it predates the
  index-version-refresh logic now in `pipeline.go`. Update it (and fold in 1.1
  when built).
- **`.gitignore` awareness:** the walker skips hidden dirs + configured
  `skip_dirs`, but does not read `.gitignore` — generated artifacts in
  non-hidden, non-configured dirs (e.g. `dist/`, `bin/`) get indexed on unknown
  repos. Parse the repo's `.gitignore` as an additional skip source.
- **Multi-language IDF inconsistency** is documented only in a code comment in
  `pipeline.go` (per-language re-index writes language-scoped IDF weights).
  Surface it in `hybrid_search.md` and the README until per-language corpus
  stats or a per-language collection convention exists.
- **Refusal detection is a phrase heuristic** (already flagged in eval README).
  Cheap improvement: require the refusal phrase AND absence of citations, cutting
  false positives from answers that merely quote the phrase.

---

## Part 2 — Feature proposals (make it useful in the real world)

Ordered by leverage, not effort.

### 2.1 MCP server mode — become the retrieval backend for coding agents [M] ★ highest leverage

**What:** `ragcodepilot serve --mcp` exposing the existing search as MCP tools
over stdio:

```
tool search_code(query, language?, repo?, limit?) → ranked chunks with paths/lines/scores
tool list_collections() → indexed repos + stats + staleness
(later) tool trace_calls(from, to) → call path        # once GraphRAG ships
```

**Why this changes the product's position.** As a standalone CLI, ragcodepilot
competes with `grep`, IDE search, and every coding agent's built-in retrieval —
a crowded field where "another terminal command" struggles for daily habit. As an
MCP server, it becomes **infrastructure those agents call**: Claude Code, Cursor,
and any MCP-capable client get persistent, indexed, hybrid semantic search over
repos the agent would otherwise re-grep on every task. The index outlives the
agent session — that's precisely the asset agents lack. This is also the honest
answer to "who is this for": not a human typing `ragcodepilot search`, but an
agent making 20 retrieval calls per task.

**Cost is genuinely M:** the search path already exists and returns structured
results; this is a protocol wrapper plus a long-running process (the
REPL/daemon triggers in `architecture_decisions.md` §5.3 — "multiple frontends"
— fire for real here, and MCP-over-stdio avoids the IPC/lifecycle costs that doc
worries about, since the client owns the process lifetime).

### 2.2 Exact-symbol fast path (before or alongside GraphRAG) [S–M]

**What:** during ingest, the Go AST chunker already knows every declared symbol.
Persist a `symbol → (file, line, chunk_id)` table (SQLite — the same store
GraphRAG plans). At query time:

```
if query matches "where is X" / "definition of X" / bare identifier X:
    exact = symbolTable.lookup(X)            # case-normalized
    if exact found:
        pin exact match at rank 1, fill rest with hybrid results
else:
    hybrid search as today
```

**Why:** the weakest query type is navigation, and most navigation misses are
"where is X defined" — an *exact-lookup* question being answered by a
*similarity* engine. This gives a deterministic, sub-millisecond answer for the
query class where fuzziness only hurts, at a fraction of GraphRAG's L. It also
**de-risks GraphRAG**: it is the `defines` edge shipped alone, and the eval delta
it produces on the structural subset tells you how much of the navigation gap
was ever graph-shaped (complementing the reachability dry-run already planned as
a prerequisite in `graphrag.md`).

### 2.3 Ship the composability pair: `--json` + `--context-lines` [S]

Phase 4 already lists these; pull them forward — they matter more now than
"polish" suggests. `--json` is a hard prerequisite for 2.1 (MCP), for scripting,
and for piping into other tools; `--context-lines N` fixes the mid-function
chunk-boundary problem readers hit today. Both are additive flags with zero risk
to the default path, which the project's own conventions favor.

### 2.4 Multi-repo workspace UX [M]

**What:** first-class support for the "index several repos, search across them"
workflow the single-collection design already technically allows:

```
ragcodepilot repos add <path> [--name]     # index + register in ~/.ragcodepilot/repos
ragcodepilot repos list                    # name, chunks, last indexed, staleness (git HEAD vs indexed SHA)
ragcodepilot repos refresh [name|--all]    # re-index registered repos
ragcodepilot search --repo api,web "..."   # cross-repo, already supported by payload filter
```

Store the indexed commit SHA per repo so `repos list` can say *"api: 14 commits
behind"* — honest staleness display instead of silent stale results.

**Why:** real codebases are multi-repo (service + client + shared lib). Cross-repo
semantic search is something IDE search genuinely cannot do and grep does badly —
it's the strongest human-facing differentiator available, and it's mostly UX over
existing plumbing.

### 2.5 Streaming `--answer` [S]

Covered in 1.3 — listed here because it is also the gateway to the REPL/chat
surface sketched in `phase5_v0_answer_mode.md` v2+. Do it before any REPL work.

### 2.6 Index-at-ref & CI-friendly indexing [M, later]

**What:** `ragcodepilot index --ref <sha>` (index a clean tree at a commit, via
git worktree/archive) plus the CI eval job from 1.4.

**Why:** teams that adopt 2.1/2.4 will next want a shared, reproducible index
built in CI rather than from someone's dirty working tree. Indexing at a ref
makes the index content-addressable ("this index is exactly commit `abc123`"),
which also gives answer-mode citations a stable anchor. Not urgent until there
are multi-user consumers — record the trigger: first request to share an index
across machines.

---

## Part 3 — Suggested sequencing

> **Superseded 2026-07-06:** this sequencing was adopted (with minor
> reshuffling) into the restructured [`../plan/mvp_roadmap.md`](../plan/mvp_roadmap.md)
> as milestones M0–M5. The roadmap is canonical; the table below is kept as the
> original proposal record.

Assuming GraphRAG's two prerequisites (reachability dry-run + AL dogfooding,
both <1 day, pending since 2026-05-28) run first — they are still the cheapest
information-per-hour available:

| Order | Item | Size | Rationale |
|---|---|---|---|
| 1 | GraphRAG prerequisites (already planned) + 2.2 symbol fast path | S–M | Cheapest signal on the navigation gap; may shrink GraphRAG's L |
| 2 | 1.3/2.5 streaming answers | S | Biggest felt-UX fix in the product today |
| 3 | 2.3 `--json` + `--context-lines` | S | Unblocks MCP + scripting |
| 4 | 2.1 MCP server | M | The product wedge; everything above feeds it |
| 5 | 1.1 incremental re-embed fix | M | Must land before external repos / watch mode at scale |
| 6 | 1.2 external-repo eval | M | Must land before promoting any model/reranker/graph "win" |
| 7 | 1.5 doctor, 1.6 secrets, 1.7 hygiene | S | Fast follows, batch as one "hardening" PR each |

The deliberate omission: **no new retrieval-quality lever is proposed here**.
The existing pipeline of levers (cheaper_levers → GraphRAG) is well-designed;
the binding constraints right now are *evidence quality* (1.2), *product surface*
(2.1, 2.5), and *scaling correctness* (1.1) — not another ranking algorithm.
