package answer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// capturedChatRequest mirrors the /api/chat request body for assertions.
type capturedChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}

func TestOllamaGenerator_RequestShape(t *testing.T) {
	t.Parallel()

	var got capturedChatRequest
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		_ = json.NewEncoder(w).Encode(ollamaChatResponse{
			Message: ollamaChatMessage{Role: "assistant", Content: "the answer [1]"},
		})
	}))
	defer srv.Close()

	gen := NewOllamaGenerator(srv.URL, "qwen2.5-coder:7b")
	prompt := Prompt{
		Question: "how does indexing work?",
		Chunks: []ChunkContext{
			{Index: 1, FilePath: "pipeline.go", Lines: "1-10", Symbol: "Run", Content: "code"},
		},
	}

	answerText, err := gen.Generate(context.Background(), prompt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/api/chat" {
		t.Errorf("path = %q, want /api/chat", gotPath)
	}
	if got.Model != "qwen2.5-coder:7b" {
		t.Errorf("model = %q, want qwen2.5-coder:7b", got.Model)
	}
	if got.Stream {
		t.Error("stream should be false for v0")
	}
	if len(got.Messages) != 2 {
		t.Fatalf("messages length = %d, want 2 (system, user)", len(got.Messages))
	}
	if got.Messages[0].Role != "system" || got.Messages[0].Content != SystemPrompt {
		t.Errorf("first message should be the frozen system prompt; got role=%q", got.Messages[0].Role)
	}
	if got.Messages[1].Role != "user" {
		t.Errorf("second message role = %q, want user", got.Messages[1].Role)
	}
	if !strings.Contains(got.Messages[1].Content, "how does indexing work?") {
		t.Errorf("user message should embed the question; got:\n%s", got.Messages[1].Content)
	}
	if answerText != "the answer [1]" {
		t.Errorf("answer = %q, want %q", answerText, "the answer [1]")
	}
}

func TestOllamaGenerator_ModelNotFoundHint(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"model 'qwen2.5-coder:7b' not found"}`)
	}))
	defer srv.Close()

	gen := NewOllamaGenerator(srv.URL, "qwen2.5-coder:7b")
	_, err := gen.Generate(context.Background(), Prompt{Question: "q"})
	if err == nil {
		t.Fatal("expected an error for 404")
	}
	if !strings.Contains(err.Error(), "ollama pull qwen2.5-coder:7b") {
		t.Errorf("error should hint at `ollama pull`; got: %v", err)
	}
}

func TestOllamaGenerator_DefaultModelFallback(t *testing.T) {
	t.Parallel()

	gen := NewOllamaGenerator("http://localhost:11434", "")
	if gen.model != DefaultGenerativeModel {
		t.Errorf("empty model should fall back to %q, got %q", DefaultGenerativeModel, gen.model)
	}
}

func TestOllamaGenerator_ConnectionRefusedHint(t *testing.T) {
	t.Parallel()

	// Port 1 is reserved and refuses connections — exercises the conn-refused hint.
	gen := NewOllamaGenerator("http://localhost:1", "qwen2.5-coder:7b")
	_, err := gen.Generate(context.Background(), Prompt{Question: "q"})
	if err == nil {
		t.Fatal("expected a connection error")
	}
	if !strings.Contains(err.Error(), "ollama serve") {
		t.Errorf("error should hint at `ollama serve`; got: %v", err)
	}
}

func TestOllamaGenerator_Warmup(t *testing.T) {
	t.Parallel()

	var got capturedChatRequest
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		// Ollama responds to an empty-messages load request with an empty message.
		_ = json.NewEncoder(w).Encode(ollamaChatResponse{
			Message: ollamaChatMessage{Role: "assistant", Content: ""},
		})
	}))
	defer srv.Close()

	gen := NewOllamaGenerator(srv.URL, "qwen2.5-coder:7b")
	if err := gen.Warmup(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/api/chat" {
		t.Errorf("path = %q, want /api/chat", gotPath)
	}
	if got.Model != "qwen2.5-coder:7b" {
		t.Errorf("model = %q, want qwen2.5-coder:7b", got.Model)
	}
	if len(got.Messages) != 0 {
		t.Errorf("warmup should send empty messages (load request), got %d", len(got.Messages))
	}
}

func TestOllamaGenerator_WarmupModelNotFoundHint(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"model 'qwen2.5-coder:7b' not found"}`)
	}))
	defer srv.Close()

	gen := NewOllamaGenerator(srv.URL, "qwen2.5-coder:7b")
	err := gen.Warmup(context.Background())
	if err == nil {
		t.Fatal("expected an error for 404")
	}
	if !strings.Contains(err.Error(), "ollama pull qwen2.5-coder:7b") {
		t.Errorf("warmup error should hint at `ollama pull`; got: %v", err)
	}
}

func TestOllamaGenerator_ImplementsInterface(t *testing.T) {
	t.Parallel()
	// Compile-time checks are in ollama.go:
	//   var _ Generator = (*OllamaGenerator)(nil)
	//   var _ Warmer    = (*OllamaGenerator)(nil)
}
