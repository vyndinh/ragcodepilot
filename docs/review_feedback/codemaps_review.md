# Codemaps-Inspired Explore Mode — Review & Pushback

Honest feedback on `docs/brainstorm/codemaps_analysis.md`.
The idea has good bones; the proposal is over-scoped and quietly softens guardrails the previous vision review put in place.

---

## TL;DR

1. **The proposal has good bones.** Splitting `search --answer` from `explore`, building the call graph at indexing time, and the graceful LLM fallback are all the right instincts.
2. **It significantly understates the engineering cost.** Static call graph over Go interface dispatch requires `go/types` (the current chunker only uses `go/parser`); clustering "into logical parts" is hand-waved; bubbletea TUI is a new dependency surface.
3. **Codemaps' core value comes from visual graphs in an IDE.** A terminal TUI rendering the same data is fundamentally a lesser UX. This is "Codemaps-inspired information architecture," not "Codemaps in a CLI."
4. **The proposed edits to `system_vision_review.md` soften the very guardrails that protected the project from scope creep.** Pushing TUI from "avoid" to "now planned" because of one feature is moving goalposts.
5. **Recommend a graduated rollout:** v0 = 1-week `--show-callers/callees` flag on `search`. v1 = interactive `explore` command via stdin-loop with ANSI clear (no bubbletea, no new deps). v2 (bubbletea) only when a real structural limitation hits — not on user request alone.

---

## 1. What the doc gets right

| Decision | Why it's right |
|---|---|
| Splitting `search --answer` from `explore` | They answer different questions ("what does this do?" vs "how is this structured?"). Keeping them separate avoids overloading one command. |
| Call graph built at indexing time, stored as Qdrant payload | Right place — query-time should be fast; expensive analysis happens once. |
| LLM fallback ("mechanical default, LLM when available") | Pragmatic; doesn't make the feature LLM-gated. |
| Acknowledging Codemaps and ragcodepilot have different rendering media | Honest framing (though see §2.7 — this is understated). |
| Adding a Mode-2 eval for structural quality | The instinct is right: structural features need structural metrics. |
| Indexing-time vs query-time phase diagram | Clarifies what runs once vs per-query. Good design discipline. |

These are real contributions to the thinking. Don't discard them; the criticisms below are about cost and sequencing, not about whether the design ideas are sound.

---

## 2. What the doc understates

### 2.1 Static call graph extraction in Go is harder than "walk `CallExpr` nodes"

The current chunker (`internal/ingest/chunker_go.go`) imports `go/ast`, `go/parser`, `go/token`. It does **not** import `go/types`. That matters because:

- `go/parser` gives you the syntactic call site (e.g. `embedder.Embed(ctx, texts)`). It does **not** tell you which concrete type `embedder` is.
- Resolving that requires `go/types`, which type-checks the whole package and needs all dependencies importable. That's a different, heavier machinery than the current parse-and-walk approach.
- Interface dispatch is **fundamentally approximate**. A call on an interface receiver can dispatch to any type implementing it. You can sometimes infer via local construction (`var e Embedder = NewOllamaEmbedder(...)`), but not in general — especially across package boundaries.
- Anonymous functions, function values stored in fields, channels (`go func()`), goroutines, reflection, code-gen, build-tagged files — all break naive static analysis.

The brainstorm's own example shows `Run` calling `Embedder.Embed`. That's an interface call. The doc presents it as if it resolves cleanly. In real codebases, expect a sizeable fraction of edges to be missing or wrong, and the "execution flow ordering" in Explore Mode will look authoritative but be lossy.

**Suggestion:** before committing to the architecture, prototype a call graph extractor on a 10-20 file Go package and measure: what fraction of CallExprs do you resolve to a unique target? What fraction stay ambiguous? Use that to set expectations.

### 2.2 "Cluster into logical parts" is the hand-wave

The doc says "cluster by call-graph connectivity, order by execution flow." That's a sketch, not a design.

Real call graphs have:

- **Cycles** — mutual recursion, callback patterns, retry loops.
- **Diamonds** — multiple paths to the same function (utility helpers, common error paths).
- **Multiple entry points** — `main`, test mains, exported library APIs all root different traversals.
- **Noise** — test helpers, dead code, generated stubs.

Ragcodepilot's pipeline is unusually linear. Pick three other repos (something webby, something CLI, something library-style) and look at their call graphs. The "Indexing Pipeline / Search Pipeline" cleanness in the doc's examples is not representative.

**Suggestion:** name a concrete algorithm and validate it on 3-5 real repos before designing the UX around its output. Candidates:

- Louvain community detection on the call graph (with edge weights).
- Topological order on the SCC-condensed graph (cycles collapsed to single nodes).
- Hierarchical clustering on a combined cosine-similarity + structural-distance metric.

