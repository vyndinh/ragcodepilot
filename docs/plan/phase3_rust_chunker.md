# Phase 3 — Rust AST Chunker Implementation Plan

> ## ⏸ Status: Deferred (2026-05-14)
>
> Phase 3 is **parked**, not cancelled. The full plan below stays intact for revival; do not delete or rewrite.
>
> **Reason for deferral.** Phase 5 v0 (minimal `--answer` mode) was prioritized ahead of Phase 3 to validate the RAG product direction before adding more language coverage. See `docs/plan/phase5_v0_answer_mode.md` and the updated sequence in `docs/plan/mvp_roadmap.md`.
>
> **Revival triggers.** Un-defer Phase 3 if any of:
> - Phase 5 v0 dogfooding surfaces "I want to search Rust code" as a real need.
> - You start Phase C (Rust→Go vector-DB refactor) and want AST-aware chunks of the Rust reference codebase.
> - A Rust-heavy user reports the regex fallback is unusable for their work.
>
> Until then, the plan and its task tracker (`docs/task_tracker/phase3_rust_chunker.md`) sit as a ready-to-execute artifact — multiple rounds of review have wrung most of the P1 issues out of it, so revival starts close to "begin coding."

---

Ship one new language's AST-based chunker (**Rust**) using a `syn`-based sidecar binary. Reranker is **deferred** from Phase 3 to a future phase with a pre-committed activation path.

**Exit criteria:**
- `hit@5` on Rust-targeted golden queries is higher with AST chunking than with the regex fallback (apples-to-apples comparison via binary-disabled re-index, both runs against the same mixed Go+Rust corpus).
- No regression on Go-language queries against the **Phase 3 pre-baseline** (a hybrid eval run on the mixed Go+Rust corpus with Rust files chunked via regex fallback). Tolerance: ±2pp on `hit@5`. The corpus is the same; only the chunker varies. Comparing directly to `baseline_v2.json` is useful historical context but not a strict gate (different corpus).
- Indexing a Rust file with `rs-chunk` missing falls back to regex with a clear warning, not a hard failure.
- Full test suite passes with `-race`.

---

## Context

Phase 2 closed strong on 2026-05-13: hybrid mode `hit@5` = 0.895 (+10.5pp over dense, +25pp on navigation queries), with clean same-corpus baselines committed (`docs/eval/baseline_v2.json` for hybrid, `docs/eval/baseline_v2_dense.json` for dense). The MVP roadmap originally scoped Phase 3 as two sub-phases: cross-encoder reranking + a non-Go AST chunker.

The decision for Phase 3 is to **defer the reranker** and ship the **chunker alone**:

