# Building ragcodepilot — The Story

How a learning project about vector databases turned into a working RAG system, one decision at a time.

---

## The starting question

It began with a simple idea: *learn how vector databases work by building something real on top of one.*

The original plan was ambitious — build a vector database engine from scratch in Go, then use it for code search. Custom storage, custom HNSW indexes, custom WAL, the works. On paper, it would teach everything about vector DB internals.

In practice, it would take months before you could search for anything.

So the plan flipped. Instead of building the engine first and the app second, build the app first and study the engine later. Use Qdrant — a real, production vector database — as the foundation. Get to a working search result in days, not months. Learn by using, then learn by reading, then (maybe someday) learn by rebuilding.

This was the first important decision: **top-down, not bottom-up.**

---

## Phase 1 — Making it work at all

The first goal was embarrassingly simple: type a question in English, get back relevant source code.

The pieces fell into place one at a time:

**The file walker.** Recursively scan a directory, skip `.git` and `node_modules` and `vendor`, find source files by extension. Simple, boring, necessary.

**The chunker.** You can't embed an entire file — it's too long and too noisy. So you split files into chunks. The first version was a sliding window: take 40 lines, slide forward by 30, repeat. Crude, but it worked. Every chunk got metadata: file path, language, line numbers.

**The embedder.** This is where text becomes math. Each chunk of code goes through `nomic-embed-text`, a model running locally via Ollama, and comes out as an array of 768 floating-point numbers — a *vector*. The magic property: code that means similar things produces vectors that are close together in 768-dimensional space. A Rust function called `recover_from_wal()` and a Go function called `recoverWAL()` end up near each other, even though not a single character matches.

**The vector database.** Qdrant, running in a Docker container. Each chunk becomes a *point*: its vector (for searching by meaning) plus its payload (file path, language, content — for filtering and display). Qdrant's job is simple but critical: given a query vector, find the nearest stored vectors. Fast.

**The search path.** When you type `ragcodepilot search "how does crash recovery work"`, the query goes through the same embedder, becomes a vector, and Qdrant finds the stored vectors closest to it. The top results come back with their source code and metadata, ranked by cosine similarity.

It worked. Not perfectly — the chunks were arbitrary, the ranking was rough, and exact function names sometimes got lost in the semantic haze. But you could ask a question in plain English and get back actual relevant code. That felt like magic.

Along the way, small details turned out to matter more than expected:

- **Dimension auto-detection.** The embedder returns 768-dimensional vectors, but what if you switch models? Instead of hardcoding the number, the system detects it from the first response and validates every batch after. One wrong dimension in a batch would silently corrupt search results forever. This small defensive choice prevented hours of debugging later.