None of these is obvious — you have to try them.

### 2.3 LLM titles carry most of the user-perceived quality

Compare the two outputs in the brainstorm:

- **With LLM:** `1. Index Command: CLI Entry to Pipeline Initialization`
- **Without LLM:** `1. cmd/ragcodepilot/main.go — main, resolveEmbedder, runIndex`

The fallback is `ls` + `grep`. The brainstorm claims "structural grouping, ordering, and drill-down views are identical in both cases. Only the labels change." That's technically true and practically misleading. The labels are 60-70% of what makes the feature feel like Codemaps.

**Suggestion:** state honestly that this feature is most valuable when LLM is present. The fallback is for graceful degradation, not for feature parity. Don't lean on the fallback as a defense of the feature's value.

### 2.4 Eval Mode 2 needs expensive ground truth

The proposed metrics (`part_count_accuracy`, `part_coverage`, `part_ordering`, `part_coherence`, `cross_reference_accuracy`, `irrelevant_inclusion`) all require labeled topics. For each topic, somebody (probably you) needs to decide:

- The "right" number of parts.
- Which files belong to which part.
- The expected execution order.
- What counts as "related" for coherence.

Realistic cost: 1-2 hours of careful labeling per topic. 20 topics → 20-40 hours of one-time labeling, plus relabeling whenever the chunker or clustering changes. The labels also bake in a single human's mental model — different people will disagree on the "right" decomposition.

**Suggestion:** start with 3-5 topics. Compute the metrics. See if they actually discriminate good from bad designs before scaling to 20.

### 2.5 The TUI choice is a spectrum, not a binary

The brainstorm proposes bubbletea — the most expensive option on a wide spectrum. The original framing of this review reacted by recommending the cheapest (no interactivity, re-invoke with `--part N`). Both extremes miss the middle ground:

| Option | What it is | New dep? | Effort |
|---|---|---|---|
| stdin read-line loop | Print menu → read key → re-render. Like `git add -p`. | None | 2-3 days |
| stdin loop + ANSI clear | Above plus `\033[2J\033[H` and reverse-video selection. Like `gh auth login`. | None | 2-3 days |
| Minimal bubbletea | Single list view, no panes, no mouse. | bubbletea, lipgloss | ~1 week |
| Full bubbletea | Multi-pane + scrolling preview + search-within. | bubbletea + viewport + textinput + lipgloss | 2-3 weeks |

All four deliver keyboard shortcuts. The first two do it without a framework.

**How `\033[2J\033[H` works:** These are two ANSI escape codes that every modern terminal understands. `\033` is the ESC character (ASCII 27). `[2J` means "erase entire display." `[H` means "move cursor to row 1, column 1 (home)." Combined, they clear the screen and put the cursor at the top-left — so the next print starts fresh. This gives the illusion of a TUI redraw with zero dependencies.

Pseudocode:

```
loop forever:
    clear the terminal screen and move cursor to top-left
    render the current part's content to screen
    print navigation hint: "[n] Next  [p] Prev  [1-6] Jump  [q] Quit"

    wait for a single keypress from the user

    if key is 'n' → advance to next part
    if key is 'p' → go back to previous part
    if key is 'q' → exit the loop and return
    if key is '1' through '6' → jump directly to that numbered part
```

This is the same pattern `git add --patch` and `gh auth login` use. The user sees an interactive menu with keyboard navigation. Under the hood, it's just `fmt.Print` + `os.Stdin` — no framework, no dependencies, fully testable by injecting a fake stdin.

The full spectrum:

```
No interactivity        ← cheapest                    most expensive →        Full TUI
      │                       │                              │                     │
  --part N flag        stdin read-line              stdin + ANSI clear         bubbletea
  (re-invoke)          (like git add -p)            (like gh auth login)     (like lazygit)
      │                       │                              │                     │
  prints text,         shows menu,                  clears screen,           full framework,
  exits                waits for key,               redraws in-place,        panes, scroll,
                       reprints below               feels like a TUI         mouse, resize
```

**Suggestion:** v1 of Explore Mode should be the **stdin-loop with ANSI clear** option — ~100-200 lines of one Go file, no new dependencies, fully testable via injected stdin. Upgrade to bubbletea only when a real structural limitation hits (split pane, scrolling code preview, resize/mouse, fuzzy search inside the TUI). The gate for v2 is **limitation hit, not user request alone** — a single user saying "I want a fancier UI" isn't enough; the bottleneck has to be real.

The cost critique against bubbletea still applies — Model-View-Update state, opaque test surface, dependency commitment — but it applies to *building bubbletea now*. Building bubbletea *later, after a stdin-loop version has proven the interaction model* is a much smaller bet.

### 2.6 Performance budget ignores cold-start