- `hit@5` is already 0.895; rerank's expected ceiling is ~2-3pp on top — diminishing returns.
- All viable rerank paths add real dependency: Python sidecar with sentence-transformers (new pip env + process), pure-Go ONNX (CGo + heavy integration), or LLM-as-reranker (vision review's "avoid early" list).
- Hybrid mode dropped `hit@1` by 10pp (0.526 → 0.421) — that's exactly the gap rerank would target — but this is a known trade Phase 2 made deliberately. Addressing it deserves its own focused phase, not a half-baked addition.
- Doing only the chunker keeps Phase 3 scope tight, validates the AST-via-sidecar architecture, and produces a clean same-corpus baseline (`baseline_v3.json`) before any further algorithm churn.

Language: **Rust**. Aligns with Phase C's vector-DB ambition (which references a Rust reference implementation), and seeds an AST-aware corpus for that future work.

---

## Resolved Design Decisions

### 1. Reranker: deferred for Phase 3

Python sidecar (daemon) with a cross-encoder model is **pre-committed as the activation path** when metrics warrant. Starting candidate: `cross-encoder/ms-marco-MiniLM-L6-v2` (verify the canonical HuggingFace path at activation — sentence-transformers has used both `L6` and `L-6` forms historically). MS MARCO is trained on web search passages, not code; treat it as the first candidate, not the final model. Eval at activation time will choose between MS MARCO MiniLM, `BAAI/bge-reranker-v2-m3`, or a code-specific reranker based on observed code-relevance quality. Triggers:

- `hit@1` drops below 0.55 on a future phase baseline, **or**
- `MRR@5` regresses below 0.60, **or**
- `--answer` mode (Phase 5) needs precision in top-3 results.

See "Reranker activation plan" at the end of this document for the pre-committed design.

### 2. AST integration: Rust sidecar binary using `syn`

Mirrors the sidecar pattern that would have been used for a Python `ast` chunker. ~100 lines of Rust. Single artifact to build. No CGo: the Go binary stays pure-Go.

Considered and rejected for Phase 3: **tree-sitter via go-tree-sitter with CGo**. Multi-language uniform (good when more languages get added later), but CGo introduces cross-platform build complexity and the Go binary loses its "single static binary" property. For one language right now, sidecar is simpler. If/when a third non-Go language gets requested, switch to tree-sitter then.

### 3. Distribution: `cargo build --release`, one-time setup

Binary lives at `internal/ingest/rschunk/target/release/rs-chunk` after the user runs `cargo build --release` once. If the binary is missing at runtime, the Rust chunker falls back to the regex chunker and prints a one-line setup hint. Indexing never fails because Rust isn't built.

Considered and rejected: **embed cross-compiled binary via `go:embed`**. Would simplify distribution to a single Go binary, but requires a per-platform build matrix at compile time. Not worth it for a local dev tool.

### 4. Eval corpus: embedded fixture repo

~20 small `.rs` files (~10-30 lines each) live at **`testdata/fixtures/rust_repo/`** at the project root (not inside the Rust crate's `testdata/`). Two reasons for the project-root location:

1. **`ragcodepilot index --language go,rust .` from the repo root needs to see these files.** Putting them inside `internal/ingest/rschunk/testdata/` would either be skipped by the walker (if `testdata/` is in `skip_dirs`) or require an awkward index command.
2. **Go unit tests and eval use different working directories.** A single canonical fixture path that both can reach without relative-path gymnastics is worth the structural separation.

Covers free functions, structs + impl, enums, traits, async, generics, inline `mod`. Portable, fast, no external repo dependency.

The Rust crate's own `testdata/` (`internal/ingest/rschunk/testdata/`) holds the **smaller** fixture for Rust unit tests of the sidecar binary itself (a handful of files exercising parser edge cases). The two fixture sets are deliberately separate: eval-scale corpus at project root, sidecar unit-test corpus next to the Rust code.

Considered and rejected: **external repo** (e.g. `serde`, `clap`). Brings a third-party codebase into the test loop; adds latency and external maintenance risk for marginal eval-realism gain.

---

## Architecture

```
go run ./cmd/ragcodepilot index <repo>
            │
            ▼
   internal/ingest/pipeline.go
            │
            ▼  (Rust files detected by .rs extension)
   internal/ingest/chunker_rust.go
            │
            ▼  os/exec: invoke rs-chunk binary
   internal/ingest/rschunk/target/release/rs-chunk   ◀── built via cargo build --release
            │
            ▼  reads file paths on stdin, emits JSON-lines on stdout
   for each .rs file:
     syn::parse_file(source)
     walk top-level: Fn, Struct, Enum, Impl (methods), Trait, Mod
     emit: {file, kind, name, start_line, end_line}  // boundaries only — no content
            │
            ▼
   Go side per file:
     1. Read file bytes (already on disk; the Go side knows the abs path).
     2. For each item record from the sidecar: slice content[start_line:end_line]
        and construct a model.CodeChunk with kind/name/lines/content.
     3. Compute gaps (lines NOT covered by any item record) and emit "block"
        chunks for non-empty gaps. Mirrors collectGapChunks in chunker_go.go.
        This captures use/const/static/type aliases/macro_rules! — code that
        lives at the top level but isn't a named item.
            │
            ▼
   existing pipeline: enrich → embed → upsert (unchanged)
```

**Key properties:**

- **Rust is index-time only.** Search doesn't depend on the Rust binary or toolchain.
- **One `rs-chunk` process per `ragcodepilot index` run** (not per file). `syn` is fast; startup ~5ms; cost is amortized across the whole corpus.
- **Stdin/stdout JSON-lines IPC.** No HTTP server, no socket setup. Trivial to test.
- **`syn` is the only parser dependency.** The crate also pulls in `serde` + `serde_json` for the JSON-lines output and `proc-macro2` (transitively, with `span-locations` for line numbers) — all small, all stable. No `proc-macro` invocation needed for chunking.
- **Boundaries-only protocol.** The sidecar emits line ranges; the Go side reads the actual file content and slices it. This keeps the sidecar simple and avoids carrying file bytes through the JSON channel.
- **Gap chunks mirror Go AST behavior.** The Go side computes the complement of named-item ranges and emits "block" chunks for those gaps (`use` declarations, `const`/`static`, `type` aliases, top-level `macro_rules!`, module-level docs). Without this, non-item Rust code would be silently dropped from the corpus.
- **Graceful fallback:** if `rs-chunk` binary is missing, the Rust chunker falls back to the regex chunker with a clear setup hint, instead of failing the index.

---

## Component Breakdown

### 1. Rust sidecar crate

**New files:**
- `internal/ingest/rschunk/Cargo.toml`
- `internal/ingest/rschunk/src/main.rs`
- `internal/ingest/rschunk/.gitignore` (ignores `target/`)

**`Cargo.toml` shape:**

```toml
[package]
name = "rs-chunk"
version = "0.1.0"
edition = "2021"

[dependencies]
syn = { version = "2", features = ["full", "extra-traits"] }
serde = { version = "1", features = ["derive"] }
serde_json = "1"
proc-macro2 = { version = "1", features = ["span-locations"] }
```

**Build-time verification:** with the `span-locations` feature enabled (default on `proc-macro2` 1.x), `Span::start()` and `Span::end()` return correct `LineColumn` values on stable Rust when used outside proc-macro context (our case). As the first cargo-build verification step, run `rs-chunk` against a sample file and confirm `start_line`/`end_line` are non-zero. If line numbers come back as 0 or 1 everywhere, check that `span-locations` is listed in the features for `proc-macro2` in `Cargo.lock` (not just `Cargo.toml`) and that `proc-macro2` is a reasonably recent version. Do **not** reach for `PROC_MACRO2_SEMVER_EXEMPT` — that env var gates unstable/private APIs (`Span::resolved_at`, etc.), not the line/column accessors we use.

**`src/main.rs` pseudocode:**

```
read file paths from stdin (one per line)
for each path:
    source = read_to_string(&path)
    match syn::parse_file(&source):
        Ok(file) → walk file.items:
            Item::Fn(f)     → emit {kind: "function", name: f.sig.ident, lines: f.span}
            Item::Struct(s) → emit {kind: "struct",   name: s.ident,    lines: s.span}
            Item::Enum(e)   → emit {kind: "enum",     name: e.ident,    lines: e.span}
            Item::Trait(t)  → emit {kind: "trait",    name: t.ident,    lines: t.span}
            Item::Impl(i)   → for method in i.items: emit {kind: "method", name: "{Type}::{method}", lines: method.span}
            Item::Mod(m)    → if inline, recurse into m.content
            (other variants: best-effort, skip with no emit)
        Err(e) → emit {file, error: e.to_string()}
```

**Output JSON-line schema:**

```json
{"file": "src/lib.rs", "kind": "function", "name": "parse_config", "start_line": 10, "end_line": 45}
{"file": "src/lib.rs", "kind": "method",   "name": "Config::load",  "start_line": 22, "end_line": 38}
{"file": "src/lib.rs", "kind": "struct",   "name": "Config",        "start_line": 5,  "end_line": 50}
```

Verify standalone before any Go wiring:

```bash
cd internal/ingest/rschunk
cargo build --release
# Smoke test: pipe a fixture path. The sidecar reads paths from stdin and
# opens them via read_to_string, so extension doesn't matter.
echo "testdata/smoke.rs.txt" | ./target/release/rs-chunk
# Expected: a JSON-line per top-level item, with non-zero start_line/end_line.
# If line numbers come back as 0 or 1, see the build-time verification note above.
```

**Why `.rs.txt`, not `.rs`?** `config.yaml`'s `skip_dirs` (line 27) does NOT exclude `testdata/`, so `ragcodepilot index --language go,rust .` would otherwise pick up `internal/ingest/rschunk/testdata/*.rs` and pollute the eval corpus with sidecar-fixture content. The language-detection layer keys off file extension, so `.rs.txt` files are invisible to the indexer. The sidecar itself doesn't care about extensions (it `read_to_string` whatever path you hand it on stdin), so Cargo tests can still use the fixtures.

Same convention applies to all sidecar unit-test fixtures: use `.rs.txt` (or any non-Rust extension) for any committed Rust source under `internal/ingest/rschunk/testdata/`. Cargo tests that need to invoke real Rust compilation can generate temp files at test time instead.

### 2. Go-side launcher

**New file:** `internal/ingest/chunker_rust.go`

```
function chunkRustFiles(
    absPaths: list of strings,
    repoRoot: string,        // for computing relative paths
    repo: string,            // for generateChunkID + CodeChunk.Repo
    chunkSize, overlap: int, // for splitLargeBlock / gap chunks
    cfg: Config,             // for DetectLanguage (and any future config-driven behavior)
) → (list of CodeChunk, Error)
  // Signature mirrors chunkGoFile(filePath, repoRoot, repo, chunkSize, overlap, cfg).
  // The only difference: this takes a slice of paths because the sidecar
  // amortizes one Rust process across many files.
  //
  // 1. Resolve binary path. Try in order:
  //    a. $RAGCODEPILOT_RS_CHUNK (env override)
  //    b. internal/ingest/rschunk/target/release/rs-chunk (module-relative)
  //    c. rs-chunk (on $PATH)
  // 2. If missing → return errRustBinaryMissing (sentinel; pipeline falls back)
  // 3. exec.Command(binPath); stdin = strings.Join(absPaths, "\n")
  // 4. Stream stdout line-by-line; decode each JSON line as itemRecord
  // 5. On non-zero exit OR malformed JSON OR impossible line spans → return
  //    a hard error (NOT errRustBinaryMissing). These should fail the index
  //    loud so we don't silently measure regex chunks under the AST banner.
  // 6. Per-file syn::parse_file errors (itemRecord with error field): log,
  //    then fall back to chunkGeneric for that file using chunkSize/overlap
  //    (mirrors chunker_go.go:31-33).
  // 7. For each file with successful items:
  //    a. relPath = filepath.Rel(repoRoot, absPath)
  //    b. language = cfg.DetectLanguage(absPath)  // "rust"
  //    c. Read file bytes.
  //    d. For each itemRecord: slice lines[start_line-1:end_line] and emit
  //       a model.CodeChunk with kind, name, content, lines.
  //       If item exceeds chunkSize lines, splitLargeBlock to keep chunk size sane.
  //    e. Compute gaps = lines NOT covered by any item record. Hand off to
  //       collectGapChunks(lines, covered, relPath, repo, language, chunkSize, overlap)
  //       — reuse the existing helper from chunker_go.go.
  // 8. All chunks use generateChunkID(repo, relPath, name, index) for IDs —
  //    same convention and stability properties as chunkGoFile.
```

The fail-loud paths in step 5 are deliberate: a corrupted sidecar output that silently degrades to regex would make the eval lie about what's being measured. Only the "binary not present at all" case falls back gracefully.

**Helper reuse.** `chunkRustFiles` should call into the same `collectGapChunks`, `buildGapChunks`, and `splitLargeBlock` helpers used by `chunkGoFile` (defined in `chunker_go.go`). These are not Go-specific despite their current location — they take `relPath`, `repo`, `language`, `chunkSize`, `overlap` and emit generic-shaped chunks. If keeping them in `chunker_go.go` feels wrong now that they're shared across languages, an optional cleanup is to move them to `chunker.go`. Out of scope for the strict Phase 3 gate; nice-to-have follow-up.

**New file:** `internal/ingest/chunker_rust_test.go`

- `t.Skip()` if `rs-chunk` binary is not findable.
- Test fixture: 5-10 small `.rs` files exercising:
  - Free functions (various signatures, async, generic)
  - Structs with impl blocks (methods named `Type::method`)
  - Enums and traits
  - Inline `mod` blocks
  - Syntax errors (chunker should report and continue, not abort)

### 3. Pipeline routing

**Modified files:** `internal/ingest/pipeline.go`, `internal/ingest/chunker.go`

**Routing pattern divergence.** The existing per-file dispatch in `ChunkFile` (`chunker.go`) was fine for Go because Go AST parsing is per-file and in-process. Rust uses a single batched sidecar call for performance (one Python-style startup amortized across N files), which doesn't fit the per-file dispatch shape. Two options for handling this:

- **Option A — Two-tier dispatch (chosen).** Pipeline-level pre-pass: collect all `.rs` paths, call `chunkRustFiles(rustPaths, repoRoot, repo, chunkSize, overlap, cfg)` once, get back a list of CodeChunk. Then proceed with the existing per-file loop for non-Rust files via `ChunkFile`. `ChunkFile` itself doesn't gain a Rust branch. The pipeline already has all the context (`repoRoot`, `repo`, chunk sizes, `cfg`) at the call site.
- **Option B — Per-file dispatch with internal batching.** `ChunkFile` accumulates Rust paths on first call, defers, batches all at end of pipeline. More implicit; worse for debugging.

Option A is chosen for explicitness. The divergence MUST be documented in both `pipeline.go` (the batching call site) and `chunker.go` (a comment explaining why Rust isn't handled here).

**Fallback policy — narrow, not blanket.**

- `errRustBinaryMissing` → log a one-line warning, fall through to regex for `.rs` files. Indexing continues.
- Per-file `syn::parse_file` errors (single file has invalid Rust syntax) → sidecar emits `{file, error}`. Go side logs the parse error and **falls back to the generic sliding-window chunker for that file**, mirroring `chunker_go.go`'s behavior on Go parse failure (`chunker_go.go:31-33`). Indexing continues. The file's content stays in the corpus, just chunked less precisely.
- Sidecar non-zero exit, malformed JSON, impossible line spans (start > end, end > line count) → hard error. Fail the index. Eval must not silently measure regex chunks under the AST banner.

```
warning: rs-chunk binary not found; falling back to regex chunker for Rust files.
To enable AST-based Rust chunking, run:
  (cd internal/ingest/rschunk && cargo build --release)
```

### 4. Rust eval fixture corpus

**New directory:** `testdata/fixtures/rust_repo/` *(at the project root, NOT inside the Rust crate's `testdata/`)*

This is the corpus that `ragcodepilot index --language go,rust .` will pick up and the corpus that golden queries target. The path matches what's specified in the Files table and in Decision 4 of §Resolved Design Decisions.

**Why the explicit `--language go,rust` filter?** `config.yaml` defines 18 language families including `markdown`, `yaml`, `toml`, `json`, and now `python` (we added a `.py` helper at `docs/eval/compare.py`). Bare `ragcodepilot index .` would pull all of those into the corpus — including the multi-thousand-line `docs/eval/baseline_v*.json` reports themselves, which would dominate the chunk count and break IDF in ways unrelated to chunker behavior. Phase 2 captured its baselines with `--language go`; Phase 3 expands to `--language go,rust` and that's the only language change. Every Phase 3 command uses this filter.

The Rust crate has its own `internal/ingest/rschunk/testdata/` directory for sidecar **parser** unit tests (small files exercising `syn` edge cases via `cargo test`). That's a separate, smaller corpus — see Component 1's file list. The two corpora exist for different purposes and should not be conflated:

| Corpus | Path | Purpose | Consumer |
|---|---|---|---|
| **Eval fixture** | `testdata/fixtures/rust_repo/` | Retrieval eval; ~20 files; realistic-shaped Rust code | `ragcodepilot index --language go,rust .` + `eval` |
| **Sidecar unit-test fixture** | `internal/ingest/rschunk/testdata/` | Parser edge cases; small files | `cargo test` inside the Rust crate |

Eval fixture content (~20 `.rs` files, 10-30 lines each):

- Free functions (various signatures)
- Structs + impl blocks
- Enums + match patterns
- Traits + impl-for blocks
- Async functions
- Generic functions with bounds
- A `mod.rs` and 2-3 subdirectory modules

**Update:** `docs/eval/golden.yaml` — add 5-8 Rust-targeted golden queries hitting symbols in the fixture repo:

- Navigation: "where is `parse_config` defined?"
- Concept: "how does the config loader work?"
- Behavior: "what happens when the config file is missing?"

Each Rust query MUST include `filters: { languages: ["rust"] }` (mirrors the `languages: ["go"]` pattern on existing Go queries). Without this, the search would run against the full mixed corpus and Go chunks could outrank the intended Rust targets — diluting the AST-vs-regex signal we're trying to measure. `--id-prefix=rust_` filters which **queries** the eval runs; `filters.languages` constrains which **chunks** each query searches. Both are needed.

Tag with `type:` matching the existing convention (`navigation` / `concept` / `behavior`).

**Note on corpus stability** (per the methodology added in Phase 2's follow-up): adding Rust files to the indexed corpus changes the chunk set. The Rust fixture adds ~40-60 chunks to the 350-chunk Go corpus. IDF shifts will be small but nonzero.

**Regression tolerance: ±2pp on Go-query `hit@5`.** Any movement larger than that triggers investigation before attributing the change to the chunker. Same-corpus comparison (Rust-AST vs Rust-regex, both with the mixed Go+Rust corpus) is the strict gate; v2-vs-v3 cross-corpus comparison is informational only.

### 5. Re-baseline + eval

The capture sequence is structured so that **all binary-state transitions happen at predictable points**, and **Rust-filtered eval artifacts are captured in the same binary state as their full counterparts**. The strict gate requires Rust-only AST-vs-regex JSONs, not aggregate comparisons.

Captures are linear and copy-paste safe: each block runs in the binary state described by its header. **Do not reorder.** Both Rust-filtered and Go-filtered captures happen WITHIN their respective binary-state phase.

```bash
# ════════════════════════════════════════════════════════════════════
# PHASE 1: BINARY PARKED — regex chunker for .rs
# ════════════════════════════════════════════════════════════════════
test -e internal/ingest/rschunk/target/release/rs-chunk && \
  mv internal/ingest/rschunk/target/release/rs-chunk{,.parked}
go run ./cmd/ragcodepilot collections delete code_chunks
go run ./cmd/ragcodepilot index --language go,rust .   # warns; uses regex for .rs

# 1a. Full eval, regex side → no-regression anchor + cross-corpus context
go run ./cmd/ragcodepilot eval --mode hybrid --output json \
    > docs/eval/baseline_v3_pre.json

# 1b. Rust-filtered eval, regex side → "before" half of the STRICT Rust gate
go run ./cmd/ragcodepilot eval --mode hybrid --id-prefix=rust_ --output json \
    > /tmp/rust_regex.json

# 1c. Go-filtered eval, regex side → "before" half of the Go regression gate
go run ./cmd/ragcodepilot eval --mode hybrid --exclude-id-prefix=rust_ --output json \
    > /tmp/go_regex.json

# ════════════════════════════════════════════════════════════════════
# BUILD BINARY, UNPARK
# ════════════════════════════════════════════════════════════════════
test -e internal/ingest/rschunk/target/release/rs-chunk.parked && \
  mv internal/ingest/rschunk/target/release/rs-chunk{.parked,}
(cd internal/ingest/rschunk && cargo build --release)

# ════════════════════════════════════════════════════════════════════
# PHASE 2: BINARY PRESENT — AST chunker for .rs
# ════════════════════════════════════════════════════════════════════
go run ./cmd/ragcodepilot collections delete code_chunks
go run ./cmd/ragcodepilot index --language go,rust .   # AST chunker for .rs

# 2a. Full eval, AST side → canonical v3 baseline + dense reference
go run ./cmd/ragcodepilot eval --mode hybrid --output json \
    > docs/eval/baseline_v3.json
go run ./cmd/ragcodepilot eval --mode dense  --output json \
    > docs/eval/baseline_v3_dense.json

# 2b. Rust-filtered eval, AST side → "after" half of the STRICT Rust gate
go run ./cmd/ragcodepilot eval --mode hybrid --id-prefix=rust_ --output json \
    > /tmp/rust_ast.json

# 2c. Go-filtered eval, AST side → "after" half of the Go regression gate
go run ./cmd/ragcodepilot eval --mode hybrid --exclude-id-prefix=rust_ --output json \
    > /tmp/go_ast.json

# ════════════════════════════════════════════════════════════════════
# COMPARISONS
# ════════════════════════════════════════════════════════════════════

# 3. STRICT Rust gate: AST vs regex, Rust queries only, same corpus.
#    Pass: hit@5 on Rust queries is strictly higher with AST than regex.
docs/eval/compare.py /tmp/rust_regex.json /tmp/rust_ast.json --labels=regex,ast

# 4. Go regression gate: AST vs regex, Go queries only, same corpus.
#    Pass: hit@5 on Go queries within ±2pp between regex and AST runs.
docs/eval/compare.py /tmp/go_regex.json /tmp/go_ast.json --labels=regex,ast

# 5. Cross-corpus context (informational, NOT a gate).
docs/eval/compare.py docs/eval/baseline_v2.json docs/eval/baseline_v3.json \
    --labels=v2_hybrid,v3_hybrid
```

**CLI flags added in this phase:**

- `--id-prefix=PREFIX` — keep only queries whose ID starts with `PREFIX`.
- `--exclude-id-prefix=PREFIX` — drop queries whose ID starts with `PREFIX`.

Both follow the existing `--type` flag pattern (`cmd/ragcodepilot/main.go:152, 360-368`). Composable: include filter is applied first, then exclude. Document in the README update.

**Why `/tmp/` artifacts instead of committed `baseline_v3_*.json`?** The Rust-filtered and Go-filtered runs are slices of the full baselines — derivable post-hoc if `compare.py` later gains `--id-prefix`/`--exclude-id-prefix`. Keeping them in `/tmp/` signals "intermediate computation." If reproducibility-by-commit becomes important, promote them to `docs/eval/` later.

### 6. Documentation

- **Update `docs/plan/mvp_roadmap.md`:** mark Phase 3's chunker sub-phase done with the Rust pick; reranker deferred with revisit trigger.
- **Update `docs/eval/README.md`:** note the new `baseline_v3*.json` files and the Rust corpus expansion.
- **Update root `README.md`:** add a "Building the Rust chunker (optional)" subsection under setup; document **both** `--id-prefix` and `--exclude-id-prefix` flags on the `eval` subcommand (both are needed to run the Phase 3 gates).
- **New file `docs/plan/chunker_rust.md`:** brief design doc covering the Rust sidecar architecture, the `syn` choice, binary lookup order, missing-binary fallback, and the `syn` version migration story (syn 1 → 2 was a breaking change; pin a major version).

---

## Implementation Order

```
1. Rust sidecar crate (syn-based) + Cargo build      (pure Rust, no Go side yet)
2. Go-side launcher (chunker_rust.go) + tests        (wires sidecar into pipeline-tested seams)
3. Pipeline routing + fallback                        (.rs files → Rust chunker, with regex fallback)
4. Embedded eval fixture (~20 .rs files)              (testdata for unit + retrieval eval)
5. Re-baseline + eval comparison                       (baseline_v3.json + apples-to-apples chunker delta)
6. Documentation                                       (mvp_roadmap, eval README, root README, design doc)
```

Step 1 can be developed and tested in isolation (no Go dependency). Steps 2-3 are tightly coupled; do them together. Step 5 is the validation gate.

---

## Files to Touch / Create

| File | Action | Purpose |
|---|---|---|
| `internal/ingest/rschunk/Cargo.toml` | NEW | Rust crate manifest |
| `internal/ingest/rschunk/Cargo.lock` | NEW (checked in) | Pin dependency versions for reproducible builds (this is a binary crate, not a library) |
| `internal/ingest/rschunk/src/main.rs` | NEW | Sidecar binary using `syn` |
| `internal/ingest/rschunk/.gitignore` | NEW | Ignore `target/`, keep `Cargo.lock` |
| `internal/ingest/rschunk/testdata/...` | NEW | Sidecar unit-test fixtures (parser edge cases) |
| `testdata/fixtures/rust_repo/...` | NEW | Eval fixture (~20 `.rs` files) at project root |
| `internal/ingest/chunker_rust.go` | NEW | Go-side launcher + content extraction + gap collection |
| `internal/ingest/chunker_rust_test.go` | NEW | Unit tests (skipped if binary missing) |
| `internal/ingest/chunker.go` | MODIFY | Comment in `ChunkFile` explaining why Rust isn't dispatched here |
| `internal/ingest/pipeline.go` | MODIFY | Pre-pass: collect `.rs` paths, batch through Rust chunker, fall back to regex on missing-binary |
| `cmd/ragcodepilot/main.go` | MODIFY | Add `--id-prefix` and `--exclude-id-prefix` flags to eval subcommand; honor both in `runEval` alongside the existing `typeFilter` logic (line 360-368). Include is applied before exclude. |
| `docs/eval/golden.yaml` | MODIFY | 5-8 new Rust-targeted queries with `rust_*` IDs |
| `docs/eval/baseline_v3.json` | NEW | Post-Phase-3 hybrid baseline (mixed corpus) |
| `docs/eval/baseline_v3_dense.json` | NEW | Post-Phase-3 dense reference (mixed corpus) |
| `docs/eval/baseline_v3_pre.json` | NEW | Pre-AST same-corpus baseline (regex chunker on `.rs`) — Go-query regression anchor |
| `docs/plan/chunker_rust.md` | NEW | Design doc for the sidecar approach |
| `docs/plan/mvp_roadmap.md` | MODIFY | Mark sub-phase done; rerank deferred |
| `README.md` | MODIFY | Optional setup note for the Rust chunker; document `--id-prefix` and `--exclude-id-prefix` eval flags |

---

## Out of Scope

- **Reranker** — explicitly deferred. See activation plan below.
- **Python chunker** — deferred. Choosing Rust over Python is a single-phase pick; revisit when there's user demand.
- **Tree-sitter** — deferred. Re-evaluate if a third non-Go language gets requested.
- **Cross-compiled embedded binary** — keep distribution simple: `cargo build` per machine.
- **Macro-expanded chunking** — `syn` parses source as-written, not macro-expanded. A `lazy_static!{ ... }` body isn't navigable. Acceptable trade.
- **External Rust crate eval** — fixture repo only; users can index real Rust crates themselves once the chunker ships.

---

## Reranker activation plan

The point of pre-committing this now is to skip re-litigating the choice later. When trigger metrics fire, the implementation is locked in.

**Architecture** — Python sidecar daemon, mirrors the Rust chunker's process-management style but with persistent lifetime (Python model load is expensive; Rust `syn` startup is negligible, hence the different choices).

```
ragcodepilot search/eval (Go parent process)
        │
        │ first rerank request → spawn daemon; subsequent → reuse
        ▼
   internal/rerank/reranker.go
        │
        ▼  long-lived stdin/stdout JSON-lines pipe
   internal/rerank/pyrerank/rerank.py  (sleeps on stdin between requests)
        │
        ▼  per request: reads {query, [chunks]} line, emits {scores: [...]} line
   sentence-transformers
        │
        ▼  cross-encoder/ms-marco-MiniLM-L6-v2  (~80MB; verify HF path at activation)
        ▼
   Go reads scores, sorts top-50 by rerank score, returns top-10
```

**Pre-committed implementation choices** (don't re-debate at activation):

- **Model:** starting candidate is `cross-encoder/ms-marco-MiniLM-L6-v2` (~80MB, CPU-friendly). Verify the canonical HuggingFace path at activation — sentence-transformers has used both `L6` and `L-6` forms historically. **Known limitation:** MS MARCO is trained on web search passages, not code. It may score natural-language comments higher than actual function definitions. If eval at activation shows poor code-relevance scoring, swap to `BAAI/bge-reranker-v2-m3` or a code-specific reranker. The eval at activation chooses the final model.

- **IPC: long-running daemon, stdin/stdout JSON-lines.** Not one-process-per-search. Reasoning: one-shot means each search pays ~2-3s for Python startup + model load + `import sentence_transformers`. On a 23-query eval that's ~69s of pure overhead before any scoring. A daemon is barely more complex than one-shot — the same JSON protocol, just kept alive across requests. The daemon scope is deliberately minimal: single child process, lifetime tied to the Go parent (terminate on parent exit or pipe close), no port, no socket, no health check. Restart on crash by spawning a new child for the next request.

- **Distribution:** `python3 -m pip install -r requirements.txt` as a one-time setup, alongside the Rust binary build.

- **Latency budget:** rerank adds ≤200ms warm to the search path. Cold-start (first request after spawn) may add 1-3s for model load; that's a one-time cost per `ragcodepilot` invocation. Real per-request and warm-path numbers must be measured locally at activation — don't rely on published benchmarks.

- **Toggle:** `--rerank` flag on `search` and `eval` subcommands, off by default. When the flag is on, rerank runs on every query for that invocation. The flag is a simple on/off; it does NOT consult golden-query tags (runtime search has no tags). Eval can independently filter by tag/prefix and run rerank on a subset that way.

- **Test strategy:** fake reranker (returns input order) for Go-side wiring tests; real reranker tested manually + via eval at activation.

**Files when activated:** `internal/rerank/reranker.go`, `internal/rerank/pyrerank/rerank.py`, `internal/rerank/pyrerank/requirements.txt`, `internal/rerank/reranker_test.go`, `docs/plan/reranking.md` (with the eval delta numbers when known).

**Activation triggers** (any one of):

- `hit@1` drops below 0.55 on a future phase baseline.
- `MRR@5` regresses below 0.60.
- `--answer` mode (Phase 5) needs precision in top-3 results.

If hybrid `hit@1` stays ≥0.55 and `MRR@5` ≥0.60 through Phase 4-5, reranker stays deferred indefinitely.

---

## Verification

Local-only verification, no CI integration yet.

**Unit / build checks** (independent of eval):

```bash
# Rust crate
(cd internal/ingest/rschunk && cargo build --release)
(cd internal/ingest/rschunk && cargo test --release)  # if any tests

# Go
go test ./internal/ingest/... -v -race -count=1
go test ./... -count=1 -race
go build ./...

# Missing-binary fallback (Go-side; exercises sentinel error path)
mv internal/ingest/rschunk/target/release/rs-chunk{,.disabled}
go run ./cmd/ragcodepilot index --language go,rust .  # should warn, not fail
mv internal/ingest/rschunk/target/release/rs-chunk{.disabled,}
```

**End-to-end eval gates: run Step 5 in `## Component Breakdown` → `### 5. Re-baseline + eval` (above).** Don't duplicate the command sequence here — Step 5 is the single source of truth for the pre-baseline → AST → Rust-filtered → Go-filtered capture order. The exit criteria for this phase are exactly the three comparisons listed there (STRICT Rust gate, Go regression gate, cross-corpus context).

---

## Why this scope

The original Phase 3 (rerank + chunker) doubled surface area for marginal `hit@5` ceiling left. Deferring rerank lets Phase 3 ship with one clean architectural commitment (Rust sidecar) that proves the pattern without locking in tree-sitter+CGo for the rest of the project. The Rust pick also seeds Phase C: navigating a Rust vector-DB reference codebase is much easier with AST-aware chunking already in place.

Reranker isn't dead — it's parked behind real revisit triggers, with the recognition that hybrid mode delivered more than expected and the rerank ROI is now less clear than it was when the MVP roadmap was written.
