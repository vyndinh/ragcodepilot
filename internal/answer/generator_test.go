package answer

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestFakeGenerator_ReturnsResponse(t *testing.T) {
	t.Parallel()

	gen := NewFakeGenerator("test answer")
	prompt := Prompt{
		Question: "how does indexing work?",
		Chunks: []ChunkContext{
			{Index: 1, FilePath: "internal/ingest/pipeline.go", Lines: "42-78", Symbol: "Pipeline.Run", Content: "func (p *Pipeline) Run() {}"},
		},
	}

	got, err := gen.Generate(context.Background(), prompt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "test answer" {
		t.Errorf("got %q, want %q", got, "test answer")
	}
}

func TestFakeGenerator_RecordsPrompt(t *testing.T) {
	t.Parallel()

	gen := NewFakeGenerator("ok")
	prompt := Prompt{
		Question: "what is hybrid search?",
		Chunks: []ChunkContext{
			{Index: 1, FilePath: "a.go", Lines: "1-10", Symbol: "Foo", Content: "code"},
			{Index: 2, FilePath: "b.go", Lines: "20-30", Symbol: "Bar", Content: "more code"},
		},
	}

	_, _ = gen.Generate(context.Background(), prompt)

	if gen.LastPrompt.Question != "what is hybrid search?" {
		t.Errorf("LastPrompt.Question = %q, want %q", gen.LastPrompt.Question, "what is hybrid search?")
	}
	if len(gen.LastPrompt.Chunks) != 2 {
		t.Errorf("LastPrompt.Chunks length = %d, want 2", len(gen.LastPrompt.Chunks))
	}
}

func TestFakeGenerator_CallCount(t *testing.T) {
	t.Parallel()

	gen := NewFakeGenerator("ok")
	prompt := Prompt{Question: "q"}

	for range 3 {
		_, _ = gen.Generate(context.Background(), prompt)
	}

	if gen.CallCount != 3 {
		t.Errorf("CallCount = %d, want 3", gen.CallCount)
	}
}

func TestFakeGenerator_ReturnsError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("model unavailable")
	gen := &FakeGenerator{Err: wantErr}
	prompt := Prompt{Question: "q"}

	_, err := gen.Generate(context.Background(), prompt)
	if !errors.Is(err, wantErr) {
		t.Errorf("got error %v, want %v", err, wantErr)
	}
}

func TestFakeGenerator_DefaultResponse(t *testing.T) {
	t.Parallel()

	gen := &FakeGenerator{} // no Response set
	prompt := Prompt{
		Question: "where is the chunker?",
		Chunks: []ChunkContext{
			{Index: 1, FilePath: "chunker.go", Lines: "1-50", Symbol: "ChunkFile", Content: "code"},
		},
	}

	got, err := gen.Generate(context.Background(), prompt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "where is the chunker?") {
		t.Errorf("default response should contain the question, got %q", got)
	}
	if !strings.Contains(got, "1 chunk(s)") {
		t.Errorf("default response should mention chunk count, got %q", got)
	}
	if !strings.Contains(got, "ChunkFile") {
		t.Errorf("default response should mention chunk symbol, got %q", got)
	}
}

func TestFakeGenerator_ImplementsInterface(t *testing.T) {
	t.Parallel()

	// Compile-time check is in fake.go (var _ Generator = (*FakeGenerator)(nil)).
	// No runtime test needed — the compiler enforces this.
}

func TestChunkContext_ZeroValue(t *testing.T) {
	t.Parallel()

	var c ChunkContext
	if c.Index != 0 || c.FilePath != "" || c.Symbol != "" || c.Content != "" {
		t.Error("zero-value ChunkContext should have empty fields")
	}
}

func TestPrompt_EmptyChunks(t *testing.T) {
	t.Parallel()

	gen := &FakeGenerator{}
	prompt := Prompt{
		Question: "anything",
		Chunks:   []ChunkContext{},
	}

	got, err := gen.Generate(context.Background(), prompt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "0 chunk(s)") {
		t.Errorf("should handle empty chunks, got %q", got)
	}
}
