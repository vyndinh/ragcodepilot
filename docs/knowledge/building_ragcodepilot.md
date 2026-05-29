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

Plans for minimal versions love the phrase "v1 maybe." Real use has a way of dragging those things back into v0.

Three of them did.

**The model wouldn't warm up.** The first `--answer` call took 30 seconds. You sat there, watching the cursor blink. None of that time was generation — it was the model paging back into memory. Ollama unloads idle models; on a cold start the LLM had to be loaded before a single token came out. With `stream: false` the HTTP client's clock kept ticking the whole time, and the 60-second timeout barely held.

A longer timeout would have hidden the problem. The right fix was an *optional interface*: `Warmer`, with a single `Warmup(ctx)` method that the CLI could type-assert and call before the timed generation. `OllamaGenerator` implements it; `FakeGenerator` doesn't have to. Combined with one line in the README — `OLLAMA_KEEP_ALIVE=-1` — the second call onward hit a warm model and finished in 5–15 seconds. Cold start became something you paid once per session, on purpose, instead of something that ambushed every demo.

**The answers wouldn't sit still.** Same question, same chunks, same model — and yet the answer drifted from run to run. The cause was Ollama's default sampling: temperature around 0.8, designed to feel "creative." Useful for a chatbot, fatal for an eval that needed to compare runs. The fix was a one-line change to the request body: `temperature: 0`, fixed `seed`. Greedy decoding. Boring, reproducible, and — as a quiet side benefit — noticeably less prone to invention.

**The eval and the product disagreed on how many chunks to feed.** The eval forces `--limit ≥ 10` because that's what `recall@10` needs. The product shipped with `--limit 5` by default. So every eval run was scoring answers built from 10 chunks while real users got 5 — measuring a configuration nobody actually ran. The fix was to decouple them. `--limit` controls *retrieval* (how deep we look). `--answer-limit` (default 5) controls *prompt* (how many of those chunks the generator sees). The eval keeps its deep window for recall metrics; the answer reflects the shipped product.

Three small changes. None of them were on the v0 plan. Each one landed because dogfooding made the thing it was solving impossible to ignore.

---

## The Tier B insight — verify before fixing

A new eval mode shipped: `eval --answer`. It runs the usual retrieval eval, then for every query also runs the generator and scores the answer on three deterministic, reference-free checks — well-formed (non-empty), citations valid (every `[N]` resolves to a chunk that was actually in the prompt), and refusal-on-negative (for unanswerable questions, did the model say "I don't have enough information" instead of inventing one).

The first run printed a number that stopped us cold: **refusal rate on negatives = 0.50.**

Half the unanswerable questions had gotten confident answers. That's a hallucination-floor breach — the kind of signal that justifies pulling forward a low-confidence guardrail, maybe a faithfulness check, possibly a re-think of the prompt itself.

Before doing any of that, the next step was just to read the actual answers.

All four refused. *In prose.* Two used wording the heuristic caught ("does not contain"). Two used wording it didn't ("is not implemented in the provided code chunks"). The model was behaving correctly. It was the *measurement* that was undercounting.

The fix was a one-line widening of the refusal-phrase list. Refusal rate jumped to 1.00. No prompt change. No guardrail. No v1 feature pulled forward. Just a metric that now matched reality.

A small alarm, a smaller fix, and a rule worth carrying forward: a metric is a *measurement*, not the ground truth. The instinct to fix what a number says is broken is exactly wrong when the number itself is the bug. **Always read the raw output before changing the system to chase a number.**

---

## Test files are not neutral

Negative queries are the eval's hallucination floor. If the model invents an answer to *"where is the OAuth middleware?"* — and this project has no OAuth — that's a real failure to catch. So when one of those queries came back with a citation pointing inside `internal/eval/dataset_test.go`, the answer needed an explanation, fast.

The explanation was simple and slightly humiliating: `dataset_test.go` contains the literal string `oauth_middleware` as a test fixture — the negative query is *defined right there in code*. By indexing `*_test.go` files, the eval had been quietly leaking its own answer key into search results. The model wasn't hallucinating; it was reading the test that defined the negative case.

The fix was a config switch. `skip_file_patterns` in `config.yaml` defaults to `["*_test.go"]`. An explicit empty list (`[]`) is the escape hatch for anyone who actually wants test code indexed.

The re-baseline that followed exposed a second silent failure. Re-indexing walked into `.claude/worktrees/` — a git worktree left over from an earlier subtask. That meant a *full second copy* of the codebase was being indexed alongside the real one. Every chunk had a doppelgänger. Hit@1 dropped 10 percentage points from the duplicates competing with their originals, and failure top-1s suddenly came from inside the worktree's path.

