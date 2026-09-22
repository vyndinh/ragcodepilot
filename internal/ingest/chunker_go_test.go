package ingest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dinhvy/ragcodepilot/internal/config"
	"github.com/dinhvy/ragcodepilot/internal/model"
)

func TestChunkGoFile_FunctionExtraction(t *testing.T) {
	t.Parallel()

	src := `package example

import "fmt"

// Greet returns a greeting message.
func Greet(name string) string {
	return fmt.Sprintf("Hello, %s!", name)
}

// Farewell returns a farewell message.
func Farewell(name string) string {
	return "Goodbye, " + name
}
`
	chunks := chunkGoSource(t, src)

	// Should produce: 1 block (package + import) + 2 functions
	funcChunks := filterByType(chunks, "function")
	if len(funcChunks) != 2 {
		t.Fatalf("expected 2 function chunks, got %d", len(funcChunks))
	}

	// First function should be Greet
	if funcChunks[0].Name != "Greet" {
		t.Errorf("first function name = %q, want Greet", funcChunks[0].Name)
	}
	if !strings.Contains(funcChunks[0].Content, "// Greet returns") {
		t.Error("Greet chunk should include doc comment")
	}
	if !strings.Contains(funcChunks[0].Content, "return fmt.Sprintf") {
		t.Error("Greet chunk should include function body")
	}

	// Second function should be Farewell
	if funcChunks[1].Name != "Farewell" {
		t.Errorf("second function name = %q, want Farewell", funcChunks[1].Name)
	}
}

func TestChunkGoFile_MethodReceiver(t *testing.T) {
	t.Parallel()

	src := `package example

type Server struct{}

func (s *Server) Start() error {
	return nil
}

func (s *Server) Stop() {
}
`
	chunks := chunkGoSource(t, src)

	funcChunks := filterByType(chunks, "function")
	if len(funcChunks) != 2 {
		t.Fatalf("expected 2 method chunks, got %d", len(funcChunks))
	}

	if funcChunks[0].Name != "Start" {
		t.Errorf("first method name = %q, want Start", funcChunks[0].Name)
	}
	if funcChunks[1].Name != "Stop" {
		t.Errorf("second method name = %q, want Stop", funcChunks[1].Name)
	}
}

func TestChunkGoFile_MethodReceiverIdentityPreventsCollisions(t *testing.T) {
	t.Parallel()

	src := `package example

type Server struct{}
type Client struct{}

func (s *Server) Close() {}
func (c *Client) Close() {}
`
	chunks := filterByType(chunkGoSource(t, src), "function")
	if len(chunks) != 2 {
		t.Fatalf("expected 2 method chunks, got %d", len(chunks))
	}
	if chunks[0].Name != "Close" || chunks[1].Name != "Close" {
		t.Fatalf("method names = %q, %q; want both Close", chunks[0].Name, chunks[1].Name)
	}
	if chunks[0].ID == chunks[1].ID {
		t.Fatalf("same-name methods collided on ID %q", chunks[0].ID)
	}
}

func TestChunkGoFile_PackageFunctionAndMethodIdentityPreventCollisions(t *testing.T) {
	t.Parallel()

	src := `package example

type Server struct{}

func Close() {}
func (s *Server) Close() {}
`
	chunks := filterByType(chunkGoSource(t, src), "function")
	if len(chunks) != 2 {
		t.Fatalf("expected 2 function chunks, got %d", len(chunks))
	}
	if chunks[0].Name != "Close" || chunks[1].Name != "Close" {
		t.Fatalf("function names = %q, %q; want both Close", chunks[0].Name, chunks[1].Name)
	}
	if chunks[0].ID == chunks[1].ID {
		t.Fatalf("package function and method collided on ID %q", chunks[0].ID)
	}
}

