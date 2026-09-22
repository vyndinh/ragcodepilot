# MCP Server Mode — Deferred Decision

**Status: deferred, low priority. Updated 2026-09-22.** The proposed M2 milestone
and agent-first pivot were removed from the [active roadmap](mvp_roadmap.md).
No MCP server, SDK choice, tool schema, or implementation schedule is committed.

The current product remains a local code-search CLI with optional answers.
External-repo evidence and indexing reliability have clearer immediate value.
MCP adds a process lifecycle, protocol surface, output budgets, and repository
provenance requirements. There is no recorded task-level adoption evidence that
justifies those costs yet. The earlier detailed draft is available in Git history.

## Revisit conditions

Reopen only if repeated real coding tasks need retrieval from an external index
and the current CLI workflow is a demonstrated obstacle. Compare task correctness,
completion, latency, and context cost with and without the integration. An agent
calling the tool unprompted is not sufficient evidence of benefit.

If reopened, write a fresh bounded proposal that specifies:

- A versioned response schema with repository/worktree identity, resolvable paths,
  chunk IDs, file hashes, and completed-index provenance; timestamps alone do not
  establish freshness.
- Protocol-only stdout, diagnostics on stderr, request deadlines/cancellation,
  bounded concurrency, total output budgets, and startup/recovery behavior.
- Read-only search/metadata first, shared CLI retrieval semantics, and parity
  verification. Do not make answer streaming or GraphRAG prerequisites.
- A separate decision for multiple workspaces or shared services. Existing repo
  basename identifiers can collide; stable identities and dirty/partial-index
  handling must precede any broader promise.

Multi-repo workspace commands and shared/team deployment are also deferred.
Keeping this decision record preserves links without retaining an active feature plan.
