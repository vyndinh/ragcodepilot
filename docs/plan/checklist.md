# Progress checklist — ragcodepilot

Tracking progress against [system_design.md](system_design.md) build phases.

Last updated: 2026-05-08

## Phase 1: Minimal semantic search ✅

### Project setup
- [x] Initialize Go module (`go mod init`)
- [x] Create CLI entry point (`cmd/ragcodepilot/main.go`)
- [x] Set up project directory structure (`internal/` packages)
- [x] Create Docker Compose for Qdrant (`docker-compose.yml`)

### Data model
- [x] Define `CodeChunk` and `SearchResult` types with JSON tags (`internal/model/chunk.go`)

### Ingestion pipeline
- [x] File walker — recursively find source files, skip non-source dirs (`internal/ingest/walker.go`)
- [x] Language detection by file extension (`internal/config/config.go`)
- [x] Line-based text chunker with overlap (`internal/ingest/chunker.go`)
- [x] Pipeline orchestrator — walk → chunk → enrich → embed → upsert (`internal/ingest/pipeline.go`)
- [x] Connect embedder to pipeline — embed chunks before upsert
- [x] Upsert embedded chunks to Qdrant (batched, 32 per batch)

### Embedding
- [x] Define `Embedder` interface (`internal/embedding/embedder.go`)
- [x] Implement fake embedder for pipeline testing (`internal/embedding/fake.go`)
- [x] Implement Ollama embedder with `nomic-embed-text` model (`internal/embedding/ollama.go`)
- [x] Embedding dimension auto-detection from first model response
- [x] `ValidateVectorBatch` helper for dimension consistency checks (`internal/embedding/validate.go`)
- [x] Collection dimension mismatch produces clear error with fix instructions
- [x] Search-time query vector dimension validation against collection

### Qdrant
- [x] Qdrant client — EnsureCollection, Upsert, Search, List, Delete (`internal/qdrant/client.go`)
- [x] Batch upsert (64 points per gRPC call)
- [x] Start Qdrant via Docker and verify end-to-end

### Search
- [x] Search service — embed query → Qdrant search → format results (`internal/search/searcher.go`)
- [x] Terminal result formatting with scores and metadata
- [x] CLI `search` command wired to real search flow

### Configuration
- [x] Externalize language/extension mappings to `config.yaml`
- [x] Config loader with YAML support + built-in defaults (`internal/config/config.go`)
- [x] CLI index command auto-loads `config.yaml` when present, falls back to built-in defaults

### Testing (Phase 1)
- [x] Unit tests for index config resolution: missing, valid, invalid, unreadable `config.yaml`
- [x] Unit tests for index language filtering
- [x] Unit tests for vector batch validation (9 table-driven test cases)
- [x] Unit tests for chunk enrichment (5 test cases + chunkTypeLabel)
- [x] All tests pass with `-race` flag

### Code quality (Go skills review)
- [x] Functional options pattern on `Pipeline` (`WithChunkSize`, `WithChunkOverlap`)
- [x] Unexported struct fields — control API surface
- [x] JSON struct field tags on serialized types
- [x] Error return from subcommands instead of `os.Exit` (single handling rule)
- [x] Preallocated slices and maps
- [x] `filepath.Ext` instead of custom extension parser
- [x] O(1) skip-dir lookup via `map[string]struct{}`
- [x] Build-time version injection via `-ldflags`
- [x] `sdkClient` interface for Qdrant client testability (`internal/qdrant/client.go`)
- [x] `vectorStore` interface for pipeline testability (`internal/ingest/pipeline.go`)

### Search quality
- [x] Chunk enrichment: prepend file path, language, and chunk type/name to embedding input (`internal/ingest/enrichment.go`)
- [x] Embedding dimension auto-detection and validation (`docs/plan/embedding_dimension_validation.md`)
- [x] Verified: enrichment improves search relevance for natural-language queries

### Documentation
- [x] System design document (`docs/plan/system_design.md`)
- [x] Plan comparison: old vs new (`docs/plan/plan_comparison.md`)
- [x] Embeddings explained (`docs/knowledge/embeddings_explained.md`)
- [x] RAG architecture parts (`docs/knowledge/rag_parts.md`)
- [x] Platform comparison (`docs/knowledge/compare.md`)
- [x] Root README with system overview, startup guide, build command, CLI examples, config behavior, and limitations (`README.md`)

---

## Phase 2: Filtering and better parsing (in progress)

### Filtering ✅
- [x] Add `--language` flag to search command
- [x] Add language payload filtering in Qdrant search requests
- [x] Apply `--language` filtering during index
- [x] Add `--repo` flag to search command
- [x] Add repo payload filtering in Qdrant search requests (AND/OR composition)

### Function-level chunking ✅
- [x] Go AST-based function chunker (`internal/ingest/chunker_go.go`)
- [x] ChunkFile router: Go → AST, others → sliding window
- [x] Gap chunks for non-function code (imports, types, vars)
- [x] Large function splitting (>80 lines → sliding window fallback)
- [x] Syntax error graceful fallback to generic chunker
- [x] Chunker design doc (`docs/plan/function_level_chunker.md`)

### Testing (Phase 2) ✅
- [x] 8 Go AST chunker tests (function, method, block, empty, syntax error, large split, sort, metadata)
- [x] 2 router tests (`ChunkFile` routes Go → AST, Python → generic)
- [x] 5 search filter tests (no filter, language-only, repo-only, both, result mapping)
- [x] All 41 tests pass with `-race`

### Documentation (Phase 2) ✅
- [x] Updated README: Ollama setup, `--repo` examples, CLI flag tables, re-indexing guide
- [x] Updated system_design.md: enrichment step, Ollama model, project structure
- [x] Feedback analysis (`docs/review_feedback/feedback_analysis.md`)

### Remaining (Phase 2)
- [x] Add Qdrant payload indexes for `repo`, `language`, `file_path` (filtered search performance)
- [ ] Fix point ID strategy: `repo+file+symbol+chunk_index` instead of `repo+file+start_line`
- [ ] Re-indexing: file-hash change detection + delete stale points by filter
- [ ] Regex heuristic chunkers for Python/Rust (optional, see `docs/plan/function_level_chunker.md`)

---

## Phase 3: Hybrid search

- [ ] Simple eval harness (`ragcodepilot eval` with `hit@k`, `MRR@5`) — build before hybrid search
- [ ] Add sparse vectors for BM25 keyword matching
- [ ] Implement hybrid search with RRF score fusion
- [ ] Add exact function name search
- [ ] Add observability hooks (embedding/Qdrant latency timing)

---

## Phase 4: Polish and learn internals

- [x] Collection management CLI commands (list, delete)
- [x] Search result formatting (code with line numbers and context)
- [ ] Study Qdrant internals (map to `Vector_DB_core.md`)

---

## Test coverage summary

| Package | Tests | Coverage area |
|---------|-------|---------------|
| `cmd/ragcodepilot` | 4 | Config resolution |
| `internal/embedding` | 9 | Vector batch validation |
| `internal/ingest` | 18 | Pipeline, chunker (Go AST + router), enrichment, language filter |
| `internal/qdrant` | 10 | Collection CRUD, search filters (4 combos), payload mapping |
| **Total** | **41** | All pass with `-race` |