The fix was another default: skip hidden directories. The same convention `git` and `rg` use — descend into source dirs, leave VCS and tooling state alone.

Two defaults, both shipped together, both correcting failures that had been invisible up to that point. The eval had run the whole time. The metrics had printed. They had just been measuring a corpus nobody had asked for.

---

## Corpus drift — the silent regression

After Phase 5 v0 shipped, we re-ran the eval on a freshly-indexed repo, expecting the same `hit@5 = 0.895` that had closed out Phase 2.

What came back was **0.789.**

Nothing about the algorithm had changed. BM25, stemming, RRF — all identical. The only thing different was the *codebase being indexed*. Phase 5 had added a whole new package (`internal/answer`) and extended a few others. Those new files became new chunks. The new chunks competed with the old ones for top-K slots. Familiar queries started losing them.

`hasher_concept` was the most jarring example. That query — *"how are file hashes computed for re-indexing"* — had been carefully recovered by the Snowball stemming work back in Phase 2. It had been a clean win, the kind of fix you stop worrying about. Now it was failing again. Not because stemming had broken. Because new hash-adjacent code now ranked higher.

The recovery came from the test-file hygiene above. Excluding `*_test.go` shed about a third of the chunks, and hit@5 returned to 0.895 on the cleaner corpus.

The rule that came out of this episode: **a baseline isn't an algorithm score. It's an algorithm score against a specific corpus.** Before declaring "the new reranker added 5pp" or "stemming regressed," re-baseline on the current corpus. Otherwise the change you tested and the change you measured aren't the same change, and the conclusion is meaningless.

---

## The recall gap — a new diagnostic

Choosing between two unbuilt features is hard. Choosing while staring at a single `hit@5` number is mostly guessing.

The decision in front of us was reranker vs GraphRAG. Reranking would only pay off if relevant chunks were actually being *retrieved* but ranked too low — outside the top-5 we feed the LLM. If those chunks were missing from the top-10 entirely, no amount of reordering would surface them, and reranking would be effort spent on the wrong floor. GraphRAG made the opposite bet. We needed a quick way to tell which world we lived in.

The check was already implied by two numbers we already had: `recall@5` and `recall@10`. Their *difference* tells the story.

- A **wide gap** (`recall@10 − recall@5 ≥ 0.10`) means the right chunks are getting retrieved but ranking outside top-5. **Reranking has headroom.**
- A **narrow gap** (`< 0.10`) means the right chunks aren't being retrieved at all. **Embedding or chunking is the floor; reranking can't help.**

One number, one decision. We made it a permanent line in the eval report — `recall gap = X.XX (≥0.10 → reranker headroom)` — so future choices don't have to re-derive the logic from raw metrics every time.

A subtle but useful framing: this number doesn't measure answer quality. It's a *forecast tool*. It tells you whether the next experiment is worth running before you build it.

---

## Phase 6 — GraphRAG, and a deliberate bet

By the time `hit@5` was back at 0.89, the eval was pointing at one weak spot. Navigation queries were the only category still below 1.0, and not by accident.

*"Where is X defined."* *"What calls Y."* *"What would break if I change Z."*

These aren't similarity questions. The answer lives in the *edges* between chunks — definitions, callers, imports — not in the chunks themselves. Pure vector + BM25 retrieval is built for similarity, and similarity isn't what's missing.

The original roadmap had Phase 3 = cross-encoder reranking next. We almost built it. Then we asked one more question: even if a reranker did a perfect job, could it surface a "what calls X" answer if the caller never lexically named X? It couldn't. Reranking can only reorder what hybrid already retrieved.

So the next phase pivoted: **GraphRAG.** A graph layer over the codebase, with edges for `defines`, `calls`, and `imports`, populated by a Go AST pass at index time, stored in local SQLite. At query time, hybrid still picks the top-50 seeds. Then we expand — 1–2 hops along the graph — re-score, and return.

To even test this, the eval needed structural queries it didn't have. So the golden set grew by 16 hand-curated multi-hop questions: *"what calls `Pipeline.Run`"*, *"trace from `runSearch` to `qdrant.Client.Search`"*, *"what would break if I change `Embedder.Embed`'s signature."* The eval doubled its navigation count and gained two new patterns: traces and change-impact.

The hybrid baseline on this new subset (`baseline_v7_structural.json`):

- hit@5 = 0.875 (14 of 16 pass)
- recall@5 = 0.60, recall@10 = 0.75
- **recall gap = 0.15** — comfortably above the reranker-headroom threshold

