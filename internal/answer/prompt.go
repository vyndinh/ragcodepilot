package answer

import (
	"fmt"
	"strings"
)

// SystemPrompt is the v0 frozen system message sent to the generator. It tells
// the model to ground its answer in the retrieved chunks and cite them by
// bracket number. v1 may make this configurable; v0 hardcodes it so golden
// tests can lock the contract.
const SystemPrompt = "You are answering questions about a code repository. Use only the provided\n" +
	"code chunks to answer. If the chunks don't contain enough information,\n" +
	"say so explicitly — do not invent details. Reference specific chunks by\n" +
	"their number in brackets like [1] when citing code."

// BuildPrompt renders the system and user messages for the v0 answer prompt.
// The system message is the frozen SystemPrompt constant. The user message
// embeds the question and the retrieved chunks in the order given.
//
// Provider-agnostic: returns plain strings so the Ollama generator (Step 3)
// can wrap them into the /api/chat message array, and future providers can
// adapt without touching this builder.
func BuildPrompt(question string, chunks []ChunkContext) (system, user string) {
	var b strings.Builder
	b.WriteString("Question: ")
	b.WriteString(question)
	b.WriteString("\n\nContext:\n")
	if len(chunks) == 0 {
		b.WriteString("(no chunks retrieved)\n")
	} else {
		for i, c := range chunks {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(renderChunk(c))
		}
	}
	b.WriteString("\nAnswer the question based on the chunks above.\n")
	return SystemPrompt, b.String()
}

// renderChunk formats a single chunk as a header line plus its content. The
// content is rendered as-is (no extra indentation) so the model sees the
// source's original indentation. A trailing newline is added if missing so
// successive chunks separate cleanly.
//
// Header shape: "[N] <repo>/<filepath>:<lines>, <chunk_type>: <symbol>"
//   - <repo>/ is prepended only when Repo is set, so multi-repo collections
//     produce unambiguous citations.
//   - The trailing segment uses ChunkType as the label ("function:", "struct:",
//     "block", ...) so non-function chunks don't get mislabeled.
//   - When ChunkType is empty but Symbol is set, Symbol is appended without a
//     label as a graceful fallback.
//   - When both ChunkType and Symbol are empty, the segment is omitted.
func renderChunk(c ChunkContext) string {
	path := c.FilePath
	if c.Repo != "" {
		path = c.Repo + "/" + c.FilePath
	}
	header := fmt.Sprintf("[%d] %s:%s", c.Index, path, c.Lines)
	if seg := chunkLabel(c.ChunkType, c.Symbol); seg != "" {
		header += ", " + seg
	}
	content := c.Content
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return header + "\n" + content
}

// chunkLabel renders the trailing "<chunk_type>: <symbol>" segment of the
// header. Empty inputs produce an empty segment so the caller can decide
// whether to emit the separating comma.
func chunkLabel(chunkType, symbol string) string {
	switch {
	case chunkType != "" && symbol != "":
		return chunkType + ": " + symbol
	case chunkType != "":
		return chunkType
	case symbol != "":
		return symbol
	default:
		return ""
	}
}
