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
	"code chunks to answer. If the chunks do not contain enough information,\n" +
	"say so explicitly and do not invent details. Cite the chunk numbers that\n" +
	"support your answer using brackets like [1]. Do not cite chunks that were\n" +
	"not provided."

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
// Header shape: "[N] {repo/}path:lines{, type: symbol}". The repo prefix is
// included only when set (disambiguates multi-repo collections). The symbol is
// labeled by ChunkType ("function", "block", "file", ...) so non-function chunks
// are not mislabeled; an empty ChunkType falls back to a neutral "symbol".
func renderChunk(c ChunkContext) string {
	location := c.FilePath
	if c.Repo != "" {
		location = c.Repo + "/" + c.FilePath
	}
	header := fmt.Sprintf("[%d] %s:%s", c.Index, location, c.Lines)
	if c.Symbol != "" {
		label := c.ChunkType
		if label == "" {
			label = "symbol"
		}
		header += fmt.Sprintf(", %s: %s", label, c.Symbol)
	}
	content := c.Content
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return header + "\n" + content
}