func TestChunkGoFile_BlockChunks(t *testing.T) {
	t.Parallel()

	src := `package example

import (
	"fmt"
	"os"
)

var Version = "1.0"

type Config struct {
	Host string
	Port int
}

func Run() {}
`
	chunks := chunkGoSource(t, src)

	blockChunks := filterByType(chunks, "block")
	if len(blockChunks) == 0 {
		t.Fatal("expected at least one block chunk for imports/vars")
	}

	allBlockContent := ""
	for _, c := range blockChunks {
		allBlockContent += c.Content + "\n"
	}
	if !strings.Contains(allBlockContent, "import") {
		t.Error("block chunks should contain import")
	}
	if !strings.Contains(allBlockContent, "var Version") {
		t.Error("block chunks should contain var declaration")
	}

	typeChunks := filterByType(chunks, "type")
	if len(typeChunks) != 1 {
		t.Fatalf("expected 1 type chunk for Config, got %d", len(typeChunks))
	}
	if typeChunks[0].Name != "Config" {
		t.Errorf("type chunk name = %q, want Config", typeChunks[0].Name)
	}
	if !strings.Contains(typeChunks[0].Content, "type Config struct") {
		t.Error("type chunk should contain the struct declaration")
	}
}

func TestChunkGoFile_TypeAndInterfaceExtraction(t *testing.T) {
	t.Parallel()

	src := `package example

// Embedder turns text into vectors.
type Embedder interface {
	Embed() error
	Dimension() int
}

type Config struct {
	Host string
}

type Handler func() error
`
	chunks := chunkGoSource(t, src)

	ifaces := filterByType(chunks, "interface")
	if len(ifaces) != 1 {
		t.Fatalf("expected 1 interface chunk, got %d", len(ifaces))
	}
	if ifaces[0].Name != "Embedder" {
		t.Errorf("interface name = %q, want Embedder", ifaces[0].Name)
	}
	if !strings.Contains(ifaces[0].Content, "type Embedder interface") {
		t.Error("interface chunk should include the type keyword and name")
	}
	if !strings.Contains(ifaces[0].Content, "// Embedder turns text into vectors.") {
		t.Error("interface chunk should include the doc comment")
	}

	types := filterByType(chunks, "type")
	if len(types) != 2 {
		t.Fatalf("expected 2 type chunks (Config, Handler), got %d", len(types))
	}
	names := map[string]bool{}
	for _, c := range types {
		names[c.Name] = true
	}
	if !names["Config"] || !names["Handler"] {
		t.Errorf("type names = %v, want Config and Handler", names)
	}
}

func TestChunkGoFile_GroupedTypes(t *testing.T) {
	t.Parallel()

	src := `package example

type (
	Reader interface {
		Read([]byte) (int, error)
	}
	Buffer struct {
		n int
	}
)
`
	chunks := chunkGoSource(t, src)

	ifaces := filterByType(chunks, "interface")
	if len(ifaces) != 1 || ifaces[0].Name != "Reader" {
		t.Fatalf("grouped interface = %+v, want Reader", ifaces)
	}
	types := filterByType(chunks, "type")
	if len(types) != 1 || types[0].Name != "Buffer" {
		t.Fatalf("grouped type = %+v, want Buffer", types)
	}
	if !strings.Contains(ifaces[0].Content, "Reader interface") {
		t.Errorf("Reader chunk content = %q", ifaces[0].Content)
	}
	if !strings.Contains(types[0].Content, "Buffer struct") {
		t.Errorf("Buffer chunk content = %q", types[0].Content)
	}
}

func TestChunkGoFile_EmptyFile(t *testing.T) {
	t.Parallel()

	src := `package example
`
	chunks := chunkGoSource(t, src)

	// Should produce a single block chunk with the package declaration.
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk for package declaration")
	}
	funcChunks := filterByType(chunks, "function")
	if len(funcChunks) != 0 {
		t.Errorf("expected 0 function chunks, got %d", len(funcChunks))
	}
}

func TestChunkGoFile_SyntaxErrorFallback(t *testing.T) {
	t.Parallel()

	// Invalid Go code should fall back to generic chunker.
	src := `package example

func broken( {
	this is not valid go
}
`
	chunks := chunkGoSource(t, src)

	// Should still produce chunks via the generic fallback.
	if len(chunks) == 0 {
		t.Fatal("expected chunks from generic fallback")
	}
}

