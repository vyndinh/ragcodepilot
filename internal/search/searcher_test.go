package search

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/dinhvy/ragcodepilot/internal/embedding"
	"github.com/dinhvy/ragcodepilot/internal/model"
	"github.com/dinhvy/ragcodepilot/internal/qdrant"
)

func TestParseSearchMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want SearchMode
	}{
		{name: "empty defaults to hybrid", in: "", want: SearchModeHybrid},
		{name: "dense", in: "dense", want: SearchModeDense},
		{name: "sparse", in: "sparse", want: SearchModeSparse},
		{name: "hybrid", in: "hybrid", want: SearchModeHybrid},
		{name: "trim and lowercase", in: " HYBRID ", want: SearchModeHybrid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseSearchMode(tt.in)
			if err != nil {
				t.Fatalf("ParseSearchMode(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("ParseSearchMode(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseSearchModeRejectsUnknown(t *testing.T) {
	t.Parallel()

	_, err := ParseSearchMode("magic")
	if err == nil {
		t.Fatal("expected error for unknown mode")
	}
	if !strings.Contains(err.Error(), "supported: dense, sparse, hybrid") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearchWithTimingsDenseModeEmbedsAndValidates(t *testing.T) {
	t.Parallel()

	client := &recordingClient{
		results: []model.SearchResult{{Chunk: model.CodeChunk{FilePath: "internal/search/searcher.go"}}},
	}
	embedder := &recordingEmbedder{vectors: [][]float32{{1, 2, 3}}}
	searcher := NewSearcher(client, embedder)

	results, _, err := searcher.SearchWithTimings(
		context.Background(), "code_chunks", "ChunkFile", SearchModeDense, 5, []string{"go"}, []string{"ragcodepilot"},
	)
	if err != nil {
		t.Fatalf("SearchWithTimings() unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	if embedder.calls != 1 {
		t.Fatalf("embedder calls = %d, want 1", embedder.calls)
	}
	if client.validateCalls != 1 || client.validateSize != 3 {
		t.Fatalf("ValidateCollectionVectorSize calls/size = %d/%d, want 1/3", client.validateCalls, client.validateSize)
	}
	if client.mode != qdrant.SearchModeDense {
		t.Fatalf("mode = %q, want dense", client.mode)
	}
	if !slices.Equal(client.dense, []float32{1, 2, 3}) {
		t.Fatalf("dense vector = %v, want [1 2 3]", client.dense)
	}
	if client.sparse != nil {
		t.Fatalf("sparse vector = %#v, want nil", client.sparse)
	}
	if client.limit != 5 || !slices.Equal(client.languages, []string{"go"}) || !slices.Equal(client.repos, []string{"ragcodepilot"}) {
		t.Fatalf("filters/limit not forwarded: limit=%d languages=%v repos=%v", client.limit, client.languages, client.repos)
	}
}

func TestSearchWithTimingsSparseModeSkipsEmbedding(t *testing.T) {
	t.Parallel()

	client := &recordingClient{}
	embedder := &recordingEmbedder{vectors: [][]float32{{1, 2, 3}}}
	searcher := NewSearcher(client, embedder)

	_, _, err := searcher.SearchWithTimings(
		context.Background(), "code_chunks", "ChunkFile", SearchModeSparse, 5, nil, nil,
	)
	if err != nil {
		t.Fatalf("SearchWithTimings() unexpected error: %v", err)
	}
	if embedder.calls != 0 {
		t.Fatalf("embedder calls = %d, want 0", embedder.calls)
	}
	if client.validateCalls != 0 {
		t.Fatalf("ValidateCollectionVectorSize calls = %d, want 0", client.validateCalls)
	}
	if client.mode != qdrant.SearchModeSparse {
		t.Fatalf("mode = %q, want sparse", client.mode)
	}
	if client.dense != nil {
		t.Fatalf("dense vector = %v, want nil", client.dense)
	}
	if client.sparse == nil || len(client.sparse.Indices) == 0 || len(client.sparse.Values) == 0 {
		t.Fatalf("expected non-empty sparse query vector, got %#v", client.sparse)
	}
}

func TestSearchWithTimingsHybridModeBuildsDenseAndSparseQueries(t *testing.T) {
	t.Parallel()

	client := &recordingClient{}
	embedder := &recordingEmbedder{vectors: [][]float32{{1, 2, 3}}}
	searcher := NewSearcher(client, embedder)

	_, _, err := searcher.SearchWithTimings(
		context.Background(), "code_chunks", "ChunkFile", SearchModeHybrid, 5, nil, nil,
	)
	if err != nil {
		t.Fatalf("SearchWithTimings() unexpected error: %v", err)
	}
	if embedder.calls != 1 {
		t.Fatalf("embedder calls = %d, want 1", embedder.calls)
	}
	if client.validateCalls != 1 || client.validateSize != 3 {
		t.Fatalf("ValidateCollectionVectorSize calls/size = %d/%d, want 1/3", client.validateCalls, client.validateSize)
	}
	if client.mode != qdrant.SearchModeHybrid {
		t.Fatalf("mode = %q, want hybrid", client.mode)
	}
	if !slices.Equal(client.dense, []float32{1, 2, 3}) {
		t.Fatalf("dense vector = %v, want [1 2 3]", client.dense)
	}
	if client.sparse == nil || len(client.sparse.Indices) == 0 || len(client.sparse.Values) == 0 {
		t.Fatalf("expected non-empty sparse query vector, got %#v", client.sparse)
	}
}

func TestSearchWithTimingsDefaultModeIsHybrid(t *testing.T) {
	t.Parallel()

	client := &recordingClient{}
	embedder := &recordingEmbedder{vectors: [][]float32{{1, 2, 3}}}
	searcher := NewSearcher(client, embedder)

	_, _, err := searcher.SearchWithTimings(
		context.Background(), "code_chunks", "ChunkFile", "", 5, nil, nil,
	)
	if err != nil {
		t.Fatalf("SearchWithTimings() unexpected error: %v", err)
	}
	if client.mode != qdrant.SearchModeHybrid {
		t.Fatalf("mode = %q, want hybrid", client.mode)
	}
}

func TestSearchWithTimingsRejectsUnknownModeBeforeWork(t *testing.T) {
	t.Parallel()

	client := &recordingClient{}
	embedder := &recordingEmbedder{vectors: [][]float32{{1, 2, 3}}}
	searcher := NewSearcher(client, embedder)

	_, _, err := searcher.SearchWithTimings(
		context.Background(), "code_chunks", "ChunkFile", SearchMode("magic"), 5, nil, nil,
	)
	if err == nil {
		t.Fatal("expected error for unknown mode")
	}
	if embedder.calls != 0 {
		t.Fatalf("embedder calls = %d, want 0", embedder.calls)
	}
	if client.searchCalls != 0 {
		t.Fatalf("client Search calls = %d, want 0", client.searchCalls)
	}
}

type recordingEmbedder struct {
	calls   int
	texts   [][]string
	vectors [][]float32
	err     error
}

func (e *recordingEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	e.calls++
	e.texts = append(e.texts, slices.Clone(texts))
	return e.vectors, e.err
}

func (e *recordingEmbedder) Dimension() int {
	if len(e.vectors) == 0 || len(e.vectors[0]) == 0 {
		return 0
	}
	return len(e.vectors[0])
}

type recordingClient struct {
	validateCalls int
	validateName  string
	validateSize  uint64
	validateErr   error

	searchCalls int
	collection  string
	dense       []float32
	sparse      *embedding.SparseVector
	mode        qdrant.SearchMode
	limit       uint64
	languages   []string
	repos       []string
	results     []model.SearchResult
	searchErr   error
}

func (c *recordingClient) ValidateCollectionVectorSize(_ context.Context, name string, vectorSize uint64) error {
	c.validateCalls++
	c.validateName = name
	c.validateSize = vectorSize
	return c.validateErr
}

func (c *recordingClient) Search(_ context.Context, collection string, denseVector []float32, sparseVector *embedding.SparseVector, mode qdrant.SearchMode, limit uint64, languages, repos []string) ([]model.SearchResult, error) {
	c.searchCalls++
	c.collection = collection
	c.dense = slices.Clone(denseVector)
	if sparseVector != nil {
		sv := embedding.SparseVector{
			Indices: slices.Clone(sparseVector.Indices),
			Values:  slices.Clone(sparseVector.Values),
		}
		c.sparse = &sv
	}
	c.mode = mode
	c.limit = limit
	c.languages = slices.Clone(languages)
	c.repos = slices.Clone(repos)
	return c.results, c.searchErr
}
