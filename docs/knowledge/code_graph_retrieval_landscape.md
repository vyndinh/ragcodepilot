# Code-Graph Retrieval — Concepts and Historical References

This reference preserves terminology from the former graph proposal. It contains
no implementation plan. Current work is defined by the
[local CLI roadmap](../plan/mvp_roadmap.md).

## Distinguish the retrieval mechanisms

A code graph represents relationships such as definitions, references, and calls.
It can support navigation when the needed relationship is represented accurately.
Static syntax alone does not resolve every call: aliases, interfaces, dynamic
dispatch, and incomplete dependencies can leave relationships uncertain.

The former proposal concerned code relationships, rather than LLM extraction and
community summaries over prose. The name “GraphRAG” obscured that distinction.
There is no implemented graph retrieval mode or `--graph` flag in ragcodepilot.

A reranker changes the order of an existing candidate pool. It cannot recover a
chunk absent from that pool. Graph expansion can introduce connected candidates,
but only where its extracted relationships reach useful evidence. Neither mechanism
establishes better answers by its presence alone.

## Lessons from the local evaluation

The historical v7 structural set had 16 queries, with 14 already passing hit@5.
All seven queries whose recall@10 exceeded recall@5 already passed hit@5. The
remaining issue was evidence completeness, so binary hit-rate gains could not
validate a fix for those seven cases.

The AL=5 versus AL=8 experiment increased generation p50 by about 55% while
answer-shape metrics stayed flat. It did not establish an answer-content benefit,
and it did not establish that graph expansion or reranking would improve results.
See [the recorded experiment](retrieval_quality_decisions.md#25-practical-implications-for-current-decisions).

Prior art can explain possible mechanisms; it does not establish their value for
this repository. Use named failures, frozen inputs, required-evidence coverage,
and measured latency when interpreting retrieval changes. The
[evaluation guide](../eval/README.md#interpreting-coverage-and-isolating-changes)
contains those rules.

## Sources

Links retained from the 2026-05-29 research notes for historical context. They
are not current runtime recommendations or a feature-selection list. Recheck
the primary documentation before relying on a specific capability or version.

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
