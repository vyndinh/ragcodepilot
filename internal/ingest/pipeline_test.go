package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dinhvy/ragsearch/internal/config"
	"github.com/dinhvy/ragsearch/internal/model"
)

func TestPipeline_filterFilesByLanguage(t *testing.T) {
	tests := []struct {
		name      string
		languages []string
		files     []string
		want      []string
	}{
		{
			name:      "no language filter keeps all files",
			languages: nil,
			files:     []string{"main.go", "lib.rs", "README.md"},
			want:      []string{"main.go", "lib.rs", "README.md"},
		},
		{
			name:      "single language keeps matching files",
			languages: []string{"go"},
			files:     []string{"main.go", "lib.rs", "README.md"},
			want:      []string{"main.go"},
		},
		{
			name:      "multiple languages keep matching files in original order",
			languages: []string{"go", "rust"},
			files:     []string{"main.go", "lib.rs", "README.md", "internal/search.go"},
			want:      []string{"main.go", "lib.rs", "internal/search.go"},
		},
		{
			name:      "unknown language keeps no files",
			languages: []string{"zig"},
			files:     []string{"main.go", "lib.rs"},
			want:      []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPipeline(config.Default(), nil, nil, "code_chunks", WithLanguages(tt.languages))

			got := p.filterFilesByLanguage(tt.files)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("filterFilesByLanguage() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestPipeline_RunValidatesEmbeddingBatchBeforeStore(t *testing.T) {
	tests := []struct {
		name      string
		fileCount int
		batches   [][][]float32
		wantErr   string
	}{
		{
			name:      "wrong vector count",
			fileCount: 1,
			batches: [][][]float32{
				{{1, 2, 3}, {4, 5, 6}},
			},
			wantErr: "expected 1 vectors, got 2",
		},
		{
			name:      "empty vector",
			fileCount: 1,
			batches: [][][]float32{
				{{}},
			},
			wantErr: "vector 0 is empty",
		},
		{
			name:      "inconsistent dimensions",
			fileCount: 2,
			batches: [][][]float32{
				{{1, 2, 3}, {4, 5}},
			},
			wantErr: "inconsistent dimensions in batch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoPath := writeTestRepo(t, tt.fileCount)
			store := &recordingStore{}
			p := NewPipeline(
				config.Default(),
				&scriptedEmbedder{batches: tt.batches},
				store,
				"code_chunks",
			)

			err := p.Run(context.Background(), repoPath)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
			if store.ensureCalls != 0 {
				t.Fatalf("EnsureCollection calls = %d, want 0", store.ensureCalls)
			}
			if store.upsertCalls != 0 {
				t.Fatalf("Upsert calls = %d, want 0", store.upsertCalls)
			}
		})
	}
}

func TestPipeline_RunRejectsSecondBatchDimensionMismatchBeforeUpsert(t *testing.T) {
	const firstBatchSize = 32
	repoPath := writeTestRepo(t, firstBatchSize+1)
	store := &recordingStore{}
	p := NewPipeline(
		config.Default(),
		&scriptedEmbedder{
			batches: [][][]float32{
				repeatedVectors(firstBatchSize, 3),
				repeatedVectors(1, 2),
			},
		},
		store,
		"code_chunks",
	)

	err := p.Run(context.Background(), repoPath)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "dimension mismatch: expected 3, got 2") {
		t.Fatalf("error = %q, want dimension mismatch", err.Error())
	}
	if store.ensureCalls != 1 {
		t.Fatalf("EnsureCollection calls = %d, want 1", store.ensureCalls)
	}
	if store.ensureSize != 3 {
		t.Fatalf("ensured vector size = %d, want 3", store.ensureSize)
	}
	if store.upsertCalls != 1 {
		t.Fatalf("Upsert calls = %d, want 1", store.upsertCalls)
	}
	if got := store.upsertBatchSizes[0]; got != firstBatchSize {
		t.Fatalf("first upsert batch size = %d, want %d", got, firstBatchSize)
	}
}

func TestPipeline_RunEnsuresCollectionWithDetectedDimension(t *testing.T) {
	repoPath := writeTestRepo(t, 1)
	store := &recordingStore{}
	p := NewPipeline(
		config.Default(),
		&scriptedEmbedder{
			batches: [][][]float32{
				{{1, 2, 3, 4}},
			},
		},
		store,
		"code_chunks",
	)

	if err := p.Run(context.Background(), repoPath); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if store.ensureCalls != 1 {
		t.Fatalf("EnsureCollection calls = %d, want 1", store.ensureCalls)
	}
	if store.ensureCollection != "code_chunks" {
		t.Fatalf("ensured collection = %q, want code_chunks", store.ensureCollection)
	}
	if store.ensureSize != 4 {
		t.Fatalf("ensured vector size = %d, want 4", store.ensureSize)
	}
	if store.upsertCalls != 1 {
		t.Fatalf("Upsert calls = %d, want 1", store.upsertCalls)
	}
	if got := store.upsertBatchSizes[0]; got != 1 {
		t.Fatalf("upsert batch size = %d, want 1", got)
	}
}

type scriptedEmbedder struct {
	batches [][][]float32
	calls   int
}

func (e *scriptedEmbedder) Embed(_ context.Context, _ []string) ([][]float32, error) {
	if e.calls >= len(e.batches) {
		return nil, fmt.Errorf("unexpected embed call %d", e.calls+1)
	}
	batch := e.batches[e.calls]
	e.calls++
	return batch, nil
}

func (e *scriptedEmbedder) Dimension() int {
	return 0
}

type recordingStore struct {
	ensureCalls      int
	ensureCollection string
	ensureSize       uint64
	upsertCalls      int
	upsertBatchSizes []int
}

func (s *recordingStore) EnsureCollection(_ context.Context, name string, vectorSize uint64) error {
	s.ensureCalls++
	s.ensureCollection = name
	s.ensureSize = vectorSize
	return nil
}

func (s *recordingStore) Upsert(_ context.Context, _ string, chunks []model.CodeChunk, _ [][]float32) error {
	s.upsertCalls++
	s.upsertBatchSizes = append(s.upsertBatchSizes, len(chunks))
	return nil
}

// writeTestRepo creates a temp directory with fileCount small Python files.
// Uses .py extension so files go through the generic chunker (not Go AST),
// producing exactly 1 chunk per file for predictable pipeline testing.
func writeTestRepo(t *testing.T, fileCount int) string {
	t.Helper()

	repoPath := t.TempDir()
	for i := 0; i < fileCount; i++ {
		path := filepath.Join(repoPath, fmt.Sprintf("file_%03d.py", i))
		content := fmt.Sprintf("# test file %d\nprint('hello')\n", i)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write source file: %v", err)
		}
	}
	return repoPath
}

func repeatedVectors(count, dim int) [][]float32 {
	vectors := make([][]float32, count)
	for i := range vectors {
		vectors[i] = make([]float32, dim)
		vectors[i][0] = 1
	}
	return vectors
}