The doc lists ~285ms total, with 200ms for LLM titles. Reality:

- Ollama loading a model on first call is 2-5 seconds, sometimes 10+ if the model isn't pinned in memory.
- Embedding cold-start is also non-trivial (~500ms-1s for `nomic-embed-text` on a typical laptop).
- Qdrant cold-start (first gRPC connection) adds another ~50-100ms.

The "warm" budget is ~285ms. The user's first invocation of `explore` is materially slower — well into "the tool feels sluggish" territory.

**Suggestion:** budget both warm and cold separately. Consider keeping Ollama models pinned (`OLLAMA_KEEP_ALIVE=-1`) and noting that in docs.

### 2.7 Codemaps' visual graphs are not interchangeable with text trees

The doc says "different rendering medium, same data." True for data. False for UX.

Visual graphs let you see structure at a glance — relationships are spatial, you take them in parallel. Text trees serialize structure into a sequential reading order. The information is the same; the cognitive cost of recovering structure from it is much higher.

This is part of why Codemaps is IDE-integrated. The brainstorm's terminal-based version inherits the data architecture and loses the UX punch.

**Suggestion:** reframe the ambition. "Codemaps-inspired information architecture for the terminal" is honest and defensible. "Codemaps in a CLI" sets up false expectations.

---

## 3. Where the doc conflicts with `system_vision_review.md`

The brainstorm's §"Impact on system_vision_review.md" proposes seven edits. I think most should be paused, not committed.

| Brainstorm proposes | Vision review said | My verdict |
|---|---|---|
| Update Q1 to include "explore codebase structure through hierarchical drill-down" | Outward product = ranked source-code chunks | **Conditional yes.** Only after Explore Mode ships and dogfooding shows it's used. Don't promise capabilities in the system description before they exist. |
| Q3 gaps "superseded" by Explore Mode | No file/repo overview; no traversal | **No.** Don't mark gaps as solved before the solution ships. Add a footnote ("tracked in Explore Mode proposal") rather than strike them. |
| Q5 #7 ("bare result formatting") reframed | P3 weakness | **No.** Result formatting (JSON, context lines, group by file) and Explore Mode are different things. Better formatting is still its own P3 item independent of structural exploration. |
| Q6 "File-level summary / TOC" upgraded from P3/S to P2/M-L | P3, small effort | **Mixed.** A priority bump is reasonable *if* you commit to Explore Mode. But the doc is upgrading a 1-week feature into a multi-month feature — that's a different item, not the same one reprioritized. Add Explore Mode as a new row; leave the TOC row alone. |
| Q8 "avoid TUI" → reinterpret as "TUI is now OK for explore mode" | "CLI works; UI is polish, not value" | **Partial pushback — narrow the ban, don't blanket-relax it.** The vision review's ban was protecting against premature commitment to a TUI framework (bubbletea) or a web UI — that protection still applies to the brainstorm's bubbletea proposal. But a stdin-loop interactive mode (see §2.5) doesn't violate the ban's spirit: no framework, no new dependencies, ~100-200 lines of one file, fully testable. Distinguish the two: stdin-loop interactivity is fine for v1; bubbletea remains gated on a real structural limitation (split pane, scrolling code preview, etc.), not on user request alone. The vision review should be **narrowed** to "avoid TUI frameworks before retrieval quality is solid" — not relaxed to "TUI is now planned." |
| Q8 "cross-repo call graphs = over-engineering" → "intra-repo OK" | Cross-repo was the original concern | **Yes, the original was about cross-repo.** Intra-repo is fine. But "intra-repo Go-only call graphs with type resolution" is still significant work — flag the real cost rather than reclassifying as small. |
| Q10 add Phase 4.5 for Explore Mode | 6 phases, Phase 1 = eval harness | **Conditional yes, with two caveats.** (a) Phase 1 (eval) must actually be built before Phase 4.5 starts — the brainstorm acknowledges this in one line, then proceeds to spec the feature in detail anyway. (b) Phase 4.5 will eat 4-8 weeks, not "between Phase 4 and Phase 5." Update the timeline honestly. |

**Meta-point:** the brainstorm is doing what brainstorms tend to do — getting excited about a new idea and quietly relaxing the constraints that were inconvenient. The vision review's job was to be those constraints. If one feature can override two of them, the review wasn't doing real work. Resist this drift.

---

## 4. Is integrating Codemaps-like visuals realistic?