- **Enrichment.** Raw code embeds poorly for natural-language queries. The word "chunker" doesn't appear in `func ChunkFile(...)` — the function *is* the chunker, but the embedding model doesn't know that from the code alone. The fix: before embedding, prepend metadata to give the model context.

  Here's what happens without and with enrichment:

  ```
  ┌─────────────────────────────────────────────────────────────────────┐
  │                     WITHOUT ENRICHMENT  ❌                         │
  │                                                                     │
  │   What goes to the embedding model:                                │
  │   ┌───────────────────────────────────────────┐                    │
  │   │ func ChunkFile(path string, lines int)    │                    │
  │   │     []Chunk {                              │                    │
  │   │     // split file into overlapping chunks  │                    │
  │   │     ...                                    │                    │
  │   │ }                                          │                    │
  │   └───────────────────────────────────────────┘                    │
  │                        │                                            │
  │                        ▼                                            │
  │               Embedding Model                                      │
  │                        │                                            │
  │                        ▼                                            │
  │   Model understands: "a function that takes a path and splits"     │
  │   Model does NOT know: this is the CHUNKER in the INGEST pipeline  │
  │                                                                     │
  │   Query: "how does code chunking work?"                            │
  │   Match score: ~0.45  (weak — no word "chunk" in the question      │
  │                        connects to "ChunkFile" in the code)        │
  └─────────────────────────────────────────────────────────────────────┘

  ┌─────────────────────────────────────────────────────────────────────┐
  │                      WITH ENRICHMENT  ✅                            │
  │                                                                     │
  │   What goes to the embedding model:                                │
  │   ┌───────────────────────────────────────────┐                    │
  │   │ File: internal/ingest/chunker.go          │ ← metadata header  │
  │   │ Language: go                              │    prepended       │
  │   │ Type: function                            │    before          │
  │   │ Name: ChunkFile                           │    embedding       │
  │   │ ─────────────────────────────────────     │                    │
  │   │ func ChunkFile(path string, lines int)    │ ← same raw code   │
  │   │     []Chunk {                              │                    │
  │   │     // split file into overlapping chunks  │                    │
  │   │     ...                                    │                    │
  │   │ }                                          │                    │
  │   └───────────────────────────────────────────┘                    │
  │                        │                                            │
  │                        ▼                                            │
  │               Embedding Model                                      │
  │                        │                                            │
  │                        ▼                                            │
  │   Model understands: "a Go function called ChunkFile in the        │
  │                        ingest/chunker that splits files"            │
  │                                                                     │
  │   Query: "how does code chunking work?"                            │
  │   Match score: ~0.82  (strong — "chunker", "ChunkFile",           │
  │                        "ingest" all connect to "chunking")         │
  └─────────────────────────────────────────────────────────────────────┘
  ```

  The critical detail: enrichment only changes **what the embedding model sees**. The raw code stored in Qdrant's payload is unchanged — you still see the original source in search results. It's like putting a label on a box before filing it: the contents don't change, but the filing system knows where to find it.

  ```
  Ingestion pipeline with enrichment:

  Source file                    Enriched text              Raw code
  ┌──────────┐    ┌──────────┐  ┌──────────────┐          ┌──────────────┐
  │ .go file │───▶│ Chunker  │─▶│ enrichFor     │──────────▶│ Qdrant       │
  └──────────┘    └──────────┘  │ Embedding()  │          │ payload:     │
                                └──────┬───────┘          │  content =   │
                                       │                   │  (raw code)  │
                                       ▼                   └──────────────┘
                                ┌──────────────┐
                                │ Embedding    │──────────▶ Qdrant vector:
                                │ Model        │           [0.82, -0.31, ...]
                                └──────────────┘
                                                           (captures meaning
                                                            WITH context)
  ```

---

## The vision review — an honest look

Before building more features, the project got an honest review. The feedback was blunt:

1. **"The system is not RAG. It's semantic code search."** The project was named `ragcodepilot` but had no generation — no "G" in RAG. The README was honest about this; the name wasn't.

2. **"The biggest weakness is the missing evaluation harness."** Without metrics, every improvement was a guess. Did the enrichment actually help? By how much? Nobody knew.

3. **"Don't add generation to look more AI."** The retrieval-only design was correct for code search. Add generation only if a real need surfaces.

4. **"Build the eval before adding any retrieval feature."** You can't tell if hybrid search helps if you can't measure what "helps" means.

This review reordered everything. Instead of jumping to hybrid search, the next priority became: *measure what you already have.*

---

## Phase 1.5 — The evaluation harness

A golden dataset: 23 hand-written queries covering four types:

- **Navigation** — "where is `ChunkFile` defined?" (expects a specific file and symbol)
- **Concept** — "how does code chunking work?" (expects relevant files)
- **Behavior** — "what happens when vector dimensions are inconsistent?" (expects validation code)
- **Negative** — "where is the OAuth middleware?" (expects *nothing* — this project has no OAuth)

For each query, the system runs the search, checks whether the expected result appears in the top-k, and computes metrics:

- **hit@5** — did the right chunk appear anywhere in the top 5? This is the number that matters most for RAG: if the right chunk is in the top 5, an LLM can find and use it.
- **MRR@5** — how high did the first correct result rank? Top-1 scores 1.0, top-2 scores 0.5, not found scores 0.

The first baseline: `hit@5 = 0.789`, `MRR@5 = 0.632`. Not bad, but not great. Navigation queries — the "where is X" questions — were the weakest. Dense vector search doesn't understand identifiers well. When you search for `ChunkFile`, the embedding model sees general concepts about chunks and files, not the specific identifier.

Now there was a number to beat.

---

## Phase 2 — The keyword problem

Pure semantic search has a blind spot: exact names. If you search for `ValidateVectorBatch`, the embedding model converts your query into a meaning-vector somewhere near "check vectors" and "batch validation." But it doesn't know that `ValidateVectorBatch` is a *specific function name* that should match *exactly*.

