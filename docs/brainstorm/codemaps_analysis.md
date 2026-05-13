# Codemaps Analysis: What It Is & How It Relates to Our System

Reference document comparing Windsurf's Codemaps feature with ragcodepilot's architecture.
Written to inform future roadmap decisions around structural code understanding.

---

## What is Codemaps?

Codemaps is a feature in Windsurf (by Cognition AI, released Nov 2025) that generates **AI-powered, interactive architectural diagrams** of a codebase. It solves one core problem: helping developers build an accurate mental model of how code is organized and how components relate to each other.

### How it works

1. **Just-in-time analysis.** When a developer asks "how does the auth system work?", the AI agent performs a targeted scan of the relevant parts of the codebase — not a pre-built static index.

2. **Multi-signal indexing.** Codemaps combines:
   - AST parsing and symbol-level indexing
   - Dependency and import analysis
   - Call graph traversal (which function calls which)
   - Semantic understanding via embeddings

3. **Structured output.** The analysis produces:
   - **Visual graphs** — interactive diagrams where nodes represent modules/functions/files and edges represent relationships. Clicking a node jumps to the source code.
   - **Trace guides** — narrative textual explanations of how data flows through the system and why code is grouped together.

4. **Agentic integration.** Generated Codemaps can be referenced in prompts via `@codemap`, giving the AI agent high-level architectural context for subsequent tasks. This reduces hallucinations and improves code generation relevance.

### Key design decisions

- **Generated on demand**, not pre-computed. Each Codemap is tailored to a specific question or task.
- **Combines structure + semantics.** Not just "find similar code" (vector search) or "find exact symbol" (grep/ctags). It understands both similarity *and* relationships.
- **IDE-integrated.** Deeply tied to Windsurf's editor — click-to-navigate, visual rendering, inline agent collaboration.
- **LLM-powered synthesis.** Uses models like SWE-1.5 and Claude Sonnet 4.5 to generate the narrative explanations.

---

## Is Codemaps a RAG system?

**Yes, but a sophisticated one.** It goes well beyond the standard "embed → retrieve → generate" pipeline:

| Component | Standard RAG | Codemaps | ragcodepilot (current) |
|---|---|---|---|
| **Indexing** | Chunk text → embed → store vectors | AST parsing + symbol indexing + dependency analysis + semantic embedding | Chunk code → embed → store vectors |
| **Retrieval** | Nearest-neighbor vector search | Multi-step: symbol lookup + dependency graph traversal + semantic search | Vector-only cosine similarity |
| **Context assembly** | Top-K chunks concatenated | Structured graph of relationships + relevant code + architectural context | Top-K chunks returned as-is |
| **Generation** | LLM generates answer from chunks | LLM generates visual maps + narrative explanations from structured context | None (retrieval only) |
| **Output** | Text answer | Interactive diagram + trace guides + source links | Ranked code snippets |

The critical difference: Codemaps understands **structure** (what calls what, what depends on what), not just **similarity** (what text looks like what).

---

## What ragcodepilot does today

Our system covers the **semantic similarity** axis well:

- File walker → language detection → chunker (Go AST for Go, sliding window for others)
- Enrichment (prepend metadata: language, file path, function name, package)
- Ollama embedding (nomic-embed-text, 768d vectors)
- Qdrant vector storage with filtered retrieval (by language, by repo)
- Re-indexing with file-hash change detection

What it **does not do**:

- No structural understanding (which function calls which)
- No dependency/import graph
- No file or package overview mode
- No traversal across call chains
- No LLM-powered synthesis or explanation

These are exactly the gaps identified in `docs/review_feedback/system_vision_review.md` (Q3, Q5).

---

## What ideas are worth borrowing

### Practical for a CLI tool (worth building)

| Idea | Description | Effort | Roadmap fit |
|---|---|---|---|
| **`explore` command** | Interactive hierarchical exploration — shows numbered parts, drill-down into each for structural detail | M-L | Phase 4-5 |
| **Call graph extraction** | Walk Go AST `CallExpr` nodes to map which functions call which. Store as payload metadata alongside chunks in Qdrant | M | Phase 3 (extends existing Go AST chunker) |
| **Result grouping by structure** | Search results grouped by call chain instead of flat cosine-similarity ranking | M | Phase 4 (UX polish) |
| **Structural context in `--answer` mode** | When generating LLM answers, include call relationships in the prompt, not just raw code | S | Phase 5 |

