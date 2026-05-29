# Code-Graph Retrieval — Industry Landscape & Prior Art

> Reference doc that situates ragcodepilot's "GraphRAG" (Phase 6) against how
> the field actually does structure-aware code retrieval. Use it to sanity-check
> whether a design choice is conventional, ahead of the curve, or off the path —
> and to borrow patterns that are already proven elsewhere.
>
> Currently covers:
>
> 1. **Naming** — why "GraphRAG" is a loaded term and what we actually build
> 2. **Prior art** — Aider repo-map, Sourcegraph SCIP/LSIF, language servers, Microsoft GraphRAG
> 3. **The standard retrieval-quality ladder** — and where our sequencing diverges
> 4. **What's genuinely beyond standard RAG** — the queries only a graph answers
> 5. **Takeaways for ragcodepilot**

**Companion docs:**

- [`../plan/graphrag.md`](../plan/graphrag.md) — the Phase 6 design this doc evaluates
- [`retrieval_quality_decisions.md`](retrieval_quality_decisions.md) — the *quality* trade-off doc (reranker §1, metrics §2, AL=8 A/B §2.5)
- [`architecture_decisions.md`](architecture_decisions.md) — the *shape* trade-off doc (CLI vs daemon, watch mode)
- [`../plan/mvp_roadmap.md`](../plan/mvp_roadmap.md) — phase plan and product direction

---

## Table of contents