The fix is hybrid search — combine semantic similarity (dense vectors) with keyword matching (sparse vectors).

**The sparse vector pipeline** works differently from dense. Instead of a neural network, it uses BM25 — a decades-old algorithm that scores documents based on which query words they contain and how rare those words are. A code-aware tokenizer splits `ValidateVectorBatch` into `["validate", "vector", "batch"]`, removes stop words like `func` and `return`, and weights each token by how rare it is across the whole corpus. The result is a *sparse vector*: an array that's mostly zeros, with non-zero values only for the tokens that appear.

At search time, Qdrant runs both searches in parallel — dense (meaning) and sparse (keywords) — and combines them with **Reciprocal Rank Fusion**: a simple formula that rewards chunks ranked highly by *either* method. A chunk that's #1 in keyword search and #50 in semantic search still gets a meaningful combined score.

The initial implementation used TF-IDF (the simpler predecessor to BM25). It passed all exit criteria:

> Navigation hit@5 jumped from 0.500 to 0.750 — **+25 percentage points.**

But a follow-up experiment with BM25 did even better. The key difference: BM25 has *saturation*. In TF-IDF, a word appearing 10 times scores 10× more than appearing once. In BM25, the curve flattens — 10 occurrences score only about 1.8× more than 1. This matters in code, where a utility function might repeat a variable name dozens of times without being more relevant.

BM25 with `k1=0.5` (aggressive saturation, good for short code chunks) pushed hit@1 up by **+15.8 percentage points**.

But one query broke: `hasher_concept` — "how are file hashes computed for re-indexing." The query says `hashes` (plural); the code has `HashFile` (singular). The tokenizer treated these as different words. Fix: **Snowball stemming.** The tokenizer now adds `hash` alongside `hashes`, matching both forms. The broken query recovered, and every metric was equal or better than TF-IDF.

Final Phase 2 numbers: **hit@5 = 0.895, MRR@5 = 0.699.** The right chunk was in the top 5 for nearly 90% of queries.

---

## The pivot — retrieval is good enough, now what?

The original roadmap said: Phase 3 = reranking + Rust chunker, Phase 4 = UX polish, Phase 5 = *maybe* add generation if someone asks for it.

But Phase 2's results changed the calculus. With hit@5 at 0.895, the retrieval was strong enough to feed an LLM. And the question the vision review had flagged — "does generation add value to this system?" — was still unanswered. Every other planned improvement (reranking, Rust chunking, UX polish) was an *internal* improvement. None would tell you whether RAG was the right product shape.

So Phase 5 jumped the queue. The reasoning:

1. **The retrieval gate was cleared.** hit@5 = 0.895 means the right chunk is in the top-5 for ~90% of queries. An LLM doesn't need perfect retrieval; it needs good-enough context.
2. **Reranking only refines existing signal.** It reorders the top-50 but can't add information that isn't there. Generation *uses* the existing signal to answer a fundamentally different question.
3. **The cost of being wrong is 4 days.** If `--answer` proves useless, kill it and go back to reranking. If you spend 2 weeks on reranking first, you still don't know if RAG is the right direction.

---

## Phase 5 v0 — The "G" in RAG

The implementation was deliberately minimal. A new package — `internal/answer/` — with four files:

- **`generator.go`** — an interface with one method: `Generate(prompt) → string`. Simple enough that a fake implementation fits in 20 lines.
- **`prompt.go`** — builds the LLM prompt: a system message ("answer from the chunks, don't invent") plus a user message (the question + the top-5 chunks with citation numbers).
- **`ollama.go`** — HTTP client to Ollama's `/api/chat` endpoint. Same server already used for embeddings, just a different model (`qwen2.5-coder:7b` instead of `nomic-embed-text`).
- **`fake.go`** — returns a canned response for tests.

The CLI got three new flags: `--answer` (enable generation), `--generator` (ollama or fake), and `--ollama-generative-model` (which LLM to use).

The flow: search as before → take the top-5 chunks → build a prompt → send to the LLM → print the answer followed by the source chunks. Without `--answer`, the output is byte-identical to before.

No streaming, no citation validation, no faithfulness checks. The point wasn't to build the best possible answer mode — it was to build the smallest possible one and use it for a week to see if generation adds value over raw chunks.

