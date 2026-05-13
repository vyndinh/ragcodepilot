package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dinhvy/ragcodepilot/internal/config"
	"github.com/dinhvy/ragcodepilot/internal/embedding"
	"github.com/dinhvy/ragcodepilot/internal/model"
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
	if got := store.upsertSparseSizes[0]; got != firstBatchSize {
		t.Fatalf("first sparse batch size = %d, want %d", got, firstBatchSize)
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
	if got := store.upsertSparseSizes[0]; got != 1 {
		t.Fatalf("sparse batch size = %d, want 1", got)
	}
}

func TestPipeline_RunGeneratesSparseVectorsPerBatch(t *testing.T) {
	const totalFiles = 33 // 32 + 1 to force two batches
	repoPath := writeTestRepo(t, totalFiles)
	store := &recordingStore{}
	p := NewPipeline(config.Default(), &fakeEmbedder{dim: 4}, store, "code_chunks")

	if err := p.Run(context.Background(), repoPath); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if store.upsertCalls != 2 {
		t.Fatalf("Upsert calls = %d, want 2", store.upsertCalls)
	}

	wantBatchSizes := []int{32, 1}
	if !reflect.DeepEqual(store.upsertBatchSizes, wantBatchSizes) {
		t.Fatalf("upsert batch sizes = %v, want %v", store.upsertBatchSizes, wantBatchSizes)
	}
	if !reflect.DeepEqual(store.upsertSparseSizes, wantBatchSizes) {
		t.Fatalf("sparse batch sizes = %v, want %v", store.upsertSparseSizes, wantBatchSizes)
	}
	if store.upsertSparseNonEmpty == 0 {
		t.Fatal("expected non-empty sparse vectors across upsert batches")
	}
}

