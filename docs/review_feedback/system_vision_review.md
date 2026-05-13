# System Vision & Product Direction Review

Honest answers to the ten strategic questions in `docs/questions.md`.
Grounded in the planning docs (`docs/plan/system_design.md`, `docs/plan/checklist.md`, `docs/plan/vecdb/`, the `_with_feedback` variants under `docs/review_feedback/`) and the current implementation (`internal/search`, `internal/ingest`, `internal/qdrant`, `cmd/ragcodepilot`).

---

## Executive summary

1. **The system is not RAG. It's semantic code search.** The `ragsearch` / `ragcodepilot` naming is misleading. The README is already honest about this ("It focuses on retrieval, not answer generation") — the project name should match the README.
2. **Retrieval-only is the right call for the documented use case** (refactoring, code discovery). `system_design.md` already justifies it: "for code refactoring, you want to read the actual source code, not a paraphrased summary." Don't reverse this without a clear new user need.
3. **The biggest concrete weakness is the missing evaluation harness.** You cannot tell if hybrid search, reranking, or any chunker change helps or hurts. Build the eval before adding any retrieval feature.
4. **Phase 3 priorities, in order:** (a) eval harness, (b) hybrid search (BM25 + vector + RRF), (c) reranking. Defer LLM generation.
5. **The Phase C goal (build a vector DB in Go) is an entirely separate project.** Treat it that way — don't let it steer product decisions, and don't start it until search MVP is solid.

---

## Q1 — What is the system's purpose?

**Direct answer:** Two stacked goals, both legitimate but worth naming explicitly:

- **Outward product:** A local, privacy-respecting, CLI-native semantic code search tool that lets a developer ask natural-language questions over one or more local Git repos and get ranked source-code chunks back.
- **Inward goal:** A learning vehicle. The plan tree in `docs/plan/vecdb/` and `plan_comparison.md` makes it clear the long arc is: build an app on Qdrant → study Qdrant's internals → eventually re-implement a vector DB in Go (the "Phase C" Rust→Go refactor).

The current app is the on-ramp to that learning path, not the destination. Both goals are fine; the trap is conflating them when prioritizing.

---

## Q2 — Is this a good product idea?

**Direct answer:** Yes, with caveats.

- **For the learning goal:** Excellent. The stack (chunking, embedding, vector DB, filtered retrieval) covers the full vector-search surface area without overreach. The pedagogical structure in `system_design.md` and `plan_comparison.md` is genuinely strong.
- **For the product goal:** It's a crowded space. Sourcegraph, GitHub code search, Cursor's `@workspace`, Cody, and Aider all do semantic code retrieval to some degree. The defensible niche is narrow but real:
  - **Local-only, offline, no telemetry, no vendor lock-in.**
  - **CLI-native** (scripts, pipes, no UI dependency).
  - **Works on any repo you have read access to**, including monorepos and proprietary code that can't be sent to a cloud service.

**Suggestion:** State this niche explicitly in the README. "ragcodepilot is for developers who need semantic code search on private code without sending anything to a third party." That's a clearer pitch than "another RAG project."

---

## Q3 — Does the current implementation support real user needs?

**Direct answer:** Partially. It's solid for "find code that matches an idea." It's weak for "understand and reason about" — which is half the user's stated goal.

What works today:
- File walker → language detection → chunker (Go AST for Go, sliding window elsewhere) → enrichment → Ollama embedding → Qdrant upsert.
- Filtered retrieval by `--language` and `--repo`.
- Re-indexing with file-hash change detection.

What's missing for the "understand" half:
- **No exact-symbol search.** Vector-only retrieval can miss `func Run()` lookups because dense embeddings smear identifiers. Hybrid search (BM25 + vector) is the standard fix.
- **No reranking.** Top-K from a single embedder is the final answer; you get whatever cosine similarity says. A cheap cross-encoder reranker would close this gap.
- **No file or repo overview mode.** "Explain how this package fits together" is a real query that gets a list of chunks, not a structure.
- **No traversal.** "How does X work?" benefits from following imports/calls, which the system can't do — it returns disconnected chunks.

Half of "understand and reason" is achievable without an LLM (better retrieval, better presentation). The other half (synthesis) requires generation, which is intentionally out of scope today.

---

## Q4 — Is RAG the right approach?

**Direct answer:** Be precise — this is **R, not RAG**. The retrieval-only design is correct for the documented use case. Adding **G** (generation) is appropriate only when (a) users want explanations rather than source-code snippets, and (b) retrieval quality is already strong enough that the LLM has good context to work with.

A few concrete points:

