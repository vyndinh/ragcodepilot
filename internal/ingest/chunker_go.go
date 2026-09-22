package ingest

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	gotoken "go/token"
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
// per function/method and per named type/interface. Remaining code (imports,
// vars, consts) is collected into "block" chunks. If the file has syntax
// errors, it falls back to the generic sliding-window chunker.
func chunkGoFile(filePath, repoRoot, repo string, chunkSize, overlap int, cfg *config.Config) ([]model.CodeChunk, error) {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filePath, err)
	}

	fset := gotoken.NewFileSet()
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

	// Track lines covered by named declarations so remaining gaps (imports,
	// vars, consts) can be collected as "block" chunks.
	covered := make([]bool, len(lines))

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			startPos := d.Pos()
			if d.Doc != nil {
				startPos = d.Doc.Pos()
			}
			startLine := fset.Position(startPos).Line
			endLine := fset.Position(d.End()).Line
			markCovered(covered, startLine, endLine)
			chunks = append(chunks, namedGoChunks(lines, startLine, endLine, relPath, repo, language, chunkSize, overlap, "function", d.Name.Name, goFuncIdentity(fset, d))...)
		case *ast.GenDecl:
			if d.Tok != gotoken.TYPE {
				continue
			}
			for i, s := range d.Specs {
				spec, ok := s.(*ast.TypeSpec)
				if !ok || spec.Name == nil || spec.Name.Name == "_" {
					continue
				}
				startLine, endLine := typeSpecLines(fset, d, spec, i)
				markCovered(covered, startLine, endLine)
				name := spec.Name.Name
				chunks = append(chunks, namedGoChunks(lines, startLine, endLine, relPath, repo, language, chunkSize, overlap, goTypeChunkKind(spec), name, "type:"+name)...)
			}
		}
	}

	// Collect leftover code (imports, vars, consts) as gap chunks.
	gapChunks := collectGapChunks(lines, covered, relPath, repo, language, chunkSize, overlap)
	chunks = append(chunks, gapChunks...)

	// Sort chunks by start line for deterministic output.
	sortChunksByStartLine(chunks)

	return chunks, nil
}

func markCovered(covered []bool, startLine, endLine int) {
	for i := startLine - 1; i < endLine && i < len(covered); i++ {
		if i >= 0 {
			covered[i] = true
		}
	}
}

// typeSpecLines returns 1-based inclusive source lines for one TypeSpec.
// The first spec in a GenDecl includes the `type` keyword and group doc;
// the last spec includes a closing `)` of a grouped declaration.
func typeSpecLines(fset *gotoken.FileSet, decl *ast.GenDecl, spec *ast.TypeSpec, specIndex int) (startLine, endLine int) {
	startPos := spec.Pos()
	if spec.Doc != nil {
		startPos = spec.Doc.Pos()
	}
	if specIndex == 0 {
		if decl.Doc != nil {
			startPos = decl.Doc.Pos()
		} else {
			startPos = decl.Pos()
		}
	}
	endPos := spec.End()
	if specIndex == len(decl.Specs)-1 {
		endPos = decl.End()
	}
	return fset.Position(startPos).Line, fset.Position(endPos).Line
}

func goTypeChunkKind(spec *ast.TypeSpec) string {
	if _, ok := spec.Type.(*ast.InterfaceType); ok {
		return "interface"
	}
	return "type"
}

func namedGoChunks(lines []string, startLine, endLine int, relPath, repo, language string, chunkSize, overlap int, chunkType, name, identity string) []model.CodeChunk {
	nLines := endLine - startLine + 1
	if nLines > maxFunctionLines {
		return splitLargeBlock(lines, startLine, endLine, relPath, repo, language, chunkSize, overlap, chunkType, name, identity)
	}
	return []model.CodeChunk{{
		ID:        generateChunkIDWithIdentity(repo, relPath, identity, 0),
		Repo:      repo,
		FilePath:  relPath,
		Language:  language,
		ChunkType: chunkType,
		Name:      name,
		Content:   joinLines(lines, startLine, endLine),
		StartLine: startLine,
		EndLine:   endLine,
	}}
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
		return splitLargeBlock(lines, startIdx+1, endIdx+1, relPath, repo, language, chunkSize, overlap, "block", "", "")
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
func splitLargeBlock(lines []string, startLine, endLine int, relPath, repo, language string, chunkSize, overlap int, chunkType, name, identity string) []model.CodeChunk {
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
			ID:        generateChunkIDWithIdentity(repo, relPath, identity, start),
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

// goFuncIdentity returns a stable declaration identity while keeping the
// display name unchanged. Receiver identity prevents methods such as
// (*Server).Close and (*Client).Close from overwriting each other.
func goFuncIdentity(fset *gotoken.FileSet, decl *ast.FuncDecl) string {
	if decl.Recv == nil || len(decl.Recv.List) == 0 {
		return "func:" + decl.Name.Name
	}

	var receiver bytes.Buffer
	if err := format.Node(&receiver, fset, decl.Recv.List[0].Type); err != nil {
		// go/parser already accepted the declaration. Keep a deterministic
		// fallback if formatting ever fails for a future Go syntax node.
		receiver.WriteString("receiver")
	}
	return "method:" + receiver.String() + "." + decl.Name.Name
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