### Not practical right now (defer)

| Idea | Why not |
|---|---|
| **IDE integration** | Scope creep. The vision review explicitly lists "IDE plugin" as "avoid building too early." |
| **Just-in-time LLM analysis** | Requires an always-running LLM. Conflicts with offline/lightweight identity. |
| **Multi-model agent orchestration** | Complexity explosion. Codemaps uses SWE-1.5 + Claude Sonnet 4.5 — we use one local model. |

---

## Explore Mode — Codemaps-inspired hierarchical exploration

The core idea borrowed from Codemaps: instead of returning flat ranked chunks, present the codebase as **numbered logical parts** that the user can drill into. Each part represents a coherent step in a flow, and expanding it reveals the structural detail — files, functions, call chains, and code.

### How it would work

**Step 1: User explores a topic**

```
$ ragcodepilot explore "Indexing Pipeline"

 Explore: Indexing Pipeline
 CLI command parsing, embedder resolution, Qdrant client initialization, pipeline execution

 1. Index Command: CLI Entry to Pipeline Initialization
 2. Ingestion Pipeline: File Walking, Chunking, and Change Detection
 3. Go AST-Based Function Chunking
 4. Embedding & Upsert: Enrichment to Vector Storage
 5. Search Command: CLI Entry to Searcher Initialization
 6. Search Pipeline: Query Embedding to Vector Similarity Search

 [1-6] View details  [q] Quit
```

The system analyzes the codebase structurally (AST + call graph + semantic search) and groups results into logical parts. Each part has a clear title and represents a coherent step in the overall flow.

**Step 2: User drills into a part**

```
 > 1

 ━━ 1. Index Command: CLI Entry to Pipeline Initialization ━━

 CLI Entry Point: main.go
 └── main()
     └── switch os.Args[1]
         ├── 1a. Index command routing
         │       case "index": → parse flags (collection, language, embedder, ollama-url, ollama-model)
         │
         ├── 1b. Embedder resolution
         │       resolveEmbedder(*embedderType, *ollamaURL, *ollamaModel)
         │       └── switch embedderType
         │           ├── "ollama" → NewOllamaEmbedder(ollamaURL, ollamaModel)
         │           └── "fake"   → NewFakeEmbedder(384)
         │
         └── 1c. Execute index operation
                 runIndex(repoPath, *collection, languages, *qdrantHost, *qdrantPort, emb)
                 ├── resolveIndexConfig()      → loads config.yaml or uses defaults
                 ├── qdrant.NewClient(host, port) → gRPC connection to Qdrant
                 └── ingest.NewPipeline(cfg, embedder, client, collection, opts...)
                     └── pipeline.Run(ctx, repoPath)  → see Part 2

 [n] Next part  [p] Previous  [1-6] Jump  [q] Quit
```

**Step 3: Continue drilling**

```
 > 2

 ━━ 2. Ingestion Pipeline: File Walking, Chunking, and Change Detection ━━

 Pipeline.Run(ctx, repoPath)
 ├── 2a. Repository identification
 │       repoName := filepath.Base(repoPath)  → extracts "ragcodepilot" from path
 │
 ├── 2b. Collection setup
 │       EnsureCollection(ctx, collection, dimension)
 │       └── creates Qdrant collection if not exists
 │       EnsurePayloadIndexes(ctx, collection)
 │       └── creates indexes on: file_path, language, repo, chunk_hash
 │
 ├── 2c. Change detection
 │       ListPointsByRepo(ctx, collection, repoName)
 │       └── fetches all existing points for this repo
 │       Hasher.HashFile(path)
 │       └── SHA256 of file contents → compare with stored hash
 │       Categorize: unchanged / modified / new / deleted
 │
 ├── 2d. File walking
 │       Walker.Walk(repoPath, languages)
 │       ├── filepath.WalkDir → recursive traversal
 │       ├── config.ShouldSkip(dir) → skip node_modules, .git, vendor, etc.
 │       └── config.HasLanguage(ext) → filter by file extension
 │
 └── 2e. Per-file processing
         For each modified/new file:
         ├── Chunker.ChunkFile(path, content)  → see Part 3
         ├── Enricher.EnrichChunk(chunk)        → see Part 4
         ├── Embedder.Embed(ctx, texts)         → see Part 4
         └── Client.BatchUpsert(ctx, ...)       → see Part 4

 [n] Next part  [p] Previous  [1-6] Jump  [q] Quit
```

