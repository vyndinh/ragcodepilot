# M0 self-repository baseline — 2026-09-22

**Completed: self-repository refresh only.** The external-repository portion of M0
remains open. This run measures the unmodified implementation at merged revision
`fed7f22fd016311078d2112c4681e1aafb59bd3a` using a clean `git archive` snapshot.
It does not introduce a retrieval change or establish external quality.

## Results

| Metric | Full set | Structural subset |
|---|---|---|
| Queries / positives / negatives | 39 / 35 / 4 | 16 / 16 / 0 |
| Query errors | 0 | 0 |
| hit@1 | 31/35 = 0.8857 | 15/16 = 0.9375 |
| hit@5 | 34/35 = 0.9714 | 15/16 = 0.9375 |
| MRR@5 | 0.9238 | 0.9375 |
| Expected-file recall@5 | 0.8162 | 0.6917 |
| Expected-file recall@10 | 0.8886 | 0.8188 |
| Negative pass at RRF 0.02 | 2/4 = 0.5000 | Not applicable: no negatives |
| Total query p50 / p95 | 22 / 33 ms | 21 / 62 ms |

Navigation hit@5 is 23/24; concept is 7/7 and behavior is 4/4. The new collection
contains 248 chunks from 28 non-test Go files. Initial indexing took 15.187 seconds,
including model loading and collection creation. This is a small self-corpus
observation, not a large-repository performance claim.

- [Full report](../../baseline_v9.json) and [structural report](../../baseline_v9_structural.json)
- [Manifest](manifest.json): source/tree/archive hashes, binary/dependency metadata,
  model digest, hardware, collection configuration, preprocessing, and limits
- [Inputs](inputs.json): 147 frozen source-file hashes, 28 indexed-file hashes,
  dataset/config hashes, query IDs and filters
- [Chunks](chunks.json): 248 IDs, source locations, content and enriched-input hashes
- [Failures](failures.json): all 13 negative/coverage findings
- [Execution](execution.json), [index log](index.log), [build info](build-info.txt),
  [model metadata](model.json), and [validation](validation.json)

Historical v8 also reports hit@5 34/35 and the same three hit/negative failures.
Its corpus/model provenance is not equivalent to this manifest. Do not interpret
MRR, coverage, or latency differences as isolated improvements or regressions.
All older reports and the golden labels remain unchanged.

## Named failures and interpretation

- **`pipeline_run_callers_structural`:** the only positive hit@5 miss. Neither
  top 5 nor top 10 includes expected `cmd/ragcodepilot/main.go`; the top result
  is the `Pipeline` type in `internal/ingest/pipeline.go`, not its caller.
- **`oauth_middleware_negative`:** top result is `setDim` in
  `internal/embedding/ollama.go`, score 0.032002047, exceeding the fixed 0.02 ceiling.
- **`grpc_gateway_router_negative`:** top result is `Close` in
  `internal/qdrant/client.go`, score 0.032291666, exceeding the same ceiling.

The negatives are retrieval diagnostics, not answer-refusal tests. Keep the
threshold unchanged and carry these as known failures into future comparisons.

Eleven positives have incomplete expected-file coverage at top 5:

| Query | Recall@5 / @10 | Expected files missing at top 5 |
|---|---|---|
| `reindex_change_detection_behavior` | 0.500 / 0.500 | `internal/ingest/hasher.go` |
| `collection_dimension_mismatch_behavior` | 0.500 / 0.500 | `internal/search/searcher.go` |
| `eval_limit_guard_behavior` | 0.500 / 1.000 | `cmd/ragcodepilot/main.go` |
| `chunkfile_callers_structural` | 0.000 / 0.000 | `internal/ingest/pipeline.go` |
| `pipeline_run_callers_structural` | 0.000 / 0.000 | `cmd/ragcodepilot/main.go` |
| `searcher_search_callers_structural` | 0.000 / 1.000 | `cmd/ragcodepilot/main.go`, `internal/eval/runner.go` |
| `trace_answer_flag_to_ollama_chat_structural` | 0.500 / 0.500 | `cmd/ragcodepilot/main.go` |
| `trace_index_to_qdrant_upsert_structural` | 0.667 / 1.000 | `cmd/ragcodepilot/main.go` |
| `change_impact_embedder_embed_structural` | 0.600 / 0.600 | `internal/search/searcher.go`, `internal/ingest/pipeline.go` |
| `change_impact_generator_generate_structural` | 0.800 / 1.000 | `internal/eval/runner.go` |
| `propagate_skip_file_pattern_structural` | 0.500 / 1.000 | `internal/ingest/walker.go` |

`chunkfile_callers_structural` and `searcher_search_callers_structural` score
hit@5 through accepted symbols while expected-file recall@5 is zero. The current
file-OR-symbol metric can credit a callee or search implementation for a caller
question. Preserve these labels for this baseline; any label correction must be
a separate versioned comparison. File recall itself is only a proxy for complete
symbol/relationship evidence, so inspect the relevant source chunks manually.

