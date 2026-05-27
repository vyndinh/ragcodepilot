package answer

import (
	"fmt"

	"github.com/dinhvy/ragcodepilot/internal/model"
)

// ContextsFromResults maps retrieved search results into 1-based, citation-ready
// chunk contexts for the answer prompt. Order is preserved (best result first),
// so citation [1] always refers to the top-ranked chunk.
func ContextsFromResults(results []model.SearchResult) []ChunkContext {
	chunks := make([]ChunkContext, len(results))
	for i, r := range results {
		chunks[i] = ChunkContext{
			Index:     i + 1,
			Repo:      r.Chunk.Repo,
			FilePath:  r.Chunk.FilePath,
			Lines:     fmt.Sprintf("%d-%d", r.Chunk.StartLine, r.Chunk.EndLine),
			Symbol:    r.Chunk.Name,
			ChunkType: r.Chunk.ChunkType,
			Content:   r.Chunk.Content,
		}
	}
	return chunks
}
