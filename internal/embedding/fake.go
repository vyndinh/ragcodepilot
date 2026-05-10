package embedding

import (
	"context"
	"math"
	"math/rand"
)

// FakeEmbedder generates deterministic pseudo-random vectors for testing.
// It produces consistent embeddings for the same input text, allowing
// pipeline testing without a real model. Search results will be random
// but the upsert → search flow can be validated end-to-end.
type FakeEmbedder struct {
	dim int
}

// NewFakeEmbedder creates a fake embedder with the given vector dimension.
func NewFakeEmbedder(dim int) *FakeEmbedder {
	return &FakeEmbedder{dim: dim}
}

// Embed generates deterministic pseudo-random vectors seeded from the input text.
// The same text always produces the same vector.
func (f *FakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, len(texts))
	for i, text := range texts {
		vectors[i] = f.deterministicVector(text)
	}
	return vectors, nil
}

// Dimension returns the dimensionality of the generated vectors.
func (f *FakeEmbedder) Dimension() int {
	return f.dim
}

// deterministicVector generates a unit-length vector from the text hash.
// Using a seeded RNG ensures the same text always yields the same vector.
func (f *FakeEmbedder) deterministicVector(text string) []float32 {
	// Seed from text content for determinism.
	var seed int64
	for _, c := range text {
		seed = seed*31 + int64(c)
	}

	rng := rand.New(rand.NewSource(seed))
	vec := make([]float32, f.dim)

	// Generate and L2-normalize so cosine similarity works properly.
	var norm float64
	for j := range vec {
		v := rng.Float64()*2 - 1 // [-1, 1]
		vec[j] = float32(v)
		norm += v * v
	}
	norm = math.Sqrt(norm)
	for j := range vec {
		vec[j] /= float32(norm)
	}

	return vec
}

// Compile-time check: FakeEmbedder implements Embedder.
var _ Embedder = (*FakeEmbedder)(nil)