The structural run repeats the full run's per-query quality metrics exactly.
Equal-score ordering changes occur in `chunkfile_callers_structural` (including a
top-10 boundary tie) and `change_impact_generator_generate_structural`. These did
not change metrics here; this pair of runs does not establish exact-rank stability.

## Runtime and comparability

- Apple M1, 8 logical CPUs, 16 GiB RAM; macOS 26.6.2; Go 1.26.3 darwin/arm64.
- Ollama 0.33.3, `nomic-embed-text:latest`, F16, 768 dimensions; digest
  `0a109f422b47e3a30ba2b10eca18548e944e8a23073ee3f3e947efcf3c45e59f`.
- Qdrant 1.19.1; image digest recorded in the manifest. Collection
  `m0_self_fed7f22_20260922_094004` was absent before indexing and is retained for inspection.
- Representation `sparse-bm25-snowball-ident-v2+go-types-v1`; hybrid RRF k=60,
  limit 10, dense/sparse prefetch 20 each; dataset language filters are Go.
- No model was resident before indexing. One unmeasured hybrid search followed
  indexing, then full evaluation, then structural evaluation. Each command opens
  fresh connections. Query latencies describe these warm-model observations only;
  unrelated machine activity was not controlled.
- The installed model parameters say `num_ctx 8192`, while Ollama reports a loaded
  context length of 2048. The current client sends no truncation/options overrides
  and retains no truncation telemetry. The exact runtime/model and input hashes
  are pinned; this evidence does not assert that every input avoided truncation.

## Reproduce without changing the working index

Check the recorded runtime versions and full model digest before a comparison.
Use a fresh collection every time, retain the basename `ragsearch` (point IDs use
it), and run from the frozen directory so `config.yaml` resolves consistently.
The following writes new reports under a temporary directory:

```bash
M0_REV=fed7f22fd016311078d2112c4681e1aafb59bd3a
M0_ROOT=$(mktemp -d /tmp/rag-m0-replay.XXXXXX)
M0_COLLECTION="m0_self_replay_$(date -u +%Y%m%d_%H%M%S)"
mkdir "$M0_ROOT/ragsearch"
git archive "$M0_REV" | tar -x -C "$M0_ROOT/ragsearch"
cd "$M0_ROOT/ragsearch"
GOCACHE="$M0_ROOT/go-cache" go build -trimpath -o "$M0_ROOT/ragcodepilot" ./cmd/ragcodepilot

curl -fsS http://127.0.0.1:11434/api/tags > "$M0_ROOT/models.json"
python3 - "$M0_ROOT/models.json" <<'CHECK_MODEL'
import json, sys
models = json.load(open(sys.argv[1]))["models"]
model = next(m for m in models if m["name"] == "nomic-embed-text:latest")
assert model["digest"] == "0a109f422b47e3a30ba2b10eca18548e944e8a23073ee3f3e947efcf3c45e59f"
CHECK_MODEL

"$M0_ROOT/ragcodepilot" index --language go --collection "$M0_COLLECTION" --ollama-model nomic-embed-text:latest .
"$M0_ROOT/ragcodepilot" search --language go --mode hybrid --limit 10 --collection "$M0_COLLECTION" --ollama-model nomic-embed-text:latest "where is ChunkFile defined" > "$M0_ROOT/warmup.txt"
"$M0_ROOT/ragcodepilot" eval --dataset docs/eval/golden.yaml --mode hybrid --limit 10 --collection "$M0_COLLECTION" --ollama-model nomic-embed-text:latest --output json > "$M0_ROOT/full.json"
"$M0_ROOT/ragcodepilot" eval --dataset docs/eval/golden.yaml --mode hybrid --limit 10 --subtype structural --collection "$M0_COLLECTION" --ollama-model nomic-embed-text:latest --output json > "$M0_ROOT/structural.json"
```

If the model digest or runtime differs, record a new manifest and treat it as a
new condition. Do not overwrite these reports or silently replace the installed
model. The exact executed arguments, including explicit loopback addresses, are
in `execution.json`; the example uses the CLI's equivalent localhost defaults.

## Manual acceptance before a future comparison

1. Verify source, dataset, config, model/preprocessing, representation, and limits
   against the manifest. Freeze the indexed source independently of experiment code.
2. Reject query errors, missing query IDs, and incompatible control/candidate inputs.
3. Preserve accepted hits and inspect all 11 incomplete-coverage cases. Track
   passing and failing negatives separately under the same calibrated policy.
4. Inspect required caller files and source chunks; symbol-only hits are not proof
   of complete structural evidence. Keep answer correctness outside these scores.
5. Record per-query gains/losses and warm/cold conditions alongside aggregates.
   If equal-score ties affect a decision, resolve that uncertainty before acceptance.
6. Save uniquely named reports and manifests. Complete the separate external-repo
   portion of M0 before choosing the M3 implementation increment.
