package ingest

import (
	"strings"
	"testing"

	"github.com/dinhvy/ragsearch/internal/model"
)

func TestEnrichForEmbedding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		chunk    model.CodeChunk
		contains []string // substrings that must appear in the output
		excludes []string // substrings that must NOT appear
	}{
		{
			name: "function with all fields",
			chunk: model.CodeChunk{
				FilePath:  "internal/ingest/walker.go",
				Language:  "go",
				ChunkType: "function",
				Name:      "WalkFiles",
				Content:   "func WalkFiles() {}",
			},
			contains: []string{
				"File: internal/ingest/walker.go",
				"Language: go",
				"Function: WalkFiles",
				"func WalkFiles() {}",
			},
		},
		{
			name: "block without name",
			chunk: model.CodeChunk{
				FilePath:  "internal/config/config.go",
				Language:  "go",
				ChunkType: "block",
				Content:   "package config",
			},
			contains: []string{
				"File: internal/config/config.go",
				"Language: go",
				"Type: Block",
				"package config",
			},
			excludes: []string{
				"Function:",
				"Block: \n", // should not have empty name after label
			},
		},
		{
			name: "unknown chunk type without name",
			chunk: model.CodeChunk{
				FilePath:  "README.md",
				Language:  "markdown",
				ChunkType: "unknown",
				Content:   "# README",
			},
			contains: []string{
				"File: README.md",
				"Language: markdown",
				"Type: Chunk",
				"# README",
			},
		},
		{
			name: "empty chunk type defaults to chunk",
			chunk: model.CodeChunk{
				FilePath:  "main.go",
				Language:  "go",
				ChunkType: "",
				Name:      "main",
				Content:   "func main() {}",
			},
			contains: []string{
				"Chunk: main",
				"func main() {}",
			},
		},
		{
			name: "content is never duplicated",
			chunk: model.CodeChunk{
				FilePath:  "test.go",
				Language:  "go",
				ChunkType: "function",
				Name:      "TestFunc",
				Content:   "func TestFunc() {}",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := enrichForEmbedding(tt.chunk)

			for _, s := range tt.contains {
				if !strings.Contains(result, s) {
					t.Errorf("expected result to contain %q, got:\n%s", s, result)
				}
			}

			for _, s := range tt.excludes {
				if strings.Contains(result, s) {
					t.Errorf("expected result NOT to contain %q, got:\n%s", s, result)
				}
			}

			// Content must appear exactly once (not duplicated).
			if tt.chunk.Content != "" {
				count := strings.Count(result, tt.chunk.Content)
				if count != 1 {
					t.Errorf("content appears %d times, want exactly 1", count)
				}
			}

			// Metadata must be separated from content by a blank line.
			if !strings.Contains(result, "\n\n"+tt.chunk.Content) {
				t.Errorf("expected blank line before content, got:\n%s", result)
			}
		})
	}
}

func TestChunkTypeLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"function", "Function"},
		{"block", "Block"},
		{"unknown", "Chunk"},
		{"", "Chunk"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			if got := chunkTypeLabel(tt.input); got != tt.expected {
				t.Errorf("chunkTypeLabel(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
