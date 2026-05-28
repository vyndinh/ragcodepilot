# Architecture & Performance Decisions

> Reference doc for trade-off analyses on architecture and performance shape — *how the system is organized as processes*, not how retrieval is scored. Add to it as the system evolves; each section is a self-contained analysis you can revisit before making the next decision.
>
> Currently covers:
>
> 1. **CLI vs daemon** — why ragcodepilot stays a thin CLI for now
> 2. **Per-invocation cost budget** — what each `search` actually pays for
> 3. **Incremental indexing** — when (and how) to add a watch mode without inventing a daemon
> 4. **Cold-start latency** — the real user-visible pain and the cheap fix
> 5. **When to revisit** — concrete triggers for adding a daemon or REPL

**Companion docs:**

- [`../plan/mvp_roadmap.md`](../plan/mvp_roadmap.md) — phase plan and product direction
- [`../plan/phase5_v0_answer_mode.md`](../plan/phase5_v0_answer_mode.md) — current generation work; §v2+ REPL mode is the related future surface
- [`retrieval_quality_decisions.md`](retrieval_quality_decisions.md) — the *quality* trade-off doc (this one is the *shape* trade-off doc)

---

## Table of contents

1. [CLI vs daemon — why we stay a thin CLI](#1-cli-vs-daemon--why-we-stay-a-thin-cli)
2. [Per-invocation cost budget](#2-per-invocation-cost-budget)
3. [Incremental indexing without a daemon](#3-incremental-indexing-without-a-daemon)
4. [Cold-start latency: the real pain](#4-cold-start-latency-the-real-pain)
5. [When to revisit — triggers for a daemon or REPL](#5-when-to-revisit--triggers-for-a-daemon-or-repl)
6. [Bottom line](#6-bottom-line)

---

## 1. CLI vs daemon — why we stay a thin CLI

### 1.1 The general argument for a daemon

The intuition (correct for IDE-class products like Cursor, Sourcegraph, Aider in server mode):

> A long-running backend keeps state hot. Per-command invocations reload config, reconnect to DB, check repo state, warm caches, and re-initialize clients. A daemon avoids all of that.
>
> ```
> CLI command → local daemon → RAG engine already running → vector DB / cache / index
> ```

### 1.2 Why it doesn't fit ragcodepilot today

**The long-running daemon already exists — it's Qdrant + Ollama.** Adding a third process whose only job is to forward to two existing daemons buys very little.

Concretely:

- **Index state** lives in **Qdrant**'s persistent collection. The CLI doesn't *load* an index — it queries one. There is almost nothing for a ragcodepilot daemon to keep in memory between calls.
- **Embedding model state** lives in **Ollama**. With `OLLAMA_KEEP_ALIVE=-1`, `nomic-embed-text` (and the future generative model) stay loaded across calls. Ollama is the warmup target, not ragcodepilot.
- **Per-command init cost is dominated by things a daemon can't help with** — see §2.

The CLI is intentionally the thin frontend. Its job is to parse flags, embed one query, call Qdrant, format results. None of those are stateful enough to justify a separate process.

### 1.3 Costs a daemon would add

1. **IPC layer** (Unix socket / loopback HTTP / gRPC) — new failure surface, new versioning concern (CLI version vs daemon version mismatch).
2. **Lifecycle management** — start-on-demand vs always-on, macOS `launchd` integration, port/socket allocation, stale-process cleanup, "why is the daemon hung" support burden.
3. **State invalidation bugs** — if the daemon caches anything (config, collection metadata, repo path), it must invalidate when the underlying file/collection changes. This is the class of bug that makes daemons painful.
4. **Loss of "it's just a Go binary"** — a meaningful product property today.

### 1.4 What a daemon *would* actually help with

Being fair to the proposal — these are the real wins, not the imagined ones:

- **File watching for incremental indexing** (the strongest case — see §3).
- **Cross-frontend coordination** if ragcodepilot ever has more than one frontend (CLI + editor plugin + web UI) sharing one index pipeline.
- **OS-level integration** (auto-start on login, status bar item).

None of these are true today. The CLI is the only frontend, indexing is invoked manually, and there's no OS integration story. Daemon = solving problems we don't have.

---

## 2. Per-invocation cost budget

A daemon's value depends on what each CLI command actually pays for at startup. Honest numbers for `ragcodepilot search "query"`:

| Cost | Magnitude | Notes |
|---|---|---|
| Go process start | ~10 ms | Static binary, cold cache slightly more. |
| YAML config parse (`config.yaml`) | <1 ms | Small file. |
| Qdrant gRPC dial to `localhost:6334` | few ms | TCP + gRPC handshake. |
| Ollama HTTP client construction | ~0 ms | Just a struct; no connect. |
| First Ollama call (warm model) | 10–50 ms | Embedding `nomic-embed-text` on a query. |
| First Ollama call (cold model) | **5–30 s** | Model load — *not* something a ragcodepilot daemon fixes. |
| Qdrant hybrid search | ~28 ms p50 (`baseline_v4`) | The actual retrieval cost. |
| `--answer` LLM generation | 1–5 s typical, more on cold start | Dwarfs everything else; daemon can't help. |
| **Total warm-path retrieval** | **~50–100 ms** | The thing a daemon would speed up. |
| **Total warm-path `--answer`** | **~1–5 s** | Dominated by generation. |

**Daemon savings, optimistically:** 10–30 ms off the warm path. That's perceptible only if you're chaining many queries in a script — and even then, the right answer is a REPL or a script that batches, not a daemon.

The user-visible slow path is **model load** (5–30 s on cold start), and the fix is `OLLAMA_KEEP_ALIVE=-1` — see §4.

---

## 3. Incremental indexing without a daemon

This is the **one criterion from the original proposal that genuinely holds up**: if the repository changes often, re-running `ragcodepilot index` after every change is annoying, and ad-hoc re-indexing means `search` may see stale data.

### 3.1 The naive daemon answer

```
file watcher (in daemon) → change queue → background re-index → next search sees fresh data
```

This is what the original proposal had in mind. It works, but it requires the entire daemon scaffolding (§1.3).

### 3.2 The simpler answer: `index --watch`

A single long-running mode of the existing CLI:

```
ragcodepilot index --watch --language go .
```

Internally:

```
function RunIndexWatch(repoPath, languages):
    initial_index(repoPath, languages)        // reuse existing pipeline
    watcher = fsnotify.NewWatcher(repoPath)
    for event in watcher.Events:
        if shouldReindex(event.path, languages):
            debounce(event.path)
            reindex_files(changed_files)      // upsert affected chunks only
```

Properties:

- **One process, owned by the user.** Run in a tmux pane or background it; kill it when you don't want it.
- **No IPC.** No new failure modes between processes.
- **No version-mismatch.** The same binary that watches is the one that indexes; if you upgrade ragcodepilot, you restart `index --watch`. Done.
- **Reuses the existing pipeline verbatim.** The chunker, embedder, and Qdrant client are unchanged; we just wrap them in a loop.
- **Same UX pattern as `tsc --watch`, `cargo watch`, `webpack --watch`.** Users understand this shape.

### 3.3 Effort and trade-offs

| Item | Estimate | Notes |
|---|---|---|
| `index --watch` MVP | **S–M** | `fsnotify`, debounce, chunk-level upsert/delete on file change. |
| Per-file incremental re-index | S (already partially there) | Existing pipeline handles per-file upserts; needs delete-on-removed-file. |
| Daemon equivalent | **L** | Plus IPC, lifecycle, versioning, OS integration. |

**Verdict:** when incremental indexing becomes a real friction, build `index --watch`, not a daemon. Reserve the daemon for the day there are multiple frontends (§5).

---

## 4. Cold-start latency: the real pain

Per [phase5_v0_answer_mode.md §Risks](../plan/phase5_v0_answer_mode.md), the first call after Ollama starts is **5–30 seconds** while the model loads. Subsequent calls are 1–5 s for generation, ~50–100 ms for embedding-only retrieval.

**This is the latency users actually feel**, and it has nothing to do with the CLI vs daemon question — it's about whether Ollama keeps the model resident.

### 4.1 The fix

Document and recommend:

```
export OLLAMA_KEEP_ALIVE=-1
```

(or a long duration like `24h` if `-1` is too aggressive for the user's hardware).

This pins the model in Ollama's process. Every ragcodepilot call after the first hits a warm model. Cost: a few GB of RAM held by `ollama serve`.

### 4.2 Why this matters more than the daemon question

A daemon saves ~10–30 ms of Go startup. `OLLAMA_KEEP_ALIVE=-1` saves **5–30 seconds** on the cold path. The order of magnitude isn't even close.

If the user's complaint is "ragcodepilot feels slow," the answer is almost always "the model wasn't loaded," not "the CLI re-initialized."

### 4.3 Adjacent fixes

- **Auto-warm before `--answer`** ✅ **shipped.** `runSearch` pre-loads the generative
  model via the optional `answer.Warmer` interface (`OllamaGenerator.Warmup` sends an
  empty-messages `/api/chat` load request) before the timed `Generate` call. This pulls
  the cold-start cost out of the generation timeout — important because with `stream:false`
  the client timeout must cover the *entire* generation, so bundling model-load into it was
  tripping the timeout. Combined with `OLLAMA_KEEP_ALIVE`, the warm model persists across
  invocations. Generators that don't need warming (FakeGenerator, future cloud providers)
  simply don't implement `Warmer` and are skipped via type assertion.
- **Warmup ping in `index --watch`** (later): if the user is running watch mode anyway, send
  a tiny embedding call once at start to ensure the embedder is loaded before the first
  `search` arrives.
- **`ragcodepilot warmup` subcommand** (later): explicit one-liner that pings the embedding
  and generative models so the user can warm them before a demo, without issuing a real query.
- **Streaming `--answer`** (v1 candidate, pull forward if sync wait dominates dogfooding pain):
  `stream:true` makes time-to-first-token the relevant latency instead of total generation,
  and sidesteps the awaiting-headers timeout entirely.

Both S, both optional, neither requires a daemon.

---

## 5. When to revisit — triggers for a daemon or REPL

These are the concrete signals that would flip the trade-off. Until at least one fires, the CLI-only shape is right.

### 5.1 Triggers for `index --watch` (small step)

- User re-runs `ragcodepilot index` >3×/day on the same repo.
- Searches return stale chunks more than rarely.
- The repo is large enough that a full re-index takes >30 s.

Effort: **S–M**. Already discussed in §3.

### 5.2 Triggers for REPL mode (`ragcodepilot chat`)

The criteria are already documented in [phase5_v0_answer_mode.md §v2+: REPL mode](../plan/phase5_v0_answer_mode.md):

- ≥3 follow-up `--answer` queries per session, clearly building on the prior turn.
- Provider/model selection becomes daily friction.
- Generator flag set grows beyond 5–6.
- A user explicitly asks for interactive mode.

The REPL gives "long-running process with hot state" without the IPC layer. It's the right answer to "I want my state to persist across queries" — one process, one user, lifetime bounded by the REPL session.

### 5.3 Triggers for a true daemon

Only when **at least one** of these is real:

- **Multiple frontends.** CLI + editor plugin + web UI all need to share one indexer / one file watcher / one pre-warmed embedder.
- **OS integration.** Auto-start on login, status bar item, system tray, file-association on macOS.
- **Cross-process coordination.** One shared watch loop serving many editors, or shared rate-limiting against a remote provider.
- **Background work the user doesn't initiate.** Periodic reindex, telemetry export, scheduled evals.

If none of these are real, a daemon is overhead without payoff.

### 5.4 Cost ladder

| Step | Effort | Adds | Removes |
|---|---|---|---|
| `OLLAMA_KEEP_ALIVE=-1` in README | S | Documentation line | The 5–30 s cold start |
| `ragcodepilot warmup` subcommand | S | One file | Explicit warmup before demos |
| `index --watch` mode | S–M | `fsnotify` dep, watch loop, debounce | Manual re-index after every edit |
| `ragcodepilot chat` REPL | M–L | TUI library (`bubbletea`), slash commands, session state | Per-query re-parsing of flags; cold provider/model selection |
| Full daemon | L–XL | IPC, lifecycle, OS integration, versioning, cleanup | Per-process startup (already small); enables multi-frontend |

The ladder is meant to be climbed only as far as evidence justifies. Today, **step 1** is enough.

---

## 6. Bottom line

- **Qdrant + Ollama are already the daemons.** ragcodepilot is intentionally a thin CLI over them. State that would normally live in a backend process (the HNSW index, the loaded embedding model) already lives in those services.
- **Per-command startup is ~10–50 ms.** A daemon saves ~10–30 ms of that — perceptually nothing.
- **The real latency villain is model cold-start (5–30 s).** Fixed by `OLLAMA_KEEP_ALIVE=-1`. No daemon required.
- **Incremental indexing is the one valid driver** for a long-running process. Solve it with `index --watch`, not a daemon — same UX value, none of the IPC overhead.
- **REPL mode is the right "long-running" step after `--watch`.** Already planned for after Phase 5 v0 dogfooding ([phase5_v0_answer_mode.md §v2+: REPL mode](../plan/phase5_v0_answer_mode.md)).
- **Reserve a true daemon for multi-frontend or OS-integration pressure.** Until that pressure is real, it's solving a problem we don't have.

When in doubt: **measure the latency the user actually feels, fix the dominant term first.** Today that's model cold-start, not process startup.
