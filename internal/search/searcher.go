// Package search handles query processing, embedding, and result formatting.
package search

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dinhvy/ragcodepilot/internal/embedding"
	"github.com/dinhvy/ragcodepilot/internal/model"
	"github.com/dinhvy/ragcodepilot/internal/qdrant"
)

// Searcher coordinates query embedding, vector search, and result formatting.
type Searcher struct {
	client   *qdrant.Client
	embedder embedding.Embedder
}

// Timings holds per-stage latencies for a single search call. Used by the eval
// harness to break down where time is spent without duplicating the search path.
type Timings struct {
	Embed  time.Duration
	Qdrant time.Duration
	Total  time.Duration
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
	results, _, err := s.SearchWithTimings(ctx, collection, query, limit, languages, repos)
	return results, err
}

// SearchWithTimings is identical to Search but also returns per-stage latencies.
// Used by the eval harness so it runs through the exact same code path as
// production search while still capturing where time is spent.
func (s *Searcher) SearchWithTimings(ctx context.Context, collection, query string, limit uint64, languages, repos []string) ([]model.SearchResult, Timings, error) {
	var t Timings
	totalStart := time.Now()

	// Embed the query text.
	embedStart := time.Now()
	vectors, err := s.embedder.Embed(ctx, []string{query})
	t.Embed = time.Since(embedStart)
	if err != nil {
		t.Total = time.Since(totalStart)
		return nil, t, fmt.Errorf("embedding query: %w", err)
	}

	// Validate the query vector.
	queryDim, err := embedding.ValidateVectorBatch(vectors, 0)
	if err != nil {
		t.Total = time.Since(totalStart)
		return nil, t, fmt.Errorf("validating query vector: %w", err)
	}

	// Validate collection dimension matches the query vector.
	if err := s.client.ValidateCollectionVectorSize(ctx, collection, uint64(queryDim)); err != nil {
		t.Total = time.Since(totalStart)
		return nil, t, fmt.Errorf("validating collection dimension: %w", err)
	}

	// Search Qdrant.
	qdrantStart := time.Now()
	results, err := s.client.Search(ctx, collection, vectors[0], nil, qdrant.SearchModeDense, limit, languages, repos)
	t.Qdrant = time.Since(qdrantStart)
	t.Total = time.Since(totalStart)
	if err != nil {
		return nil, t, fmt.Errorf("searching qdrant: %w", err)
	}

	return results, t, nil
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
