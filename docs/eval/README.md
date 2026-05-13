# Evaluation Harness

Offline retrieval evaluation for ragcodepilot.
Loads a YAML golden dataset, runs each query through the existing search path, and reports `hit@k`, `MRR@5`, `recall@10`, and per-stage latency percentiles.

This is Phase 1 of `docs/plan/mvp_roadmap.md`. The harness is the scoreboard for every retrieval-quality change that follows (hybrid search, reranking, chunker upgrades).

---

## Quick start

```bash
# 1. Start Qdrant
docker compose up -d

# 2. Make sure Ollama is running and the embedding model is pulled
ollama pull nomic-embed-text

# 3. Index ragcodepilot's own repo
go run ./cmd/ragcodepilot index --language go .

# 4. Run the eval
go run ./cmd/ragcodepilot eval --dataset docs/eval/golden.yaml
```

Sample output:

```text
Dataset:    docs/eval/golden.yaml
Collection: code_chunks
Embedder:   ollama/nomic-embed-text
Run ID:     2026-05-12T10-00-00Z
Queries:    8 (positive 6, negative 2, errors 0)

Retrieval metrics (positive queries only):
  hit@1:        0.67
  hit@3:        0.83
  hit@5:        0.83
  MRR@5:        0.72
  recall@10:    0.71

Negative queries pass rate: 1.00

Latency (ms):
  total p50/p95:   58 / 94
  embed p50/p95:   34 / 62
  qdrant p50/p95:  22 / 31

By type:
  behavior     n=2  hit@5=1.00  MRR@5=0.75
  concept      n=2  hit@5=1.00  MRR@5=0.75
  navigation   n=2  hit@5=0.50  MRR@5=0.50
  negative     n=2  pass_rate=1.00
```

For machine-readable output:

```bash
go run ./cmd/ragcodepilot eval --output json > docs/eval/baseline_v1.json
```

---

## CLI flags

| Flag | Default | Notes |
|---|---|---|
| `--dataset` | `docs/eval/golden.yaml` | Path to the YAML golden set |
| `--collection` | `code_chunks` | Qdrant collection to query |
| `--output` | `human` | `human` (text) or `json` |
| `--limit` | `10` | Per-query result limit; must be ≥10 for `recall@10` |
| `--type` | (none) | Filter to queries with this type (e.g. `navigation`) |
| `--qdrant-host` | `localhost` | |
| `--qdrant-port` | `6334` | gRPC port |
| `--embedder` | `ollama` | `ollama` or `fake` |
| `--ollama-url` | `http://localhost:11434` | |
| `--ollama-model` | `nomic-embed-text` | |

Run `ragcodepilot eval --type navigation` to focus on a single category while iterating.

---

## Metrics

All metrics are computed over **positive queries only** (those with expected files or symbols). Negative queries contribute to `negative_pass_rate` separately.

| Metric | What it measures |
|---|---|
| `hit@k` | Mean fraction of queries where at least one expected file or symbol appears in the top `k`. |
| `MRR@k` | Mean reciprocal rank of the first relevant result. Rewards putting the right answer at the top. |
| `recall@k` | Mean fraction of *expected files* that appear in the top `k`. Symbols don't count toward recall — they live inside expected files. |
| `negative_pass_rate` | Fraction of negative queries whose top-1 score is below the configured threshold (or returns no results). |
| `latency_*_p50/p95_ms` | Percentile latencies, broken out by stage. `embed` is Ollama; `qdrant` is the vector search RPC; `total` is end-to-end per query. |

A result is **relevant** when its `file_path` is in the expected file list OR its `name` (function/symbol) is in the expected symbol list.

---

## Golden dataset schema

A minimal positive query:

```yaml
queries:
  - id: my_query_id              # unique within the file
    query: "what the user types"
    type: navigation             # or concept, behavior, negative
    filters:
      languages: ["go"]          # passed to qdrant filter
      repos: ["ragcodepilot"]    # optional
    expected:
      files:
        - internal/foo/bar.go
      symbols:
        - FooBar
```

A negative query:

```yaml
queries:
  - id: oauth_middleware
    query: "where is the OAuth middleware"
    type: negative
    filters:
      languages: ["go"]
    negative:
      top1_score_below: 0.55     # top-1 score must be strictly less than this
```

**Type tags** are case-sensitive strings: `navigation`, `concept`, `behavior`, `negative`. They drive the per-type breakdown in the report; pick whichever fits.

**Symbols** match the chunk's `name` field — for Go this is the function/method name extracted by the AST chunker.

**Files** match the chunk's `file_path` field — the repo-relative path stored in Qdrant payload.

---

## Adding a new query

1. Pick a real question you'd want answered by `ragcodepilot search`.
2. Run the search manually and inspect the result — which file is the actual answer in? What symbol?
3. Add the YAML entry. Use the most specific expected file; you can include 1-3 acceptable alternatives in `expected.files`.
4. Re-run `ragcodepilot eval` and confirm the new query shows up.
5. Commit both the YAML and an updated `baseline_*.json` in the same PR.

Keep the golden set focused. 20-30 hand-curated queries are more useful than 200 hastily-written ones.

---

## What's not measured (yet)

- **Filter correctness.** The eval doesn't verify that all returned chunks honor the language/repo filter. Add later (vision review's feedback `filter_violation_count`).
- **Result-shape validation.** No check that returned chunks contain non-empty `content`, valid line numbers, etc.
- **Comparison mode.** No `eval compare baseline.json candidate.json` yet. For now, manually diff the JSON.
- **CI gating.** No regression policy. The harness reports; you decide what to do.

Items intentionally deferred — see `docs/review_feedback/rag_evaluation_metrics_with_feedback.md` for the full backlog and roadmap.

---

## Hooking into Phase 2 and 3

This harness is the measurement contract for:

- **Phase 2 (hybrid search):** Tag queries with `type: navigation` (mostly exact-symbol). Phase 2's exit criterion is ≥10pp `hit@5` lift on those, with no regression on `concept` queries. Compare `baseline_v1.json` (dense) vs `baseline_v2.json` (hybrid).
- **Phase 3 (reranking):** Add an `ambiguous` tag to queries that have weak top-1 results. Reranking should lift their `MRR@5`.
- **Phase 3 (chunker upgrades):** When adding a non-Go chunker, also add 3-5 golden queries targeting that language.

Do not delete or rewrite existing queries during a refactor — that destroys the regression-detection value. Add new queries; supersede old ones in a labeled batch with explicit before/after metrics.
