package answer

import (
	"testing"

	"github.com/dinhvy/ragcodepilot/internal/model"
)

func TestContextsFromResults(t *testing.T) {
	t.Parallel()

	results := []model.SearchResult{
		{Chunk: model.CodeChunk{Repo: "ragcodepilot", FilePath: "a.go", StartLine: 1, EndLine: 10, Name: "Foo", ChunkType: "function", Content: "code A"}},
		{Chunk: model.CodeChunk{FilePath: "b.go", StartLine: 20, EndLine: 30, ChunkType: "block", Content: "code B"}}, // no Repo, no Name
	}

	chunks := ContextsFromResults(results)
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(chunks))
	}
	if chunks[0].Index != 1 || chunks[1].Index != 2 {
		t.Errorf("indices should be 1-based sequential, got %d and %d", chunks[0].Index, chunks[1].Index)
	}
	if chunks[0].Lines != "1-10" {
		t.Errorf("Lines = %q, want 1-10", chunks[0].Lines)
	}
	if chunks[0].Repo != "ragcodepilot" || chunks[0].Symbol != "Foo" || chunks[0].ChunkType != "function" {
		t.Errorf("unexpected mapping for first chunk: %+v", chunks[0])
	}
	if chunks[1].Repo != "" || chunks[1].Symbol != "" {
		t.Errorf("missing Repo/Name should map to empty, got %+v", chunks[1])
	}
	if chunks[1].FilePath != "b.go" || chunks[1].Content != "code B" || chunks[1].ChunkType != "block" {
		t.Errorf("unexpected mapping for second chunk: %+v", chunks[1])
	}
}
