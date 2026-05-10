// Package search handles query processing, embedding, and result formatting.
package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/dinhvy/ragsearch/internal/embedding"
	"github.com/dinhvy/ragsearch/internal/model"
	"github.com/dinhvy/ragsearch/internal/qdrant"
)

// Searcher coordinates query embedding, vector search, and result formatting.
type Searcher struct {
	client   *qdrant.Client
	embedder embedding.Embedder
}

// NewSearcher creates a new Searcher with the given Qdrant client and embedder.
func NewSearcher(client *qdrant.Client, embedder embedding.Embedder) *Searcher {
	return &Searcher{
		client:   client,
		embedder: embedder,
	}
}

// Search embeds the query, validates dimensions, searches Qdrant, and returns results.
func (s *Searcher) Search(ctx context.Context, collection, query string, limit uint64, languages, repos []string) ([]model.SearchResult, error) {
	// Embed the query text.
	vectors, err := s.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}

	// Validate the query vector.
	queryDim, err := embedding.ValidateVectorBatch(vectors, 0)
	if err != nil {
		return nil, fmt.Errorf("validating query vector: %w", err)
	}

	// Validate collection dimension matches the query vector.
	if err := s.client.ValidateCollectionVectorSize(ctx, collection, uint64(queryDim)); err != nil {
		return nil, fmt.Errorf("validating collection dimension: %w", err)
	}

	// Search Qdrant.
	results, err := s.client.Search(ctx, collection, vectors[0], limit, languages, repos)
	if err != nil {
		return nil, fmt.Errorf("searching qdrant: %w", err)
	}

	return results, nil
}

// FormatResults formats search results for terminal display.
func FormatResults(results []model.SearchResult) string {
	if len(results) == 0 {
		return "No results found."
	}

	var b strings.Builder
	for i, r := range results {
		fmt.Fprintf(&b, "\n--- Result %d (score: %.4f) ---\n", i+1, r.Score)
		fmt.Fprintf(&b, "Repo: %s  File: %s  Lang: %s\n", r.Chunk.Repo, r.Chunk.FilePath, r.Chunk.Language)
		fmt.Fprintf(&b, "Lines %d-%d", r.Chunk.StartLine, r.Chunk.EndLine)
		if r.Chunk.Name != "" {
			fmt.Fprintf(&b, "  Name: %s", r.Chunk.Name)
		}
		fmt.Fprintf(&b, "\n\n%s\n", r.Chunk.Content)
	}

	return b.String()
}