| Scope | Realistic? | Cost |
|---|---|---|
| Go-only, intra-repo, non-interactive, mechanical titles | **Yes** | ~2 weeks for v0 (callers/callees flag) + ~2-3 weeks for v1 (text-rendered `explore`) |
| + LLM-generated titles | **Yes** | +3-5 days |
| + bubbletea TUI with drill-down | **Possible but discouraged** | +1-2 weeks, plus ongoing test/maintenance burden |
| + Multi-language (Python/JS/TS/Rust) call graphs | **Not in this calendar year** | Tree-sitter + per-language call resolvers; months of work; uncertain accuracy |
| + Visual graph rendering like Codemaps | **No** | Terminal can't compete with IDE-rendered graphs. Different medium. |
| + Codemaps UX parity | **No** | The IDE integration is much of the value. CLI can't match. |

**Direct answer to "is this realistic?":** Yes for a Go-only, non-TUI, v0-scoped version. No for the brainstorm's full ambition in any reasonable timeline. Reframe as "Codemaps-inspired information architecture" — a different, smaller product that's defensible on its own.

---

## 5. Suggested smaller path

A conservative sequence that lets you bail at each step:

**v0 — 1 week.** Add `--show-callers` and `--show-callees` flags to existing `search` command. For Go results, print one extra line per result with the function's callers and callees, populated at index time. No clustering, no TUI, no LLM. **Validation question:** does structural context next to each result subjectively improve "I understand this result"?

**v1 — 2-3 weeks (only if v0 shows value).** Add `ragcodepilot explore <topic>` as an interactive stdin-loop command. Print menu → read keypress → re-render. Use ANSI clear (`\033[2J\033[H`) and reverse-video for selection. No bubbletea, no new dependencies. Testable via injected stdin.

**v1.5 — 3-5 days.** Add Ollama-generated titles to v1's output. Mechanical fallback preserved.

**v2 — defer until a real limitation hits.** Migrate from the stdin loop to minimal bubbletea when one of these is true: (a) you want a split pane with parts list + scrollable code preview, (b) resize/mouse handling becomes a problem, (c) fuzzy search inside the TUI is needed. Not "a user said they want it" — the structural limitation has to be real.

This sequence:

- Each step is evaluated before committing to the next.
- `--show-callers/callees` is independently valuable even if v1+ never ships.
- All expensive items (clustering algorithm validation, TUI, multi-language) are deferred past the point where you'd know they're worth it.

---

## 6. Strategic context: does this beat hybrid search + reranking?

Per the vision review, the single biggest weakness of ragcodepilot today is retrieval quality:

- No eval harness (you can't measure).
- No hybrid search (vector-only misses exact-symbol queries).
- No reranking (top-K from one embedder is the final answer).

Explore Mode is **downstream** of "are the retrieved results accurate?" If you build structural grouping on top of mediocre retrieval, the groups will look authoritative while pointing at the wrong code. Sequence matters:

1. Phase 1 — eval harness.
2. Phase 2 — hybrid search.
3. Phase 3 — reranking.

Do not reorder. Explore Mode comes after these, not in parallel. The brainstorm acknowledges this in passing; the rest of the doc doesn't behave like it.

---

## 7. Concrete fixes to the brainstorm doc

Line-referenced to `docs/brainstorm/codemaps_analysis.md`:

| Lines | Fix |
|---|---|
| ~232 (LLM title table row) | "Generate titles → Yes" understates that title quality drives most of the perceived value. Add a sentence noting the feature degrades meaningfully without LLM. |
| 251-258 (Without-LLM example) | Be honest that this output is closer to `ls + grep` than to Codemaps. Don't claim equivalence with the LLM version. |
| 200-218 (Implementation approach) | Pick a concrete clustering algorithm and reference it. "Cluster by connectivity" is not a design. |
| 372-391 (Payload schema) | `calls: [...]` and `called_by: [...]` need to specify whether they include the fully-qualified package path. Without it, name collisions across packages will silently merge. |
| 446-457 (Performance budget) | Add a cold-start row. State first-invocation latency separately from warm-path. |
| 605-655 (Eval metric specs) | For each metric, state whose labor produces the ground truth and how long. This is the real bottleneck, not the math. |
| 498-559 (Edits to `system_vision_review.md`) | Downgrade most of these to "pending validation by dogfooding" rather than commit-now edits. See §3 above for per-edit verdict. |

---

## 8. Final verdict

**The idea is good. The proposal is over-scoped. The scope-creep on `system_vision_review.md` needs to be reversed.**

Three concrete next steps:

1. **Build the eval harness (Phase 1)** — the vision review's actual P1. Without it, you can't tell if Explore Mode or any other change helps.
2. **Build hybrid search + reranking (Phases 2-3)** — fix retrieval before adding visualization on top.
3. **Build v0 of Explore Mode (`--show-callers/callees`)** as a one-week experiment. Decide whether to escalate to v1+ based on dogfooding evidence.

**Do not edit `system_vision_review.md` to accommodate Explore Mode until at least v1 has shipped and proven its value.** The vision review's guardrails are doing real work; relaxing them prematurely is how this project ends up with three half-built features and no users.
