# MCP Server Mode — Design Doc

**Status:** Draft. Not started. Proposed 2026-07-06 in
[`../improvement/production_readiness_and_features.md`](../improvement/production_readiness_and_features.md) §2.1
as the highest-leverage product feature. **Slotted as milestone M2 in the
restructured [`mvp_roadmap.md`](mvp_roadmap.md)** (after M0 navigation-bet
decision and M1 streaming answers); the shared `--json` serializer is the M2
lead-in task.

---

## Goal

Add `ragcodepilot serve --mcp`: a long-running mode that exposes the existing
retrieval path as **MCP (Model Context Protocol) tools over stdio**, so any
MCP-capable client — Claude Code, Cursor, other agents — can call ragcodepilot's
persistent, indexed, hybrid code search instead of re-grepping the repo on every
task.

**Exit criterion:** a Claude Code session configured with the ragcodepilot MCP
server can (a) call `search_code` and receive ranked chunks with file paths,
line ranges, and scores; (b) call `list_collections` and see indexed repos with
chunk counts and staleness; with retrieval results **identical** to the CLI
`search` command for the same query and flags (same code path, no fork).

## Why

- **Repositions the product.** As a human-facing CLI, ragcodepilot competes
  with grep, IDE search, and each agent's built-in retrieval. As an MCP server
  it becomes infrastructure those agents *call* — the persistent index that
  outlives an agent session is exactly the asset agents lack. The realistic
  primary user is an agent making many retrieval calls per task, not a human
  typing queries.
- **Cheap relative to its leverage.** The search path already exists and
  returns structured results (`SearchResult{Chunk, Score}`); this is a protocol
  wrapper plus a process loop, not new retrieval machinery.
- **Resolves the daemon question honestly.** `architecture_decisions.md` §5.3
  reserves a long-running process for "multiple frontends" — MCP is that
  trigger firing for real. stdio transport avoids the IPC/lifecycle/versioning
  costs that doc warns about: the *client* owns the process lifetime (spawn on
  session start, kill on exit), so there is no port allocation, no stale-daemon
  cleanup, no version skew between CLI and server (same binary).

## Non-goals

- **No HTTP/SSE transport in v0.** stdio only. HTTP serves multi-client /
  remote setups; that's a later decision with real auth implications.
- **No indexing through MCP in v0.** Tools are read-only (search + metadata).
  Indexing stays a CLI action (`ragcodepilot index`), keeping the server free
  of long-running write operations and permission questions. Revisit after
  dogfooding (an `index_repo` tool is the obvious v1 candidate).
- **No answer generation through MCP.** The calling agent *is* the LLM; it
  wants chunks, not a second model's synthesis. `--answer` stays a CLI feature.
- **No auth / multi-tenancy.** Local, single-user, same trust domain as the
  CLI (NF1 in `system_design.md`).

## Tool surface (v0)

```
tool search_code
  input:
    query      string   (required)
    language   string   (optional; e.g. "go")
    repo       string   (optional; repo name filter)
    limit      int      (optional; default 5, max 50)
    mode       string   (optional; dense|sparse|hybrid, default hybrid)
  output (structured):
    results: [
      { file_path, start_line, end_line, language, chunk_type,
        name, score, content }
    ]
    collection, mode, timings {embed_ms, qdrant_ms, total_ms}

tool list_collections
  input:  (none)
  output:
    collections: [ { name, chunk_count, repos: [names], dimension } ]
```

Design notes:

- **Output is the JSON shape of today's `model.SearchResult`** — the same
  structs, serialized. This is deliberately shared with the planned CLI
  `--json` flag (Phase 4): one serializer, two consumers. Build the serializer
  once here and the `--json` flag becomes trivial (or vice versa).
- **Tool descriptions are prompt engineering.** The `search_code` description
  must tell the agent *when* to prefer it over grep: "semantic + keyword hybrid
  search over an indexed repository; best for natural-language questions,
  concept lookup, and symbol search; returns ranked code chunks with paths and
  line numbers."
- **Content size guard:** cap each chunk's `content` in the response
  (e.g. first ~100 lines) with a `truncated: true` marker — agents can read the
  file at the returned path if they need more. Keeps tool results within client
  context budgets.

**v1 candidates (gated on dogfooding):** `index_repo` / `refresh_repo` (write
path), `get_chunk_context(file, line, n_lines)`, and — once Phase 6 ships —
`trace_calls(from, to)` and graph-expanded search.

## Architecture

```
MCP client (Claude Code / Cursor / ...)
    │  spawns process, owns lifetime
    ▼
ragcodepilot serve --mcp            ← stdio JSON-RPC loop
    │
    ├── tool dispatch → internal/search.Searcher   (existing, unchanged)
    │                     └── Embedder (Ollama)    (existing)
    │                     └── qdrant.Client        (existing)
    └── list_collections → qdrant.Client           (existing CRUD)
```

- **One Searcher per server process, constructed at startup** from the same
  flags/config the CLI uses (`--collection`, `--qdrant-*`, `--ollama-*`).
  Long-lived process means the gRPC connection and HTTP client are reused
  across calls — the "warm path" the architecture doc says a daemon would buy
  is obtained for free here.