- **Naming.** `ragcodepilot` and the `ragsearch` folder both imply generation. The README is honest about retrieval-only. Either rename the project (`codeseek`, `repoindex`, `localcodesearch`) or commit to adding a generation mode behind an opt-in flag (`--answer`) later.
- **Don't add G to look more "AI."** The skip is documented in `system_design.md:25` and the rationale (line 29) is correct for refactoring. Add generation only if a real user need surfaces.
- **If you do add G later,** make it optional. The default should remain "return chunks." Cite chunk IDs in any generated answer so users can verify.

---

## Q5 — Main weaknesses

| # | Severity | Weakness | Why it matters |
|---|---|---|---|
| 1 | P1 | No evaluation harness | Can't measure if any change helps or hurts; spec exists in `docs/review_feedback/rag_evaluation_metrics_with_feedback.md` but isn't built |
| 2 | P1 | Misleading project name | `ragcodepilot` / `ragsearch` imply generation; README contradicts the name |
| 3 | P1 | No hybrid search | Vector-only misses exact-symbol queries; BM25 + RRF is well-understood and Qdrant supports it natively |
| 4 | P2 | No reranking | Single embedder's top-K is the final answer; precision on ambiguous queries suffers |
| 5 | P2 | Only Go uses AST chunking | Other languages use 80%-accurate regex; per-language quality is uneven |
| 6 | P2 | No query understanding | No rewriting, expansion, or intent detection — short queries get poor recall |
| 7 | P3 | Bare result formatting | Plain-text walls; no grouping by file, no surrounding context lines, no JSON output for tooling |
| 8 | P3 | No watch / incremental mode | Active-development workflow requires manual re-index |
| 9 | P3 | Single-vector embedding per chunk | No separation between identifier-heavy embedding and prose-heavy embedding; some content (e.g., long files with both) is poorly represented |

