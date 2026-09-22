package ingest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dinhvy/ragcodepilot/internal/config"
	"github.com/dinhvy/ragcodepilot/internal/model"
)

func TestDenseCachePutGetAndInvalidation(t *testing.T) {
	cache, err := openDenseCache(t.TempDir(), "collection")
	if err != nil {
		t.Fatalf("openDenseCache: %v", err)
	}
	key := denseCacheKey("model-a", "representation-v1", "enriched input")
	vector := []float32{1, 2, 3}
	if _, ok := cache.get(key, 3); ok {
		t.Fatal("expected cache miss before put")
	}
	if err := cache.put(key, vector); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, ok := cache.get(key, 3)
	if !ok || len(got) != 3 || got[1] != 2 {
		t.Fatalf("get = %v, %v; want stored vector", got, ok)
	}
	if _, ok := cache.get(key, 2); ok {
		t.Fatal("expected dimension mismatch to invalidate cache entry")
	}
	if denseCacheKey("model-a", "representation-v2", "enriched input") == key {
		t.Fatal("representation change reused dense cache key")
	}
	if denseCacheKey("model-b", "representation-v1", "enriched input") == key {
		t.Fatal("model identity change reused dense cache key")
	}
	if denseCacheKey("model-a", "representation-v1", "changed input") == key {
		t.Fatal("input change reused dense cache key")
	}
}

type countingEmbedder struct {
	base  *fakeEmbedder
	calls int
}

func (e *countingEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	e.calls++
	return e.base.Embed(ctx, texts)
}

func (e *countingEmbedder) Dimension() int { return e.base.Dimension() }

func TestPipelineReusesDenseCacheOnRepresentationRefresh(t *testing.T) {
	repoPath := t.TempDir()
	filePath := filepath.Join(repoPath, "app.py")
	if err := os.WriteFile(filePath, []byte("# stable\nprint('same')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash, err := HashFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	stateDir := t.TempDir()
	oldState := map[string]model.FileIndexState{"app.py": {FileHash: hash, IndexVersion: "old-representation"}}

	firstEmbedder := &countingEmbedder{base: &fakeEmbedder{dim: 4}}
	firstStore := &recordingStore{existingStates: oldState}
	first := NewPipeline(config.Default(), firstEmbedder, firstStore, "cache-test", WithRunStateDir(stateDir))
	if err := first.Run(context.Background(), repoPath); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if firstEmbedder.calls == 0 {
		t.Fatal("first run did not call embedder")
	}

	secondEmbedder := &countingEmbedder{base: &fakeEmbedder{dim: 4}}
	secondStore := &recordingStore{existingStates: oldState}
	second := NewPipeline(config.Default(), secondEmbedder, secondStore, "cache-test", WithRunStateDir(stateDir))
	if err := second.Run(context.Background(), repoPath); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if secondEmbedder.calls != 0 {
		t.Fatalf("second run embedder calls = %d, want 0 from dense cache reuse", secondEmbedder.calls)
	}
}
