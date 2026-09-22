# Evaluation Harness

Offline retrieval evaluation for ragcodepilot.
Loads a YAML golden dataset, runs each query through the existing search path, and reports `hit@k`, `MRR@5`, `recall@10`, and per-stage latency percentiles. With `--answer`, it additionally generates an answer per query and reports reference-free answer metrics — see [Answer-mode evaluation](#answer-mode-evaluation----answer-tier-b).

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
  recall@5:     0.66
  recall@10:    0.71
  recall gap:   0.05 (<0.10 → embedding/chunking is the floor)

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
go run ./cmd/ragcodepilot eval --output json > /tmp/ragcodepilot-eval.json
```

---

## Comparing runs — `compare.py`

`docs/eval/compare.py` is a stdlib-only Python helper for comparing eval JSON reports. Pass one path to summarize; pass two or more to get a side-by-side table plus pairwise deltas against the first report.

```bash
# Summarize one report
docs/eval/compare.py docs/eval/baseline_v2.json

# Regression check: candidate vs current baseline
docs/eval/compare.py docs/eval/baseline_v2.json /tmp/candidate.json

# Phase 2 sweep (this is the table in docs/plan/hybrid_search.md)
docs/eval/compare.py \
  docs/eval/baseline_v1.json \
  /tmp/eval_dense.json \
  /tmp/eval_sparse.json \
  /tmp/eval_hybrid.json \
  --labels=baseline_v1,dense_p2,sparse_p2,hybrid_p2
```

Output is a fixed-width table covering hit@1/3/5, MRR@5, recall@10, per-type hit@5 (navigation / concept / behavior), negative pass rate, and total p50/p95 latency. The deltas block reports each candidate's gap to the baseline in percentage points — useful for the "did this change help or hurt?" check.

The script is intentionally read-only and stdlib-only (no `pip install`) so it works on any machine that can run Python 3.10+. If you need to plot trends across many runs, parse the JSON directly — the eval `--output json` schema is stable.

---

## CLI flags

| Flag | Default | Notes |
|---|---|---|
| `--dataset` | `docs/eval/golden.yaml` | Path to the YAML golden set |
| `--collection` | `code_chunks` | Qdrant collection to query |
| `--output` | `human` | `human` (text) or `json` |
| `--limit` | `10` | Per-query result limit; must be ≥10 for `recall@10` |
| `--type` | (none) | Filter to queries with this type (e.g. `navigation`) |
| `--subtype` | (none) | Filter to queries with this subtype (e.g. `structural` — combines with `--type` as an AND) |
| `--qdrant-host` | `localhost` | |
| `--qdrant-port` | `6334` | gRPC port |
| `--embedder` | `ollama` | `ollama` or `fake` |
| `--ollama-url` | `http://localhost:11434` | |
| `--ollama-model` | `nomic-embed-text` | embedding model |
| `--answer` | `false` | Also generate answers and score reference-free answer metrics (Tier B) |
| `--generator` | `ollama` | Generator for `--answer`: `ollama` or `fake` |
| `--ollama-generative-model` | `qwen2.5-coder:7b` | generative model for `--answer` |
| `--answer-limit` | `5` | Top chunks fed to the generator (retrieval metrics still use `--limit`) |

Run `ragcodepilot eval --type navigation` to focus on a single category while iterating.

---

## Metrics

All metrics are computed over **positive queries only** (those with expected files or symbols). Negative queries contribute to `negative_pass_rate` separately.

| Metric | What it measures |
|---|---|
| `hit@k` | Mean fraction of queries where at least one expected file or symbol appears in the top `k`. |
| `MRR@k` | Mean reciprocal rank of the first relevant result. Rewards putting the right answer at the top. |
| `recall@k` | Mean fraction of *expected files* that appear in the top `k` (reported at k=5 and k=10). Symbols don't count toward recall — they live inside expected files. |
| `recall gap` | `recall@10 − recall@5`. Diagnostic for *what to fix next*: a large gap (≥0.10) means relevant chunks are retrieved but ranked outside the top-5 → **reranking** has headroom; a small gap (<0.10) means the misses are absent from the top-10 → **embedding/chunking** is the floor (reranking can't surface what retrieval didn't return). |
| `negative_pass_rate` | Fraction of negative queries whose top-1 score is below a **mode-calibrated** ceiling (or that return no results). Dataset `top1_score_below` is the dense cosine ceiling (0.55). Hybrid RRF and sparse BM25 ignore a cosine-calibrated YAML value: RRF fails only on dual-prefetch agreement (ceiling 0.02, between single-list ~0.017 and dual-list ~0.033); BM25 uses ceiling 10 (unbounded scores; 0.55 would always fail). |
| `latency_*_p50/p95_ms` | Percentile latencies, broken out by stage. `embed` is Ollama; `qdrant` is the vector search RPC; `total` is end-to-end per query. |

A result is **relevant** when its `file_path` is in the expected file list OR its `name` (function/symbol) is in the expected symbol list.

---

## Answer-mode evaluation — `--answer` (Tier B)

`ragcodepilot eval --answer` runs the **normal retrieval eval and, additionally, generates an answer for every query** and scores it. It exists to put a number on answer mode without the cost and flakiness of an LLM-as-judge.

### The verification ladder (where Tier B sits)

Verifying `--answer` splits into three tiers of increasing cost and decreasing determinism:

| Tier | What it checks | How | Status |
|---|---|---|---|
| **A** | Plumbing: retrieve → prompt → generate → format | `--generator fake`, deterministic, no Ollama | unit tests |
| **B** | Answer *shape* on real generation: well-formed, citations resolve, refuses when it should | reference-free rules over real output | **this section** |
| **C** | Answer *correctness*: are the claims actually supported by the chunks? | LLM-as-judge (faithfulness, semantic citation precision) | deferred to v1 |

Tier B is the sweet spot: it runs **real** generation (so it catches real model behavior) but grades with **deterministic, reference-free** rules (so it's reproducible and needs no judge model or hand-written gold answers).

### What "reference-free" means

The metrics are computed purely from **the answer text plus the chunks that were placed in its prompt** — no reference/gold answer is required. That's what makes Tier B cheap to maintain: adding a golden query costs nothing extra on the answer side.

### How a query is scored

```
for each query q in the golden set:
    results = search(q)                      # identical retrieval path to normal eval (top --limit)
    chunks  = top --answer-limit of results  # 1-based, citation-ready (default 5)
    answer  = generator.Generate(q, chunks)  # greedy: temperature 0, fixed seed
    cites          = parse "[N]" markers in answer
    valid, dangling = partition cites on (1 ≤ N ≤ len(chunks))
    well_formed    = trimmed answer is non-empty
    refused        = answer contains a refusal phrase   # heuristic — see caveat
```

**Why `--answer-limit` is separate from `--limit`:** retrieval metrics need a deep
window (`--limit` ≥ 10 for `recall@10`), but `search --answer` ships a shallow
context (top 5) to keep generation fast and citations clean. Feeding the eval's
top-10 to the generator would measure a config users never run — more chunks change
both citation behavior and refusal behavior. `--answer-limit` (default 5) makes the
answer prompt match the shipped product while retrieval metrics keep their deep window.

Each query keeps an `answer` block in the JSON report; the run-level rollup is the `answer` object.

### Metrics

| Metric | Computed over | What it means |
|---|---|---|
| `well_formed_rate` | all non-errored answers | The generator produced non-empty text. The floor. |
| `cited_rate` | positive queries | Fraction whose answer cites at least one `[N]` chunk. |
| `all_citations_valid_rate` | positive queries that cited | Fraction whose citations **all** resolve to a provided chunk (no dangling refs). |
| `dangling_citations` | all answers | Total count of `[N]` refs pointing outside the provided chunk set. |
| `refusal_rate_negative` | negative queries | Fraction that correctly declined ("not enough information") instead of inventing an answer. **The hallucination floor.** |
| `generate_p50/p95_ms` | all answers | Generation latency percentiles. |

Citation metrics are scoped to **positive** queries (negatives are expected to refuse and not cite, so including them would distort the rates). Refusal is scoped to **negative** queries.

### Design choices

- **Greedy decoding (temperature 0 + fixed seed).** Generation is deterministic given a fixed prompt and model version, so re-runs are comparable. This applies to `search --answer` too, not just eval.
- **Report-only, never gated.** Answer metrics are printed and serialized but **never change the exit code**. The fast, deterministic retrieval gate stays clean; answer quality is observed, not enforced. (Only retrieval errors still fail the run, as before.)
- **Refusal is a heuristic.** It matches phrases like "not enough information" / "do not contain" / "cannot answer". It is a *diagnostic*, not ground truth — it can miss a creatively-worded refusal or fire on an answer that merely quotes such a phrase. Good enough for a reported floor, not for gating.

### Cost

One LLM call **per query**, so a full `--answer` run is **minutes, not milliseconds**. Set `OLLAMA_KEEP_ALIVE=-1` and let the harness warm the model once before the loop (it does this automatically). Use `--type negative` or a small dataset while iterating on prompts.

### Sample output (answer section)

```text
Answer metrics (reference-free; generator: ollama/qwen2.5-coder:7b):
  generated:                8 (errors 0)
  well-formed rate:         1.00
  cited rate (positive):    0.83
  all-citations-valid:      1.00
  dangling citations:       0
  refusal rate (negative):  1.00
  generate p50/p95 (ms):    2100 / 4800
```

When `--answer` is not passed, this section is omitted entirely and the report is byte-identical to a retrieval-only run.

---

## Golden dataset schema

A minimal positive query:

```yaml
queries:
  - id: my_query_id              # unique within the file
    query: "what the user types"
    type: navigation             # or concept, behavior, negative
    subtype: structural          # optional refinement; e.g. structural multi-hop queries
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
      top1_score_below: 0.55     # dense cosine ceiling; hybrid/sparse use mode defaults
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

## Baselines and the corpus-stability assumption

Updated 2026-09-22. Saved reports are observations tied to their source, query,
model, and score-calibration snapshots; they are not evergreen acceptance targets.

| Report | Scope | Use |
|---|---|---|
| `baseline_v1.json` | Earlier dense corpus | Historical |
| `baseline_v2*.json` | Phase 2 corpus | Historical hybrid/dense comparisons |
| `baseline_v6.json` | 23 queries, 19 positives; historical 182-chunk corpus | Historical retrieval; hybrid 1.00 negatives used ineffective 0.55 cutoff |
| `baseline_v7.json` | 39 queries, 35 positives | Historical full set after structural additions |
| `baseline_v7_structural.json` | 16 positives, zero negatives | Historical completeness diagnostic; 14/16 already pass hit@5 |
| `baseline_v8.json` | Historical main report: 39 queries, 35 positives, 199 Go chunks at capture | Predates later chunker changes; hit@5 34/35, negative pass 2/4 at RRF 0.02 |
| `baseline_v9.json` | Fresh pinned self run at `fed7f22`: 39 queries, 35 positives, 248 Go chunks | hit@5 34/35, recall@5 0.8162, negative pass 2/4; zero query errors |
| `baseline_v9_structural.json` | Same pinned index: 16 positives, zero negatives | hit@5 15/16, recall@5 0.6917; zero query errors |

The [v9 run record](runs/2026-09-22-self-fed7f22/README.md) includes the manifest,
source/query/config hashes, model digest, chunk IDs, exact commands, and named
failure inventory. This completes only M0's self-repository refresh; the external
repository remains open. Do not infer an isolated algorithm win by comparing v9
with older reports whose corpus and runtime manifests differ or are missing.

**Negative semantics:** the reconciled runner uses score-family calibration
(RRF 0.02). The historical hybrid 0.55 cosine cutoff exceeded the RRF maximum
of about 0.0333, so 1.00 pass was vacuous. The same
two saved v6 negatives fail when replayed at that ceiling. Threshold agreement
alone does not measure semantic irrelevance or answer faithfulness. Preserve known
failures and apply fixed, independently calibrated policies for new score families.

**Paired experiment workflow:**

1. Freeze source revision/file manifest, query set/hash, config/filters, chunk IDs
   and counts, representation version, model artifact/preprocessing, runtime
   versions, retrieval mode, and candidate/output limits. Pin hardware and warm
   state for latency. Keep tuning and held-out queries separate.
2. Index control and candidate from the same frozen source into separate fresh
   experiment collections as needed. Do not delete the working index. Changing
   model names in an existing collection can incorrectly skip unchanged files.
3. Run fresh full, structural, and the selected external-repo evaluations in both arms. Preserve
   the control report actually generated by the experiment. If chunking is the
   treatment, record both chunk manifests while keeping source and labels fixed.
4. Validate manifest/query compatibility and query errors before comparing.
   `compare.py` only prints deltas; it does not enforce these checks or CI gates.
5. Compare named hits, required-file recall, manually inspected symbol/relationship
   coverage, negative failures under valid calibration, and latency. A structural
   run with no negatives cannot certify negative-query behavior.
6. Retain paired reports and manifests under unique experiment/revision names;
   update the evidence pointer only after review. Keep old reports unchanged.

Start with manual checks on the self-corpus and one representative external
repository. A second external repo is required before generalization claims or
broad retrieval promotion. Nightly/on-demand retrieval CI is deferred until this
manual workflow is stable and repeated often enough to warrant automation. If
automated later, reject incompatible inputs, missing required query classes,
query errors, and new regressions against accepted cases. Corpus-specific
floors need measured agreement; historical self-corpus hit@5 0.85 and vacuous
negative 1.00 do not establish universal floors. Known failures remain explicit
follow-up work. Tier B answer metrics remain report-only.

Corpus drift can move ranking without changing an algorithm. The earlier Phase 1
versus Phase 2 comparison changed the indexed source population; experiment code
must not accidentally become part of one arm's corpus.

### Interpreting coverage and isolating changes

Historical v7 structural hit@5 passes 14/16 queries. All seven queries with a
recall@10–recall@5 gap already pass hit@5: the missing evidence concerns completeness.
A file-level hit can still return the wrong chunk from the right file. Preserve
accepted hits and inspect required files, symbols, and relationships separately.
A larger recall gap can also result from better recall@10; it is not automatically
a regression or proof that a new component is needed.

For any future retrieval comparison:

- Choose target query IDs, the primary metric, and resource budget before running.
  Use coverage for incomplete context and ranking metrics when coverage suffices.
- Keep candidate depth fixed when isolating ordering changes. Increasing retrieval
  depth and changing ordering together cannot establish which caused the result.
- Record index/query model artifacts and preprocessing. Equal vector dimensions
  do not establish compatibility between models.
- Calibrate changed score families on separate tuning inputs, then freeze the
  policy before evaluating held-out queries. Raw scores from different families
  are not interchangeable.
- Report per-query regressions, known failures, sample counts, and latency alongside
  aggregate gains. Answer shape alone cannot establish content quality.

---

## What's not measured (yet)

- **Answer correctness / faithfulness (Tier C).** `--answer` checks answer *shape* (well-formed, citations resolve, refuses on negatives) but **not** whether the claims are actually supported by the cited chunks. Content review is manual; no automated judging feature is scheduled. See the [verification ladder](#the-verification-ladder-where-tier-b-sits).
- **Filter correctness.** The eval doesn't verify that all returned chunks honor the language/repo filter.
- **Result-shape validation.** No check that returned chunks contain non-empty `content`, valid line numbers, etc.
- **Comparison mode.** No built-in `eval compare` yet. `compare.py` prints report deltas; manifest checks and per-query review are still manual.
- **CI gating.** Automated retrieval gates are deferred. Use the manual frozen-input comparison policy above; existing unit/site CI is unchanged. Answer metrics remain report-only.

These are measurement limitations. The [roadmap](../plan/mvp_roadmap.md) owns delivery scope; historical review logs are not an active backlog.

---

## Relationship to the roadmap

The implemented hybrid and answer-mode reports remain historical evidence.
Current evaluation work is M0 in the [roadmap](../plan/mvp_roadmap.md): a fresh
self-corpus baseline (v9 complete), one external repository (open), and a named failure inventory.
This guide defines comparison practice; it does not schedule retrieval features.

Do not silently delete or rewrite existing queries during a refactor. Add new
queries or supersede old ones in a labeled batch with explicit before/after results.
