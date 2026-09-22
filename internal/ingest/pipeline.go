package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/dinhvy/ragcodepilot/internal/config"
	"github.com/dinhvy/ragcodepilot/internal/embedding"
	"github.com/dinhvy/ragcodepilot/internal/model"
)

type vectorStore interface {
	EnsureCollection(ctx context.Context, name string, vectorSize uint64) error
	EnsurePayloadIndexes(ctx context.Context, collection string) error
	Upsert(ctx context.Context, collection string, chunks []model.CodeChunk, vectors [][]float32, sparseVectors []embedding.SparseVector) error
	ScrollFileStates(ctx context.Context, collection, repo string, languages []string) (map[string]model.FileIndexState, error)
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
	runStateDir  string
	metricsSink  func(IndexMetrics)
}

// IndexMetrics describes one indexing attempt. Durations are milliseconds;
// counts distinguish dense embedding work from sparse refresh work.
type IndexMetrics struct {
	Status               string `json:"status"`
	Error                string `json:"error,omitempty"`
	Collection           string `json:"collection"`
	FilesScanned         int    `json:"files_scanned"`
	ChunksGenerated      int    `json:"chunks_generated"`
	DenseCalls           int    `json:"dense_calls"`
	DenseInputs          int    `json:"dense_inputs"`
	DenseCacheHits       int    `json:"dense_cache_hits"`
	DenseCacheMisses     int    `json:"dense_cache_misses"`
	SparseVectorsBuilt   int    `json:"sparse_vectors_built"`
	UpsertBatches        int    `json:"upsert_batches"`
	ChunkMS              int64  `json:"chunk_ms"`
	SparseStatsMS        int64  `json:"sparse_stats_ms"`
	DenseMS              int64  `json:"dense_ms"`
	UpsertMS             int64  `json:"upsert_ms"`
	SourceVerificationMS int64  `json:"source_verification_ms"`
	TotalMS              int64  `json:"total_ms"`
}