---

## Phase 5 v0 — what dogfooding changed mid-stream

The plan said: *ship the minimal version, dogfood for a week, decide what to build next.* That happened — but during the dogfooding, three things slipped from "v1 maybe" into "v0 must," because the eval and the experience demanded them.

**Auto-warming.** The first `--answer` call took 30+ seconds. Not generation — model loading. Ollama loads the model into RAM on first request; if it's been idle long enough, it unloads. With `stream:false`, the HTTP client timeout has to cover the entire generation, model-load included. The fix wasn't a longer timeout — it was an *optional interface*: `Warmer` with a `Warmup(ctx)` method that `OllamaGenerator` implements and `FakeGenerator` doesn't. The CLI does a type assertion: warm if it can, skip if it can't. Combined with `OLLAMA_KEEP_ALIVE=-1` (documented in the README), subsequent calls hit a warm model and complete in 5–15 seconds.

**Greedy decoding.** Default Ollama sampling (temperature ~0.8) produced different answers run-to-run for the same prompt. The eval couldn't be reproducible. The fix: set `temperature: 0` and a fixed `seed`. Generation is now deterministic given a fixed prompt and model. Side benefit — greedy decoding tends to be more grounded, less prone to invention.

**`--answer-limit`.** The eval forces `--limit ≥ 10` for `recall@10`, but the product shipped `--limit 5` by default. That meant the eval was scoring answers built from 10 chunks while users got 5 — measuring a configuration nobody ran. The fix: decouple them. `--limit` controls the retrieval window; `--answer-limit` (default 5) controls how many of those chunks the generator sees. The eval keeps its top-10 view for recall metrics while the answer reflects the shipped product.

Three small changes, each driven by something the metric or the experience surfaced. None were in the original v0 plan.

---

## The Tier B insight — verify before fixing

A new eval mode landed: `eval --answer`. It runs the normal retrieval eval, then for every query also runs the generator and scores the answer with three deterministic, reference-free metrics:

