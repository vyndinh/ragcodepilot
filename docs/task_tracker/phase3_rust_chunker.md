# Phase 3 — Rust AST Chunker: Task Tracker

> ## ⏸ Status: Deferred (2026-05-14)
>
> Phase 3 is parked pending Phase 5 v0 dogfooding. Do not start the checklist below. See the deferral banner in `docs/plan/phase3_rust_chunker.md` for revival triggers, and `docs/plan/mvp_roadmap.md` for the current phase sequence.
>
> The tracker is preserved as-is so it can be picked up unchanged when Phase 3 is un-deferred.

---

Working tracker for the plan at [`docs/plan/phase3_rust_chunker.md`](../plan/phase3_rust_chunker.md).
Update inline as work progresses. Each task lists the section of the plan it implements.

---

## Status legend

- `[ ]` pending
- `[~]` in progress
- `[x]` done
- `[!]` blocked (note the blocker)
- `[-]` skipped/superseded (note why)

## Progress at a glance

| Step | Component | Status | Notes |
|---|---|---|---|
| 1 | Rust sidecar crate | [ ] | |
| 2 | Go-side launcher | [ ] | depends on step 1 |
| 3 | Pipeline routing + narrow fallback | [ ] | depends on step 2 |
| 4 | Eval fixture corpus + golden queries | [ ] | independent of steps 1-3 |
| 5 | Eval CLI: `--id-prefix` + `--exclude-id-prefix` flags | [ ] | required for step 6 comparisons |
| 6 | Re-baseline + eval gate | [ ] | depends on steps 1-5 |
| 7 | Documentation | [ ] | finalize after step 6 confirms exit criteria |

Exit-criteria check: `[ ]` (will mark when all four pass — see end of doc).

---

## Step 1 — Rust sidecar crate

Plan §Component Breakdown #1.

- [ ] Create directory `internal/ingest/rschunk/`
- [ ] Write `Cargo.toml` with deps: `syn 2 [full, extra-traits]`, `serde 1 [derive]`, `serde_json 1`, `proc-macro2 1 [span-locations]`
- [ ] Write `src/main.rs` — read file paths on stdin, parse with `syn::parse_file`, walk top-level items (Fn, Struct, Enum, Trait, Impl methods, inline Mod), emit boundaries-only JSON-lines on stdout
- [ ] Emit per-file parse errors as `{file, error}` JSON-lines and continue (do not abort the batch)
- [ ] Add `.gitignore` for `target/`; keep `Cargo.lock`
- [ ] Create `testdata/smoke.rs.txt` (single file, mixed items) for build-time smoke check. **Use `.rs.txt` extension**, not `.rs`, so `ragcodepilot index --language go,rust .` doesn't pick it up — `config.yaml` skip_dirs (line 27) does not exclude `testdata/`. The sidecar reads paths from stdin and doesn't care about extension.
- [ ] Run `cargo build --release` — confirm clean build on stable Rust
- [ ] Run smoke test (`echo testdata/smoke.rs.txt | ./target/release/rs-chunk`) and verify `start_line`/`end_line` are non-zero (else: fix the `span-locations` build per plan notes)
- [ ] Commit `Cargo.lock`

**Done when:** `rs-chunk` binary exists, smoke test passes, JSON output validates.

---

## Step 2 — Go-side launcher

Plan §Component Breakdown #2.

### chunker_rust.go

