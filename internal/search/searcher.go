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
	client   vectorClient
	embedder embedding.Embedder
}

type vectorClient interface {
	ValidateCollectionVectorSize(ctx context.Context, name string, vectorSize uint64) error
	Search(ctx context.Context, collection string, denseVector []float32, sparseVector *embedding.SparseVector, mode qdrant.SearchMode, limit uint64, languages, repos []string) ([]model.SearchResult, error)
}

// SearchMode controls which retrieval path the searcher uses.
type SearchMode = qdrant.SearchMode

const (
	// SearchModeDense uses only the dense embedding vector.
	SearchModeDense = qdrant.SearchModeDense
	// SearchModeSparse uses only the sparse token vector.
	SearchModeSparse = qdrant.SearchModeSparse
	// SearchModeHybrid fuses dense and sparse retrieval via Qdrant RRF.
	SearchModeHybrid = qdrant.SearchModeHybrid
	// DefaultSearchMode is the user-facing default from the hybrid search plan.
	DefaultSearchMode = SearchModeHybrid
)

// Timings holds per-stage latencies for a single search call. Used by the eval
// harness to break down where time is spent without duplicating the search path.
type Timings struct {
	Embed  time.Duration
	Qdrant time.Duration
	Total  time.Duration
}

// NewSearcher creates a new Searcher with the given Qdrant client and embedder.
func NewSearcher(client vectorClient, embedder embedding.Embedder) *Searcher {
	return &Searcher{
		client:   client,
		embedder: embedder,
	}
}

// ParseSearchMode parses a CLI/search mode string.
func ParseSearchMode(mode string) (SearchMode, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", string(SearchModeHybrid):
		return SearchModeHybrid, nil
	case string(SearchModeDense):
		return SearchModeDense, nil
	case string(SearchModeSparse):
		return SearchModeSparse, nil
	default:
		return "", fmt.Errorf("unknown search mode %q (supported: dense, sparse, hybrid)", mode)
	}
}

// Search prepares the requested query vector(s), searches Qdrant, and returns results.
func (s *Searcher) Search(ctx context.Context, collection, query string, mode SearchMode, limit uint64, languages, repos []string) ([]model.SearchResult, error) {
	results, _, err := s.SearchWithTimings(ctx, collection, query, mode, limit, languages, repos)
	return results, err
}

// SearchWithTimings is identical to Search but also returns per-stage latencies.
// Used by the eval harness so it runs through the exact same code path as
// production search while still capturing where time is spent.
func (s *Searcher) SearchWithTimings(ctx context.Context, collection, query string, mode SearchMode, limit uint64, languages, repos []string) ([]model.SearchResult, Timings, error) {
	var t Timings
	totalStart := time.Now()

	mode, err := normalizeSearchMode(mode)
	if err != nil {
		t.Total = time.Since(totalStart)
		return nil, t, err
	}

	var denseVector []float32
	if mode == SearchModeDense || mode == SearchModeHybrid {
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
		denseVector = vectors[0]

		// Validate collection dimension matches the query vector.
		if err := s.client.ValidateCollectionVectorSize(ctx, collection, uint64(queryDim)); err != nil {
			t.Total = time.Since(totalStart)
			return nil, t, fmt.Errorf("validating collection dimension: %w", err)
		}
	}

	var sparseVector *embedding.SparseVector
	if mode == SearchModeSparse || mode == SearchModeHybrid {
		sv := embedding.TokenizeQuery(query)
		sparseVector = &sv
	}

	// Search Qdrant.
	qdrantStart := time.Now()
	results, err := s.client.Search(ctx, collection, denseVector, sparseVector, mode, limit, languages, repos)
	t.Qdrant = time.Since(qdrantStart)
	t.Total = time.Since(totalStart)
	if err != nil {
		return nil, t, fmt.Errorf("searching qdrant: %w", err)
	}

	return results, t, nil
}

func normalizeSearchMode(mode SearchMode) (SearchMode, error) {
	if mode == "" {
		return DefaultSearchMode, nil
	}
	switch mode {
	case SearchModeDense, SearchModeSparse, SearchModeHybrid:
		return mode, nil
	default:
		return "", fmt.Errorf("unknown search mode %q (supported: dense, sparse, hybrid)", mode)
	}
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
