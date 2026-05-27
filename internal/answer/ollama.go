package answer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultGenerativeModel is the v0 default model for --answer. Code-tuned and
// small enough to run on a developer laptop. Configurable via the CLI.
const DefaultGenerativeModel = "qwen2.5-coder:7b"

// defaultGenerateTimeout bounds a single generation call. Generation is slower
// than embedding, and cold-start (first model load) can take 30-120 seconds
// depending on hardware. 180s gives enough headroom for first load.
const defaultGenerateTimeout = 180 * time.Second

// OllamaGenerator produces answers via the Ollama /api/chat endpoint. It mirrors
// the OllamaEmbedder pattern: a small HTTP client over the same local Ollama
// server already used for embeddings. Synchronous (non-streaming) for v0.
type OllamaGenerator struct {
	baseURL string
	model   string
	client  *http.Client
}

// NewOllamaGenerator creates a generator that calls the local Ollama server.
// model should be a generative model like "qwen2.5-coder:7b". A zero or empty
// model falls back to DefaultGenerativeModel.
func NewOllamaGenerator(baseURL, model string) *OllamaGenerator {
	if model == "" {
		model = DefaultGenerativeModel
	}
	return &OllamaGenerator{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: defaultGenerateTimeout},
	}
}

// ollamaChatMessage is a single message in the /api/chat request/response.
type ollamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}

type ollamaChatResponse struct {
	Message ollamaChatMessage `json:"message"`
}

// Generate builds the system+user prompt from the retrieved chunks and sends it
// to Ollama's /api/chat endpoint, returning the assistant's answer text.
func (o *OllamaGenerator) Generate(ctx context.Context, prompt Prompt) (string, error) {
	system, user := BuildPrompt(prompt.Question, prompt.Chunks)

	reqBody := ollamaChatRequest{
		Model: o.model,
		Messages: []ollamaChatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Stream: false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("creating chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return "", o.connectionError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", o.statusError(resp.StatusCode, body)
	}

	var chatResp ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decoding chat response: %w", err)
	}

	return chatResp.Message.Content, nil
}

// Warmup loads the model into memory without generating an answer. Ollama treats
// an empty messages array as a load request and responds as soon as the model is
// resident, so this pulls the one-time cold-start cost out of the first Generate
// call. Safe to call repeatedly: if the model is already loaded, Ollama returns
// immediately.
func (o *OllamaGenerator) Warmup(ctx context.Context) error {
	reqBody := ollamaChatRequest{
		Model:    o.model,
		Messages: []ollamaChatMessage{},
		Stream:   false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshaling warmup request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("creating warmup request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return o.connectionError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return o.statusError(resp.StatusCode, body)
	}

	// Drain the (empty) body so the connection can be reused.
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// connectionError adds an actionable hint when Ollama is unreachable or times out.
func (o *OllamaGenerator) connectionError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("ollama chat timed out after %s: %w", defaultGenerateTimeout, err)
	}
	// net/http surfaces "connection refused" inside the wrapped error string.
	if strings.Contains(err.Error(), "connection refused") {
		return fmt.Errorf("calling ollama chat API: %w (is Ollama running? try `ollama serve`)", err)
	}
	return fmt.Errorf("calling ollama chat API: %w", err)
}

// statusError adds an actionable hint when the model is missing.
func (o *OllamaGenerator) statusError(code int, body []byte) error {
	if code == http.StatusNotFound || strings.Contains(strings.ToLower(string(body)), "not found") {
		return fmt.Errorf("ollama chat API returned %d: %s (run `ollama pull %s`)", code, string(body), o.model)
	}
	return fmt.Errorf("ollama chat API returned %d: %s", code, string(body))
}

// Compile-time checks: OllamaGenerator implements Generator and Warmer.
var (
	_ Generator = (*OllamaGenerator)(nil)
	_ Warmer    = (*OllamaGenerator)(nil)
)
