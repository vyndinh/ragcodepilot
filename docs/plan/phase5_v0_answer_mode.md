# Phase 5 v0 — `--answer` Mode (Minimal RAG)

Add the smallest possible answer-generation layer on top of existing hybrid search. **This is the first feature that turns ragcodepilot from semantic grep into a RAG system.** Default behavior stays retrieval-only; `--answer` is opt-in.

**Exit criteria (qualitative, not metric-gated):**

- `ragcodepilot search --answer "question"` returns a generated answer plus the source chunks it used.
- Works end-to-end against local Ollama with a generative model (default: `qwen2.5-coder:7b`).
- Existing `search` without `--answer` produces identical output to today (no regression on the retrieval-only path).
- Fake generator implementation for unit tests; full Go suite passes with `-race`.
- Used for one week of dogfooding before deciding what (if anything) to build next.

---

## Why a new phase

The user just asked the question the MVP roadmap was designed to wait for: "does generation add value to this system?" Three reasons that question is ready to be answered now:

1. **Phase 2 cleared the retrieval-quality gate by a wide margin.** `hit@5 = 0.895` means the right chunk is in the top-5 for nearly every query. An LLM can synthesize from messy context — it doesn't need the 2-3 percentage points a reranker would add.
2. **The vision review's two preconditions for adding generation are both met:** (a) the user wants explanations, not just chunks; (b) retrieval quality is strong. Both are now true.
3. **No alternative phase produces this signal.** Rust chunking, UX polish, and reranking are all *internal* improvements — they make the existing thing better. `--answer` is the only feature whose result tells you whether RAG is the right product shape for this project. Without it, every downstream decision is speculative.

This is **v0** deliberately. Scope is minimal: smallest thing that lets you ask a question and get a synthesized answer. No citation parsing, no faithfulness checks, no streaming. The point of v0 is to produce a real user experience to react to, not to ship the final design.

---

## Main goals

1. **Validate the product question.** Is generation what users want here, or do raw chunks already serve the use case?
2. **Establish the generation seam.** Define the `Generator` interface, prompt template, and Ollama integration once — so v1 has a stable shape to build on.
3. **Surface the failure modes.** Hallucinations, wrong-chunk-in-context, cold-start latency — v0 makes them visible so v1 can decide which are worth fixing.
4. **Cost: ~1 week of focused work.** If it stretches beyond that, scope is wrong; cut more.

Explicitly **not** goals for v0:

- Faithfulness or answer-quality metrics. (Save for v1, gated on whether v0 proves useful.)
- Citation validation. (Print chunks beneath the answer; let humans verify.)
- Streaming responses. (Synchronous is fine for v0.)
- Multiple LLM providers. (Ollama only.)
- Cost tracking. (Local Ollama has no per-call cost.)

---

## Resolved design decisions

### 1. LLM model: `qwen2.5-coder:7b` as default, configurable

Code-tuned, ~4 GB on disk, runs on a developer laptop. Smaller than `llama3.1:8b`, more code-aware than `llama3.2:3b`. Configurable via a new flag so swapping is one CLI argument.

**Why not** `llama3.2:3b` (smaller, faster cold-start): worse on code synthesis, hallucinates more.
**Why not** `llama3.1:8b` (more general): not code-tuned; slightly slower; no clear quality win for code questions.
**Why not** a cloud API (Claude, GPT): introduces $$ per query, network dependency, vendor lock-in. v0 stays local.

### 2. Generation path: Ollama HTTP `/api/chat`

Same infrastructure pattern as the existing embedder. No new Go dependency. Synchronous (non-streaming) for v0.

**Why** `/api/chat` over `/api/generate`: message-based shape (system + user) is more idiomatic for RAG prompts and gives us a forward-compatible structure if v1 adds multi-turn or tool calls.

### 3. Package: new `internal/answer/`

Mirrors `internal/embedding/`'s pattern (interface + Ollama impl + fake for tests). Keeps generation isolated from `internal/search/`.

```
internal/answer/
    generator.go        // Generator interface + Answer struct
    ollama.go           // OllamaGenerator: HTTP client to /api/chat
    fake.go             // FakeGenerator: returns a canned response (for tests)
    prompt.go           // Prompt template construction
    generator_test.go   // Interface + prompt-template tests
```

**Why not** put it in `internal/search/`: the search layer is about retrieval. Generation is a separate concern that consumes search results. Mixing them makes both harder to test.

### 4. Prompt template: frozen for v0, tunable for v1