- [ ] Create `internal/ingest/chunker_rust.go`
- [ ] Define `errRustBinaryMissing` sentinel
- [ ] Implement binary path resolution: `$RAGCODEPILOT_RS_CHUNK` → repo-relative `internal/ingest/rschunk/target/release/rs-chunk` → `$PATH`
- [ ] Implement `chunkRustFiles(absPaths: list of strings, repoRoot, repo: string, chunkSize, overlap: int, cfg: Config) → (list of CodeChunk, Error)`. Signature mirrors `chunkGoFile` (which takes the same `repoRoot, repo, chunkSize, overlap, cfg` set) — only difference is the batched list of paths instead of a single `filePath`. The function needs these to compute `relPath`, generate stable IDs, detect language, split oversized items, and build gap chunks. Pipeline call site already has them all.
  - [ ] If binary missing → return `errRustBinaryMissing`
  - [ ] `exec.Command` with stdin = joined paths
  - [ ] Stream stdout, decode JSON-lines
  - [ ] On non-zero exit OR malformed JSON OR impossible line spans (start > end, end > line count) → return hard error (NOT `errRustBinaryMissing`)
  - [ ] **Per-file `syn::parse_file` errors** (sidecar emits `{file, error}` JSON-line): log the parse error and **fall back to `chunkGeneric` for that single file** — mirrors `chunker_go.go:31-33` (Go's `parser.ParseFile` syntax-error fallback). Don't skip the file; broken Rust still has searchable content.
- [ ] Per-file content extraction: read file bytes, slice `content[start_line-1:end_line]`, populate `model.CodeChunk.Content`
- [ ] **Reuse helpers from `chunker_go.go`**: call `splitLargeBlock` for items exceeding `chunkSize`, and `collectGapChunks(lines, covered, relPath, repo, language, chunkSize, overlap)` for non-item ranges. These helpers are not Go-specific; they take the same parameters. Optional follow-up: move them to `chunker.go` if their cross-language reuse feels mis-located (NOT a Phase 3 gate).
- [ ] Use existing `generateChunkID(repo, relPath, name, index)` for IDs (same convention as `chunkGoFile`)

### chunker_rust_test.go

- [ ] Create `internal/ingest/chunker_rust_test.go`
- [ ] `t.Skip()` if binary not findable
- [ ] Test cases:
  - [ ] Free functions (sync, async, generic)
  - [ ] Struct with impl — methods named `Type::method`
  - [ ] Enums and traits emit as separate chunks
  - [ ] Inline `mod` blocks recurse correctly
  - [ ] Syntax errors in one file: that file falls back to generic chunker (per `chunker_go.go:31-33` precedent); batch continues for the rest
  - [ ] Gap chunks captured for `use` declarations, top-level `const`/`static`
  - [ ] Protocol-violation cases (mock stdout with malformed JSON, mock non-zero exit) → hard error, NOT silent fallback

**Done when:** all tests pass, fallback policy matches plan §3.

---

## Step 3 — Pipeline routing + narrow fallback

Plan §Component Breakdown #3.

- [ ] Modify `internal/ingest/pipeline.go`:
  - [ ] Pre-pass: collect all `.rs` file paths from the walk result
  - [ ] If any: call `chunkRustFiles(allRustPaths, repoRoot, repo, chunkSize, overlap, cfg)` once, get a list of CodeChunk. All six args are available at the pipeline call site.
  - [ ] On `errRustBinaryMissing`: log warning (with `cargo build` setup hint), fall through to existing regex/sliding-window chunker for those paths
  - [ ] On hard error from `chunkRustFiles`: propagate as index failure (do NOT silently fall back)
- [ ] Modify `internal/ingest/chunker.go` (`ChunkFile`): add a comment explaining why Rust isn't dispatched in this per-file router (it's batched at pipeline level for sidecar amortization)
- [ ] Manual fallback verification (rename binary to `.disabled`, run `ragcodepilot index --language go,rust .`, confirm warning + successful index using regex for `.rs` files)

**Done when:** `.rs` files are AST-chunked when binary is present, regex-chunked with a clear warning when missing, hard-failed when sidecar misbehaves.

---

## Step 4 — Eval fixture corpus + golden queries

Plan §Component Breakdown #4.

### Fixture corpus

