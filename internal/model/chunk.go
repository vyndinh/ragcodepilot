// Package model defines the core data types shared across the application.
package model

// CodeChunk represents a single chunk of source code that will be embedded and stored.
type CodeChunk struct {
	// ID is a deterministic identifier derived from repo + file path + symbol name + chunk index.
	// Named chunks use the symbol for stability across line shifts; unnamed blocks use start_line.
	ID string `json:"id"`

	// Repo is the repository name or path this chunk came from.
	Repo string `json:"repo"`

	// FilePath is the relative file path within the repository.
	FilePath string `json:"file_path"`

	// Language is the programming language (e.g., "go", "rust", "python").
	Language string `json:"language"`

	// ChunkType describes the kind of chunk (e.g., "function", "block", "file").
	ChunkType string `json:"chunk_type"`

	// Name is the name of the function, struct, or other named entity (if any).
	Name string `json:"name,omitempty"`

	// Content is the raw source code text of this chunk.
	Content string `json:"content"`

	// StartLine is the 1-based starting line number in the original file.
	StartLine int `json:"start_line"`

	// EndLine is the 1-based ending line number in the original file.
	EndLine int `json:"end_line"`

	// IndexedAt is the timestamp when this chunk was indexed.
	IndexedAt string `json:"indexed_at,omitempty"`

	// FileHash is the SHA-256 hash of the source file content at indexing time.
	// Used for change detection during re-indexing.
	FileHash string `json:"file_hash,omitempty"`
}

// SearchResult represents a single result returned from a search query.
type SearchResult struct {
	// Chunk is the matched code chunk.
	Chunk CodeChunk `json:"chunk"`

	// Score is the similarity score (higher is more relevant).
	Score float32 `json:"score"`
}
