package answer

import (
	"context"
	"fmt"
	"strings"
)

// FakeGenerator returns a canned response for testing. It records the last
// prompt it received so tests can assert on the input without hitting Ollama.
type FakeGenerator struct {
	// Response is the fixed answer text returned by Generate.
	Response string

	// Err, if non-nil, is returned as the error from Generate.
	Err error

	// LastPrompt stores the most recent prompt passed to Generate.
	LastPrompt Prompt

	// CallCount tracks how many times Generate has been called.
	CallCount int
}

// NewFakeGenerator creates a FakeGenerator that returns the given response.
func NewFakeGenerator(response string) *FakeGenerator {
	return &FakeGenerator{Response: response}
}

// Generate records the prompt and returns the canned response (or error).
func (f *FakeGenerator) Generate(_ context.Context, prompt Prompt) (string, error) {
	f.LastPrompt = prompt
	f.CallCount++
	if f.Err != nil {
		return "", f.Err
	}
	if f.Response != "" {
		return f.Response, nil
	}
	// Default: echo back a summary so tests can verify the prompt was received.
	return fmt.Sprintf(
		"Fake answer for %q using %d chunk(s): %s",
		prompt.Question,
		len(prompt.Chunks),
		chunkSummary(prompt.Chunks),
	), nil
}

// chunkSummary returns a short description of the chunks for the default response.
func chunkSummary(chunks []ChunkContext) string {
	names := make([]string, len(chunks))
	for i, c := range chunks {
		if c.Symbol != "" {
			names[i] = c.Symbol
		} else {
			names[i] = c.FilePath
		}
	}
	return strings.Join(names, ", ")
}

// Compile-time check: FakeGenerator implements Generator.
var _ Generator = (*FakeGenerator)(nil)
