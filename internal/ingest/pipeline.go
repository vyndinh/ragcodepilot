package ingest

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/dinhvy/ragcodepilot/internal/config"
	"github.com/dinhvy/ragcodepilot/internal/embedding"
	"github.com/dinhvy/ragcodepilot/internal/model"
)

type vectorStore interface {
	EnsureCollection(ctx context.Context, name string, vectorSize uint64) error
	EnsurePayloadIndexes(ctx context.Context, collection string) error
	Upsert(ctx context.Context, collection string, chunks []model.CodeChunk, vectors [][]float32, sparseVectors []embedding.SparseVector) error
	ScrollFileHashes(ctx context.Context, collection, repo string, languages []string) (map[string]string, error)
	DeleteByFilePaths(ctx context.Context, collection, repo string, filePaths []string) error
	DeleteStaleChunksByFilePath(ctx context.Context, collection, repo, filePath, currentHash string) error
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
// On re-index, it uses file hashes to skip unchanged files, delete stale points,
// and only re-embed files that have changed.
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

	// Step 2: Ensure payload indexes exist for efficient filtered scroll/delete.
	// This is a no-op if the collection doesn't exist yet or indexes already exist.
	if err := p.store.EnsurePayloadIndexes(ctx, p.collection); err != nil {
		return fmt.Errorf("ensuring payload indexes: %w", err)
	}

	// Step 3: Hash all source files on disk.
	diskHashes, err := HashFiles(files)
	if err != nil {
		return fmt.Errorf("hashing files: %w", err)
	}

	// Step 4: Get existing file hashes from Qdrant.
	// Scope the scroll to the same languages being indexed, so language-filtered
	// re-indexing (e.g. --language go) doesn't see points for other languages
	// and misclassify them as stale.
	existingHashes, err := p.store.ScrollFileHashes(ctx, p.collection, repoName, p.languageKeys())
	if err != nil {
		return fmt.Errorf("scrolling existing hashes: %w", err)
	}

	// Step 5: Classify files.
	// Convert absolute paths to relative paths for comparison with Qdrant payloads.
	var filesToIndex []string // new or changed files (absolute paths)
	var staleFiles []string   // exist in Qdrant but not on disk (relative paths)
	var changedFiles []string // exist in Qdrant with different hash (relative paths)
	skipped := 0

	relHashes := make(map[string]string, len(diskHashes))
	absToRel := make(map[string]string, len(diskHashes))
	for absFile, hash := range diskHashes {
		rel, err := filepath.Rel(absPath, absFile)
		if err != nil {
			rel = absFile
		}
		relHashes[rel] = hash
		absToRel[absFile] = rel
	}

	for absFile, hash := range diskHashes {
		rel := absToRel[absFile]
		existingHash, exists := existingHashes[rel]
		if exists && existingHash == hash {
			// File unchanged — skip.
			skipped++
			continue
		}
		if exists {
			// File changed — mark for post-upsert cleanup.
			changedFiles = append(changedFiles, rel)
		}
		// New or changed — add to index list.
		filesToIndex = append(filesToIndex, absFile)
	}

	// Stale files: exist in Qdrant but not on disk.
	for existingRel := range existingHashes {
		if _, onDisk := relHashes[existingRel]; !onDisk {
			staleFiles = append(staleFiles, existingRel)
		}
	}

	changedCount := len(changedFiles)
	newCount := len(filesToIndex) - changedCount
	staleCount := len(staleFiles)

	fmt.Printf("Change detection: %d unchanged (skip), %d changed, %d new, %d stale\n",
		skipped, changedCount, newCount, staleCount)

	// Step 6: Delete stale file points immediately (no replacement coming).
	if len(staleFiles) > 0 {
		if err := p.store.DeleteByFilePaths(ctx, p.collection, repoName, staleFiles); err != nil {
			return fmt.Errorf("deleting stale points: %w", err)
		}
		fmt.Printf("Deleted points for %d stale files\n", len(staleFiles))
	}

	if len(filesToIndex) == 0 {
		if len(staleFiles) > 0 {
			fmt.Println("Cleaned up stale files — no new indexing needed")
		} else {
			fmt.Println("Everything up to date — nothing to index")
		}
		return nil
	}

	// Step 7: Chunk files to index.
	indexedAt := time.Now().UTC().Format(time.RFC3339)
	allChunks := make([]model.CodeChunk, 0, len(filesToIndex)*2)
	for _, file := range filesToIndex {
		chunks, err := ChunkFile(file, absPath, repoName, p.chunkSize, p.chunkOverlap, p.cfg)
		if err != nil {
			fmt.Printf("warning: skipping %s: %v\n", file, err)
			continue
		}
		rel := absToRel[file]
		hash := relHashes[rel]
		for i := range chunks {
			chunks[i].IndexedAt = indexedAt
			chunks[i].FileHash = hash
		}
		allChunks = append(allChunks, chunks...)
	}
	fmt.Printf("Generated %d chunks from %d files\n", len(allChunks), len(filesToIndex))

	if len(allChunks) == 0 {
		return fmt.Errorf("no chunks generated from %s", repoName)
	}

	// Step 8: Embed and upsert in batches.
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
		if err := p.store.Upsert(ctx, p.collection, batch, vectors, nil); err != nil {
			return fmt.Errorf("upserting batch %d-%d: %w", start, end, err)
		}

		fmt.Printf("Indexed %d/%d chunks\n", end, len(allChunks))
	}

	// Step 9: Remove orphaned chunks for changed files.
	// Filters by file_hash != current_hash so only old-hash points (chunks whose
	// start_line shifted and were not overwritten by the upsert) are removed.
	// New-hash points upserted in Step 8 are preserved. If the process crashes
	// before here, old points remain searchable until the next run.
	for _, rel := range changedFiles {
		if err := p.store.DeleteStaleChunksByFilePath(ctx, p.collection, repoName, rel, relHashes[rel]); err != nil {
			return fmt.Errorf("deleting stale chunks for %s: %w", rel, err)
		}
	}
	if len(changedFiles) > 0 {
		fmt.Printf("Cleaned up stale chunks for %d changed files\n", len(changedFiles))
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

// languageKeys returns the language filter keys as a slice.
// Returns nil when no language filter is set (full re-index).
func (p *Pipeline) languageKeys() []string {
	if len(p.languages) == 0 {
		return nil
	}
	keys := make([]string, 0, len(p.languages))
	for lang := range p.languages {
		keys = append(keys, lang)
	}
	return keys
}