func TestPipeline_RunRefreshesUnchangedFilesWhenAnyFileChanges(t *testing.T) {
	repoPath := t.TempDir()
	unchangedPath := filepath.Join(repoPath, "unchanged.py")
	changedPath := filepath.Join(repoPath, "changed.py")
	if err := os.WriteFile(unchangedPath, []byte("# stable helper\nprint('same')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changedPath, []byte("# changed helper\nprint('new')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	unchangedHash, err := HashFile(unchangedPath)
	if err != nil {
		t.Fatal(err)
	}
	store := &recordingStore{
		existingHashes: map[string]string{
			"unchanged.py": unchangedHash,
			"changed.py":   "old_hash_does_not_match",
		},
	}
	p := NewPipeline(config.Default(), &fakeEmbedder{dim: 4}, store, "code_chunks")

	if err := p.Run(context.Background(), repoPath); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if store.upsertCalls != 1 {
		t.Fatalf("Upsert calls = %d, want 1", store.upsertCalls)
	}
	if got := store.upsertBatchSizes[0]; got != 2 {
		t.Fatalf("upsert batch size = %d, want 2 current files", got)
	}
	if got := store.upsertSparseSizes[0]; got != 2 {
		t.Fatalf("sparse batch size = %d, want 2 current files", got)
	}
}

func TestPipeline_RunRefreshesUnchangedFilesAfterStaleDelete(t *testing.T) {
	repoPath := t.TempDir()
	keptPath := filepath.Join(repoPath, "kept.py")
	if err := os.WriteFile(keptPath, []byte("# kept helper\nprint('ok')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	keptHash, err := HashFile(keptPath)
	if err != nil {
		t.Fatal(err)
	}
	store := &recordingStore{
		existingHashes: map[string]string{
			"kept.py":    keptHash,
			"removed.py": "doesnt_matter",
		},
	}
	p := NewPipeline(config.Default(), &fakeEmbedder{dim: 4}, store, "code_chunks")

	if err := p.Run(context.Background(), repoPath); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if store.deleteCalls != 1 {
		t.Fatalf("Delete calls = %d, want 1", store.deleteCalls)
	}
	if !reflect.DeepEqual(store.deleteFilePaths, []string{"removed.py"}) {
		t.Fatalf("deleted file paths = %v, want [removed.py]", store.deleteFilePaths)
	}
	if store.upsertCalls != 1 {
		t.Fatalf("Upsert calls = %d, want 1 to refresh remaining sparse vectors", store.upsertCalls)
	}
	if got := store.upsertBatchSizes[0]; got != 1 {
		t.Fatalf("upsert batch size = %d, want 1 current file", got)
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

// fakeEmbedder always returns uniform vectors matching the input count.
type fakeEmbedder struct {
	dim int
}

func (e *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	return repeatedVectors(len(texts), e.dim), nil
}

func (e *fakeEmbedder) Dimension() int {
	return e.dim
}

type recordingStore struct {
	ensureCalls          int
	ensureCollection     string
	ensureSize           uint64
	upsertCalls          int
	upsertBatchSizes     []int
	upsertSparseSizes    []int
	upsertSparseNonEmpty int
	deleteCalls          int
	deleteFilePaths      []string
	existingHashes       map[string]string
}

func (s *recordingStore) EnsureCollection(_ context.Context, name string, vectorSize uint64) error {
	s.ensureCalls++
	s.ensureCollection = name
	s.ensureSize = vectorSize
	return nil
}

func (s *recordingStore) Upsert(_ context.Context, _ string, chunks []model.CodeChunk, _ [][]float32, sparseVectors []embedding.SparseVector) error {
	s.upsertCalls++
	s.upsertBatchSizes = append(s.upsertBatchSizes, len(chunks))
	s.upsertSparseSizes = append(s.upsertSparseSizes, len(sparseVectors))
	for _, sv := range sparseVectors {
		if len(sv.Indices) > 0 || len(sv.Values) > 0 {
			s.upsertSparseNonEmpty++
		}
	}
	return nil
}

func (s *recordingStore) ScrollFileHashes(_ context.Context, _, _ string, _ []string) (map[string]string, error) {
	if s.existingHashes != nil {
		return s.existingHashes, nil
	}
	return make(map[string]string), nil
}

func (s *recordingStore) EnsurePayloadIndexes(_ context.Context, _ string) error {
	return nil
}

func (s *recordingStore) DeleteByFilePaths(_ context.Context, _, _ string, filePaths []string) error {
	s.deleteCalls++
	s.deleteFilePaths = append(s.deleteFilePaths, filePaths...)
	return nil
}

func (s *recordingStore) DeleteStaleChunksByFilePath(_ context.Context, _, _, _ string, _ string) error {
	s.deleteCalls++
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

// --- Delete ordering tests ---

// orderingStore records the order of operations to verify temporal ordering
// between delete and upsert calls.
type orderingStore struct {
	ops            []string          // ordered log: "delete:path", "upsert"
	existingHashes map[string]string // pre-existing {file_path: file_hash}
}

func (s *orderingStore) EnsureCollection(context.Context, string, uint64) error { return nil }
func (s *orderingStore) EnsurePayloadIndexes(context.Context, string) error     { return nil }

func (s *orderingStore) ScrollFileHashes(_ context.Context, _, _ string, _ []string) (map[string]string, error) {
	return s.existingHashes, nil
}

func (s *orderingStore) DeleteByFilePaths(_ context.Context, _, _ string, filePaths []string) error {
	for _, fp := range filePaths {
		s.ops = append(s.ops, "delete:"+fp)
	}
	return nil
}

func (s *orderingStore) DeleteStaleChunksByFilePath(_ context.Context, _, _, filePath, _ string) error {
	s.ops = append(s.ops, "delete_changed:"+filePath)
	return nil
}

func (s *orderingStore) Upsert(_ context.Context, _ string, chunks []model.CodeChunk, _ [][]float32, _ []embedding.SparseVector) error {
	s.ops = append(s.ops, "upsert")
	return nil
}

func TestPipeline_RunDeletesChangedFilesAfterUpsert(t *testing.T) {
	t.Parallel()

	// Create a repo with one Python file.
	repoPath := t.TempDir()
	filePath := filepath.Join(repoPath, "app.py")
	if err := os.WriteFile(filePath, []byte("# changed content\nprint('v2')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Simulate that app.py was previously indexed with a different hash.
	store := &orderingStore{
		existingHashes: map[string]string{
			"app.py": "old_hash_does_not_match",
		},
	}

	p := NewPipeline(config.Default(), &fakeEmbedder{dim: 4}, store, "test_collection")

	if err := p.Run(context.Background(), repoPath); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	// Verify operation order: upsert must come BEFORE delete of changed file.
	upsertIdx := -1
	deleteIdx := -1
	for i, op := range store.ops {
		if op == "upsert" && upsertIdx == -1 {
			upsertIdx = i
		}
		if op == "delete_changed:app.py" {
			deleteIdx = i
		}
	}
	if upsertIdx == -1 {
		t.Fatal("expected upsert call, got none")
	}
	if deleteIdx == -1 {
		t.Fatal("expected delete_changed:app.py call, got none; ops: " + fmt.Sprint(store.ops))
	}
	if deleteIdx < upsertIdx {
		t.Errorf("changed-file delete (op %d) happened before upsert (op %d); want delete after upsert\nops: %v",
			deleteIdx, upsertIdx, store.ops)
	}
}

// captureStore extends orderingStore to also record the hash passed to
// DeleteStaleChunksByFilePath, so tests can verify the correct hash is used.
type captureStore struct {
	orderingStore
	capturedHash string
}

func (s *captureStore) DeleteStaleChunksByFilePath(_ context.Context, _, _, filePath, currentHash string) error {
	s.ops = append(s.ops, "delete_changed:"+filePath)
	s.capturedHash = currentHash
	return nil
}

func TestPipeline_RunDeletesChangedFileChunksWithCurrentHash(t *testing.T) {
	t.Parallel()

	repoPath := t.TempDir()
	filePath := filepath.Join(repoPath, "app.py")
	if err := os.WriteFile(filePath, []byte("# v2\nprint('hello')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Simulate app.py was previously indexed with a different hash.
	store := &captureStore{
		orderingStore: orderingStore{
			existingHashes: map[string]string{
				"app.py": "old_hash_does_not_match",
			},
		},
	}

	p := NewPipeline(config.Default(), &fakeEmbedder{dim: 4}, store, "test_collection")

	if err := p.Run(context.Background(), repoPath); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if store.capturedHash == "" {
		t.Fatal("DeleteStaleChunksByFilePath was not called for the changed file")
	}
	if store.capturedHash == "old_hash_does_not_match" {
		t.Fatal("DeleteStaleChunksByFilePath called with old hash — would delete freshly upserted chunks")
	}
}

func TestPipeline_RunDeletesStaleFiles(t *testing.T) {
	t.Parallel()

	// Create a repo with one file — but Qdrant has two (the other is stale).
	repoPath := t.TempDir()
	filePath := filepath.Join(repoPath, "kept.py")
	if err := os.WriteFile(filePath, []byte("# still here\nprint('ok')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Hash the kept file to simulate it being new (not in existingHashes).
	// removed.py is in Qdrant but not on disk → stale.
	store := &orderingStore{
		existingHashes: map[string]string{
			"removed.py": "doesnt_matter",
		},
	}

	p := NewPipeline(config.Default(), &fakeEmbedder{dim: 4}, store, "test_collection")

	if err := p.Run(context.Background(), repoPath); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	// Verify stale file deletion happens (order relative to upsert doesn't matter
	// for stale files, but it should be present).
	found := false
	for _, op := range store.ops {
		if op == "delete:removed.py" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected delete:removed.py in ops, got: %v", store.ops)
	}
}