```
SYSTEM:
You are answering questions about a code repository. Use only the provided
code chunks to answer. If the chunks don't contain enough information,
say so explicitly — do not invent details. Reference specific chunks by
their number in brackets like [1] when citing code.

USER:
Question: {question}

Context:
[1] internal/ingest/pipeline.go:42-78, function: Pipeline.Run
    {chunk content}

[2] internal/qdrant/client.go:120-145, function: EnsureCollection
    {chunk content}

...

Answer the question based on the chunks above.
```

The template is hardcoded in `internal/answer/prompt.go`. v1 might make it a configurable file; v0 doesn't.

### 5. Output format

Print the answer text, then print the source chunks beneath it. Human-readable. No JSON output mode for v0 (Phase 4 territory).

```
Answer: Change detection works by hashing file contents at index time...
[1] suggests the SHA-256 hash is stored in the chunk's file_hash payload field.

Sources:
[1] internal/ingest/pipeline.go:42-78, Pipeline.Run
[2] internal/qdrant/client.go:285-340, ScrollFileHashes
[3] internal/ingest/hasher.go:12-28, HashFiles
```

### 6. Retrieval coupling

`--answer` reuses the existing hybrid search path verbatim. Top-5 chunks (configurable via `--limit`) feed the prompt. No change to the search code.

If real-world `hit@1` proves too weak for good answers, that's the signal to **activate the deferred reranker** (its existing plan in `phase3_rust_chunker.md` activates on the exact trigger criteria already documented). No reranker work in Phase 5 v0.

---

## Architecture

```
ragcodepilot search "question" --answer
            │
            ▼
   cmd/ragcodepilot/main.go (--answer flag)
            │
            ▼
   internal/search/searcher.go
            │  SearchWithTimings(ctx, ..., top-5 chunks)
            ▼
   internal/answer/generator.go
            │  buildPrompt(question, chunks) → prompt string
            │  generator.Generate(ctx, prompt) → answer string
            ▼
   internal/answer/ollama.go
            │  HTTP POST /api/chat with {model, messages}
            ▼
   Ollama (qwen2.5-coder:7b)
            │
            ▼
   Answer + source chunks printed to stdout
```

Key properties:

- **No sidecar.** Generation is just another HTTP call to the same Ollama server already used for embeddings.
- **No new Go dependency.** Reuses stdlib `net/http` + existing JSON serialization.
- **Pure Go binary preserved.** No CGo, no Python, no external service besides Ollama.
- **Backwards-compatible.** `search` without `--answer` produces byte-identical output to today.

---

## Implementation order

```
1. Generator interface + fake implementation             (~half day)
2. Prompt template builder + tests                        (~half day)
3. Ollama generator (HTTP client to /api/chat)            (~1 day)
4. CLI wiring: --answer flag, format output                (~half day)
5. Manual end-to-end testing against real Ollama           (~half day)
6. Documentation: README setup note, plan-doc result notes (~half day)
```

Total: ~3-4 days. Add a day or two of buffer for prompt iteration if first results are disappointing.

---

## Component breakdown

### 1. Generator interface (`internal/answer/generator.go`)

```
type Generator interface {
    Generate(ctx Context, prompt Prompt) (string, error)
}

type Prompt struct {
    Question string
    Chunks   []ChunkContext
}

type ChunkContext struct {
    Index    int    // 1-based for citation
    FilePath string
    Lines    string // "42-78"
    Symbol   string // function/class name, if any
    Content  string
}
```

The interface is intentionally simple. v1 can extend with streaming, token budgets, etc.

### 2. Prompt builder (`internal/answer/prompt.go`)

```
function BuildPrompt(question, chunks) -> (system, user)
```

Tested with golden-string assertions: given a fixed question + fixed chunks, verify the rendered prompt matches expectation byte-for-byte. Prevents accidental drift.

### 3. Ollama generator (`internal/answer/ollama.go`)

```
type OllamaGenerator struct {
    URL     string         // default: http://localhost:11434
    Model   string         // default: qwen2.5-coder:7b
    Timeout Duration       // default: 60s
}

Generate(ctx, prompt):
    POST {URL}/api/chat with {model, messages: [system, user]}
    Parse response.message.content
    Return as plain string
```

Error handling:
- Connection refused → wrap with hint: "Is Ollama running? Try `ollama serve`."
- Model not found → wrap with hint: "Run `ollama pull qwen2.5-coder:7b`."
- Timeout → return error, don't retry in v0.

### 4. Fake generator (`internal/answer/fake.go`)

Canned-response generator used by tests and as a `--generator=fake` CLI fallback for hermetic e2e tests.

### 5. CLI wiring (`cmd/ragcodepilot/main.go`)

Add to the `search` subcommand:

```
--answer                            Generate an answer using the retrieved chunks
--generator string                  Generator: ollama, fake (default "ollama")
--ollama-generative-model string    Generative model for --answer (default "qwen2.5-coder:7b")
```

Flow in `runSearch`:

```
if --answer:
    results = searcher.Search(...)  // top-5 chunks
    gen = resolveGenerator(--generator, --ollama-url, --ollama-generative-model)
    prompt = answer.BuildPrompt(query, chunksToContext(results))
    text, err = gen.Generate(ctx, prompt)
    print "Answer: ", text
    print "\nSources:"
    print FormatResultsBrief(results)
else:
    existing behavior, unchanged
```

### 6. Tests

- `prompt_test.go`: golden-string tests for prompt rendering.
- `generator_test.go`: tests using `FakeGenerator` that exercise the CLI flow without hitting Ollama.
- `ollama_test.go`: skipped if Ollama not reachable (`t.Skip()`). Asserts the HTTP request shape is correct via a `httptest.Server`.
- No e2e test of LLM quality. v0 is qualitative; quality is judged by dogfooding.

---

## Files to touch / create

| File | Action | Purpose |
|---|---|---|
| `internal/answer/generator.go` | NEW | `Generator` interface, `Prompt` and `ChunkContext` types |
| `internal/answer/ollama.go` | NEW | Ollama HTTP client for `/api/chat` |
| `internal/answer/fake.go` | NEW | Fake generator for tests |
| `internal/answer/prompt.go` | NEW | Prompt template builder |
| `internal/answer/generator_test.go` | NEW | Interface + prompt-template tests |
| `internal/answer/ollama_test.go` | NEW | HTTP-shape tests with httptest |
| `cmd/ragcodepilot/main.go` | MODIFY | Add `--answer`, `--generator`, `--ollama-generative-model` flags; wire into `runSearch` |
| `internal/search/searcher.go` | MAYBE MODIFY | Add `FormatResultsBrief` helper if existing formatter is too verbose for the "Sources:" section |
| `docs/plan/mvp_roadmap.md` | MODIFY | Re-order: Phase 5 v0 moves ahead of Phase 3 (Rust) and Phase 4 (UX); document the pivot rationale |
| `docs/plan/phase3_rust_chunker.md` | MODIFY | Add header note: "Deferred pending Phase 5 v0 dogfooding signal — see `phase5_v0_answer_mode.md`" |
| `docs/task_tracker/phase3_rust_chunker.md` | MODIFY | Same deferred note |
| `docs/task_tracker/phase5_v0_answer_mode.md` | NEW (after plan approval) | Per-task tracker following the convention |
| `README.md` | MODIFY (after ship) | Document `--answer` flag and `ollama pull qwen2.5-coder:7b` setup step |

---

## Out of scope (revisit in v1 if v0 proves useful)

- **Citation parsing & validation.** v0 prints chunks beneath the answer; users verify by reading. v1 parses `[N]` references from the answer and validates they exist + checks the cited chunk supports the claim.
- **Faithfulness metric.** v0 has no automatic check that the answer is grounded in the retrieved chunks. v1 might add an LLM-as-judge pass that scores each claim against the source chunks.
- **Streaming.** v0 is synchronous. v1 can switch to streaming once the prompt + model choice are settled.
- **Multi-turn / conversational.** v0 is single-shot question → answer. No history, no follow-ups.
- **Refuse-on-low-confidence guardrail.** v0 always generates. v1 can refuse when top-1 retrieval score is below threshold.
- **External LLM providers (Anthropic, OpenAI).** v0 stays Ollama-only. v1 might add a generic HTTP client interface.
- **Token budget management.** v0 always sends top-5 chunks; if they exceed the model's context window, the model will truncate or error. v1 adds a budget-aware packing algorithm.
- **Cost tracking.** Not relevant for local Ollama.
- **Chunk expansion (sibling chunks, parent class context).** v0 sends what retrieval returns. v1 might expand the context.

---

## Risks, dependencies, tradeoffs

### Risks

1. **Hallucinations.** The LLM produces a confident-sounding answer that contradicts or fabricates content not in the chunks. Mitigation in v0: always print the source chunks. v1 can add automatic faithfulness checks if v0 confirms this is the dominant failure mode.

2. **Wrong-chunks-in-context.** If hybrid retrieval misses the right chunk, the LLM has bad input and produces a wrong-but-confident answer. Mitigation in v0: same — show the chunks so users see what fed the answer. This is also the trigger to activate the deferred reranker if it happens too often.

