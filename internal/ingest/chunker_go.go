package ingest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/dinhvy/ragcodepilot/internal/config"
	"github.com/dinhvy/ragcodepilot/internal/model"
)

// maxFunctionLines is the threshold above which a single function is split
// using the sliding-window fallback to keep chunks at a reasonable size.
const maxFunctionLines = 80

// chunkGoFile parses a Go source file using go/ast and produces one chunk
// per function/method declaration. Code between functions (imports, types,
// package-level vars) is collected into "block" chunks. If the file has
// syntax errors, it falls back to the generic sliding-window chunker.
func chunkGoFile(filePath, repoRoot, repo string, chunkSize, overlap int, cfg *config.Config) ([]model.CodeChunk, error) {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filePath, err)
	}

	fset := token.NewFileSet()
	file, parseErr := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if parseErr != nil {
		// Syntax error — fall back to generic chunker.
		return chunkGeneric(filePath, repoRoot, repo, chunkSize, overlap, cfg)
	}

	language := cfg.DetectLanguage(filePath)
	relPath, err := filepath.Rel(repoRoot, filePath)
	if err != nil {
		relPath = filePath
	}

	lines := strings.Split(string(src), "\n")
	if len(lines) == 0 {
		return nil, nil
	}

	var chunks []model.CodeChunk

	// Track which lines are covered by function declarations so we can
	// collect the gaps as "block" chunks.
	covered := make([]bool, len(lines))

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		// Determine the start line, including the doc comment if present.
		startPos := fn.Pos()
		if fn.Doc != nil {
			startPos = fn.Doc.Pos()
		}
		startLine := fset.Position(startPos).Line // 1-based
		endLine := fset.Position(fn.End()).Line   // 1-based

		// Mark lines as covered.
		for i := startLine - 1; i < endLine && i < len(lines); i++ {
			covered[i] = i < len(lines)
		}

		name := fn.Name.Name
		content := joinLines(lines, startLine, endLine)

		funcLines := endLine - startLine + 1
		if funcLines > maxFunctionLines {
			// Large function — split with sliding window.
			subChunks := splitLargeBlock(lines, startLine, endLine, relPath, repo, language, chunkSize, overlap, "function", name)
			chunks = append(chunks, subChunks...)
		} else {
			chunks = append(chunks, model.CodeChunk{
				ID:        generateChunkID(repo, relPath, name, 0),
				Repo:      repo,
				FilePath:  relPath,
				Language:  language,
				ChunkType: "function",
				Name:      name,
				Content:   content,
				StartLine: startLine,
				EndLine:   endLine,
			})
		}
	}

	// Collect non-function code (imports, types, vars, consts) as gap chunks.
	gapChunks := collectGapChunks(lines, covered, relPath, repo, language, chunkSize, overlap)
	chunks = append(chunks, gapChunks...)

	// Sort chunks by start line for deterministic output.
	sortChunksByStartLine(chunks)

	return chunks, nil
}

// collectGapChunks gathers consecutive uncovered lines into "block" chunks.
// If a gap exceeds maxFunctionLines, it is split with a sliding window.
func collectGapChunks(lines []string, covered []bool, relPath, repo, language string, chunkSize, overlap int) []model.CodeChunk {
	var chunks []model.CodeChunk
	gapStart := -1

	for i := range lines {
		if !covered[i] {
			if gapStart == -1 {
				gapStart = i
			}
		} else {
			if gapStart != -1 {
				chunks = append(chunks, buildGapChunks(lines, gapStart, i-1, relPath, repo, language, chunkSize, overlap)...)
				gapStart = -1
			}
		}
	}
	// Handle trailing gap.
	if gapStart != -1 {
		chunks = append(chunks, buildGapChunks(lines, gapStart, len(lines)-1, relPath, repo, language, chunkSize, overlap)...)
	}

	return chunks
}

// buildGapChunks creates one or more "block" chunks from a range of lines.
// The range is 0-indexed (inclusive). Empty gaps are skipped.
func buildGapChunks(lines []string, startIdx, endIdx int, relPath, repo, language string, chunkSize, overlap int) []model.CodeChunk {
	content := joinLines(lines, startIdx+1, endIdx+1) // convert to 1-based
	if strings.TrimSpace(content) == "" {
		return nil
	}

	gapLines := endIdx - startIdx + 1
	if gapLines > maxFunctionLines {
		return splitLargeBlock(lines, startIdx+1, endIdx+1, relPath, repo, language, chunkSize, overlap, "block", "")
	}

	return []model.CodeChunk{{
		ID:        generateChunkID(repo, relPath, extractName(content, language), startIdx+1),
		Repo:      repo,
		FilePath:  relPath,
		Language:  language,
		ChunkType: "block",
		Name:      extractName(content, language),
		Content:   content,
		StartLine: startIdx + 1,
		EndLine:   endIdx + 1,
	}}
}

// splitLargeBlock splits a range of lines using the sliding-window strategy.
// startLine and endLine are 1-based inclusive.
func splitLargeBlock(lines []string, startLine, endLine int, relPath, repo, language string, chunkSize, overlap int, chunkType, name string) []model.CodeChunk {
	var chunks []model.CodeChunk

	blockLines := lines[startLine-1 : endLine]

	for start := 0; start < len(blockLines); start += chunkSize - overlap {
		end := start + chunkSize
		if end > len(blockLines) {
			end = len(blockLines)
		}

		content := strings.Join(blockLines[start:end], "\n")
		if strings.TrimSpace(content) == "" {
			continue
		}

		absStart := startLine + start
		absEnd := startLine + end - 1

		// Only the first sub-chunk inherits the function name.
		chunkName := ""
		if start == 0 {
			chunkName = name
		}

		chunks = append(chunks, model.CodeChunk{
			ID:        generateChunkID(repo, relPath, name, start),
			Repo:      repo,
			FilePath:  relPath,
			Language:  language,
			ChunkType: chunkType,
			Name:      chunkName,
			Content:   content,
			StartLine: absStart,
			EndLine:   absEnd,
		})

		if end >= len(blockLines) {
			break
		}
	}

	return chunks
}

// joinLines extracts lines[startLine-1:endLine] and joins them with newlines.
// startLine and endLine are 1-based inclusive.
func joinLines(lines []string, startLine, endLine int) string {
	if startLine < 1 {
		startLine = 1
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	return strings.Join(lines[startLine-1:endLine], "\n")
}

// sortChunksByStartLine sorts chunks in-place by their StartLine.
func sortChunksByStartLine(chunks []model.CodeChunk) {
	for i := 1; i < len(chunks); i++ {
		for j := i; j > 0 && chunks[j].StartLine < chunks[j-1].StartLine; j-- {
			chunks[j], chunks[j-1] = chunks[j-1], chunks[j]
		}
	}
}
