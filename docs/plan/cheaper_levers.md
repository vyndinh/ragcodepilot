# Cheaper Levers — Conditional Embedding and Reranker Experiments

**Status: deferred experiments, not started. Updated 2026-09-22.** M0/M3 in the
[roadmap](mvp_roadmap.md) must establish current failures before either experiment
enters M5. A conventional technique is not automatically useful for this corpus.
Keep Go + Ollama + Qdrant and the default hybrid path until evidence supports a change.

## Evidence and selection

Historical v6 recall gap was 0.132; the v7 structural gap was 0.150. The latest
reviewed main v8 report has a full-set gap of about 0.082, and main has subsequent
chunker changes. These are different snapshots, not a controlled algorithm A/B.
Refresh the baseline instead of treating old navigation failures as current.

All seven v7 structural queries with recall@10 greater than recall@5 already pass
hit@5. Their missing evidence concerns completeness. Measure required-file recall
and inspect required symbols/relationships; the existing file-level metric can
count the wrong chunk in the right file as relevant. Better embeddings might raise
recall@10 faster than recall@5, so a widening gap is not automatically a regression.

Choose by failure class:

- Missing required evidence from a sufficiently deep candidate pool: consider a
  compatible local embedding model or chunking change.
- Required evidence in candidates but below top-5: consider reranking.
- Missing evidence reachable only through supported structural relationships:
  consider the [GraphRAG reachability study](graphrag.md).
- Exact-definition failure: try existing named payload/identifier capabilities
  before a new model or symbol database.

## Shared experiment contract

Use one frozen source snapshot, query set, config/filter scope, and representation
per paired experiment. Keep separate baseline/candidate collections where needed.
Record model artifacts/preprocessing, source and query hashes, chunk IDs/counts,
index versions, runtime versions, score kinds/thresholds, candidate limits, and
hardware/warm state. Candidate implementation files must not change the indexed corpus.

Historical v6 has 23 queries (19 positives); the full current set has 39 (35 positives).
Never compare those aggregates as a treatment effect. `compare.py` displays deltas
without validating compatibility or enforcing gates. Inspect manifests and per-query
results before accepting any comparison. External held-out queries are not tuning data.

Before running a candidate, freeze the targeted query IDs and the primary metric:
required-evidence recall@5 for completeness, or MRR@5 for ordering when coverage
already suffices. Pass only with a repeatable improvement on those targets, no lost
accepted hit@5 cases or required coverage elsewhere, no new passing-to-failing
negatives under valid fixed calibration, and the predeclared resource budget.
Report aggregate and per-type changes, sample counts, failures, and latency together.
A small-set win needs confirmation on the external sets before default promotion.

## Lever 1 — Alternative local embedding model [S–M, conditional]

The CLI already has `--ollama-model`, auto-detected dimensions, and `--collection`.
These support an isolated experiment but do not prove every model is a drop-in swap.
Index and query encoding must use compatible model artifacts and preprocessing.
Dimension checks detect size changes only; same-size models can be incompatible.

### Availability and inference spike [S]

Before choosing a candidate, verify and record:

- A working local runtime, model license/artifact digest, and memory footprint.
- Required query versus document instructions, pooling/normalization, context
  limits, and truncation behavior. Today's wrapper sends raw query text and
  enriched documents through one API; add a model-specific adapter if required.
- Whether existing path/language/symbol enrichment is suitable for that model.
- Index throughput, warm query p50/p95, cold startup, and hardware requirements.

Prior candidate ideas included code-specialized Nomic/Jina models and alternative
general text embedders. They are research leads, not verified runtime choices or
promised quality gains. Check the selected model's primary documentation when the
experiment opens. A sidecar remains local-first but can make the experiment too
costly. Cloud embedding is outside the current scope.

### Paired evaluation

Keep both collections disposable and distinct from the user's working index. The
following command template applies only after runtime/preprocessing compatibility
is established; replace paths and model placeholder with pinned experiment inputs.
Use fresh unique collection names for each experiment, not a reused collection that
may skip same-file/model changes.

```bash
ragcodepilot index --language go --collection exp_base_frozen --ollama-model nomic-embed-text /path/to/frozen-repo
ragcodepilot index --language go --collection exp_candidate_frozen --ollama-model CANDIDATE_MODEL /path/to/frozen-repo

ragcodepilot eval --dataset /path/to/frozen-golden.yaml --collection exp_base_frozen --ollama-model nomic-embed-text --mode hybrid --limit 10 --output json > /tmp/base_full.json
ragcodepilot eval --dataset /path/to/frozen-golden.yaml --collection exp_candidate_frozen --ollama-model CANDIDATE_MODEL --mode hybrid --limit 10 --output json > /tmp/candidate_full.json
ragcodepilot eval --dataset /path/to/frozen-golden.yaml --collection exp_base_frozen --ollama-model nomic-embed-text --mode hybrid --limit 10 --subtype structural --output json > /tmp/base_structural.json
ragcodepilot eval --dataset /path/to/frozen-golden.yaml --collection exp_candidate_frozen --ollama-model CANDIDATE_MODEL --mode hybrid --limit 10 --subtype structural --output json > /tmp/candidate_structural.json

python3 docs/eval/compare.py /tmp/base_full.json /tmp/candidate_full.json
python3 docs/eval/compare.py /tmp/base_structural.json /tmp/candidate_structural.json
```

