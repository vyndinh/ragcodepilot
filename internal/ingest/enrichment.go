package ingest

import (
	"fmt"
	"strings"

	"github.com/dinhvy/ragcodepilot/internal/model"
)

// enrichForEmbedding prepends structured metadata to a chunk's raw code
// content before sending it to the embedding model. The metadata includes
// file path, language, and chunk type/name, giving the embedding model
// human-readable context that improves semantic search for natural-language
// queries.
//
// The original chunk.Content is NOT modified — only the text sent to the
// embedder changes. Qdrant payload still stores the raw code.
func enrichForEmbedding(chunk model.CodeChunk) string {
	var b strings.Builder

	fmt.Fprintf(&b, "File: %s\n", chunk.FilePath)
	fmt.Fprintf(&b, "Language: %s\n", chunk.Language)

	label := chunkTypeLabel(chunk.ChunkType)
	if chunk.Name != "" {
		fmt.Fprintf(&b, "%s: %s\n", label, chunk.Name)
	} else {
		fmt.Fprintf(&b, "Type: %s\n", label)
	}

	b.WriteString("\n")
	b.WriteString(chunk.Content)

	return b.String()
}

// chunkTypeLabel returns a human-readable label for the chunk type.
// Uses an explicit switch instead of the deprecated strings.Title.
func chunkTypeLabel(chunkType string) string {
	switch chunkType {
	case "function":
		return "Function"
	case "block":
		return "Block"
	default:
		return "Chunk"
	}
}