3. **Cold-start latency.** First call after Ollama starts is 5-30 seconds while the generative model loads. Mitigation: document `OLLAMA_KEEP_ALIVE=-1` in the README setup section. Subsequent calls are fast (1-5 seconds depending on prompt size and hardware).

4. **Prompt brittleness.** Small wording changes in the system prompt can cause large quality swings. Mitigation in v0: freeze one prompt template. Don't tune iteratively until enough dogfooding data exists to know what to tune.

5. **Result format change is irreversible without notice.** Users scripting against `search` output won't be affected (default unchanged), but `--answer` output format may evolve. Mitigation: document v0 output as unstable.

### Dependencies

- **Ollama must have a generative model pulled.** Default is `qwen2.5-coder:7b` (~4 GB). Document `ollama pull qwen2.5-coder:7b` as a one-time setup step. The existing `nomic-embed-text` (for embeddings) stays unchanged.
- **No new Go dependencies.** Pure stdlib + existing project deps.
- **Existing `search.Searcher.SearchWithTimings`** stays unchanged. `--answer` mode wraps around it.

### Tradeoffs

- **Speed vs quality.** `qwen2.5-coder:7b` is the middle ground. Smaller (faster) hurts code reasoning; bigger (slower) is marginal gain on typical questions.
- **Simple vs streaming.** Synchronous is simpler; streaming would feel more responsive. v0 chooses simple.
- **Strict vs permissive prompt.** Strict prompts ("answer only from chunks") reduce hallucinations but cause refusals when the chunks are imperfect. Permissive prompts give better-looking answers more often but hallucinate more. v0 starts strict; revisit based on dogfooding.

---

## Connection to existing roadmap

Phase 5 v0 **moves ahead of Phase 3 (Rust chunker) and Phase 4 (UX polish)** in the sequence. The MVP roadmap originally placed `--answer` last because it was speculative ("don't start unless a real user need has surfaced"). That gate just opened.

Effective new ordering:

1. ~~Phase 3 — Rust chunker~~ → **deferred** (parked plan stays in `docs/plan/phase3_rust_chunker.md`, marked deferred)
2. ~~Phase 4 — UX polish~~ → deferred until after Phase 5 v0 dogfooding (some polish items may be informed by what `--answer` reveals about output format)
3. **Phase 5 v0 — minimal `--answer` mode** ← this plan
4. **Decision point after v0 dogfooding (1-2 weeks of real use):**
   - "This is useful, but answers are sometimes wrong/incomplete" → Phase 5 v1 (citation parsing, faithfulness eval, prompt iteration)
   - "Retrieval misses too much for good answers" → activate the deferred reranker per its existing plan
   - "I keep ignoring the answer and reading the chunks" → kill `--answer`, invest in Phase 4 UX polish for chunk presentation
   - "I want this for Python/Rust too" → un-defer Phase 3
5. Decision-driven next phase

---

## Verification

```bash
# Setup (one-time)
ollama pull qwen2.5-coder:7b
docker compose up -d  # Qdrant
go run ./cmd/ragcodepilot index --language go .

# Unit tests
go test ./internal/answer/... -v -race -count=1
go test ./... -count=1 -race  # full suite

# Build check
go build ./...

# Manual end-to-end
go run ./cmd/ragcodepilot search "how does change detection work" --answer
go run ./cmd/ragcodepilot search "where is hybrid search assembled" --answer
go run ./cmd/ragcodepilot search "what happens when sparse vector is nil" --answer

# Regression: retrieval-only path unchanged
go run ./cmd/ragcodepilot search "ChunkFile"  # same output as before
```

No automated eval gate. v0's value is qualitative. After a week of dogfooding, write a short note in this doc with:

- Which questions worked well
- Which failed and why (retrieval miss vs hallucination vs prompt issue)
- Cold-start vs warm-path latency observed
- Whether the answer or the chunks were more useful in practice

That note drives the v1 vs kill decision.

---

## Why this scope and not more

Three reasons to keep v0 small:

1. **The product question is binary.** Generation either adds value or it doesn't. v0 just needs to be good enough to make that judgment. Faithfulness eval, streaming, citation validation, etc. are all v1 features — they assume v0 said yes.

2. **Most "polish" items in v1 are gated on what v0 reveals.** If users ignore the answer and read chunks, you don't want streaming. If hallucinations are rare, you don't need faithfulness checks. If cold-start kills the UX, you might pin a smaller model. Building these now is speculation.

3. **The cost of building v0 is also the cost of being wrong.** ~4 days. If v0 proves `--answer` is the wrong direction, you've spent four days. If v0 takes two weeks of "let's just add one more thing," you've spent two weeks discovering the same answer.
