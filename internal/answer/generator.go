// Package answer provides LLM-based answer generation over retrieved code chunks.
// It turns ragcodepilot from semantic grep into a RAG system by synthesizing
// natural-language answers from the chunks that hybrid search returns.
package answer

import "context"

// Generator produces a natural-language answer from a prompt containing
// a user question and retrieved code chunks. Implementations may call
// a local model (Ollama) or a remote API.
type Generator interface {
	// Generate sends the prompt to an LLM and returns the generated answer text.
	// The prompt includes the user question and the retrieved chunks as context.
	Generate(ctx context.Context, prompt Prompt) (string, error)
}

// Warmer is an optional interface a Generator may implement to pre-load its
// model before the first Generate call. Warming moves the one-time cold-start
// cost (loading a multi-GB model into memory) out of the timed generation path,
// so Generate's timeout budget covers only generation. Generators that need no
// warmup — FakeGenerator, future stateless cloud providers — simply don't
// implement it; callers should type-assert and skip warming when absent.
type Warmer interface {
	// Warmup loads the underlying model into memory. It is safe to call when the
	// model is already resident (it returns quickly in that case).
	Warmup(ctx context.Context) error
}

// Prompt carries the user question and the code chunks that provide context
// for answer generation. The generator formats these into the LLM request.
type Prompt struct {
	// Question is the user's natural-language query.
	Question string

	// Chunks is the ordered list of retrieved code chunks that the LLM
	// should use as context. Ordered by retrieval relevance (best first).
	Chunks []ChunkContext
}

// ChunkContext is a single retrieved code chunk prepared for the LLM prompt.
// It contains enough metadata for the model to cite sources by number.
type ChunkContext struct {
	// Index is the 1-based citation number shown in the prompt (e.g., [1]).
	Index int

	// Repo is the repository name this chunk came from. Rendered to disambiguate
	// citations when a collection spans multiple repositories; omitted when empty.
	Repo string

	// FilePath is the relative file path within the repository.
	FilePath string

	// Lines is the line range as a string (e.g., "42-78").
	Lines string

	// Symbol is the function, method, or type name, if any.
	Symbol string

	// ChunkType labels the kind of chunk (e.g., "function", "block", "file") so
	// the citation header does not mislabel non-function chunks. When empty, the
	// header falls back to a neutral "symbol" label.
	ChunkType string

	// Content is the raw source code text of the chunk.
	Content string
}
