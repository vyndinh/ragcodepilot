# RAG Evaluation Metrics for ragcodepilot

## Current Position

`ragcodepilot` is retrieval-first, not a full answer-generating RAG system.

The current system indexes code, embeds chunks, stores vectors in Qdrant, and returns ranked source chunks. It does not synthesize natural-language answers with an LLM. That means evaluation should start with retrieval quality and performance, not hallucination or generated-answer quality.

## Recommendation

Add evaluation as a near-term project phase before hybrid search.

The evaluation harness should become the scoreboard for retrieval improvements such as chunk enrichment, function-level chunking, embedding model changes, filters, hybrid BM25, and reranking.

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

---

## FEEDBACK: Suggested Improvements

FEEDBACK: The evaluation direction is correct: because `ragcodepilot` currently returns ranked source chunks instead of generated answers, retrieval quality and latency should be the first-class metrics. Keep answer-generation metrics out of the initial scope until an LLM synthesis layer exists.

### FEEDBACK: Define relevance labels explicitly

FEEDBACK: The current dataset uses expected files and symbols, which is a good start, but it should distinguish between different strengths of relevance. Not all matching files are equally important.

Recommended relevance levels:

```yaml
relevance:
  required: 3      # Must appear for the query to be considered successful
  helpful: 2       # Strongly useful but not mandatory
  acceptable: 1    # Related, but not the best answer
  irrelevant: 0    # Should not rank highly
```

Example:

```yaml
queries:
  - id: chunking_overview
    query: "how does chunking work?"
    filters:
      language: ["go"]
    expected:
      required:
        - file: internal/ingest/chunker.go
          symbols: ["ChunkFile"]
      helpful:
        - file: internal/ingest/chunker_go.go
          symbols: ["chunkGoFile"]
      acceptable:
        - file: internal/ingest/pipeline.go
```

FEEDBACK: This structure makes `nDCG@k` easier to add later because graded relevance is already present in the dataset.

### FEEDBACK: Add negative retrieval cases

FEEDBACK: Even without an answer-generating LLM, the search system should be tested on queries where no strong match should exist. This prevents the system from always returning confident-looking but irrelevant chunks.

Example:

```yaml
queries:
  - id: nonexistent_oauth_middleware
    query: "where is the OAuth middleware implemented?"
    filters:
      language: ["go"]
    expected:
      no_strong_match: true
      max_score_below: 0.45
```

Recommended negative categories:

- concept not present in the indexed repository
- wrong language filter
- wrong repo filter
- misspelled symbol
- deleted file after re-index
- unsupported framework or library

### FEEDBACK: Add filter correctness metrics

FEEDBACK: Since the system supports language filtering and plans repo filtering, evaluation should check filter behavior directly, not only retrieval ranking.

Add metrics such as:

```text
language_filter_pass_rate
repo_filter_pass_rate
path_filter_pass_rate
filter_violation_count
```

Example assertion:

```yaml
queries:
  - id: go_chunker_filtered
    query: "function-level chunking"
    filters:
      language: ["go"]
    expected:
      files:
        - internal/ingest/chunker_go.go
    assertions:
      all_results_language: "go"
```

FEEDBACK: A query should fail if the correct file appears but the result set leaks results from the wrong language, repo, or path filter.

### FEEDBACK: Track query categories

FEEDBACK: Add a `type` field to each query so aggregate scores can show which search behavior is weak.

Example query types:

```yaml
type: symbol              # exact symbol lookup
type: semantic            # natural-language concept search
type: implementation      # "where is X implemented?"
type: config              # configuration lookup
type: filter              # filter-specific behavior
type: negative            # no strong match expected
```

Example aggregate output:

```text
overall_hit@5:       0.88
symbol_hit@5:        0.96
semantic_hit@5:      0.78
config_hit@5:        0.84
filter_pass_rate:    1.00
negative_pass_rate:  0.90
```

FEEDBACK: This is more actionable than one global score because it shows whether failures are caused by semantic matching, exact symbol lookup, filtering, or missing-data behavior.

### FEEDBACK: Add top-result quality checks

FEEDBACK: `hit@5` is useful, but it can hide ranking problems. If the correct result appears at rank 5, the system technically passes but may feel poor to the user.

Add checks such as:

```text
top1_is_relevant
top1_file_match
top1_symbol_match
top3_relevance_ratio
```

FEEDBACK: These metrics make ranking quality visible before adding hybrid search or reranking.

### FEEDBACK: Add result-shape validation

FEEDBACK: Because `ragcodepilot` returns source chunks, evaluation should verify that returned chunks contain the fields needed by a developer.

Suggested assertions:

```yaml
assertions:
  content_required: true
  file_path_required: true
  start_end_lines_required: true
  score_required: true
  symbol_required_for_function_chunks: true
  max_chunk_lines: 120
```

FEEDBACK: A result can be retrieval-correct but still unusable if it lacks file path, line numbers, symbol name, content, or score.

### FEEDBACK: Save reproducibility metadata in every eval report

FEEDBACK: Each eval run should include enough metadata to explain why scores changed between runs.

Recommended report metadata:

```json
{
  "run_id": "2026-05-08T08-30-00Z",
  "dataset": "docs/eval/ragcodepilot.yaml",
  "collection": "code_chunks",
  "repo_commit": "abc123",
  "embedding_model": "nomic-embed-text",
  "embedding_dimension": 768,
  "config_hash": "sha256...",
  "top_k": 5,
  "chunking_strategy": "go_ast_function_level"
}
```

FEEDBACK: This is important because changes to chunking, embedding model, enrichment, Qdrant collection settings, filters, or hybrid search can all affect results.

### FEEDBACK: Add eval modes to the CLI

FEEDBACK: Instead of one eval command doing everything, split evaluation into focused modes.

Suggested commands:

```bash
ragcodepilot eval retrieval --dataset docs/eval/ragcodepilot_retrieval.yaml
ragcodepilot eval filters --dataset docs/eval/ragcodepilot_filters.yaml
ragcodepilot eval latency --dataset docs/eval/ragcodepilot_retrieval.yaml
ragcodepilot eval compare --baseline eval/results/base.json --candidate eval/results/new.json
```

Suggested modes:

| Mode | Purpose |
|---|---|
| `retrieval` | `hit@k`, `MRR@k`, `recall@k` |
| `filters` | language, repo, and path filter correctness |
| `latency` | p50/p95 total, embedding, and Qdrant latency |
| `ingestion` | chunks/sec, embed batch latency, upsert latency |
| `compare` | compare baseline and candidate eval reports |
| `regression` | fail CI when quality or latency exceeds thresholds |

### FEEDBACK: Add a regression policy for CI

FEEDBACK: The eval harness should define when a change fails CI.

Example policy:

```yaml
regression_policy:
  min_hit_at_5: 0.85
  min_mrr_at_5: 0.70
  max_latency_p95_ms: 150
  max_filter_violations: 0
  max_allowed_hit_at_5_drop: 0.03
```

FEEDBACK: This keeps quality from silently degrading when changing chunking, embeddings, hybrid search, or reranking.

### FEEDBACK: Recommended first implementation scope

FEEDBACK: Start small. The first version should include:

```text
hit@1
hit@3
hit@5
MRR@5
recall@5
filter_violation_count
latency_p50_ms
latency_p95_ms
failed query details
JSON report output
```

FEEDBACK: Add `nDCG@k`, negative score thresholds, and compare mode after the basic runner is stable.

