# M0 external Go baseline — chi v5.2.3

**Original M0 evaluation complete; the original index-integrity defect is fixed and rechecked.**
This remains a measurement record, not a quality-release gate. The original run is
preserved below; the [post-fix recheck](../../runs/2026-09-23-idfix/README.md)
shows all generated points survive with declaration-unique IDs.

## Frozen inputs and selection

- Corpus: [go-chi/chi v5.2.3](https://github.com/go-chi/chi/tree/9b9fb55def404397748a9fc7e044efe9db1d618e),
  commit `9b9fb55def404397748a9fc7e044efe9db1d618e`, MIT licensed.
- Original runner: merged ragcodepilot revision `f46d3f169ee1de0d2652045c75547f84b211c2bf`.
  The post-fix recheck uses `047e56e` and representation
  `sparse-bm25-snowball-ident-v2+go-types-v2-identity`.
- Dataset: 20 cases, authored from source before any retrieval: 16 positives
  (8 navigation, 4 concept, 4 behavior) and 4 absent-feature negatives. Seven
  positives require multiple files; four are tagged structural flow questions.
- Each positive lists required files only, without symbol-only alternatives.
  [Label rationales and required source ranges](labels.json) were frozen alongside
  [golden.yaml](golden.yaml) at 10:19:50 UTC, before indexing at 10:20:47 UTC.
  [The freeze record](label-freeze.json) preserves their original hashes.

The router/middleware corpus exercises HTTP routing, context, wrapper interfaces,
error behavior, and cross-file calls outside ragcodepilot's domain. It is a compact
library, not a large service or proof of broad Go quality. Labels are a single
reviewer's source-informed judgment, not independently blinded annotations.

The clean archive contains 98 files; default configuration indexes 57 non-test Go
files. `_examples` is included. Both build-tag variants are indexed because the
walker indexes source rather than selecting a Go build. Upstream code was not run.

## Results

| Metric | Full set | Structural subset |
|---|---|---|
| Queries / positives / negatives | 20 / 16 / 4 | 4 / 4 / 0 |
| Query errors | 0 | 0 |
| File hit@1 | 15/16 = 0.9375 | 4/4 = 1.0000 |
| File hit@5 | 16/16 = 1.0000 | 4/4 = 1.0000 |
| MRR@5 | 0.9583 | 1.0000 |
| Expected-file recall@5 | 0.96875 | 0.8750 |
| Expected-file recall@10 | 0.96875 | 0.8750 |
| Negative pass, RRF ceiling 0.02 | 1/4 = 0.2500 | N/A: no negatives |
| Total query p50 / p95 | 21 / 23 ms | 21 / 23 ms |

[Full report](full.json) and [structural report](structural.json) retain raw CLI
output. Initial indexing took 13.189 seconds. The model was initially unloaded;
indexing and an unmeasured search preceded evaluation. These are single warm-model
observations with fresh CLI connections, not throughput or latency guarantees.

The file hit@5 result is saturated and **does not establish complete evidence**.
A source-range audit finds all predeclared nonblank evidence lines at top 5 for
only 9/16 positives in the full run, at top 10 for 11/16, and anywhere in the stored
index for 14/16. This conservative range check is separate from harness metrics:
file hits may contain the wrong method, and other equivalent context may sometimes
help a reader. No answers were generated or graded.

## Confirmed indexing defect — fix before dense-cache optimization

Indexing reports **382 generated chunks, but only 371 unique IDs and stored points**.
Seven duplicate-ID groups overwrite 11 earlier chunks. Every stored payload matches
the last generated variant of its ID. [The index audit](index-audit.json) records
all variants and retained locations; [the chunk manifest](chunks.json) records both
generated and stored chunks with content/enriched-input hashes.

The pinned implementation uses `repo:file:name:subchunk_index` for named chunks.
The AST path supplies the method name without its receiver, so two methods with
that name in the same file collide. A package function can also collide with a
method. A single named chunk uses index zero in both cases.

| File | Colliding name | Generated variants | Lost chunks |
|---|---|---:|---:|
| `_examples/rest/main.go` | Bind | 2 | 1 |
| `_examples/rest/main.go` | Render | 3 | 2 |
| `context.go` | URLParam | 2 | 1 |
| `middleware/wrap_writer.go` | Flush | 4 | 3 |
| `middleware/wrap_writer.go` | Hijack | 3 | 2 |
| `tree.go` | findEdge | 2 | 1 |
| `tree.go` | walk | 2 | 1 |

The public URLParam function at `context.go:9–15` is overwritten by the Context
method at `context.go:98–107`. This removes predeclared evidence for
`urlparam_definition` and `url_parameter_flow` despite their file hit@5 passes.
Sparse statistics also include all 382 generated inputs, including overwritten
variants. This baseline deliberately records the current defect; no runtime fix
or label relaxation was mixed into the measurement.

Reproduce the count audit using the pinned walker/config and ChunkFile on all
included Go files, group returned chunks by ID, and compare each group with
paginated Qdrant payloads. The diagnostic ran in a separate temporary runner copy;
the frozen evaluation binary and corpus were unchanged.
The follow-up recheck at `047e56e` regenerated the same chi source and retained
382/382 points. It also reran the unchanged 20-query labels; see the
[recheck record](../../runs/2026-09-23-idfix/README.md). The original 371-point
collection remains useful as a defect reproduction and is not silently rewritten.

```text
chunks = chunk all included Go files using the pinned configuration
by_id = group chunks by deterministic ID
for each ID with multiple chunks:
    record source locations and content hashes of every variant
    compare stored payload against generated variants
report generated count, unique ID count, stored count, and overwritten variants
```

## Retrieval failures and coverage review

The negative policy remains unchanged at RRF 0.02:

| Negative query | Top score | Top result | Outcome |
|---|---:|---|---|
| `oidc_jwks_negative` | 0.032051284 | `chi.go`, Router | Fail |
| `sqlite_migrations_negative` | 0.016666668 | `middleware/request_id.go`, block | Pass |
| `kafka_offsets_negative` | 0.030834913 | `middleware/throttle.go`, throttler | Fail |
| `grpc_reflection_negative` | 0.028205128 | `middleware/heartbeat.go`, Heartbeat | Fail |

Absent-feature labels were established by source inspection and an independent
keyword scan before retrieval. Dual-list RRF agreement on unrelated implementation
is not proof that the requested feature exists. These are retrieval diagnostics,
not refusal/faithfulness measurements; do not adjust thresholds to make them pass.

`method_not_allowed_flow` retrieves `mux.go` but misses required `tree.go` even at
rank 10, leaving expected-file recall at 0.5. All other positives retrieve their
expected files at top 5. [Failures](failures.json) preserves the harness findings.

The stricter, predeclared source-range review exposes these additional gaps:

| Query missing required source ranges at top 5 in full run | Complete at top 10? | Complete anywhere in index? |
|---|---|---|
| `urlparam_definition` | No | No |
| `middleware_execution_order` | No | Yes |
| `nested_route_pattern` | No | Yes |
| `url_parameter_flow` | No | No |
| `method_not_allowed_flow` | No | Yes |
| `custom_http_method_flow` | Yes | Yes |
| `panic_log_entry_flow` | Yes | Yes |

See [evidence-review.json](evidence-review.json) for exact missing lines. Full and
structural harness metrics agree, but `custom_http_method_flow` has an equal-score
Mux/Method tie across ranks 5–6. The Method evidence appears at top 5 only in the
structural run. File metrics hide this variation; do not claim stable source-level
coverage from the aggregate result.

## Artifacts and validation

- [Manifest](manifest.json): source and runner revisions, model digest, runtime,
  hardware, collection config, query limits, and artifact checksums.
- [Inputs](inputs.json): source-file, indexed-file, config, dataset and runner hashes.
- [Model metadata](model.json), [build info](build-info.txt), [execution log](execution.json),
  [index log](index.log), and [stderr](stderr.log).
- [Validation](validation.json): all 24 query outcomes independently recomputed;
  source and label hashes unchanged; returned chunks match indexed provenance.
- [Corpus license](CORPUS-LICENSE.txt); source is recoverable from the pinned upstream
  commit and is not vendored into this repository.

Evidence validation passes while index integrity fails. The original dataset,
required ranges, model, runtime implementation, and fixed threshold were not tuned
after seeing retrieval results. The collection `m0_chi_9b9fb55_20260922_101950` is
retained for inspection; existing self and working collections were untouched.

Runtime: Apple M1, 16 GiB RAM, Go 1.26.3, Ollama 0.33.3, Qdrant 1.19.1,
`nomic-embed-text:latest` F16/768d, model digest
`0a109f422b47e3a30ba2b10eca18548e944e8a23073ee3f3e947efcf3c45e59f`.
Hybrid uses limit 10, prefetch 20 per branch, RRF k=60, language Go and repo chi.
The model parameters say num_ctx 8192 but the loaded runtime reports 2048; current
client telemetry cannot establish whether inputs were truncated.

## Reproduction

Keep the corpus basename `chi`, use the pinned ragcodepilot config, verify the full
model digest and service versions, and choose a fresh unused collection. From this
repository checkout, the following freezes both source trees and writes new reports
outside the committed evidence directory:

```bash
M0_ROOT=$(mktemp -d /tmp/rag-m0-chi-replay.XXXXXX)
M0_COLLECTION="m0_chi_replay_$(date -u +%Y%m%d_%H%M%S)"
M0_DATASET="$PWD/docs/eval/external/chi-v5.2.3/golden.yaml"
mkdir "$M0_ROOT/ragsearch" "$M0_ROOT/chi"
git archive f46d3f169ee1de0d2652045c75547f84b211c2bf | tar -x -C "$M0_ROOT/ragsearch"
git clone --no-checkout https://github.com/go-chi/chi.git "$M0_ROOT/upstream"
git -C "$M0_ROOT/upstream" archive 9b9fb55def404397748a9fc7e044efe9db1d618e | tar -x -C "$M0_ROOT/chi"
cd "$M0_ROOT/ragsearch"
GOCACHE="$M0_ROOT/go-cache" go build -trimpath -o "$M0_ROOT/ragcodepilot" ./cmd/ragcodepilot

curl -fsS http://127.0.0.1:11434/api/tags > "$M0_ROOT/models.json"
python3 - "$M0_ROOT/models.json" <<'CHECK_MODEL'
import json, sys
models = json.load(open(sys.argv[1]))["models"]
model = next(m for m in models if m["name"] == "nomic-embed-text:latest")
assert model["digest"] == "0a109f422b47e3a30ba2b10eca18548e944e8a23073ee3f3e947efcf3c45e59f"
CHECK_MODEL

"$M0_ROOT/ragcodepilot" index --language go --collection "$M0_COLLECTION" --ollama-model nomic-embed-text:latest "$M0_ROOT/chi"
"$M0_ROOT/ragcodepilot" search --language go --repo chi --mode hybrid --limit 10 --collection "$M0_COLLECTION" --ollama-model nomic-embed-text:latest "URL parameter lookup" > "$M0_ROOT/warmup.txt"
"$M0_ROOT/ragcodepilot" eval --dataset "$M0_DATASET" --mode hybrid --limit 10 --collection "$M0_COLLECTION" --ollama-model nomic-embed-text:latest --output json > "$M0_ROOT/full.json"
"$M0_ROOT/ragcodepilot" eval --dataset "$M0_DATASET" --mode hybrid --limit 10 --subtype structural --collection "$M0_COLLECTION" --ollama-model nomic-embed-text:latest --output json > "$M0_ROOT/structural.json"
```

Verify golden.yaml against `label-freeze.json` before replay. A different model or
runtime is a new condition and needs a new manifest. Do not overwrite these reports
or silently replace the installed model. Exact executed commands are in execution.json.

## Current status after the fix

The collision fix is complete: receiver/declaration identity is included in Go
named-chunk IDs, the representation version is bumped, and same-hash version
refresh deletes old file points before reindexing. Focused tests and the fixed
self/chi recheck pass the point-survival gate. Dense-cache recovery is now the next
M3 implementation task; the original negative and source-coverage failures remain
open diagnostics.

## Original decision and next acceptance gate

M0 is complete as a measurement milestone: the self and external runs are pinned,
failures are named, and [manual comparison rules](../../README.md#baselines-and-the-corpus-stability-assumption)
are available. The first follow-up is a focused chunk-identity correctness fix:
include receiver/declaration identity, invalidate affected representations, and
verify same-file method/function collisions with tests and fresh isolated indexes.
Then repeat both corpora with unchanged query sets and a fixed indexed source.
Require all generated IDs to be unique, all expected points to survive, and no
accepted retrieval/coverage or negative cases to regress before cache optimization.
Keep external failures and self failures separate; their label policies and corpora
differ, so do not pool their scores or infer a cross-corpus improvement.
