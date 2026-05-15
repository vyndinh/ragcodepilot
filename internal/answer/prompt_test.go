package answer

import (
	"strings"
	"testing"
)

func TestBuildPrompt_SystemIsFrozenConstant(t *testing.T) {
	t.Parallel()

	system, _ := BuildPrompt("any question", nil)
	if system != SystemPrompt {
		t.Errorf("system message must equal SystemPrompt constant\ngot:  %q\nwant: %q", system, SystemPrompt)
	}
}

func TestBuildPrompt_TwoChunksGolden(t *testing.T) {
	t.Parallel()

	chunks := []ChunkContext{
		{
			Index:    1,
			FilePath: "internal/ingest/pipeline.go",
			Lines:    "42-78",
			Symbol:   "Pipeline.Run",
			Content:  "func (p *Pipeline) Run() {\n\treturn nil\n}",
		},
		{
			Index:    2,
			FilePath: "internal/qdrant/client.go",
			Lines:    "120-145",
			Symbol:   "EnsureCollection",
			Content:  "func EnsureCollection() error {\n\treturn nil\n}",
		},
	}

	wantUser := "Question: how does indexing work?\n" +
		"\n" +
		"Context:\n" +
		"[1] internal/ingest/pipeline.go:42-78, function: Pipeline.Run\n" +
		"func (p *Pipeline) Run() {\n" +
		"\treturn nil\n" +
		"}\n" +
		"\n" +
		"[2] internal/qdrant/client.go:120-145, function: EnsureCollection\n" +
		"func EnsureCollection() error {\n" +
		"\treturn nil\n" +
		"}\n" +
		"\n" +
		"Answer the question based on the chunks above.\n"

	_, gotUser := BuildPrompt("how does indexing work?", chunks)
	if gotUser != wantUser {
		t.Errorf("user message mismatch\n--- got ---\n%s\n--- want ---\n%s", gotUser, wantUser)
	}
}

func TestBuildPrompt_ChunkWithoutSymbol(t *testing.T) {
	t.Parallel()

	chunks := []ChunkContext{
		{
			Index:    1,
			FilePath: "internal/config/config.go",
			Lines:    "1-20",
			// Symbol intentionally empty
			Content: "package config\n",
		},
	}

	_, gotUser := BuildPrompt("what is config?", chunks)

	headerLine := "[1] internal/config/config.go:1-20\n"
	if !strings.Contains(gotUser, headerLine) {
		t.Errorf("header without symbol should be %q\ngot:\n%s", strings.TrimSuffix(headerLine, "\n"), gotUser)
	}
	if strings.Contains(gotUser, "function:") {
		t.Errorf("header should omit 'function:' segment when Symbol is empty\ngot:\n%s", gotUser)
	}
}

func TestBuildPrompt_EmptyChunks(t *testing.T) {
	t.Parallel()

	wantUser := "Question: anything\n" +
		"\n" +
		"Context:\n" +
		"(no chunks retrieved)\n" +
		"\n" +
		"Answer the question based on the chunks above.\n"

	_, gotUser := BuildPrompt("anything", nil)
	if gotUser != wantUser {
		t.Errorf("empty-chunks user message mismatch\n--- got ---\n%s\n--- want ---\n%s", gotUser, wantUser)
	}
}

func TestBuildPrompt_ContentAlreadyHasTrailingNewline(t *testing.T) {
	t.Parallel()

	chunks := []ChunkContext{
		{Index: 1, FilePath: "a.go", Lines: "1-3", Symbol: "Foo", Content: "code line\n"},
	}

	_, gotUser := BuildPrompt("q", chunks)

	// Should NOT contain a double trailing newline before the blank separator line.
	if strings.Contains(gotUser, "code line\n\n\n") {
		t.Errorf("trailing newline should be idempotent (no triple \\n)\ngot:\n%s", gotUser)
	}
	// And should still have the single blank-line separator before the closing instruction.
	if !strings.HasSuffix(gotUser, "code line\n\nAnswer the question based on the chunks above.\n") {
		t.Errorf("expected single blank line between content and closing instruction\ngot:\n%s", gotUser)
	}
}

func TestBuildPrompt_PreservesChunkOrder(t *testing.T) {
	t.Parallel()

	chunks := []ChunkContext{
		{Index: 1, FilePath: "first.go", Lines: "1-1", Symbol: "First", Content: "A"},
		{Index: 2, FilePath: "second.go", Lines: "2-2", Symbol: "Second", Content: "B"},
		{Index: 3, FilePath: "third.go", Lines: "3-3", Symbol: "Third", Content: "C"},
	}

	_, gotUser := BuildPrompt("q", chunks)

	idxFirst := strings.Index(gotUser, "First")
	idxSecond := strings.Index(gotUser, "Second")
	idxThird := strings.Index(gotUser, "Third")
	if !(idxFirst < idxSecond && idxSecond < idxThird) {
		t.Errorf("chunks should appear in input order; got positions First=%d Second=%d Third=%d", idxFirst, idxSecond, idxThird)
	}
}

func TestBuildPrompt_QuestionEmbeddedVerbatim(t *testing.T) {
	t.Parallel()

	// Including characters that some renderers might mangle (Unicode dash,
	// brackets, quotes). v0 makes no escaping promises — just verifies the
	// question is passed through as-is.
	question := `what does "Pipeline.Run" do — and how is [error] propagated?`

	_, gotUser := BuildPrompt(question, nil)
	if !strings.Contains(gotUser, question) {
		t.Errorf("question should be embedded verbatim\ngot:\n%s", gotUser)
	}
}
