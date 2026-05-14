# RAG Evaluation Metrics for ragcodepilot

## Current Position

`ragcodepilot` is retrieval-first, not a full answer-generating RAG system.

The current system indexes code, embeds chunks, stores vectors in Qdrant, and returns ranked source chunks. It does not synthesize natural-language answers with an LLM. That means evaluation should start with retrieval quality and performance, not hallucination or generated-answer quality.

## Recommendation

Add evaluation as a near-term project phase before hybrid search.

The evaluation harness should become the scoreboard for retrieval improvements such as chunk enrichment, function-level chunking, embedding model changes, filters, hybrid search (BM25 + dense + RRF), and reranking.

## Retrieval Metrics

- `hit@k`: whether any expected file, symbol, or chunk appears in the top `k` results.
- `MRR@k`: reciprocal rank of the first relevant result, capped at `k`.
- `recall@k`: how much expected relevant material appears in the top `k` results.
- `nDCG@k`: optional graded relevance metric when expected results use scores like 0, 1, 2, or 3 instead of binary relevance.

For the first version, prefer `hit@1`, `hit@3`, `hit@5`, `MRR@5`, and `recall@5`. Add `nDCG@k` later if the dataset starts grading relevance strength.

## Performance Metrics

- p50 and p95 total search latency.
- p50 and p95 embedding latency.
- p50 and p95 Qdrant query latency.
- Result count returned per query.
- Optional ingestion metrics: chunks indexed per second, embedding batch latency, and Qdrant upsert latency.

These metrics should be reported alongside retrieval quality so quality improvements do not hide unacceptable latency regressions.

## Future Answer Metrics

Only add these when `ragcodepilot` has an LLM answer-generation layer:

- Faithfulness or groundedness: generated answers must be supported by retrieved code chunks.
- Answer correctness: generated answers should match expected behavior or a reference answer.
- Citation coverage: answers should cite the right file, symbol, or chunk.
- Abstention: the system should say it does not know when retrieval does not contain enough evidence.

Until then, answer metrics are out of scope because `ragcodepilot` returns source chunks directly.

## Example Dataset

```yaml
queries:
  - id: chunking_overview
    query: "how does chunking work?"
    filters:
      language: ["go"]
    expected:
      files:
        - internal/ingest/chunker.go
        - internal/ingest/chunker_go.go
      symbols:
        - ChunkFile
        - chunkGoFile

  - id: qdrant_collection_management
    query: "Qdrant collection management"
    filters:
      language: ["go"]
    expected:
      files:
        - internal/qdrant/client.go
      symbols:
        - EnsureCollection
        - ValidateCollectionVectorSize
```

## CLI Vision

```bash
ragcodepilot eval --dataset docs/eval/ragcodepilot.yaml --collection code_chunks
```

Expected output should include per-query results and aggregate metrics:

```text
Dataset: docs/eval/ragcodepilot.yaml
Queries: 25

hit@1:    0.64
hit@3:    0.80
hit@5:    0.88
MRR@5:    0.72
recall@5: 0.78

latency_p50_ms: 42
latency_p95_ms: 91
```

The first implementation can be offline and local: load a YAML dataset, run the existing search path, compare returned payload fields against expected files and symbols, and print metrics.

## Sources

- [Retrieval-Augmented Generation for Knowledge-Intensive NLP Tasks](https://papers.nips.cc/paper_files/paper/2020/hash/6b493230205f780e1bc26945df7481e5-Abstract.html)
- [RAGAS: Automated Evaluation of Retrieval Augmented Generation](https://arxiv.org/abs/2309.15217)
- [BEIR: A Heterogeneous Benchmark for Zero-shot Evaluation of Information Retrieval Models](https://arxiv.org/abs/2104.08663)