Run only after M0 reconciles score calibration with main. Structural-only runs
contain no negatives; the full and external sets supply those checks. If changing
a score family/model requires calibration, perform it on a separate calibration
split and freeze the rules before evaluating held-out queries.

Repeat on the external frozen inputs. Preserve paired reports with descriptive
experiment/revision filenames and manifests; do not overwrite historical baselines.
A passing experiment permits a separate default-promotion decision with migration,
rollback, and model-provenance requirements. It does not automatically change defaults.

## Lever 2 — Cross-encoder reranker [M, conditional]

A reranker scores query/chunk pairs after retrieval and orders the existing
candidates. It cannot recover evidence outside that pool. Prototype only when fresh
failures show this ordering problem and a local runtime fits the budget.

### Proposed interface and score contract

```text
interface Reranker:
    rerank(context, query, candidates, output_limit) -> ordered results

result:
    chunk
    retrieval_score
    retrieval_score_kind
    rerank_score
    rerank_score_kind

interface Warmer (optional):
    warmup(context) -> success or error
```

Add an optional seam after Qdrant retrieval and before return in `internal/search`.
Default retrieval behavior remains unchanged without a reranker. Keep retrieval
scores; do not silently replace the score consumed by negative evaluation with
cross-encoder logits. The proposed result shape requires an explicit mapping to
existing formatters/eval, not a change to the meaning of `Score` alone.

Use a typed score-aware negative policy: historical cosine thresholds and main's
RRF/BM25 ceilings cannot be reused on reranker scores. Preserve the retrieval
negative diagnostic and evaluate final reranked results under separately calibrated
rules; choose the applicable gate before the experiment. A structural subset with
zero negatives cannot establish negative-query behavior.

### Control the candidate pool

Today Qdrant prefetch depth is `2 * limit`, so requesting 50 candidates instead of
10 changes retrieval as well as ranking. Separate candidate depth from output depth
in the experiment. Record three arms:

```text
A = existing default retrieval at its recorded candidate depth
pool = retrieve 50 candidates with fixed prefetch limits and filters
B = first 10 results from pool in original order
C = rerank the exact same pool, then take 10 results

compare B versus C to isolate reranking
compare A versus C to measure the complete proposed path
score required evidence at top 5 and recall at top 10
```

Keep filters, tie-breaking, truncation policy, query encoding, and corpus fixed.
Content-budget/truncation behavior must not silently omit the evidence the reranker
is supposed to score. Report coverage losses as well as ordering improvements.

### Runtime and resource decision

| Option | Size | Main cost to validate |
|---|---|---|
| Existing local cross-encoder service | M | Service lifecycle, API/model compatibility, queueing and cancellation |
| Python inference sidecar | M | Additional runtime/process and reproducible model packaging |
| In-process ONNX binding | M–L | Native runtime/build/distribution complexity |

Verify current primary runtime documentation when selecting an implementation;
do not assume Ollama provides a compatible rerank endpoint. An LLM-scoring trial
can explore a hypothesis, but its quality/latency does not validate a different
cross-encoder. Prefer a direct small-model experiment when practical.

Measure added rerank and total p50/p95, cold startup, memory, and candidate counts
on declared hardware. Added warm p95 <=200 ms is a provisional target, not a
property of cross-encoders. Fix the accepted budget before running the held-out set;
if it fails, retain the negative result and keep the feature deferred.

### Implementation outline if approved by evidence

1. Decide runtime, candidate control, score semantics, and failure/timeout policy.
2. Add `internal/rerank` with a deterministic fake and one real implementation.
3. Wire optional search/eval flags and separate rerank timing; preserve defaults.
4. Verify the disabled path, fixed-pool ordering, empty/error results, deadlines,
   score handling, and content budgets with focused tests.
5. Run the three-arm full/structural/external comparisons and retain manifests,
   reports, per-query review, and a build/promote/defer decision.

Potential touchpoints: `internal/search`, `internal/eval`, `internal/model`, CLI
flag resolution, and user documentation. Flags in this section are proposed;
there is no runnable `--rerank` command in the current implementation.

## Relationship to GraphRAG

The techniques can compose, but the roadmap does not require all of them.
A model can change candidate recall, a graph can add connected evidence, and a
reranker can choose a better bounded context from a candidate set. Each can also
introduce regressions or operational costs. Reassess named residual failures after
each accepted experiment; defer GraphRAG if those failures no longer warrant it.