This is where things got honest. The recall gap triggers the reranker rule by our own standing diagnostic. So "GraphRAG over reranker" couldn't be sold as *"reranker can't help."* It had to be sold as a **deliberate bet**: structural signal would pay more on these queries than reordering would pay on the gap. Phase 3 reranking is parked, not cancelled; the data justifies it on the gap alone, but the bet says GraphRAG will help more on the *specific* structural failures.

Per-query analysis split the 16 structural queries into three buckets:

| Bucket | Count | Shape | Lever |
|---|---|---|---|
| **A** — hard fails (chunk absent from top-10) | 2 | embedding-shaped | GraphRAG or embedding upgrade |
| **B** — partial recall (chunk in top-10, not top-5) | 7 | reranker-shaped | reranker OR cheaper: raise `--answer-limit` |
| **C** — fully resolved | 7 | hybrid is enough | none |

And out of that table came a tempting hypothesis. If Bucket B's chunks were sitting at rank 6–10, why not just *feed more chunks to the LLM* — raise `--answer-limit` from 5 to 8 — and let the model see them directly? No graph. No reranker. No new infrastructure. A free win, on paper.

So that's what we tested.

The result, on 2026-05-28, didn't match the prediction. *(Canonical A/B data:
`docs/knowledge/retrieval_quality_decisions.md` §2.5 — the figures below are
illustrative for the story; that section is the source of truth.)*

- **Retrieval, identical.** Same `hit@5 = 0.875` in both runs — sanity check passed.
- **Tier B shape metrics, flat.** Cited rate, citation validity, dangling refs — all unchanged.
- **Latency, +55% at p50.** From 23.9s to 37.1s per answer. Total wall-clock +31%.

The retrieval logic was right. The rank-6–10 chunks *were* in the prompt at AL=8. But the prediction that Tier B's *shape* metrics would lift didn't hold — and Tier B can't see whether the answer's *content* improved. That's a correctness question. Answering it needs either dogfooding judgment or a Tier C faithfulness judge.

So the gate moved. The cheap lever's "free win" turned out to be inconclusive on automated metrics, with a real latency cost on top. The next step isn't another eval run; it's a side-by-side dogfooding pass — read AL=5 and AL=8 answers on the same multi-chunk questions, judge by hand which is more complete.

And there's a subtler implication for Phase 6 hiding in this result. A true retrieval-side fix — reranker or graph — puts Bucket B's chunks *into top-5*. The LLM sees them at AL=5. Same prompt content, *without* the +55% latency. So the AL=8 finding doesn't weaken the case for Phase 6. It strengthens it.

Phase 6 stays designed, stays gated, and waits on dogfooding. Phase 2 was *"build it and measure."* Phase 6 is *"measure first, re-measure with humans, then decide whether to build."*

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

**"Choose levers in cost order"** is the rule that came out of the Bucket A/B/C analysis and the AL=8 A/B. When the data shows two ways to fix the same problem — one cheap, one expensive — try the cheap one first, even if it sounds less interesting. Sometimes the cheap lever's outcome narrows the expensive scope; sometimes it leaves the question open or even *strengthens* the case for the expensive one (the `--answer-limit 8` A/B did the latter — Tier B couldn't see content benefits, latency rose +55%, and a true retrieval-side fix would deliver the same prompt without the cost). Either way, you've learned cheaper than the alternative.

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
3. **`index --watch`** ✅ **shipped.** Long-running mode that uses `fsnotify` to detect file changes and re-runs the pipeline (debounced). Removes the manual `delete → index → eval` cycle that punctuated this whole project. Independent of Phase 6; ran in parallel with the dogfooding sub-step.

Further out:

- **Phase 5 v1** features — multi-provider (OpenAI-compatible HTTP, native Anthropic), faithfulness judge (Tier C), streaming, refusal-on-low-confidence guardrail. Gated on dogfooding signal. Current verdict: the model self-refuses cleanly on negatives (pass = 1.00), so the refusal guardrail is justified-but-not-urgent.
- **Phase C** (custom Go vector DB) — still the long-term learning goal that started this project. Original ambition, now on a distant horizon. The decision to go top-down was the right one; eventually the bottom-up tour is still worth taking.

The thread through all of this remains the same: every change carries a measurement that justifies it, and the next change waits for the measurement of the last one. The system searches code, answers questions, measures its own quality, and now has a forecast tool — the recall gap — that tells it which kind of measurement to take next. It got here one honest decision at a time.
