# Idea — Ideal End-to-End RAG Pipeline

Brainstorm. Not a commitment, not a plan. Captures what a "complete RAG pipeline" looks like, where the current system sits, and which pieces are worth building when. The active plan that follows from this is `docs/plan/phase5_v0_answer_mode.md`; everything else here is destination, not next step.

---

## The twelve stages

A complete pipeline has roughly twelve stages. The current system does about four well, two partially, and six not at all.

```
1.  Ingestion              ┐
2.  Pre-processing         │   build the corpus
3.  Chunking               │
4.  Enrichment             ┘
5.  Embedding & Indexing   ┐   store searchable representations
6.  Incremental control    ┘   (CocoIndex-style)
7.  Retrieval              ┐
8.  Reranking              │   serve a query
9.  Context assembly       │
10. Answer generation      ┘
11. Evaluation             ┐   measure + improve
12. Feedback loop          ┘
```

## What each stage does in the maximalist version

| # | Stage | Role | Produces |
|---|---|---|---|
| 1 | Ingestion | Source connectors. Filesystem today; plus git history, Slack, Notion, PRs, issues, internal wikis. Threads ACLs through so retrieved chunks respect permissions. | Document stream with provenance |
| 2 | Pre-processing | Encoding normalization. PDF/HTML/notebook extraction. Language detection. Document-level dedup. | Clean, typed documents |
| 3 | Chunking | Language-aware AST chunking (Go done; Python/Rust/TS deferred). Hierarchical chunks (chunk + parent class + file context). Soft-cap on size for long functions. | Bounded, semantically coherent chunks |
| 4 | Enrichment | Metadata prefix (current). Optional: LLM-generated summaries per chunk. Optional: synthetic Q&A pairs (HyDE-style for hard-to-find concept queries). Cross-references between chunks. | Embedding-ready text |
| 5 | Embedding & Indexing | Dense vectors (current). Sparse TF-IDF/BM25 (current). Optional: multi-vector models (ColBERT-style late interaction). Optional: structural index (call graph, import graph) as a separate retrieval channel. | Qdrant + optional graph store |
| 6 | Incremental control | Content fingerprints at multiple levels (source file, chunk, pipeline config). Dependency graph. Re-process only what's stale when chunker, embedder, or source changes. **CocoIndex's territory.** | A graph of cached intermediate artifacts |
| 7 | Retrieval | Query understanding (classify navigation / concept / behavior / code-gen intent). Query rewriting (synonyms, code-aware tokens). Multi-strategy retrieval (hybrid — current; structural via graph; keyword fallback). Filter routing. RRF fusion (current for dense + sparse only). | Top-50 candidates |
| 8 | Reranking | Cross-encoder on the top-50 (Phase 3's deferred design). Optional MMR-style diversity (avoid 5 chunks from one file). Optional LLM-as-judge for hard cases. | Top-10 reranked candidates |
| 9 | Context assembly | Token-budget aware packing for the LLM's context window. Optional: window expansion (pull sibling chunks, parent class). Attach citation IDs. | Prompt-ready context block |
| 10 | Answer generation | LLM call (local Ollama or external API). Streaming response. **Citations embedded in answer text** referencing chunk IDs from stage 9. Guardrails: refuse if top-1 retrieval score is below threshold; no fabricated citations; mark answer scope ("based on 5 chunks from `internal/qdrant/`"). | Answer text + citation list |
| 11 | Evaluation | Retrieval metrics today (hit@k, MRR, recall). Plus when generation is on: faithfulness (claims grounded in retrieved chunks), citation accuracy (cited chunks actually contain the claim), correctness vs. reference answer, refusal-when-appropriate rate. Latency budgets per stage. Cost tracking. | Per-run reports + dashboards |
| 12 | Feedback loop | User thumbs up/down. Failed queries flow into golden eval. Drift detection on metric distributions over time. | Continuously growing golden set |

## Current → ideal gap

| Stage | Today | Realistic gap |
|---|---|---|
| Ingestion | Filesystem walker | Wide — connectors are months of work each |
| Pre-processing | Language detection | Moderate — PDF/HTML is one library each |
| Chunking | Go AST + generic regex | Per language; Phase 3 plan handles one |
| Enrichment | Metadata prefix | Wide — LLM-generated summaries are expensive |
| Embedding | Dense + sparse via Ollama | Multi-vector is a model swap + schema change |
| Incremental | File-hash change detection | Wide — see CocoIndex section below |
| Retrieval | Hybrid RRF | Moderate — query understanding is mostly prompt work |
| Reranking | None | Phase 3 deferred plan is shovel-ready when triggered |
| Context assembly | None | Small — packing logic is a day of work |
| Answer generation | None | Phase 5's `--answer` mode is a week of work |
| Eval | Retrieval only | Faithfulness/citation metrics need labeled data |
| Feedback loop | None | Small if you commit to dogfooding |

The "**minimum viable RAG**" from where the system stands today is **stages 9 + 10 added to the existing 1-5 and 7**. That's roughly Phase 5 v0 in the MVP roadmap. ~1 week of focused work. Everything else in the table is real but speculative until usage demands it.

---

## CocoIndex / incremental processing — assessed

### What it is

A typed dataflow pipeline where each node memoizes its output keyed by its inputs and config. When upstream inputs change, only downstream nodes that genuinely depend on the change re-execute. For RAG:

- Change a chunker config → re-chunk only.
- Change embedder model → re-embed only (chunks whose text didn't change keep their old vectors, but those vectors are now stale and need re-embedding).
- Change a source file → re-process only that file's downstream chain.

`docs/improvement/incremental_processing_roadmap.md` already sketches the two pieces that matter:

- **Chunk-level change detection** (finer than the current file-hash).
- **Pipeline fingerprinting** (detect when chunker/embedder version invalidates the cache).

### Where it fits

Stage 6, cross-cutting through stages 3-5. Not a separate user-facing feature; infrastructure that makes the whole pipeline cheap to re-run when any input changes.

### Real benefits

- **Methodology becomes automatic.** The "corpus-stability" rules documented for Phase 2/3 (re-baseline when chunker changes) become enforced by the pipeline, not by reviewer discipline.
- **Iteration speed at scale.** When the corpus is 100K+ chunks, a full re-index takes hours. Incremental keeps changes cheap.
- **Mixed-model contamination becomes impossible.** Pipeline fingerprinting prevents the "half the corpus is `dense_v1`, half is `dense_v2`" problem that would otherwise need ops discipline to avoid.
- **Aligns with Phase C learning.** Incremental computation systems (Salsa, Adapton, build systems) are deep tech worth understanding if the goal is to build a vector DB.

### Real costs

- **Substantial infrastructure.** Dependency graph, content-addressed storage for intermediates, idempotent operators, change-propagation algorithm. Easily 2-4 weeks of design + implementation for a credible v1.
- **Doesn't help users — helps the developer iterate.** No user-facing feature comes from it. Pure developer ergonomics + correctness backstop.
- **Premature at current scale.** ~350 chunks. Full re-index takes ~30 seconds. Incremental saves seconds, not minutes.
- **Adds testing surface.** Cache invalidation bugs are subtle — wrong answers cached against wrong inputs are the bugs that don't get noticed until they're really wrong.

### Right time to build it

- When full re-index time exceeds 5 minutes (corpus > ~10K chunks, or larger files with embedding API rate limits).
- When iterating on chunker/embedder choices weekly and the wait dominates the work.
- When shipping to others and "I changed the model, please wait 4 hours" is unacceptable.

### Wrong time

- Now. The corpus is small, iteration cadence is "every few weeks per phase," and full re-index is already manageable.

### What to do in the meantime — 80% value at 5% cost

Two cheap pieces that capture most of the value without the framework:

1. **Pipeline-fingerprint metadata on each chunk.** Store `chunker_version`, `embedder_model`, `enrichment_template_hash` in the Qdrant payload. At index time, check the collection's "expected fingerprint" — if mismatch, force-prompt the user to re-index. ~50 lines of Go. Prevents mixed-model contamination without building incremental computation.
2. **Chunk-level hash in payload.** Already have `file_hash`; add `chunk_hash` (a SHA of the chunk content). When a file changes, compare per-chunk hashes and skip embedding for chunks whose content is unchanged. Saves embed calls on big files where one function changed. ~half a day of work.

The full CocoIndex-style framework can wait until scale demands it.

---

## Honest take, tying to MVP discipline

The maximalist RAG vision above is the destination if this project becomes the primary thing. Held against MVP discipline:

- The vision is genuinely correct as a long-term picture.
- The discipline says: don't build toward the vision until you've validated which parts users actually need.
- **Stages 9-10 (context assembly + answer generation) are the only ones that meaningfully change what the system does for a user.** Everything else in stages 1-8 is internal quality work, and everything in 11-12 is process.
- If shipping one more feature before dogfooding, make it minimal `--answer` mode. That tells you whether generation is even useful in this codebase, which gates almost every other decision in the maximalist pipeline.

The CocoIndex idea is real but infrastructure. Pipeline-fingerprint metadata (the cheap 80%) is worth doing whenever the pipeline next gets touched. The full framework is a Phase C-or-later commitment.

---

## Q&A — should Phase 5 be prioritized?

Captured from the discussion that produced this brainstorm.

### Q: If we're evolving ragcodepilot toward a full RAG pipeline, does that mean we should prioritize Phase 5?

**A: Yes — and that's the smarter pivot than Phase 3.** If the destination is full RAG, `--answer` mode is the most informative thing to ship next. It's the only feature whose result tells you whether the project's vision is real. Everything else (more languages, reranking, UX polish) is internal improvement that doesn't answer the question "is generation what users actually want here?"

### Why the original Phase 4 → Phase 5 order can be flipped

The MVP roadmap put `--answer` at Phase 5 with "don't start unless a real user need has surfaced." That gate just opened. Three reasons the conservative ordering relaxes:

1. **Phase 2 retrieval is already good enough to feed an LLM.** `hit@5 = 0.895` means the right chunk is in the top-5 for almost every query. A reranker would push that to maybe 0.92. The LLM doesn't care about those 2 percentage points — it can synthesize from messy context. The vision review's two preconditions for adding generation (users want explanations + retrieval is strong) are both met.
2. **Phase 4 (UX polish) is fungible.** JSON output, context lines, faster startup — none of these change what the system *is*. They make the existing thing nicer. They can come before, after, or alongside `--answer`.
3. **`--answer` is the category-defining feature.** Without it, the system is shipping semantic grep. With it, the system is shipping a RAG tool. That distinction determines almost every downstream decision about evaluation, latency, prompting, and what users actually do with the tool.

### How this reconciles with dogfooding-first advice

The earlier advice was: don't add Rust chunker — validate Go usage first. That stands, but `--answer` is a different shape of work:

| Feature | What you learn |
|---|---|
| Rust chunker | Capacity expansion. Tells you nothing about whether the system is useful. |
| `--answer` mode | Product question. Tells you whether generation is the right interaction model for your workflows. |

Dogfooding with `--answer` is genuinely informative. Dogfooding to validate Rust chunking when there's no Rust corpus is circular. Build the thing that produces signal.

### What "Phase 5 first" actually looks like

**v0 — minimal `--answer` (3-5 days):**

```
ragcodepilot search "how does change detection work" --answer
```

Flow:

1. Run existing hybrid search → top-5 chunks
2. Build a prompt: question + 5 chunks + "answer based only on these chunks; cite chunk IDs"
3. Call Ollama with a small generative model (`qwen2.5-coder:7b`)
4. Print the answer
5. Print the source chunks below the answer (so users can verify manually)

That's it. No citation parsing, no faithfulness checks, no streaming, no guardrails. The smallest thing that lets you ask a question and get an answer back.

Use it for a week. Decision tree after that:

- **"This is much better than raw chunks."** → invest more. Add citation parsing, faithfulness eval, streaming.
- **"The answers are wrong / hallucinated."** → retrieval-feeding problem (activate reranker per its deferred plan), or prompt problem, or model-too-small problem.
- **"I keep ignoring the answer and reading the chunks."** → kill `--answer`. Go back to retrieval-only and invest in chunk presentation (Phase 4).

The third outcome is the one most worth being open to. If you're already happy reading ranked chunks, generation is overhead you don't need.

### What you'd skip by doing Phase 5 first

| Skipped | Real cost |
|---|---|
| Rust chunker | None right now (no Rust corpus to search) |
| Reranker | None unless `--answer` shows weak top-1 hurts answer quality (which would trigger the deferred reranker per its existing activation plan) |
| UX polish (Phase 4) | Some — JSON output and context lines are nice. But they apply equally to `search` and `search --answer`, so they can come after. |

### What to watch for in Phase 5 v0

Three real failure modes to plan for, not solve:

1. **Hallucinated citations.** The LLM cites chunk IDs that don't exist or contain different content. v0 doesn't try to prevent this — just print the source chunks below the answer so a human can spot-check. v1 adds parsing-and-validation.
2. **Confident wrong answers.** The LLM synthesizes a confident-sounding answer from chunks that don't actually answer the question. Hard to detect without faithfulness eval. v0 mitigates by always printing the chunks (user can read both and judge). v1 adds a faithfulness check (`is_supported_by_chunks` as an LLM-as-judge call).
3. **Cold-start latency.** First call after a fresh Ollama state takes 2-10s to load the model. Subsequent calls are fast. Set `OLLAMA_KEEP_ALIVE=-1` in your environment to pin the model in memory. Mention this in the docs.

### Concrete recommendation

Skip Phase 3 (Rust chunker). Skip Phase 4 (UX polish) for now. **Build Phase 5 v0 next.** ~3-5 days. Use it for a week. Then decide whether the next phase is "make Phase 5 better" (citations, eval, guardrails) or "kill Phase 5 and go back to retrieval-only with polished UX."

That's the experiment that tells you most about what to build next. Everything else in the maximalist RAG vision branches from the answer.