- **Well-formedness** — the answer is non-empty.
- **Citation validity** — every `[N]` reference parses, and `1 ≤ N ≤ chunks_in_prompt`.
- **Refusal-on-negative** — for negative queries (questions the corpus can't answer), did the model say "I don't have enough information" rather than invent one?

The first run set off an alarm: refusal rate on negatives was **0.50** — half the unanswerable questions got confident answers. That looked like a real hallucination problem worth a low-confidence guardrail.

But before fixing anything, the next step was just to read the actual answer text. Dumped them. All four negatives had refused — in prose. Two used wording the heuristic caught ("does not contain"), two used wording it didn't ("is not implemented in the provided code chunks"). The model was behaving well; the *measurement* was undercounting.

Fix: widen the phrase markers. Refusal rate went to 1.00. No model change, no guardrail, no v1 feature pulled forward — just a corrected metric.

The lesson became a rule worth following: a metric is a measurement, not ground truth. The instinct to fix what the number says is broken is wrong when the number itself is wrong. **Read the raw output before fixing anything.**

---

## Test files are not neutral

Negative queries are the eval's hallucination floor — if the model invents an answer to "where is the OAuth middleware?", that's a real failure. So when one of those queries returned a *citation* to `internal/eval/dataset_test.go`, that needed explaining.

The answer was simple and slightly embarrassing: `dataset_test.go` contains the literal string `oauth_middleware` as a fixture for the eval's own test. By indexing `*_test.go` files, the eval was leaking its own answer key into search results — the model was reading the test that defined the negative case.

Fix: skip `*_test.go` by default. Configurable via `skip_file_patterns` in `config.yaml`; an explicit empty list (`[]`) disables the skip for users who want everything indexed.

The re-baseline after this fix surfaced something else. Re-indexing also walked into `.claude/worktrees/`, a git worktree directory left over from a previous subtask. That meant a *second copy of the entire codebase* was being indexed — every chunk had a duplicate. Hit@1 dropped 10pp from the duplicate competition; failures suddenly showed top-1 results inside the worktree's path.

Fix: skip hidden directories by default. The same rule `git` and `rg` use — descend into source dirs, not VCS or tooling state.

Two defaults — exclude test files, exclude hidden directories — that together took the eval back to honest numbers. Both were silent failures: the eval still ran, the metrics still printed, they just measured a corpus the user didn't have in mind.

---

## Corpus drift — the silent regression

A baseline isn't an algorithm score. It's an algorithm score *against a specific corpus.* This sounds obvious but it bites in practice.

After Phase 5 v0 shipped, the eval was re-run on the freshly-indexed repo. Hit@5 dropped from 0.895 to 0.789. No algorithm change.

The cause was the project itself: the Phase 5 code (`internal/answer`, the new `internal/eval` files) had been added to the corpus. New chunks competed for top-K slots. `hasher_concept` (a query that had been carefully recovered by Snowball stemming in Phase 2) failed again — not because stemming stopped working, but because new hash-adjacent code now ranked higher. Pure corpus drift.

The recovery came from the test-file hygiene above. `*_test.go` exclusion shed about a third of the chunks. Hit@5 returned to 0.895 on the cleaner corpus.

The rule that came out of this: **the corpus is part of the experiment.** Before declaring "the new reranker added 5pp" or "stemming regressed," re-baseline on the current corpus. Otherwise the change-under-test is conflated with the change-in-inputs, and the conclusion is meaningless.

---

## The recall gap — a new diagnostic

A new line in the eval report: `recall gap = recall@10 − recall@5`. One number that tells you which retrieval investment will help next.

- **Gap ≥ 0.10** — relevant chunks *are* retrieved (they're in top-10) but ranked outside top-5. Reranking has headroom to lift them.
- **Gap < 0.10** — relevant chunks aren't being retrieved at all. Embedding or chunking is the floor; reranking can't surface what retrieval didn't return.

The diagnostic prints automatically alongside hit@k and MRR@5. It started as a hand-rolled calculation that informed the GraphRAG-vs-reranker decision below; making it a permanent report line means future retrieval choices don't have to re-derive the same logic from raw numbers.

A subtle point: this metric isn't about answer quality directly — it's a *forecast tool*. It tells you whether the next experiment is worth running before you build it.

---

## Phase 6 — GraphRAG, and a deliberate bet

With retrieval back at hit@5 ≈ 0.89, the next bottleneck appeared. Navigation queries — "where is X defined", "what calls Y" — were the only type still below 1.0 on hit@5. These are *structural* questions, not similarity ones. The answer lives in the edges between chunks, not in the chunks themselves.

The original roadmap had Phase 3 = cross-encoder reranking next. But reranking only reorders what hybrid already retrieved — it can't add a signal that doesn't exist in the embedding/BM25 space. "What calls X" isn't a semantic-similarity problem; if the caller never names the callee's purpose, no amount of reranking helps.

So the next phase pivoted: **GraphRAG** — a graph layer over the code structure with edges for `defines`, `calls`, and `imports`, populated by a Go AST pass at ingestion time and stored in local SQLite. At search time, expand from hybrid's top-50 seeds via 1–2 hop neighbors and rescore.

To validate this gamble, the golden set grew by 16 hand-curated structural queries: "what calls `Pipeline.Run`", "trace from `runSearch` to `qdrant.Client.Search`", "what would break if I change `Embedder.Embed`'s signature." The eval doubled its navigation count and gained two new sub-patterns: traces and change-impact.

The hybrid baseline on the structural subset (`baseline_v7_structural.json`):

- hit@5 = 0.875 (14 of 16 pass)
- recall@5 = 0.60, recall@10 = 0.75
- **recall gap = 0.15** — well above the reranker-headroom threshold

This is where the decision got honest. The recall gap triggers the reranker rule by the standing diagnostic. So the choice "GraphRAG over reranker" can't honestly be framed as "reranker can't help" — it has to be framed as a **deliberate bet**: *structural signal will pay more on these queries than reordering pays on the gap.* Phase 3 reranking is parked, not cancelled; the data justifies it on the gap alone, but the bet says GraphRAG will help more on the specific structural failures.

Per-query analysis split the 16 structural queries into three buckets:

| Bucket | Count | Shape | Lever |
|---|---|---|---|
| **A** — hard fails (chunk absent from top-10) | 2 | embedding-shaped | GraphRAG or embedding upgrade |
| **B** — partial recall (chunk in top-10, not top-5) | 7 | reranker-shaped | reranker OR cheaper: raise `--answer-limit` |
| **C** — fully resolved | 7 | hybrid is enough | none |

The hypothesis from this split was: **`--answer-limit 8` should help 7 of 16 structural queries for free.** No graph store, no reranker, no new infrastructure — just bigger answer context, since `recall@10 ≫ recall@5` for those queries. If raising the limit closed Bucket B, GraphRAG's unique scope would narrow to Bucket A (2 queries).

So the gating sequence began with that A/B (2026-05-28). What it found is worth recording, because the result was different from the prediction:

- **Retrieval (identical, sanity).** AL=5 vs AL=8 hit@5 = 0.875 either way.
- **Tier B answer shape — flat.** CitedRate 0.938 both runs (one uncited query each, different queries), AllCitationsValidRate 1.00 both, zero dangling refs in either.
- **Latency — significantly worse.** p50 generation 23.9s → 37.1s (**+55%**), total wall-clock +31%. Every query in the structural subset got slower at AL=8.

The retrieval logic was right — those rank-6–10 chunks *are* in the prompt at AL=8 — but the prediction that *answer-shape metrics would lift* didn't hold. And Tier B can't see whether the *content* of the answer improved; that's a correctness question requiring either dogfooding judgment or a Tier C faithfulness judge.

So the gate moved: the bucket reasoning's "free win" is **inconclusive on automated metrics**, with a real latency cost on top. The next step isn't an eval run — it's a side-by-side dogfooding pass on multi-chunk questions, reading AL=5 and AL=8 answers and judging which is more complete or correct.

Subtler implication for Phase 6: a true retrieval-side fix (reranker or graph) would put Bucket B's chunks *into top-5*, so the LLM gets them at AL=5 — same prompt content, *without* the +55% latency. The eval-side AL=8 finding therefore strengthens the case for an actual retrieval fix, not weakens it.

Phase 6 stays designed and gated, with the gate updated: dogfood AL=5 vs AL=8, then decide whether to ship GraphRAG, narrow it, or accept the 2 hard fails. Phase 2 was "build it and measure"; Phase 6 is "measure first, then re-measure with humans, then decide."

---

## Where things stand now

The system as it exists:

```
Source code → Walk → Chunk → Enrich → Embed (dense + sparse) → Qdrant

Query → Embed → Hybrid search (dense + BM25 + RRF) → Top-5 chunks
                                                          │
                                          ┌───────────────┤
                                          ▼               ▼
                                    (default)        (--answer)
                                    Print chunks   → LLM → Answer + Sources
```

Seven baselines on disk, from `v1` (Phase 1 dense-only) through `v7_structural` (latest, 16-query structural subset). The full test suite passes with race detection. Five CLI commands: `index`, `search`, `eval`, `collections`, `version` — plus the `--answer`, `--answer-limit`, `--subtype` flags layered onto the existing ones.

The numbers tell a story of steady improvement — with one honest dip-and-recovery that's part of the lesson:

| Baseline | hit@5 | MRR@5 | What changed |
|---|---|---|---|
| v1 (dense only) | 0.789 | 0.632 | Phase 1 first measurement |
| v2 (+ TF-IDF hybrid) | 0.895 | 0.607 | Keyword matching added |
| v4 (+ BM25 + stemming) | 0.895 | 0.699 | Better scoring, stemming fix |
| v5_pre (post-Phase-5 corpus) | 0.789 | 0.660 | Corpus grew with Phase 5 code; same algorithm |
| v6 (+ `*_test.go` exclusion) | 0.895 | 0.673 | Test-file hygiene recovered hit@5 |
| v7 (+ 16 structural queries) | 0.886 | 0.717 | Bigger golden set; aggregate held |
| v7_structural (16-q subset) | 0.875 | 0.771 | Phase 6 comparison target |

Each number was earned by building something, measuring it, and keeping only what the eval proved helped. The `v5_pre` row is the corpus-drift story made literal — same code, refreshed input, lost 10pp — and `v6` is the recovery.

---

## The decisions that shaped the project

Looking back, a few choices mattered more than the code:

**"Measure before building"** was the most important principle. The eval harness took a few days to build. It saved weeks of wasted effort by making every subsequent decision evidence-based instead of speculative.

**"Top-down, not bottom-up"** got the project to a working state in days instead of months. The urge to build everything from scratch is strong for a learning project, but building the *application* first taught more about what a vector database needs to do than building the *engine* first would have.

**"Additive, behind a flag"** kept the system stable through multiple changes. Hybrid search defaulted to `hybrid` but supported `--mode dense` for regression checks. Answer mode defaulted to off. Every new feature was safe to ship because the old behavior was always one flag away.

**"Small scope, honest eval"** prevented scope creep. Phase 5 v0 could have included streaming, citation validation, faithfulness scoring, multi-provider support. It shipped with none of those — just enough to answer the question "does generation help?" Everything else waits for the answer.

**"Verify before fixing"** showed up during dogfooding (the refusal-rate-0.50 story). The instinct to fix what the metric says is broken is wrong when the metric itself is wrong. Always dump the raw output before changing the system in response to a number. This becomes harder to remember the more confident the metric pipeline looks.

**"The corpus is part of the experiment"** is the rule that came out of corpus drift. A baseline isn't an algorithm score; it's an algorithm score against a specific corpus. Before declaring a regression or a win, re-baseline on the corpus you're actually testing on. Otherwise the change-under-test is conflated with the change-in-inputs and the conclusion is meaningless.

**"Choose levers in cost order"** is the rule that came out of the Bucket A/B/C analysis. When the data shows two ways to fix the same problem — one cheap, one expensive — try the cheap one first, even if it sounds less interesting. The expensive one's scope often narrows after the cheap one ships.

---

## What comes next

The first product question — *does `--answer` mode add value over raw chunks?* — has a tentative yes from limited dogfooding (the model refuses on negatives, citations resolve, well-formed rate is 1.00), with the caveat that latency on 7B is real. The next decision isn't *whether* `--answer` helps; it's *which retrieval investment helps the specific failures the eval surfaced*.

The evidence so far, drawn from the Bucket A/B/C split on `baseline_v7_structural` plus the AL=8 A/B (2026-05-28):

- **Bucket B (7 queries)** has relevant chunks at rank 6–10. The cheap-lever hypothesis — raise `--answer-limit` from 5 to 8 — was tested: Tier B shape metrics stayed flat, generation latency rose +55% at p50. The retrieval logic was right (those chunks are in the prompt at AL=8), but the *content* benefit is invisible to Tier B and needs human judgment.
- **Bucket A (2 queries)** has the right chunks absent from top-10 entirely. Reranking can't help; GraphRAG might, if those chunks are reachable through `calls`/`defines` edges from hybrid's seeds.
- **Bucket C (7 queries)** is already fully resolved.

So the next concrete steps, in order:

1. **Dogfood AL=5 vs AL=8 on 3–5 multi-chunk questions.** The eval can't tell us whether AL=8's bigger prompt actually produces better answers; a human reading both side-by-side can. If the verdict says AL=8 materially helps, raise the default and narrow Phase 6 to Bucket A. If not, leave the default at 5; Phase 6 keeps its full scope.
2. **Phase 6 (GraphRAG)** — start with scope set by the dogfooding verdict. The plan is fully drafted at [`../plan/graphrag.md`](../plan/graphrag.md), with per-query gating and the dogfooding prerequisite baked in. Note: the AL=8 result actually *strengthens* the retrieval-side case, since a reranker or graph would land Bucket B's chunks in top-5 without the +55% latency penalty.
3. **`index --watch`** — independent track, S–M effort. The trigger has been met for weeks (the eval cycle keeps re-indexing manually); pick it up whenever a quiet stretch allows.

Further out:

- **Phase 5 v1** features — multi-provider (OpenAI-compatible HTTP, native Anthropic), faithfulness judge (Tier C), streaming, refusal-on-low-confidence guardrail. Gated on dogfooding signal. Current verdict: the model self-refuses cleanly on negatives (pass = 1.00), so the refusal guardrail is justified-but-not-urgent.
- **Phase C** (custom Go vector DB) — still the long-term learning goal that started this project. Original ambition, now on a distant horizon. The decision to go top-down was the right one; eventually the bottom-up tour is still worth taking.

The thread through all of this remains the same: every change carries a measurement that justifies it, and the next change waits for the measurement of the last one. The system searches code, answers questions, measures its own quality, and now has a forecast tool — the recall gap — that tells it which kind of measurement to take next. It got here one honest decision at a time.