### What makes this different from flat search results

| Flat search results (current) | Explore mode (proposed) |
|---|---|
| 5 disconnected chunks sorted by cosine similarity | Numbered logical parts organized by data flow |
| No relationship between results | Parts reference each other ("→ see Part 2") |
| User reads code snippets in isolation | User follows the execution path step by step |
| Answers "where is this code?" | Answers "how does this system work?" |
| No drill-down capability | Expand any part to see files, functions, and code |

### Implementation approach

The explore mode would combine:

1. **Semantic search** — find relevant chunks for the topic (existing capability)
2. **Call graph analysis** — order chunks by execution flow, not similarity score (needs AST `CallExpr` extraction)
3. **Structural grouping** — cluster related chunks into logical parts (needs grouping logic)
4. **Interactive TUI** — simple keyboard-driven navigation with expand/collapse (e.g., using `bubbletea` or `charmbracelet/lipgloss`)

The data pipeline:

```
User query
  → semantic search (find relevant chunks)
  → call graph lookup (how do these chunks relate?)
  → structural grouping (cluster into logical parts)
  → hierarchical rendering (numbered parts with drill-down)
```

---

## Does Explore Mode need LLM?

**The core mechanics don't need an LLM. The title quality does.**

| Step | Needs LLM? | What it does |
|---|---|---|
| Find relevant chunks | No | Semantic search — already exists |
| Build call graph | No | Go AST `CallExpr` traversal — pure static analysis |
| Group into parts | No | Cluster by call-graph connectivity |
| Order parts | No | Entry point first, then by call depth |
| **Generate titles** | **Yes** | Turns `"cmd/ragcodepilot — main, resolveEmbedder, runIndex"` into `"Index Command: CLI Entry to Pipeline Initialization"` |
| Render TUI | No | bubbletea — just formatting |

### Design decision: mechanical default, LLM when available