- [ ] Create directory `testdata/fixtures/rust_repo/` at project root
- [ ] Add ~20 small `.rs` files covering:
  - [ ] Free functions (varied signatures)
  - [ ] Structs + impl blocks (some with multiple methods)
  - [ ] Enums + match patterns
  - [ ] Traits + impl-for blocks
  - [ ] Async functions
  - [ ] Generic functions with bounds
  - [ ] `mod.rs` + 2-3 subdirectory modules
- [ ] Verify the walker picks up the fixture when running `ragcodepilot index --language go,rust .` from project root (no `skip_dirs` collision)

### Golden queries

- [ ] Update `docs/eval/golden.yaml` — add 5-8 Rust-targeted queries
- [ ] All Rust query IDs use `rust_` prefix (for `--id-prefix` filtering in step 6)
- [ ] **Each Rust query includes `filters: { languages: ["rust"] }`.** This constrains the search itself to Rust chunks only. `--id-prefix=rust_` controls which queries run; `filters.languages` controls which chunks each query searches. Without the per-query filter, Go chunks would compete for top-K and dilute the AST-vs-regex measurement.
- [ ] Cover all three types: navigation, concept, behavior
- [ ] Each query has at least one `expected.files` entry from the fixture and at least one `expected.symbols` entry where applicable
- [ ] Verify queries are answerable (manual sanity check: would I expect the search to find this?)

**Done when:** fixture indexed cleanly + golden queries committed.

---

## Step 5 — Eval CLI extension: `--id-prefix` and `--exclude-id-prefix` flags

Plan §Component Breakdown #5 (eval comparison) + Files table.