1. [Naming: "GraphRAG" means something specific in industry](#1-naming-graphrag-means-something-specific-in-industry)
2. [Prior art — who else does this, and how](#2-prior-art--who-else-does-this-and-how)
3. [The standard retrieval-quality ladder](#3-the-standard-retrieval-quality-ladder)
4. [What's genuinely beyond standard RAG](#4-whats-genuinely-beyond-standard-rag)
5. [Takeaways for ragcodepilot](#5-takeaways-for-ragcodepilot)

---

## 1. Naming: "GraphRAG" means something specific in industry

When most people say **"GraphRAG"** they mean **Microsoft's** pipeline: an
LLM reads unstructured prose, *extracts* entities and relationships, builds a
knowledge graph, then *summarizes* graph communities with another LLM pass.
It exists because prose has no ground-truth structure — the edges must be
inferred.

That is **not** what Phase 6 builds, and `graphrag.md` correctly disavows it
(Non-goals). We build **structure-aware code retrieval**: a static AST pass
extracts *ground-truth* edges (`defines` / `calls` / `imports`) — no LLM, no
inference, deterministic. The graph augments hybrid retrieval; it does not
summarize anything.

**Why the distinction matters:** anyone benchmarking us against "GraphRAG
expectations" (community summaries, global-question answering, LLM extraction
cost) is comparing apples to oranges. Internally, "structural retrieval" or
"code-graph-augmented retrieval" is the more honest label. The `--graph` flag
name is fine; the *mental model* should not be Microsoft GraphRAG.

---

## 2. Prior art — who else does this, and how

Structure-aware code retrieval is **established, not exotic**. Code is uniquely
suited to it because the AST hands you the edges for free.

| System | Structure used | How it's built | Closest to us? |
|---|---|---|---|
| **Aider repo-map** | call/definition graph | tree-sitter symbols → graph → **personalized PageRank** ranks what context to send the LLM | **Very** — almost the same idea, different ranking math |
| **Sourcegraph SCIP / LSIF** | definitions, references, implementations | compiler/indexer emits a precise code-intel graph | Same edge types; industrial-scale, cross-repo |
| **Language servers (LSP)** | go-to-def, find-refs, call hierarchy | per-language compiler front-end | Same queries ("where defined / what calls"), live not indexed |
| **GitHub code navigation** | defs/refs via tree-sitter stacks | static, per-repo | Same scope as our v0 |
| **Microsoft GraphRAG** | LLM-inferred entity graph over prose | LLM extraction + community summaries | **Not** us — different problem (unstructured text) |

The standout reference is **Aider's repo-map**: it extracts symbols with
tree-sitter, builds a dependency graph, and ranks nodes with **personalized
PageRank** (biased toward the chat context and the identifiers the user
mentions) to decide which definitions to put in the model's context. That is
the same "edges are the signal" thesis as Phase 6 — and notably its ranking is
a *graph-centrality* algorithm, not a hand-tuned additive blend (see §5).

**Implication:** the *direction* is well-supported and arguably ahead of naive
chunk-only RAG. The risk is never "is a code graph a good idea" — it's
sequencing and scoring (below).

---

## 3. The standard retrieval-quality ladder

The conventional order for improving retrieval quality, cheapest/highest-ROI
first:

1. **Structure-aware chunking** (AST/function-level) — ✅ we have this for Go.
2. **Hybrid dense + sparse + fusion** — ✅ shipped (BM25 + dense + RRF).
3. **Cross-encoder reranking** — ⏸ parked. The single most standard, lowest-
   risk next lever. Our own recall gap (0.132 > 0.10) says it has headroom.
4. **Code-specialized embeddings** — ❌ untried. We use general-purpose
   `nomic-embed-text` (768d). Code-tuned models (voyage-code-2/3,
   jina-embeddings-v2-base-code, CodeSage (Large/v2), etc.) are the standard
   fix for the exact weak spot we have:
   identifier/navigation queries. `retrieval_quality_decisions.md` lists this
   as S–M effort, "+2–5pp likely," not yet attempted.
5. **Query transformation** (HyDE, multi-query, decomposition) — not on roadmap.
6. **Bespoke structural / graph retrieval** — ▶ Phase 6.

**Where we diverge:** we jump from step 2 to step 6, parking steps 3 and 4.
That is *not* the standard sequence. The standard playbook says exhaust the
boring levers (reranker, code embeddings) first because they're cheaper and
better-documented, then reach for bespoke architecture.

The divergence is **defensible but deliberate**, and `graphrag.md` is honest
about it: navigation/multi-hop answers are *structural*, and steps 3–4 cannot
add a signal that isn't in the embedding/BM25 space. That argument is sound for
the *specific* failure class — it is just not the conventional default, and the
addressable win (a handful of the 16 structural queries) is small relative to
an L-sized build. See §5 for how to hold both truths.

---

## 4. What's genuinely beyond standard RAG

Not everything is reorderable by a reranker or rescuable by a better embedder.
Three query classes need the graph and *only* the graph:

- **Reverse traversal** — "what calls `X`" where the caller never lexically
  names X's purpose. Similarity can't find it; the edge can.
- **Multi-hop / change-impact** — "what breaks if I change `Embedder.Embed`'s
  signature." No amount of reranking surfaces a transitive dependent.
- **Path / trace** — "trace from `runSearch` to `qdrant.Client.Search`." A path
  is a graph object, not a ranked list.

This is also where the **industry is heading**: agentic coding tools
increasingly treat a repo graph / repo-map as first-class context, because an
agent navigating a codebase asks structural questions, not just similarity
ones. So Phase 6 is forward-looking for an *agentic code-understanding* product,
even if it's premature for a pure *semantic-search* product. Which product
ragcodepilot is becoming is the real fork (see §5).

---

## 5. Takeaways for ragcodepilot

**The verdict depends on which project this is** — and the repo is honestly two
at once (see [`building_ragcodepilot.md`](building_ragcodepilot.md)):

- **As a product** (search that feeds an LLM): the prioritization is
  questionable. Reranker + code-specialized embeddings are the standard,
  cheaper path and would likely capture most of the win (especially Bucket B)
  faster. Standard practice does those first.
- **As a learning project / toward agentic code understanding** (the stated
  origin): GraphRAG is the *better* choice. Building AST → graph → expansion →
  rescoring teaches far more than wiring a rerank API, and it unlocks the §4
  query classes that are genuinely beyond reranking. This is the defensible
  reading of the bet.

**Concrete recommendations, drawn from the prior art:**

1. **Graph and reranker are complements, not alternatives.** The mature pattern
   is *graph for recall, cross-encoder for precision* — graph injects missed
   candidates, reranker orders the merged set. This is now written up in
   `graphrag.md` → "Composition with reranking." Keep expansion as a
   candidate-*generation* stage so a reranker drops in later without a rewrite.
2. **The hand-tuned additive `α/β` blend is the least standard part.** Aider
   uses PageRank; mature systems use a learned reranker. Our linear blend is a
   serviceable *interim* ordering, not the end state. Treat it as such, and
   prefer graph-centrality or a reranker for the long-term ordering.
3. **Don't skip the boring levers permanently.** Even if graph ships first, a
   code-specialized embedding model and a cross-encoder reranker remain the
   highest-ROI, most-conventional improvements and should be revisited — the
   recall-gap data already argues for the reranker.
4. **Right-size expectations.** "GraphRAG" the buzzword promises more than a
   static call-graph delivers. What we ship is precise, deterministic, and
   useful for navigation — and that's the honest, defensible claim. Don't
   oversell it as Microsoft-style global reasoning.

---

## Sources

*Prior-art claims in §§1–4 verified against primary sources, 2026-05-29.* The
embedding-model names and version numbers are current as of that date; re-check
before citing exact versions, since model families iterate quickly.

- **Aider repo-map** (tree-sitter symbols + personalized PageRank over the
  call/definition graph):
  - https://aider.chat/2023/10/22/repomap.html
  - https://aider.chat/docs/repomap.html
- **Microsoft GraphRAG** (LLM entity/relationship extraction → Leiden community
  detection → LLM community summarization, for global questions over prose):
  - https://www.microsoft.com/en-us/research/blog/graphrag-new-tool-for-complex-data-discovery-now-on-github/
- **Sourcegraph SCIP / LSIF** (SCIP is the protobuf-based successor to
  JSON-based LSIF; both capture definitions, references, implementations):
  - https://sourcegraph.com/blog/announcing-scip
  - https://github.com/sourcegraph/scip/blob/main/scip.proto
- **Code-specialized embedding models**:
  - voyage-code-3 — https://blog.voyageai.com/2024/12/04/voyage-code-3/
  - jina-embeddings-v2-base-code — https://jina.ai/models/jina-embeddings-v2-base-code/
  - CodeSage (Amazon Science) — https://github.com/amazon-science/CodeSage
- **GitHub code navigation** (tree-sitter-based stack-graphs for static,
  per-repo definitions/references):
  - https://github.blog/open-source/introducing-stack-graphs/
  - https://arxiv.org/pdf/2211.01224 (Stack graphs: name resolution at scale)