The explore command works without LLM — it shows structural grouping with mechanical titles derived from file/function names. When Ollama is available (which it usually is, since it's needed for embeddings), it automatically generates human-readable titles instead. No flag, no user decision. Same command, best available output:

```
$ ragcodepilot explore "Indexing Pipeline"
```

The pipeline internally does:

```
semantic search → call graph → structural grouping → try Ollama for titles → TUI renders
                                                      ↓ (if unavailable)
                                                      fall back to file/function names
```

**Without LLM (Ollama down or unreachable):**

```
 1. cmd/ragcodepilot/main.go — main, resolveEmbedder, runIndex
 2. internal/ingest/pipeline.go, walker.go, hasher.go — Run, Walk, HashFile
 3. internal/ingest/chunker_go.go — ChunkGoFile
 4. internal/ingest/enrichment.go, embedding/ollama.go — EnrichChunk, Embed, BatchUpsert
```

**With LLM (Ollama available — normal case):**

```
 1. Index Command: CLI Entry to Pipeline Initialization
 2. Ingestion Pipeline: File Walking, Chunking, and Change Detection
 3. Go AST-Based Function Chunking
 4. Embedding & Upsert: Enrichment to Vector Storage
```

The structural grouping, ordering, and drill-down views are identical in both cases. Only the labels change. The LLM call is lightweight (~200 tokens output, ~200ms) and uses the same Ollama connection configured via `--ollama-url` and `--ollama-model`.

---

## Search `--answer` vs Explore: separate commands, different purposes

These two commands solve different problems and should not be combined:

```
ragcodepilot search "how does change detection work" --answer
ragcodepilot explore "Change Detection"
```

### What each command outputs

**`search --answer`** — LLM reads code chunks and writes a synthesized explanation:

```
$ ragcodepilot search "how does change detection work" --answer

 Answer:

 Change detection uses SHA256 file hashing to determine which files need
 re-indexing. During Pipeline.Run() [Chunk 1], the system:

 1. Fetches all existing {file_path: file_hash} pairs from Qdrant via
    ScrollFileHashes() [Chunk 3].
 2. Walks the current file system. For each file, it computes a SHA256 hash
    via HashFile() [Chunk 2] and compares it against the stored hash.
 3. Categorizes files: unchanged / modified / new / deleted.
 4. After upserting new chunks for modified files, stale chunks are cleaned
    up via DeleteStaleChunksByFilePath() [Chunk 4].

 Sources:
  [1] internal/ingest/pipeline.go:89-142
  [2] internal/ingest/hasher.go:12-28
  [3] internal/qdrant/client.go:285-340
  [4] internal/qdrant/client.go:345-380
```

**`explore`** — shows structure with drill-down into real code:

```
$ ragcodepilot explore "Change Detection"

 Explore: Change Detection
 File hashing, hash comparison, stale chunk cleanup

 1. Pipeline Orchestration: File Categorization by Hash
 2. SHA256 File Hashing
 3. Qdrant Hash Retrieval and Pagination
 4. Stale Chunk Deletion

 [1-4] View details  [q] Quit
```

Drilling into Part 1 shows the actual code structure — no LLM involved:

```
 > 1

 ━━ 1. Pipeline Orchestration: File Categorization by Hash ━━

 Pipeline.Run(ctx, repoPath)
 ├── 1a. Fetch existing hashes
 │       existingHashes := client.ScrollFileHashes(ctx, collection, repoName, languages)
 │
 ├── 1b. Walk and compare
 │       walker.Walk(repoPath, func(path, info) {
 │           hash := hasher.HashFile(path)
 │           if existingHash == hash → unchanged
 │           if existingHash != hash → modified
 │           if no existingHash      → new
 │       })
 │
 └── 1c. Identify deleted files
         remaining keys in existingHashes → deletedFiles
```

### Why they're different

| | `search --answer` | `explore` |
|---|---|---|
| **Question answered** | "What does this code do?" | "How is this system structured?" |
| **LLM input** | Full code chunks (~2000-3000 tokens) | Function/file names only (~200 tokens) |
| **LLM task** | Read code, reason about logic, write explanation | Generate short titles for groups |
| **LLM output** | Multi-paragraph explanation (~300-500 tokens) | One-line title per group (~50 tokens) |
| **If LLM is wrong** | User gets a misleading explanation | Bad label, but real code is right there in drill-down |
| **Without LLM** | Command doesn't work | Falls back to mechanical titles |

Two commands, each doing one thing well:
- **`search`** → find code (+ `--answer` for explanation)
- **`explore`** → understand structure (LLM titles are built-in, drill-down shows real code)

---

## Phase architecture: indexing vs query time

### The call graph is built during INDEXING, not at query time

Phase 4 (call graph extraction) runs during `ragcodepilot index` as an extension of the existing Go AST chunker. While parsing functions, it also records which functions call which. These relationships are stored in Qdrant as payload metadata alongside the chunks:

```json
// Today's chunk payload in Qdrant:
{
    "file_path": "internal/ingest/pipeline.go",
    "function":  "Run",
    "language":  "go",
    "repo":      "ragcodepilot",
    "file_hash": "abc123..."
}

// With call graph added:
{
    "file_path": "internal/ingest/pipeline.go",
    "function":  "Run",
    "language":  "go",
    "repo":      "ragcodepilot",
    "file_hash": "abc123...",
    "calls":     ["Walker.Walk", "Chunker.ChunkFile", "Embedder.Embed"],
    "called_by": []
}
```

### Phase ownership

```
                    INDEXING (runs once via `ragcodepilot index`)
                    ════════════════════════════════════════════
                    ┌──────────────────────────────────────────┐
                    │ 1. Walk files                             │
                    │ 2. Parse AST                              │
                    │ 3. Extract chunks                         │
                    │ 4. Extract call graph (calls/called_by)   │  ← NEW
                    │ 5. Enrich chunks                          │
                    │ 6. Embed via Ollama                       │
                    │ 7. Upsert to Qdrant (chunks + call graph) │
                    └──────────────────────────────────────────┘
                              │
                    stored in Qdrant once
                              │
            ┌─────────────────┼─────────────────┐
            ▼                                   ▼
    SEARCH (query time)                 EXPLORE (query time)
    ═══════════════════                 ════════════════════
    ┌──────────────────┐                ┌─────────────────────────────┐
    │ A. Embed query    │                │ A. Embed query              │ ← same
    │ B. Vector search  │                │ B. Vector search            │ ← same
    │ C. Return chunks  │                │ C. Call graph lookup        │ ← explore only
    └──────────────────┘                │ D. Structural grouping      │ ← explore only
                                        │ E. LLM title generation     │ ← explore only
                                        │ F. TUI rendering            │ ← explore only
                                        └─────────────────────────────┘
```

### Shared vs explore-only phases

| Phase | Search | Explore | Notes |
|---|---|---|---|
| A. Embed query | ✅ | ✅ | Same — both embed the user's query via Ollama (~50ms) |
| B. Vector search | ✅ | ✅ | Same — both query Qdrant for top-K similar chunks (~20ms) |
| C. Call graph lookup | ❌ | ✅ | Reads `calls`/`called_by` from stored payload. Fast — data is already in Qdrant |
| D. Structural grouping | ❌ | ✅ | In-memory: cluster chunks by call connectivity, order by execution flow |
| E. LLM title generation | ❌ | ✅ | Lightweight Ollama call: function names → short titles (~200ms) |
| F. TUI rendering | ❌ | ✅ | bubbletea interactive display with keyboard navigation |

### Do phases re-trigger between commands?

**Yes — each command is a separate CLI invocation.** No shared state:

```bash
$ ragcodepilot search "change detection"     # runs A → B → C (display)
$ ragcodepilot explore "Change Detection"    # runs A → B → C → D → E → F
```

Phases A and B re-run, but they're fast (~70ms total). The expensive work (AST parsing, call graph extraction, embedding chunks) was done once during `ragcodepilot index`.

**Performance breakdown for explore:**

```
A. Embed query:           ~50ms
B. Qdrant search:         ~20ms
C. Call graph lookup:     ~10ms  (reading payload fields, already in Qdrant)
D. Structural grouping:   ~5ms   (in-memory clustering)
E. LLM title generation: ~200ms  (lightweight Ollama call)
F. TUI render:            instant
─────────────────────────────────
Total:                    ~285ms
```

---

## Where this fits in the roadmap

Per `system_vision_review.md`, the priority order is:

```
Phase 1: Eval harness (baseline metrics)
Phase 2: Hybrid search (BM25 + vector + RRF)
Phase 3: Reranking + chunker upgrades ← call graph extraction fits here
Phase 4: UX polish                    ← map command + result grouping fits here
Phase 5: Optional --answer mode       ← structural context in LLM prompts fits here
```

The structural understanding features (map command, call graph, result grouping) are **Phase 3-4 work**. They depend on:

- Good AST parsing infrastructure (extended from current Go AST chunker)
- The eval harness (to measure if structural grouping improves search quality)
- Hybrid search (so exact symbol lookups work before we try to chain them)

**Don't start structural features before eval + hybrid search are done.** The call graph is useless if you can't measure whether it improves retrieval.

---

## Key takeaway

Codemaps and ragcodepilot can solve the same underlying problem — "help me understand this codebase" — through different interfaces but with the same core idea: **structured, hierarchical, drill-down exploration.**

- **Codemaps:** IDE-integrated, visual graph nodes, click-to-navigate
- **ragcodepilot explore:** Terminal-native, numbered parts, keyboard-driven drill-down

The *data* they need is identical (AST + embeddings + call relationships). The *interaction model* is similar — both present high-level structure first, then let the user dive deeper. The difference is the rendering medium: visual graphs in an IDE vs. structured text in a terminal.

The `explore` command would be ragcodepilot's answer to the "understand and reason about" gap identified in the system vision review. The navigation (grouping, ordering, drill-down) is powered by structural analysis + semantic search and works without LLM. The human-readable titles — the part that makes it feel like Codemaps — are generated by LLM when Ollama is available, falling back to mechanical file/function name labels silently. No flag needed. The `search --answer` command is separate — it provides LLM-synthesized explanations for a different use case ("what does this code do?" vs "how is this system structured?").

This is the feature that would make ragcodepilot genuinely different from grep, ripgrep, or basic vector search — not just "find similar code" but "show me how this system works, step by step."

---

## Impact on system_vision_review.md

If we commit to Explore Mode, the following sections in `docs/review_feedback/system_vision_review.md` need updating:

### Q1 — Purpose (line 22)

The outward product description says *"get ranked source-code chunks back."* Explore Mode changes this — the system also helps users **understand codebase structure**, not just find chunks.

**Current:** A tool that lets a developer ask natural-language questions and get ranked source-code chunks back.
**Updated:** A tool that lets a developer ask natural-language questions, get ranked source-code chunks, and **explore codebase structure through hierarchical drill-down**.

### Q3 — Missing for "understand" half (lines 55-56)

The review identifies two gaps that Explore Mode directly addresses:

> - No file or repo overview mode. "Explain how this package fits together"
> - No traversal. "How does X work?" benefits from following imports/calls

These should be updated to acknowledge Explore Mode as the planned solution. Both gaps are covered by the hierarchical parts + call graph traversal design.

### Q5 — Weaknesses table (line 84, item #7)

> Bare result formatting — no grouping by file, no surrounding context lines

Explore Mode supersedes this. It's not just "group by file" — it's "group by logical flow with drill-down." The weakness should be reframed to acknowledge the more ambitious solution.

### Q6 — Features table (line 104)

> P3: File-level summary / TOC command — S effort

This should be upgraded. Explore Mode is a more ambitious version of this feature:

- Priority: P3 → **P2**
- Effort: S → **M-L**
- Description: "File-level summary / TOC" → **"Explore mode: hierarchical codebase exploration with drill-down"**

### Q8 — Avoid building too early (lines 133, 136)

Two items in the "avoid" list conflict with Explore Mode:

| Current "avoid" item | Conflict |
|---|---|
| **Web UI / TUI** — "CLI works; UI is polish, not value" | Explore Mode requires a TUI (bubbletea). The TUI isn't "polish" if it's the primary interaction model for exploration. |
| **Cross-repo dependency / call graphs** — "Architectural over-engineering" | Explore Mode requires intra-repo call graphs. This isn't over-engineering — it's the data source for structural grouping. The original concern was about *cross-repo* graphs, which are still out of scope. |

These should be revised to distinguish between what's still premature (cross-repo graphs, web UI, IDE plugin) and what's now planned (intra-repo call graph, terminal TUI for explore).

### Q10 — Roadmap (lines 174-181)

Explore Mode doesn't fit neatly into the current 6-phase roadmap. It needs its own phase, likely between Phase 4 (UX polish) and Phase 5 (--answer mode):

```
Phase 1: Eval harness (baseline metrics)
Phase 2: Hybrid search (BM25 + vector + RRF)
Phase 3: Reranking + chunker upgrades + call graph extraction
Phase 4: UX polish (JSON output, context lines, result grouping)
Phase 4.5: Explore Mode (structural grouping + TUI + explore eval)  ← NEW
Phase 5: Optional --answer mode
Phase 6: Decision point
```

Explore Mode depends on Phase 3 (call graph extraction from AST) and Phase 4 (result grouping). The `--answer` mode can then be layered on top of Explore Mode to add narrative explanations to each part.

---

## Evaluation approach for Explore Mode

### The gap: current eval doesn't measure structural quality

The evaluation harness spec (`rag_evaluation_metrics_with_feedback.md`) measures **flat retrieval quality**:

- `hit@k` — did the right file appear in top K?
- `MRR@k` — how high did it rank?
- `recall@k` — how many relevant files appeared?

These are "find the needle" metrics. They answer: *"Given a query, did the right chunks show up?"*

Explore Mode answers a different question: *"Given a topic, did the system correctly decompose it into logical parts and order them by execution flow?"*

### Proposed approach: two eval modes

Don't replace the existing eval — **add alongside it**. Both are needed because the `search` command still exists, and Explore Mode's semantic search step internally depends on good flat retrieval.

#### Mode 1: Flat retrieval eval (existing, build first)

```bash
ragcodepilot eval retrieval --dataset docs/eval/retrieval.yaml
```

Measures the `search` command. Unchanged from the current spec:

```text
hit@1, hit@3, hit@5
MRR@5
recall@5
filter_violation_count
latency_p50_ms, latency_p95_ms
```

#### Mode 2: Explore eval (new, build when Explore Mode is implemented)

```bash
ragcodepilot eval explore --dataset docs/eval/explore.yaml
```

Measures the `explore` command with new structural metrics:

| Metric | What it measures | Example |
|---|---|---|
| **part_count_accuracy** | Did the system produce the right number of logical parts? | "Indexing Pipeline" → expected 4-6 parts |
| **part_coverage** | Did expected files appear in the correct parts? | `chunker.go` should be in the "Chunking" part, not "Embedding" |
| **part_ordering** | Are parts ordered by execution flow, not similarity? | Part 1 (CLI entry) before Part 2 (pipeline execution) |
| **part_coherence** | Does each part contain related functions/files only? | The "Chunking" part should contain `chunker.go` + `chunker_go.go`, not `hasher.go` |
| **cross_reference_accuracy** | Do "→ see Part X" links point to the correct part? | A reference to `pipeline.Run` should link to the part containing that function |
| **irrelevant_inclusion** | Does the exploration include unrelated code? | "Indexing Pipeline" should NOT include search-related code |

#### Explore eval dataset format

```yaml
eval_type: explore
topics:
  - id: indexing_pipeline
    topic: "Indexing Pipeline"
    expected_parts:
      - title_contains: ["CLI", "Entry", "Command"]
        must_include_files: [cmd/ragcodepilot/main.go]
      - title_contains: ["Pipeline", "Walking", "Ingestion"]
        must_include_files: [internal/ingest/pipeline.go, internal/ingest/walker.go]
      - title_contains: ["Chunking", "AST"]
        must_include_files: [internal/ingest/chunker.go, internal/ingest/chunker_go.go]
      - title_contains: ["Embedding", "Upsert"]
        must_include_files: [internal/ingest/enrichment.go]
    expected_file_order: [main.go, pipeline.go, chunker.go, enrichment.go]
    must_not_include: [internal/search/]

  - id: search_pipeline
    topic: "Search Pipeline"
    expected_parts:
      - title_contains: ["CLI", "Entry", "Command"]
        must_include_files: [cmd/ragcodepilot/main.go]
      - title_contains: ["Search", "Query", "Vector"]
        must_include_files: [internal/search/searcher.go]
    must_not_include: [internal/ingest/]
```

#### Example eval output

```text
Dataset: docs/eval/explore.yaml
Topics: 5

part_count_accuracy:       0.80   (expected parts matched in 4/5 topics)
part_coverage:             0.92   (33/36 expected files in correct parts)
part_ordering_score:       0.85   (execution order mostly correct)
part_coherence:            0.90   (90% of chunks per part are related)
cross_reference_accuracy:  1.00   (all "see Part X" links are valid)
irrelevant_inclusion:      0.05   (5% of included code was unrelated)
```

### Build sequence

| What | When | Why |
|---|---|---|
| **Flat retrieval eval** | Phase 1 (now) | Needed immediately. Explore Mode depends on good flat retrieval internally. |
| **Call graph extraction** | Phase 3 | Data foundation for structural grouping. |
| **Explore Mode feature** | Phase 4.5 | Requires call graph + result grouping. |
| **Explore eval** | Phase 4.5 (alongside feature) | Can't test structural quality without the feature. |

The flat retrieval eval is **not blocked** by Explore Mode. Build it first. The explore eval is a separate mode that gets added when the explore feature is being implemented.
