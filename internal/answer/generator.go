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
// It contains enough metadata for the model to cite sources by number and to
// disambiguate chunks that share a file path across repositories.
type ChunkContext struct {
	// Index is the 1-based citation number shown in the prompt (e.g., [1]).
	Index int

	// Repo is the repository name the chunk came from. Rendered as a path
	// prefix in the prompt header so multi-repo collections don't produce
	// ambiguous citations (two repos can share a relative file path).
	Repo string

	// FilePath is the relative file path within the repository.
	FilePath string

	// Language is the programming language ("go", "rust", ...). Carried for
	// downstream use; v0 does not render it in the prompt header because
	// the file extension already conveys it.
	Language string

	// ChunkType describes what the chunk represents ("function", "method",
	// "type", "block", "file"). Used as the label in front of Symbol so the
	// header reads accurately for non-function chunks.
	ChunkType string

	// Lines is the line range as a string (e.g., "42-78").
	Lines string

	// Symbol is the function, method, struct, or other named entity, if any.
	Symbol string

	// Content is the raw source code text of the chunk.
	Content string
}
