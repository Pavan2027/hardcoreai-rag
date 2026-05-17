package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// DefaultOllamaBaseURL is where Ollama listens by default.
	DefaultOllamaBaseURL = "http://localhost:11434"

	// DefaultModel is the embedding model to use.
	DefaultModel = "nomic-embed-text"

	// ExpectedDimensions is the vector size produced by nomic-embed-text.
	ExpectedDimensions = 768

	// maxRetries is the number of times to retry on transient failures.
	maxRetries = 3
)

// Client is an HTTP client for the Ollama embedding API.
type Client struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewClient creates a new Ollama embedding client.
// If baseURL is empty it defaults to http://localhost:11434.
// If model is empty it defaults to nomic-embed-text.
func NewClient(baseURL, model string) *Client {
	if baseURL == "" {
		baseURL = DefaultOllamaBaseURL
	}
	if model == "" {
		model = DefaultModel
	}
	return &Client{
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ollamaEmbedRequest is the JSON body sent to Ollama's /api/embed endpoint.
type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Input  string `json:"input"`
}

// ollamaEmbedResponse is the JSON body returned by Ollama's /api/embed endpoint.
type ollamaEmbedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}

// embed sends a single text string to Ollama and returns the raw float64 slice.
// It does NOT retry — retries are handled in EmbedQuery.
func (c *Client) embed(ctx context.Context, text string) ([]float64, error) {
	body, err := json.Marshal(ollamaEmbedRequest{
		Model: c.model,
		Input: text,
	})
	if err != nil {
		return nil, fmt.Errorf("embeddings.Client.embed: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embeddings.Client.embed: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embeddings.Client.embed: http error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embeddings.Client.embed: Ollama returned %d: %s", resp.StatusCode, raw)
	}

	var result ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("embeddings.Client.embed: decode response: %w", err)
	}

	if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
		return nil, fmt.Errorf("embeddings.Client.embed: empty embedding in response")
	}

	return result.Embeddings[0], nil
}