The naming issue (#2) is the cheapest to fix and arguably the most user-visible. The eval gap (#1) blocks everything else.

---

## Q6 — Features that would make it more useful

Ordered by priority + effort + value:

| Priority | Feature | Effort | Why it matters |
|---|---|---|---|
| P1 | **Evaluation harness + 20-30 golden queries** | M | Measures every subsequent change; spec already in `rag_evaluation_metrics_with_feedback.md` |
| P1 | **Hybrid search (sparse BM25 + dense vector + RRF)** | M | Fixes exact-symbol lookups; significant `hit@k` gains on technical queries |
| P1 | **Result formatting upgrade** (JSON mode, group by file, ±N context lines) | S | Real users will pipe results into other tools; current formatting blocks that |
| P2 | **Cross-encoder reranking** (top-50 → top-10) | M | Precision boost on ambiguous queries |
| P2 | **Tree-sitter chunking for Python/JS/TS/Rust** | L | Per-language AST quality; replaces brittle regex; biggest single quality lever after hybrid |
| P2 | **Optional `--answer` LLM mode** | M | Bridges to "understand" use case; preserves retrieval-only default; must cite chunk IDs |
| P3 | **File-level summary / TOC command** | S | "Show me the structure of `internal/ingest`" |
| P3 | **Watch mode** (re-index on save) | M | Active-dev workflow |
| P3 | **Query rewriting** (synonym expansion, identifier-aware) | M | Marginal but cumulative |

S/M/L are rough — S ≤ 1 week, M ≈ 1-2 weeks, L ≈ 3+ weeks of focused work.

---

## Q7 — What should I improve first?

**P1: Build the evaluation harness.**

**Reasoning:** Hybrid search, reranking, chunker upgrades — every retrieval improvement you make next will be a gamble without metrics. The spec is already written (`docs/review_feedback/rag_evaluation_metrics_with_feedback.md`). Implementing it is straightforward Go: load a YAML of queries with expected files/symbols, run each query through the existing `Searcher`, compute `hit@k`, `MRR@k`, `recall@k`, output JSON.

**Why this beats jumping straight to hybrid search:**
- You need a baseline to know if hybrid helps.
- You'll catch regressions when changing chunkers.
- The harness becomes the de-facto QA suite — useful even if you never touch retrieval again.
- It's the smallest piece of work with the largest leverage on everything that follows.

**Concrete first step:** Add a `ragcodepilot eval --dataset docs/eval/golden.yaml --collection code_chunks` command. Start with 5 hand-written queries, expand to 20-30 over the next iteration.

---

## Q8 — What should I avoid building too early?

| Avoid | Reason |
|---|---|
| **Chat / conversational UI** | Stateful, complex, no proven user need |
| **Web UI / TUI** | CLI works; UI is polish, not value |
| **LLM answer generation (full mode, not optional)** | Build only after retrieval quality is great + user demand is clear |
| **LLM-as-reranker** | Slow, expensive; cross-encoder reranker is faster and almost as good |
| **Cross-repo dependency / call graphs** | Architectural over-engineering for current scope |
| **Custom vector DB (Phase C)** | Don't start until search MVP is solid; the learning value of Phase C depends on having done Phase A and B well first |
| **Multi-vector embeddings per chunk** (code + prose separately) | Premature without eval framework to prove it helps |
| **Indexing commits, PRs, comments** | Index source files well first |
| **An IDE plugin** | Lock down the CLI surface first; plugins are integration, not value |

The Phase C trap deserves a flag: it's the most ambitious item in the project but **completely orthogonal** to making the search tool useful. If it slips three months, that's fine. If it blocks search-product progress, that's a problem.

---

## Q9 — How can I evaluate answer quality, retrieval quality, and overall usefulness?

You can't measure answer quality (no answers generated). You **can** measure retrieval quality and usefulness:

**Retrieval metrics (build these first):**
- **Golden query set** — 20-30 queries covering three categories:
  1. **Navigation:** "where is `ChunkFile` defined?" → expects specific file/symbol.
  2. **Concept:** "how does chunking work?" → expects a set of files, in approximate priority order.
  3. **Behavior:** "what happens if the embedder returns zero vectors?" → expects validation code.
- **Core metrics:** `hit@1`, `hit@3`, `hit@5`, `MRR@5`, `recall@10`.
- **Latency:** p50, p95, p99 — broken out into embedding, Qdrant query, and total. The user cares about total; you care about each piece for debugging.
- **Negative tests:** queries that should NOT match well — confirm the top result's score is below a threshold.

**Process around the metrics:**
- Run the eval before and after every retrieval change. Capture deltas in PR description.
- Spot-check 5-10 real queries manually every meaningful change. Eye-balling catches things metrics miss.
- Wire it into CI: if `internal/ingest/` or `internal/search/` changes, run eval and post deltas as a PR comment.

**Usefulness (qualitative):**
- Dogfood it on a non-trivial repo you don't have memorized. Note every time you reach for grep instead.
- After eval is in place, recruit 1-2 friends to dogfood on their own repos and log queries that returned junk.

---

## Q10 — Roadmap to a strong MVP

8-12 weeks, assuming part-time focus. Each phase has a clear exit criterion.

| Phase | Wks | Goal | Key deliverables | Exit criterion | Effort |
|---|---|---|---|---|---|
| 1 | 1-2 | **Evaluation foundation** | Golden YAML (25 queries), `ragcodepilot eval` CLI, baseline metrics committed to repo | `hit@5` baseline number published | M |
| 2 | 3-5 | **Hybrid search** | Sparse BM25 in Qdrant, dense+sparse RRF fusion, eval shows ≥10pp `hit@5` improvement on exact-symbol queries | Metrics committed; PR description shows delta | M |
| 3 | 6-7 | **Reranking + chunker upgrades** | Cross-encoder reranker (top-50 → top-10); tree-sitter for Python or Rust | `MRR@5` improves measurably; multi-language chunk quality is human-checked | M-L |
| 4 | 8 | **UX polish** | `--json` output mode, `--context-lines N` flag, grouping by file, faster startup (pre-warmed embedder?) | Can pipe results into `jq` / `fzf`; eyeball test on 10 real queries | S |
| 5 | 9-10 | **Optional `--answer` mode** | Ollama-backed answer generation with chunk-ID citations; default remains retrieval-only | Generated answers cite real chunks; doesn't hallucinate when retrieval is weak | M |
| 6 | 11-12 | **Decision point** | Either (a) start Phase C custom vector DB per `vecdb/`, or (b) extend search (watch mode, IDE plugin, multi-modal embeddings) | Explicit, written commitment to one path | — |

**Stop conditions worth respecting:**
- If `hit@5` after Phase 2 is below ~60%, fix retrieval (chunking, reranking) before adding anything else.
- If you can't recruit 2 dogfood users by Phase 4, the product goal might not be real for you — re-evaluate whether to invest more in product polish or pivot fully to the learning track.
- Don't start Phase 5 (`--answer`) without a real user telling you they want it.

---

## Honest caveat on the Phase C ambition

The plan tree contains `docs/plan/vecdb/vecdb_phase1_flat_search.md` — a roadmap to build a vector DB in Go, starting from a Rust reference. This is a serious undertaking (probably 3-6 months of focused work even with the Rust source to learn from). It's also **completely orthogonal** to making the search product better.

Two honest framings:
- **If the goal is a useful product:** Phase C may be a permanent detour you never take. That's fine. Invest the time in retrieval quality and integrations instead.
- **If the goal is learning:** Phase C is the most valuable item in the plan. But it should follow a solid Phase A (now) and Phase B (study Qdrant) — not run in parallel with them.

Pick one frame and act consistently. Trying to do both at once is how learning projects spiral and product projects stall.
