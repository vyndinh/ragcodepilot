// Package embedding defines the interface for generating vector embeddings from text.
package embedding

import "context"

// Embedder generates vector embeddings from text input.
// Implementations may call an external API (OpenAI, Cohere) or run a local model.
type Embedder interface {
	// Embed converts a batch of text strings into vector embeddings.
	// Each input string produces one embedding vector.
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// Dimension returns the dimensionality of the embedding vectors.
	Dimension() int
}
