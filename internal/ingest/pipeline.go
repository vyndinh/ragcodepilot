package ingest

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/dinhvy/ragsearch/internal/config"
	"github.com/dinhvy/ragsearch/internal/embedding"
	"github.com/dinhvy/ragsearch/internal/model"
)

type vectorStore interface {
	EnsureCollection(ctx context.Context, name string, vectorSize uint64) error
	Upsert(ctx context.Context, collection string, chunks []model.CodeChunk, vectors [][]float32) error
}

// Pipeline orchestrates the ingestion flow: walk → chunk → embed → upsert.
type Pipeline struct {
	cfg          *config.Config
	embedder     embedding.Embedder
	store        vectorStore
	collection   string
	chunkSize    int
	chunkOverlap int
	languages    map[string]struct{}
}

// Option configures a Pipeline.
type Option func(*Pipeline)

// WithChunkSize sets the target number of lines per chunk.
func WithChunkSize(n int) Option {
	return func(p *Pipeline) { p.chunkSize = n }
}

// WithChunkOverlap sets the number of overlapping lines between consecutive chunks.
func WithChunkOverlap(n int) Option {
	return func(p *Pipeline) { p.chunkOverlap = n }
}

// WithLanguages limits ingestion to files whose detected language is in languages.
func WithLanguages(languages []string) Option {
	return func(p *Pipeline) {
		if len(languages) == 0 {
			return
		}
		p.languages = make(map[string]struct{}, len(languages))
		for _, lang := range languages {
			p.languages[lang] = struct{}{}
		}
	}
}

// NewPipeline creates a new ingestion pipeline.
func NewPipeline(cfg *config.Config, embedder embedding.Embedder, store vectorStore, collection string, opts ...Option) *Pipeline {
	p := &Pipeline{
		cfg:          cfg,
		embedder:     embedder,
		store:        store,
		collection:   collection,
		chunkSize:    DefaultChunkLines,
		chunkOverlap: DefaultChunkOverlap,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Run walks the repository, chunks files, embeds them, and upserts to Qdrant.
//
// Ingestion order: walk → chunk → embed first batch → infer dimension →
// ensure/validate collection → upsert first batch → embed/upsert remaining.
func (p *Pipeline) Run(ctx context.Context, repoPath string) error {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	repoName := filepath.Base(absPath)

	// Step 1: Walk source files.
	files, err := WalkFiles(absPath, p.cfg)
	if err != nil {
		return fmt.Errorf("walking files in %s: %w", repoName, err)
	}
	files = p.filterFilesByLanguage(files)
	fmt.Printf("Found %d source files in %s\n", len(files), repoName)

	// Step 2: Chunk all files.
	indexedAt := time.Now().UTC().Format(time.RFC3339)
	allChunks := make([]model.CodeChunk, 0, len(files)*2)
	for _, file := range files {
		chunks, err := ChunkFile(file, absPath, repoName, p.chunkSize, p.chunkOverlap, p.cfg)
		if err != nil {
			fmt.Printf("warning: skipping %s: %v\n", file, err)
			continue
		}
		for i := range chunks {
			chunks[i].IndexedAt = indexedAt
		}
		allChunks = append(allChunks, chunks...)
	}
	fmt.Printf("Generated %d chunks from %d files\n", len(allChunks), len(files))

	if len(allChunks) == 0 {
		return fmt.Errorf("no chunks generated from %s", repoName)
	}

	// Step 3 + 4: Embed and upsert in batches.
	// The collection is created after the first batch is embedded so that
	// the vector dimension is inferred from the model, not hardcoded.
	const batchSize = 32
	collectionReady := false
	expectedDim := 0

	for start := 0; start < len(allChunks); start += batchSize {
		end := start + batchSize
		if end > len(allChunks) {
			end = len(allChunks)
		}
		batch := allChunks[start:end]

		// Build enriched text for embedding (metadata + code).
		// The raw chunk.Content is stored unchanged in Qdrant payload.
		texts := make([]string, len(batch))
		for i, chunk := range batch {
			texts[i] = enrichForEmbedding(chunk)
		}

		// Embed the batch.
		vectors, err := p.embedder.Embed(ctx, texts)
		if err != nil {
			return fmt.Errorf("embedding batch %d-%d: %w", start, end, err)
		}
		if len(vectors) != len(batch) {
			return fmt.Errorf("embedding batch %d-%d: expected %d vectors, got %d", start, end, len(batch), len(vectors))
		}

		detectedDim, err := embedding.ValidateVectorBatch(vectors, expectedDim)
		if err != nil {
			return fmt.Errorf("validating embedding batch %d-%d: %w", start, end, err)
		}
		expectedDim = detectedDim

		// After the first batch, infer dimension and ensure the collection.
		if !collectionReady {
			dim := uint64(detectedDim)
			fmt.Printf("Detected vector dimension: %d\n", dim)
			if err := p.store.EnsureCollection(ctx, p.collection, dim); err != nil {
				return fmt.Errorf("ensuring collection: %w", err)
			}
			collectionReady = true
		}

		// Upsert to Qdrant.
		if err := p.store.Upsert(ctx, p.collection, batch, vectors); err != nil {
			return fmt.Errorf("upserting batch %d-%d: %w", start, end, err)
		}

		fmt.Printf("Indexed %d/%d chunks\n", end, len(allChunks))
	}

	fmt.Printf("Successfully indexed %d chunks into collection %q\n", len(allChunks), p.collection)
	return nil
}

func (p *Pipeline) filterFilesByLanguage(files []string) []string {
	if len(p.languages) == 0 {
		return files
	}

	filtered := make([]string, 0, len(files))
	for _, file := range files {
		lang := p.cfg.DetectLanguage(file)
		if _, ok := p.languages[lang]; ok {
			filtered = append(filtered, file)
		}
	}
	return filtered
}
