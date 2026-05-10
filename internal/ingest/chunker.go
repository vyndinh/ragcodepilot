package ingest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dinhvy/ragcodepilot/internal/config"
	"github.com/dinhvy/ragcodepilot/internal/model"
	"github.com/google/uuid"
)

const (
	DefaultChunkLines   = 40
	DefaultChunkOverlap = 5
)

// ChunkFile reads a source file and splits it into chunks.
// For Go files, it uses go/parser to produce function-level chunks.
// For all other languages, it falls back to a line-based sliding window.
func ChunkFile(filePath, repoRoot, repo string, chunkSize, overlap int, cfg *config.Config) ([]model.CodeChunk, error) {
	if cfg.DetectLanguage(filePath) == "go" {
		return chunkGoFile(filePath, repoRoot, repo, chunkSize, overlap, cfg)
	}
	return chunkGeneric(filePath, repoRoot, repo, chunkSize, overlap, cfg)
}

// chunkGeneric splits a file into chunks using a line-based sliding window.
// Used for non-Go files or as a fallback when Go parsing fails.
func chunkGeneric(filePath, repoRoot, repo string, chunkSize, overlap int, cfg *config.Config) ([]model.CodeChunk, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filePath, err)
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return nil, nil
	}

	language := cfg.DetectLanguage(filePath)
	relPath, err := filepath.Rel(repoRoot, filePath)
	if err != nil {
		relPath = filePath
	}
	chunks := make([]model.CodeChunk, 0, len(lines)/chunkSize+1)

	for start := 0; start < len(lines); start += chunkSize - overlap {
		end := start + chunkSize
		if end > len(lines) {
			end = len(lines)
		}

		chunkContent := strings.Join(lines[start:end], "\n")

		if strings.TrimSpace(chunkContent) == "" {
			continue
		}

		name := extractName(chunkContent, language)
		chunkType := "block"
		if name != "" {
			chunkType = "function"
		}

		chunks = append(chunks, model.CodeChunk{
			ID:        generateChunkID(repo, relPath, start+1),
			Repo:      repo,
			FilePath:  relPath,
			Language:  language,
			ChunkType: chunkType,
			Name:      name,
			Content:   chunkContent,
			StartLine: start + 1,
			EndLine:   end,
		})

		// Don't create an overlap-only chunk at the end.
		if end >= len(lines) {
			break
		}
	}

	return chunks, nil
}

// generateChunkID creates a deterministic ID from repo, file path, and start line.
// This ensures re-indexing the same file produces the same IDs.
var namespaceUUID = uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")

var namePatterns = map[string]*regexp.Regexp{
	"go":         regexp.MustCompile(`func\s+(?:\([^)]+\)\s+)?(\w+)`),
	"rust":       regexp.MustCompile(`fn\s+(\w+)`),
	"python":     regexp.MustCompile(`def\s+(\w+)`),
	"javascript": regexp.MustCompile(`function\s+(\w+)`),
	"typescript": regexp.MustCompile(`function\s+(\w+)`),
	"java":       regexp.MustCompile(`(?:public|private|protected)?\s*(?:static)?\s*\w+\s+(\w+)\s*\(`),
	"c":          regexp.MustCompile(`\w+\s+(\w+)\s*\(`),
	"cpp":        regexp.MustCompile(`\w+\s+(\w+)\s*\(`),
	"ruby":       regexp.MustCompile(`def\s+(\w+)`),
	"swift":      regexp.MustCompile(`func\s+(\w+)`),
	"kotlin":     regexp.MustCompile(`fun\s+(\w+)`),
	"scala":      regexp.MustCompile(`def\s+(\w+)`),
	"php":        regexp.MustCompile(`function\s+(\w+)`),
}

func extractName(content, language string) string {
	pattern, ok := namePatterns[language]
	if !ok {
		return ""
	}
	match := pattern.FindStringSubmatch(content)
	if len(match) >= 2 {
		return match[1]
	}
	return ""
}

func generateChunkID(repo, filePath string, startLine int) string {
	input := fmt.Sprintf("%s:%s:%d", repo, filePath, startLine)
	return uuid.NewSHA1(namespaceUUID, []byte(input)).String()
}
