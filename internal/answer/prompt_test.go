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
			Index:     1,
			Repo:      "ragcodepilot",
			FilePath:  "internal/ingest/pipeline.go",
			Language:  "go",
			ChunkType: "function",
			Lines:     "42-78",
			Symbol:    "Pipeline.Run",
			Content:   "func (p *Pipeline) Run() {\n\treturn nil\n}",
		},
		{
			Index:     2,
			Repo:      "ragcodepilot",
			FilePath:  "internal/qdrant/client.go",
			Language:  "go",
			ChunkType: "function",
			Lines:     "120-145",
			Symbol:    "EnsureCollection",
			Content:   "func EnsureCollection() error {\n\treturn nil\n}",
		},
	}

	wantUser := "Question: how does indexing work?\n" +
		"\n" +
		"Context:\n" +
		"[1] ragcodepilot/internal/ingest/pipeline.go:42-78, function: Pipeline.Run\n" +
		"func (p *Pipeline) Run() {\n" +
		"\treturn nil\n" +
		"}\n" +
		"\n" +
		"[2] ragcodepilot/internal/qdrant/client.go:120-145, function: EnsureCollection\n" +
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

func TestBuildPrompt_ChunkTypeLabelsNonFunction(t *testing.T) {
	t.Parallel()

	// Each case asserts the label produced by ChunkType, not a hardcoded
	// "function:". Regression test for the prior bug where a struct chunk
	// would have been mislabeled "function: CodeChunk".
	cases := []struct {
		name       string
		chunk      ChunkContext
		wantHeader string
	}{
		{
			name: "struct uses chunk_type label",
			chunk: ChunkContext{
				Index: 1, Repo: "ragcodepilot",
				FilePath: "internal/model/chunk.go", ChunkType: "struct",
				Lines: "5-44", Symbol: "CodeChunk", Content: "type CodeChunk struct{}",
			},
			wantHeader: "[1] ragcodepilot/internal/model/chunk.go:5-44, struct: CodeChunk",
		},
		{
			name: "block chunk without symbol renders bare chunk_type",
			chunk: ChunkContext{
				Index: 1, Repo: "myrepo",
				FilePath: "scripts/setup.sh", ChunkType: "block",
				Lines: "1-40", Content: "echo hi",
			},
			wantHeader: "[1] myrepo/scripts/setup.sh:1-40, block",
		},
		{
			name: "file chunk without symbol renders bare chunk_type",
			chunk: ChunkContext{
				Index: 1, Repo: "myrepo",
				FilePath: "README.md", ChunkType: "file",
				Lines: "1-200", Content: "# Title",
			},
			wantHeader: "[1] myrepo/README.md:1-200, file",
		},
		{
			name: "no chunk_type and no symbol drops the segment entirely",
			chunk: ChunkContext{
				Index: 1, FilePath: "anonymous.txt", Lines: "1-1", Content: "x",
			},
			wantHeader: "[1] anonymous.txt:1-1",
		},
		{
			name: "no chunk_type but symbol present falls back to bare symbol",
			chunk: ChunkContext{
				Index: 1, FilePath: "legacy.go", Lines: "1-1", Symbol: "Foo", Content: "x",
			},
			wantHeader: "[1] legacy.go:1-1, Foo",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, gotUser := BuildPrompt("q", []ChunkContext{tc.chunk})
			if !strings.Contains(gotUser, tc.wantHeader+"\n") {
				t.Errorf("expected header %q in prompt; got:\n%s", tc.wantHeader, gotUser)
			}
			if strings.Contains(gotUser, "function:") && tc.chunk.ChunkType != "function" {
				t.Errorf("header should not say 'function:' when ChunkType=%q\ngot:\n%s", tc.chunk.ChunkType, gotUser)
			}
		})
	}
}

func TestBuildPrompt_RepoPrefixesPath(t *testing.T) {
	t.Parallel()

	// Two chunks with identical file paths from different repos must produce
	// distinct citations. Regression test for the multi-repo ambiguity bug.
	chunks := []ChunkContext{
		{Index: 1, Repo: "repo-a", FilePath: "internal/foo.go", ChunkType: "function", Lines: "10-20", Symbol: "Foo", Content: "a"},
		{Index: 2, Repo: "repo-b", FilePath: "internal/foo.go", ChunkType: "function", Lines: "10-20", Symbol: "Foo", Content: "b"},
	}

	_, gotUser := BuildPrompt("which one?", chunks)

	if !strings.Contains(gotUser, "[1] repo-a/internal/foo.go:10-20") {
		t.Errorf("expected repo-a prefix on chunk 1; got:\n%s", gotUser)
	}
	if !strings.Contains(gotUser, "[2] repo-b/internal/foo.go:10-20") {
		t.Errorf("expected repo-b prefix on chunk 2; got:\n%s", gotUser)
	}
}

func TestBuildPrompt_NoRepoOmitsPrefix(t *testing.T) {
	t.Parallel()

	chunks := []ChunkContext{
		{Index: 1, FilePath: "internal/config/config.go", ChunkType: "file", Lines: "1-20", Content: "package config\n"},
	}

	_, gotUser := BuildPrompt("what is config?", chunks)

	headerLine := "[1] internal/config/config.go:1-20, file\n"
	if !strings.Contains(gotUser, headerLine) {
		t.Errorf("header without Repo should not include a slash prefix; want %q\ngot:\n%s", strings.TrimSuffix(headerLine, "\n"), gotUser)
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
	if idxFirst >= idxSecond || idxSecond >= idxThird {
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