Two complementary flags:
- `--id-prefix=PREFIX` — keep only queries whose ID starts with `PREFIX` (used to isolate Rust-only runs for the STRICT gate)
- `--exclude-id-prefix=PREFIX` — drop queries whose ID starts with `PREFIX` (used to isolate Go-only runs for the regression gate, since Go queries don't share a common prefix)

- [ ] Add `--id-prefix` flag to `eval` subcommand in `cmd/ragcodepilot/main.go` (alongside the existing `--type` flag, line 152)
- [ ] Add `--exclude-id-prefix` flag to `eval` subcommand
- [ ] Honor both in `runEval` (`main.go:360-368`, where `typeFilter` is already applied). Filter happens after `LoadDataset`, before `runner.Run`. Order: include filter first, then exclude. Composable. No changes needed in `internal/eval/runner.go`.
- [ ] Verify `--id-prefix=rust_` returns only Rust queries
- [ ] Verify `--exclude-id-prefix=rust_` returns only non-Rust queries (i.e., Go queries)
- [ ] Verify both flags empty (default) preserves existing behavior
- [ ] Verify both flags set together: include applied first, then exclude

**Done when:** `ragcodepilot eval --id-prefix=rust_` runs Rust-only AND `--exclude-id-prefix=rust_` runs Go-only.

---

## Step 6 — Re-baseline + eval gate

Plan §Component Breakdown #5 + §Exit criteria.

### Baseline captures (all against the mixed Go+Rust corpus)

Capture in this order. Binary state transitions are explicit.

**Phase 1: binary parked (regex chunker for `.rs`)**

- [ ] Park binary: `mv internal/ingest/rschunk/target/release/rs-chunk{,.parked}` (if it exists)
- [ ] Re-index: `ragcodepilot collections delete code_chunks && ragcodepilot index --language go,rust .` (warns; regex used for `.rs`)
- [ ] Full eval, regex side → `docs/eval/baseline_v3_pre.json` (`--mode hybrid`)
- [ ] **Rust-filtered eval, regex side → `/tmp/rust_regex.json`** (`--mode hybrid --id-prefix=rust_`) — "before" half of the STRICT Rust gate
- [ ] **Go-filtered eval, regex side → `/tmp/go_regex.json`** (`--mode hybrid --exclude-id-prefix=rust_`) — "before" half of the Go regression gate

**Phase 2: binary built & present (AST chunker for `.rs`)**

- [ ] Unpark + build: restore `rs-chunk` from `.parked`, run `(cd internal/ingest/rschunk && cargo build --release)`
- [ ] Re-index: `ragcodepilot collections delete code_chunks && ragcodepilot index --language go,rust .` (AST used for `.rs`)
- [ ] Full eval, AST side → `docs/eval/baseline_v3.json` (`--mode hybrid`)
- [ ] Dense reference → `docs/eval/baseline_v3_dense.json` (`--mode dense`)
- [ ] **Rust-filtered eval, AST side → `/tmp/rust_ast.json`** (`--mode hybrid --id-prefix=rust_`) — "after" half of the STRICT Rust gate
- [ ] **Go-filtered eval, AST side → `/tmp/go_ast.json`** (`--mode hybrid --exclude-id-prefix=rust_`) — "after" half of the Go regression gate

### Comparisons

- [ ] **STRICT EXIT GATE — AST vs regex on Rust queries** (same corpus, same query subset):
  ```
  docs/eval/compare.py /tmp/rust_regex.json /tmp/rust_ast.json --labels=regex,ast
  ```
  Pass: `hit@5` on Rust queries is strictly higher with AST than regex.
- [ ] **Go regression gate — AST vs regex on Go queries** (same corpus, same query subset, ±2pp tolerance on hit@5):
  ```
  docs/eval/compare.py /tmp/go_regex.json /tmp/go_ast.json --labels=regex,ast
  ```
  Pass: `hit@5` on Go queries within ±2pp between regex and AST runs. This is a real programmatic gate now that `--exclude-id-prefix` exists.
- [ ] **Cross-corpus context** (informational only):
  ```
  docs/eval/compare.py docs/eval/baseline_v2.json docs/eval/baseline_v3.json
  ```

### Test suite

- [ ] `go test ./... -count=1 -race` — green
- [ ] `(cd internal/ingest/rschunk && cargo test --release)` — green (if any Rust tests exist)

**Done when:** all four exit criteria from the plan pass.

---

## Step 7 — Documentation

Plan §Component Breakdown #6.

- [ ] Create `docs/plan/chunker_rust.md` — design doc (sidecar architecture, `syn` choice, binary lookup order, fallback policy, `syn 1→2` migration story)
- [ ] Update `docs/plan/mvp_roadmap.md` — mark Phase 3 chunker sub-phase done; flag reranker as deferred with revisit trigger
- [ ] Update `docs/eval/README.md` — note `baseline_v3*.json` files, mixed-corpus implication, both `--id-prefix` and `--exclude-id-prefix` flags
- [ ] Update root `README.md` — add "Building the Rust chunker (optional)" section under setup; document both `--id-prefix` and `--exclude-id-prefix` eval flags (both are needed to run the Phase 3 gates)

**Done when:** docs reflect what shipped; future contributor can find their way around.

---

## Exit criteria gate

From the plan's Exit criteria section. Mark `[x]` only when measured, not when "looks fine":

- [ ] `hit@5` on Rust-targeted golden queries: AST > regex fallback (apples-to-apples, same mixed corpus). Numbers committed in plan doc.
- [ ] Go-query `hit@5` regression: ≤ 2pp on the AST-vs-regex same-corpus comparison via `/tmp/go_regex.json` vs `/tmp/go_ast.json` (both captured with `--exclude-id-prefix=rust_`).
- [ ] Missing-binary fallback: indexing produces clear warning, not a hard failure (manually verified once).
- [ ] Full test suite passes with `-race`.

All four checked → Phase 3 done. Update `docs/plan/mvp_roadmap.md` and close out.

---

## Blockers / open questions

(Append as they arise.)

- _none yet_

---

## Notes during implementation

(Append as work happens — anything that didn't go as planned, decisions made on the fly, surprises worth remembering for Phase 4+.)

- _none yet_
