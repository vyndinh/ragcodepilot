# Function-Level Chunker

## Current state

The chunker routes files by language:

```
ChunkFile()
  ├── Go files    → chunkGoFile()    (go/parser AST, one chunk per function)
  └── Everything  → chunkGeneric()   (40-line sliding window with 5-line overlap)
```

Go files produce precise, function-aligned chunks. All other languages fall back to the sliding window, which can split functions mid-body.

## Why Go-specific first

Go has `go/parser` + `go/ast` in the **standard library** — zero dependencies, battle-tested, handles all Go syntax. Our chunker extracts top-level `*ast.FuncDecl` nodes (functions and methods); function literals (closures) remain inside their parent function's chunk, which is the correct semantic grouping.

For other languages, function-level parsing requires either:

| Approach | Accuracy | Complexity | Dependencies |
|----------|----------|------------|--------------|
| **Go stdlib `go/parser`** | Perfect for Go | Low | None (stdlib) |
| **Regex heuristics** | ~80% for simple languages | Low | None |
| **Tree-sitter** | ~99% for 100+ languages | High | CGo, grammar files per language |
| **Per-language parsers** | High | High | One library per language |

Since this project primarily indexes Go code and is a learning project, the Go AST chunker covers 100% of the target use case at zero cost.

## Roadmap: other languages

### Phase 1 (current) — Go AST ✅

- Uses `go/parser` (stdlib)
- One chunk per `*ast.FuncDecl` (function or method)
- Gap chunks for non-function code (imports, types, vars)
- Large functions (>80 lines) split with sliding window
- Syntax error fallback to `chunkGeneric`

### Phase 2 (next, if needed) — Regex heuristics for Python / Rust

Languages with clear function boundaries can be parsed with regex + indentation/brace counting:

**Python** — `def` / `class` + indentation level:
```
def foo():       ← start of function (indentation 0)
    x = 1        ← body (indentation > 0)
    return x     ← body
                 ← blank line or next def/class = end
def bar():       ← next function
```

**Rust** — `fn` / `impl` + brace counting:
```
fn foo() {       ← start (brace count = 1)
    let x = 1;   ← body
}                ← end (brace count = 0)
```

These heuristics would live in `chunker_python.go` and `chunker_rust.go`, following the same pattern as `chunker_go.go`.

Estimated accuracy: ~80-90%. Fails on edge cases (multi-line decorators, nested functions, macros) but significantly better than the sliding window for typical code.

### Phase 3 (future) — Tree-sitter

For production-quality multi-language support, **Tree-sitter** is the right answer:

- Single API for 100+ languages
- Near-perfect AST parsing
- Used by GitHub, Neovim, Zed, and most modern code tools
- Go bindings: [`smacker/go-tree-sitter`](https://github.com/smacker/go-tree-sitter)

Tradeoffs:
- Adds CGo dependency (complicates builds and cross-compilation)
- Requires grammar `.so` files per language
- Heavier binary size

**Decision**: defer until the project needs to index non-Go repos regularly. The regex approach covers the 80% case without CGo.

## Architecture: how to add a new language

The router in `chunker.go` makes it simple:

```go
func ChunkFile(filePath, repoRoot, repo string, chunkSize, overlap int, cfg *config.Config) ([]model.CodeChunk, error) {
    switch cfg.DetectLanguage(filePath) {
    case "go":
        return chunkGoFile(filePath, repoRoot, repo, chunkSize, overlap, cfg)
    case "python":
        return chunkPythonFile(filePath, repoRoot, repo, chunkSize, overlap, cfg)
    case "rust":
        return chunkRustFile(filePath, repoRoot, repo, chunkSize, overlap, cfg)
    default:
        return chunkGeneric(filePath, repoRoot, repo, chunkSize, overlap, cfg)
    }
}
```

Each language chunker follows the same contract:
1. Read file, detect boundaries
2. Produce `[]model.CodeChunk` with `ChunkType`, `Name`, `StartLine`, `EndLine`
3. Fall back to `chunkGeneric` on error
4. Split oversized blocks with the sliding window

## Files

| File | Purpose |
|------|---------|
| `chunker.go` | Router (`ChunkFile`) + generic sliding window (`chunkGeneric`) |
| `chunker_go.go` | Go AST-based function chunker |
| `chunker_go_test.go` | 8 test cases for Go chunker |
| `chunker_python.go` | (future) Python regex chunker |
| `chunker_rust.go` | (future) Rust regex chunker |

## Feedback (resolved)

- ✅ **Closure wording**: clarified that `go/parser` parses closures but our chunker only extracts `*ast.FuncDecl`; closures stay inside their parent chunk.
- ✅ **Router tests**: added `TestChunkFile_RoutesGoToAST` and `TestChunkFile_RoutesNonGoToGeneric`.
- ✅ **Consistent roadmap**: aligned `system_design.md` and `checklist` to say Go (done) + Python/Rust (next).
- ✅ **gofmt**: ran `gofmt -w` on `chunker.go`.