func (m IndexMetrics) String() string {
	return fmt.Sprintf("status=%s files=%d chunks=%d dense_calls=%d dense_inputs=%d cache_hits=%d cache_misses=%d sparse_vectors=%d upsert_batches=%d chunk_ms=%d sparse_stats_ms=%d dense_ms=%d upsert_ms=%d source_verify_ms=%d total_ms=%d", m.Status, m.FilesScanned, m.ChunksGenerated, m.DenseCalls, m.DenseInputs, m.DenseCacheHits, m.DenseCacheMisses, m.SparseVectorsBuilt, m.UpsertBatches, m.ChunkMS, m.SparseStatsMS, m.DenseMS, m.UpsertMS, m.SourceVerificationMS, m.TotalMS)
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

// WithRunStateDir enables durable run markers and collection writer ownership.
// An empty directory keeps the pipeline library mode side-effect free; the CLI
// supplies DefaultRunStateDir for real indexing.
func WithRunStateDir(dir string) Option {
	return func(p *Pipeline) { p.runStateDir = dir }
}

// WithMetricsSink receives one metrics record after each indexing attempt.
func WithMetricsSink(sink func(IndexMetrics)) Option {
	return func(p *Pipeline) { p.metricsSink = sink }
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

// chunkerVersion is bumped when AST/chunk shape changes (e.g. extracting
// named type chunks) so unchanged files are re-indexed. Stored with the
// sparse tokenizer version in IndexVersion.
const chunkerVersion = "go-types-v2-identity"
const enrichmentVersion = "enrich-v1"

func representationVersion() string {
	return embedding.SparseIndexVersion + "+" + chunkerVersion + "+" + enrichmentVersion
}

func (p *Pipeline) currentRepresentation(identity string) string {
	base := representationVersion()
	if p.runStateDir == "" {
		return base
	}
	digest := sha256.Sum256([]byte(identity))
	return fmt.Sprintf("%s+chunk-size=%d+overlap=%d+embedder=%s", base, p.chunkSize, p.chunkOverlap, hex.EncodeToString(digest[:]))
}

type cacheIdentityProvider interface {
	CacheIdentity(context.Context) (string, error)
}

func embedderCacheIdentity(ctx context.Context, embedder embedding.Embedder) (string, error) {
	if provider, ok := embedder.(cacheIdentityProvider); ok {
		identity, err := provider.CacheIdentity(ctx)
		if err != nil {
			return "", fmt.Errorf("resolving embedder cache identity: %w", err)
		}
		if identity = normalizeCacheIdentity(identity); identity != "" {
			return identity, nil
		}
	}
	return fallbackEmbedderIdentity(embedder), nil
}

// Run walks the repository, chunks files, embeds them, and upserts to Qdrant.
// On re-index, it uses file hashes to skip unchanged files, delete stale points,
// and avoid work when the corpus is unchanged. When any file changes, sparse
// IDF is recomputed globally and all current chunks are re-upserted so sparse
// weights stay consistent across the collection.
func (p *Pipeline) Run(ctx context.Context, repoPath string) (runErr error) {
	metrics := IndexMetrics{Collection: p.collection, Status: "failed"}
	startedAt := time.Now()
	var denseCacheStore *denseCache
	defer func() {
		metrics.TotalMS = time.Since(startedAt).Milliseconds()
		if runErr == nil {
			metrics.Status = "completed"
		} else {
			metrics.Error = runErr.Error()
		}
		if denseCacheStore != nil {
			metrics.DenseCacheHits = denseCacheStore.hits
			metrics.DenseCacheMisses = denseCacheStore.misses
		}
		if p.metricsSink != nil {
			p.metricsSink(metrics)
		}
	}()
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	repoName := filepath.Base(absPath)
	lease, err := acquireRunLease(p.runStateDir, p.collection)
	if err != nil {
		return err
	}
	if lease != nil && lease.lockFile != nil {
		defer func() {
			if runErr != nil {
				_ = lease.fail(runErr)
			} else if err := lease.complete(); err != nil {
				runErr = err
			}
			if err := lease.release(); err != nil && runErr == nil {
				runErr = err
			}
		}()
	}

	// Step 1: Walk source files.
	files, err := WalkFiles(absPath, p.cfg)
	if err != nil {
		return fmt.Errorf("walking files in %s: %w", repoName, err)
	}
	files = p.filterFilesByLanguage(files)
	metrics.FilesScanned = len(files)
	fmt.Printf("Found %d source files in %s\n", len(files), repoName)

	// Step 2: Hash all source files on disk.
	diskHashes, err := HashFiles(files)
	if err != nil {
		return fmt.Errorf("hashing files: %w", err)
	}
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
	embedderIdentity, err := embedderCacheIdentity(ctx, p.embedder)
	if err != nil {
		return err
	}
	currentRepresentation := p.currentRepresentation(embedderIdentity)
	if lease != nil && lease.lockFile != nil {
		fingerprint := inputFingerprint(p.collection, repoName, currentRepresentation, p.languageKeys(), p.chunkSize, p.chunkOverlap, relHashes)
		if err := lease.begin(p.collection, absPath, currentRepresentation, fingerprint, len(files)); err != nil {
			return err
		}
	}

	// Step 3: Ensure payload indexes exist for efficient filtered scroll/delete.
	// This is a no-op if the collection doesn't exist yet or indexes already exist.
	// The durable marker is published before this call because schema setup can
	// mutate the collection just like point writes can.
	if err := p.store.EnsurePayloadIndexes(ctx, p.collection); err != nil {
		return fmt.Errorf("ensuring payload indexes: %w", err)
	}

	// Step 4: Get existing file hashes and index versions from Qdrant.
	// Scope the scroll to the same languages being indexed, so language-filtered
	// re-indexing (e.g. --language go) doesn't see points for other languages
	// and misclassify them as stale.
	existingStates, err := p.store.ScrollFileStates(ctx, p.collection, repoName, p.languageKeys())
	if err != nil {
		return fmt.Errorf("scrolling existing file states: %w", err)
	}

	// Step 5: Classify files.
	// Convert absolute paths to relative paths for comparison with Qdrant payloads.
	var filesToIndex []string        // new or changed files (absolute paths)
	var staleFiles []string          // exist in Qdrant but not on disk (relative paths)
	var changedFiles []string        // exist in Qdrant with different hash (relative paths)
	var versionRefreshFiles []string // same content, but old representation IDs/payloads
	versionRefreshes := 0
	newFiles := 0
	skipped := 0

	for absFile, hash := range diskHashes {
		rel := absToRel[absFile]
		existingState, exists := existingStates[rel]
		hashMatches := exists && existingState.FileHash == hash
		versionMatches := exists && !existingState.MixedState && existingState.IndexVersion == currentRepresentation
		if hashMatches && versionMatches {
			// File unchanged and already indexed with the current representation.
			skipped++
			continue
		}
		if !exists {
			newFiles++
		} else if !hashMatches {
			// File changed — mark for post-upsert cleanup.
			changedFiles = append(changedFiles, rel)
		} else {
			// File content is unchanged, but tokenizer/chunker representation changed.
			versionRefreshes++
			versionRefreshFiles = append(versionRefreshFiles, rel)
		}
		// New, changed, or stale-index-version files are re-indexed.
		filesToIndex = append(filesToIndex, absFile)
	}

	// Stale files: exist in Qdrant but not on disk.
	for existingRel := range existingStates {
		if _, onDisk := relHashes[existingRel]; !onDisk {
			staleFiles = append(staleFiles, existingRel)
		}
	}

	changedCount := len(changedFiles)
	staleCount := len(staleFiles)

	fmt.Printf("Change detection: %d unchanged, %d changed, %d new, %d stale, %d index-version refresh\n",
		skipped, changedCount, newFiles, staleCount, versionRefreshes)

	// Step 6: Delete stale file points immediately (no replacement coming).
	if len(staleFiles) > 0 {
		if err := p.store.DeleteByFilePaths(ctx, p.collection, repoName, staleFiles); err != nil {
			return fmt.Errorf("deleting stale points: %w", err)
		}
		fmt.Printf("Deleted points for %d stale files\n", len(staleFiles))
	}

	// A representation refresh can keep the same file hash while changing
	// deterministic chunk IDs. Delete the old file points first; the normal
	// stale-hash cleanup cannot remove them because their file hash is current.
	if len(versionRefreshFiles) > 0 {
		if err := p.store.DeleteByFilePaths(ctx, p.collection, repoName, versionRefreshFiles); err != nil {
			return fmt.Errorf("deleting points for representation refresh: %w", err)
		}
		fmt.Printf("Deleted points for %d representation-refresh files\n", len(versionRefreshFiles))
	}

	if len(filesToIndex) == 0 && len(staleFiles) == 0 {
		fmt.Println("Everything up to date — nothing to index")
		return nil
	}
	if len(files) == 0 {
		fmt.Println("Cleaned up stale files — no current source files to index")
		return nil
	}

	// Step 7: Chunk all current files. Sparse IDF is corpus-wide, so any
	// collection-changing run must refresh unchanged files with the same IDF map.
	indexedAt := time.Now().UTC().Format(time.RFC3339)
	chunkStarted := time.Now()
	allChunks := make([]model.CodeChunk, 0, len(files)*2)
	for _, file := range files {
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
			chunks[i].IndexVersion = currentRepresentation
		}
		allChunks = append(allChunks, chunks...)
	}
	metrics.ChunkMS = time.Since(chunkStarted).Milliseconds()
	metrics.ChunksGenerated = len(allChunks)
	fmt.Printf("Generated %d chunks from %d files\n", len(allChunks), len(files))

	if len(allChunks) == 0 {
		return fmt.Errorf("no chunks generated from %s", repoName)
	}
	denseCache, err := openDenseCache(p.runStateDir, p.collection)
	if err != nil {
		return err
	}
	denseCacheStore = denseCache

	// Step 7.5: Build enriched texts once and compute global IDF over the full
	// chunk corpus for this indexing run.
	//
	// IDF scope caveat: this map is computed over all current chunks in this
	// run's indexing scope after the language filter (see p.languageKeys()).
	// When re-indexing a single language with --language go, the IDF reflects
	// only Go chunks. Other languages already in the collection keep the IDF
	// weights they were written with. For a multi-language collection this
	// produces inconsistent sparse weighting across languages, which is fine
	// for filtered search (where the filter narrows results to one language
	// anyway) but not ideal for cross-language hybrid search. Workaround:
	// use a separate collection per language, or force a full re-index by
	// deleting the collection.
	allTexts := make([]string, len(allChunks))
	for i, chunk := range allChunks {
		allTexts[i] = enrichForEmbedding(chunk)
	}
	statsStarted := time.Now()
	corpusStats := embedding.ComputeCorpusStats(allTexts)
	metrics.SparseStatsMS = time.Since(statsStarted).Milliseconds()

	// Step 8: Embed and upsert in batches (dense + sparse).
	const batchSize = 32
	collectionReady := false
	expectedDim := 0

	for start := 0; start < len(allChunks); start += batchSize {
		end := start + batchSize
		if end > len(allChunks) {
			end = len(allChunks)
		}
		batch := allChunks[start:end]

		// Reuse precomputed enriched texts for this batch.
		texts := allTexts[start:end]

		vectors := make([][]float32, len(batch))
		missingTexts := make([]string, 0, len(batch))
		missingIndexes := make([]int, 0, len(batch))
		cacheKeys := make([]string, len(batch))
		for i, text := range texts {
			key := denseCacheKey(embedderIdentity, currentRepresentation, text)
			cacheKeys[i] = key
			if cached, ok := denseCache.get(key, expectedDim); ok {
				vectors[i] = cached
				if expectedDim == 0 {
					expectedDim = len(cached)
				}
				continue
			}
			missingTexts = append(missingTexts, text)
			missingIndexes = append(missingIndexes, i)
		}

		if len(missingTexts) > 0 {
			metrics.DenseCalls++
			metrics.DenseInputs += len(missingTexts)
			denseStarted := time.Now()
			embeddings, err := p.embedder.Embed(ctx, missingTexts)
			metrics.DenseMS += time.Since(denseStarted).Milliseconds()
			if err != nil {
				return fmt.Errorf("embedding batch %d-%d: %w", start, end, err)
			}
			if len(embeddings) != len(missingIndexes) {
				return fmt.Errorf("embedding batch %d-%d: expected %d vectors, got %d", start, end, len(missingIndexes), len(embeddings))
			}
			detectedDim, err := embedding.ValidateVectorBatch(embeddings, expectedDim)
			if err != nil {
				return fmt.Errorf("validating embedding batch %d-%d: %w", start, end, err)
			}
			expectedDim = detectedDim
			for i, vector := range embeddings {
				index := missingIndexes[i]
				vectors[index] = vector
				if err := denseCache.put(cacheKeys[index], vector); err != nil {
					return err
				}
			}
		}
		if _, err := embedding.ValidateVectorBatch(vectors, expectedDim); err != nil {
			return fmt.Errorf("validating cached embedding batch %d-%d: %w", start, end, err)
		}

		// Build sparse vectors from the same enriched texts using BM25 over
		// the global corpus stats (IDF + average document length).
		sparseVectors := embedding.BuildSparseVectors(texts, corpusStats)
		metrics.SparseVectorsBuilt += len(sparseVectors)
		if len(sparseVectors) != len(batch) {
			return fmt.Errorf("building sparse vectors for batch %d-%d: expected %d vectors, got %d", start, end, len(batch), len(sparseVectors))
		}

		// After the first batch, infer dimension and ensure the collection.
		if !collectionReady {
			dim := uint64(expectedDim)
			fmt.Printf("Detected vector dimension: %d\n", dim)
			if err := p.store.EnsureCollection(ctx, p.collection, dim); err != nil {
				return fmt.Errorf("ensuring collection: %w", err)
			}
			collectionReady = true
		}

		// Upsert to Qdrant.
		upsertStarted := time.Now()
		if err := p.store.Upsert(ctx, p.collection, batch, vectors, sparseVectors); err != nil {
			metrics.UpsertMS += time.Since(upsertStarted).Milliseconds()
			return fmt.Errorf("upserting batch %d-%d: %w", start, end, err)
		}
		metrics.UpsertMS += time.Since(upsertStarted).Milliseconds()
		metrics.UpsertBatches++

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
	verifyStarted := time.Now()
	if err := p.verifySourceSnapshot(absPath, relHashes); err != nil {
		metrics.SourceVerificationMS += time.Since(verifyStarted).Milliseconds()
		return err
	}
	metrics.SourceVerificationMS += time.Since(verifyStarted).Milliseconds()

	fmt.Printf("Successfully indexed %d chunks into collection %q\n", len(allChunks), p.collection)
	fmt.Println(denseCache.summary())
	return nil
}

// verifySourceSnapshot rejects completion when files changed while the index
// was being built. The next run then replays the affected scope from a fresh
// manifest instead of recording a false successful completion.
func (p *Pipeline) verifySourceSnapshot(absPath string, expected map[string]string) error {
	files, err := WalkFiles(absPath, p.cfg)
	if err != nil {
		return fmt.Errorf("verifying source snapshot: %w", err)
	}
	files = p.filterFilesByLanguage(files)
	current, err := HashFiles(files)
	if err != nil {
		return fmt.Errorf("hashing source snapshot for verification: %w", err)
	}
	actual := make(map[string]string, len(current))
	for path, hash := range current {
		rel, err := filepath.Rel(absPath, path)
		if err != nil {
			rel = path
		}
		actual[rel] = hash
	}
	if len(actual) == len(expected) {
		matches := true
		for path, hash := range expected {
			if actual[path] != hash {
				matches = false
				break
			}
		}
		if matches {
			return nil
		}
	}
	changed := make([]string, 0)
	for path, hash := range expected {
		if actual[path] != hash {
			changed = append(changed, path)
		}
	}
	for path := range actual {
		if _, ok := expected[path]; !ok {
			changed = append(changed, path)
		}
	}
	return fmt.Errorf("source changed during indexing; retry required (files: %s)", strings.Join(changed, ", "))
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