func TestChunkGoFile_LargeFunctionSplit(t *testing.T) {
	t.Parallel()

	// Create a function with > maxFunctionLines lines.
	var b strings.Builder
	b.WriteString("package example\n\nfunc BigFunc() {\n")
	for i := 0; i < maxFunctionLines+20; i++ {
		b.WriteString("\t_ = " + strings.Repeat("x", 10) + "\n")
	}
	b.WriteString("}\n")

	chunks := chunkGoSource(t, b.String())

	funcChunks := filterByType(chunks, "function")
	if len(funcChunks) < 2 {
		t.Fatalf("expected large function to be split into >= 2 chunks, got %d", len(funcChunks))
	}

	// First sub-chunk should have the function name.
	if funcChunks[0].Name != "BigFunc" {
		t.Errorf("first sub-chunk name = %q, want BigFunc", funcChunks[0].Name)
	}

	// Subsequent sub-chunks should not have the name (to avoid duplicate enrichment).
	for i := 1; i < len(funcChunks); i++ {
		if funcChunks[i].Name != "" {
			t.Errorf("sub-chunk %d should have empty name, got %q", i, funcChunks[i].Name)
		}
	}
}

func TestChunkGoFile_SortedByStartLine(t *testing.T) {
	t.Parallel()

	src := `package example

import "fmt"

func Alpha() { fmt.Println("a") }

var X = 1

func Beta() { fmt.Println("b") }
`
	chunks := chunkGoSource(t, src)

	for i := 1; i < len(chunks); i++ {
		if chunks[i].StartLine < chunks[i-1].StartLine {
			t.Errorf("chunks not sorted: chunk %d (line %d) before chunk %d (line %d)",
				i-1, chunks[i-1].StartLine, i, chunks[i].StartLine)
		}
	}
}

func TestChunkGoFile_ChunkTypeAndLanguage(t *testing.T) {
	t.Parallel()

	src := `package example

func Hello() {}
`
	chunks := chunkGoSource(t, src)
	for _, c := range chunks {
		if c.Language != "go" {
			t.Errorf("chunk language = %q, want go", c.Language)
		}
		switch c.ChunkType {
		case "function", "block", "type", "interface":
		default:
			t.Errorf("unexpected chunk type %q", c.ChunkType)
		}
	}
}

// --- helpers ---

func chunkGoSource(t *testing.T, src string) []model.CodeChunk {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	cfg := config.Default()
	chunks, err := chunkGoFile(path, dir, "testrepo", DefaultChunkLines, DefaultChunkOverlap, cfg)
	if err != nil {
		t.Fatalf("chunkGoFile() error: %v", err)
	}
	return chunks
}

func filterByType(chunks []model.CodeChunk, chunkType string) []model.CodeChunk {
	var result []model.CodeChunk
	for _, c := range chunks {
		if c.ChunkType == chunkType {
			result = append(result, c)
		}
	}
	return result
}

// --- ChunkFile router tests ---

func TestChunkFile_RoutesGoToAST(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	src := "package main\n\nfunc Hello() {}\n\nfunc World() {}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cfg := config.Default()
	chunks, err := ChunkFile(path, dir, "testrepo", DefaultChunkLines, DefaultChunkOverlap, cfg)
	if err != nil {
		t.Fatalf("ChunkFile() error: %v", err)
	}

	// AST chunker produces separate function chunks.
	funcChunks := filterByType(chunks, "function")
	if len(funcChunks) != 2 {
		t.Fatalf("expected 2 function chunks from AST chunker, got %d", len(funcChunks))
	}
	if funcChunks[0].Name != "Hello" {
		t.Errorf("first function = %q, want Hello", funcChunks[0].Name)
	}
	if funcChunks[1].Name != "World" {
		t.Errorf("second function = %q, want World", funcChunks[1].Name)
	}
}

func TestChunkFile_RoutesNonGoToGeneric(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "script.py")
	src := "def hello():\n    print('hi')\n\ndef world():\n    print('world')\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cfg := config.Default()
	chunks, err := ChunkFile(path, dir, "testrepo", DefaultChunkLines, DefaultChunkOverlap, cfg)
	if err != nil {
		t.Fatalf("ChunkFile() error: %v", err)
	}

	// Generic chunker produces a single sliding-window chunk for a small file.
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk from generic chunker")
	}
	// The generic chunker uses regex extractName, which finds "hello" for Python.
	if chunks[0].Language != "python" {
		t.Errorf("chunk language = %q, want python", chunks[0].Language)
	}
}
