package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// OllamaEmbedder generates real semantic embeddings via the Ollama /api/embed endpoint.
// The vector dimension is auto-detected from the first successful embedding response
// and validated against all subsequent responses.
type OllamaEmbedder struct {
	baseURL string
	model   string
	client  *http.Client

	mu  sync.Mutex
	dim int // 0 until first successful embedding
}

// NewOllamaEmbedder creates an embedder that calls the local Ollama server.
// model should be an embedding model like "nomic-embed-text".
// The vector dimension is inferred from the first embedding response.
func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	return &OllamaEmbedder{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{},
	}
}

type ollamaEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type ollamaEmbedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}

// Embed converts a batch of texts into vector embeddings via Ollama.
// On the first call, the vector dimension is detected and cached.
// Subsequent calls validate that the dimension remains consistent.
func (o *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := ollamaEmbedRequest{
		Model: o.model,
		Input: texts,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/embed", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling ollama embed API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama embed API returned %d: %s", resp.StatusCode, string(body))
	}

	var embedResp ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, fmt.Errorf("decoding embed response: %w", err)
	}

	if len(embedResp.Embeddings) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(embedResp.Embeddings))
	}

	// Convert float64 (JSON default) to float32 (Qdrant format).
	vectors := make([][]float32, len(embedResp.Embeddings))
	for i, emb := range embedResp.Embeddings {
		vec := make([]float32, len(emb))
		for j, v := range emb {
			vec[j] = float32(v)
		}
		vectors[i] = vec
	}

	// Validate and cache dimension.
	detectedDim, err := ValidateVectorBatch(vectors, o.Dimension())
	if err != nil {
		return nil, fmt.Errorf("validating embedding response: %w", err)
	}
	o.setDim(detectedDim)

	return vectors, nil
}

// Dimension returns the detected vector dimensionality.
// Returns 0 before the first successful embedding call.
func (o *OllamaEmbedder) Dimension() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.dim
}

func (o *OllamaEmbedder) setDim(dim int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.dim = dim
}

// Compile-time check: OllamaEmbedder implements Embedder.
var _ Embedder = (*OllamaEmbedder)(nil)