- **Warmup at startup:** send one tiny embedding call when the server starts,
  so the first agent query doesn't eat the 5–30 s Ollama cold start
  (`architecture_decisions.md` §4.3 — this is the "warmup ping" idea landing
  in its natural home).
- **SDK decision (step 1):** evaluate the official
  `modelcontextprotocol/go-sdk` first; `mark3labs/mcp-go` is the fallback with
  a larger install base. Criteria: stdio transport maturity, structured tool
  results, dependency weight. Record the choice here.
- **Errors are tool results, not crashes.** Qdrant/Ollama unreachable → the
  tool returns an error message with the fix command (same actionable-error
  text as the proposed `doctor` command — share the check functions), and the
  server stays up.

## Config surface

```
# client-side registration (example: Claude Code .mcp.json)
{ "mcpServers": { "ragcodepilot": {
    "command": "ragcodepilot",
    "args": ["serve", "--mcp", "--collection", "code_chunks"]
} } }
```

Server flags mirror the CLI: `--collection`, `--qdrant-host/port`,
`--embedder`, `--ollama-url`, `--ollama-model`. No new config.yaml keys in v0.

## Implementation order

| Step | Description | Size |
|---|---|---|
| 1 | SDK spike: minimal stdio server with a hello tool in both candidate SDKs; pick one, record decision above | S |
| 2 | Shared JSON serializer for `SearchResult` (the `--json` shape) + content-cap logic | S |
| 3 | `serve --mcp` command: server loop, Searcher construction, startup warmup, `search_code` tool | M |
| 4 | `list_collections` tool (chunk counts via Qdrant count API, repo names via scroll/facet) | S |
| 5 | Actionable-error mapping for Qdrant/Ollama-down cases | S |
| 6 | Dogfood: register in Claude Code, run real coding tasks against this repo; record findings + v1 tool wishlist here | S |
| 7 | README + docs: registration snippet, tool reference | S |

Total v0: **M**. Step 2 is shared work with Phase 4's `--json` flag — do it
once. Steps 1–2 have no dependencies and can start anytime.

## Files to touch / create

**New:**
- `internal/mcp/server.go` — server loop, tool registration, dispatch.
- `internal/mcp/tools.go` — tool schemas + handlers (thin over `search.Searcher`).
- `internal/mcp/server_test.go` — dispatch tests with fake embedder + fake store.
- `internal/model/json.go` *(or similar)* — shared result serializer + truncation.

**Touch:**
- `cmd/ragcodepilot/main.go` — `serve` command with `--mcp` flag.
- `internal/search/searcher.go` — only if a structured-result helper is needed
  (retrieval logic unchanged).
- `README.md` — MCP registration section.
- `docs/plan/mvp_roadmap.md` — add/slot this item when sequencing is decided.

## Verification

- **Unit:** tool dispatch with `FakeEmbedder` + fake store — request → schema-valid
  structured result; truncation; error mapping when store returns
  connection errors.
- **Integration (Qdrant + Ollama up):** index this repo, start the server,
  drive it with a scripted MCP client (the SDKs ship test clients):
  `search_code("where is the sparse tokenizer")` returns
  `internal/embedding/sparse.go` in top-5 — mirroring an existing golden query.
- **Parity check:** for 5 golden queries, MCP `search_code` results equal
  CLI `search --limit 5` results (same order, same scores). Guards against the
  wrapper forking the retrieval path.
- **Dogfooding gate (the one that matters):** in a Claude Code session with the
  server registered, the agent chooses `search_code` unprompted for
  code-location questions and the retrieved chunks are used in its answer.
  Record transcript observations here, not metrics — this is a product-fit
  check, not an eval.

## Risks & tradeoffs

- **Agents may prefer grep anyway.** If the tool description doesn't convince
  the agent to call it, leverage is zero. Mitigation: dogfood early (step 6),
  iterate on the description; worst case the server is still useful via
  explicit user instruction.
- **Stale index = confidently wrong answers.** An agent trusts tool output more
  than a human scanning results. Mitigation: `search_code` response includes
  the collection's last-indexed timestamp; v1 pairs naturally with the
  multi-repo staleness UX (production_readiness doc §2.4) and `refresh_repo`.
- **New dependency (MCP SDK).** Scoped to `internal/mcp`; the rest of the
  binary doesn't import it.
- **Long-running process + Ollama restarts.** The embedder client must
  tolerate Ollama restarting mid-session (reconnect per request — already the
  case with per-call HTTP).
- **Scope creep toward a daemon.** stdio-per-client keeps us honest: no ports,
  no lifecycle manager. If someone asks for a shared always-on server, that's
  the §5.3 daemon discussion — a separate decision, not this doc growing.

## Open questions

1. **Multi-collection routing:** v0 serves one collection per server process
   (one `--collection` flag). Should `search_code` accept a `collection`
   parameter instead, so one server spans repos indexed into separate
   collections? Leaning yes-in-v1, keyed off `list_collections` output.
2. **Should `limit` default higher for agents than humans?** Agents tolerate
   more results than a terminal reader; default 5 may under-serve. Decide from
   step 6 dogfooding transcripts.
3. **Sequencing vs Phase 6 / cheaper levers** — owned by `mvp_roadmap.md`.
   This doc has no retrieval-quality dependency; it wraps whatever the current
   best retrieval path is, and automatically benefits from later levers.
