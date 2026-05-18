package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"time"
)

const (
	// DefaultGeminiBaseURL is the Gemini API base URL.
	DefaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"

	// DefaultModel is the embedding model used by the ingestion pipeline.
	DefaultModel = "gemini-embedding-001"

	// ExpectedDimensions is the vector size produced by gemini-embedding-001.
	ExpectedDimensions = 768

	// maxRetries is the number of times to retry on transient failures.
	maxRetries = 3
)

// Client is an HTTP client for the Gemini embedding API.
type Client struct {
	baseURL    string
	model      string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new Gemini embedding client.
// API key is read from the GEMINI_API_KEY environment variable.
// If baseURL or model are empty they default to the Gemini production endpoint
// and gemini-embedding-001 respectively.
func NewClient(baseURL, model string) *Client {
	if baseURL == "" {
		baseURL = DefaultGeminiBaseURL
	}
	if model == "" {
		model = DefaultModel
	}
	return &Client{
		baseURL: baseURL,
		model:   model,
		apiKey:  os.Getenv("GEMINI_API_KEY"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// geminiEmbedRequest is the JSON body sent to Gemini's embedContent endpoint.
type geminiEmbedRequest struct {
	Model                string        `json:"model"`
	Content              geminiContent `json:"content"`
	OutputDimensionality int           `json:"outputDimensionality,omitempty"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

// geminiEmbedResponse is the JSON body returned by Gemini's embedContent endpoint.
type geminiEmbedResponse struct {
	Embedding struct {
		Values []float64 `json:"values"`
	} `json:"embedding"`
}

// embed sends a single text string to Gemini and returns the raw float64 slice.
// It does NOT retry — retries are handled in EmbedQuery.
func (c *Client) embed(ctx context.Context, text string) ([]float64, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("embeddings.Client.embed: GEMINI_API_KEY environment variable is not set")
	}

	reqBody := geminiEmbedRequest{
		Model: "models/" + c.model,
		Content: geminiContent{
			Parts: []geminiPart{{Text: text}},
		},
		OutputDimensionality: ExpectedDimensions,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("embeddings.Client.embed: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:embedContent?key=%s", c.baseURL, c.model, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
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
		return nil, fmt.Errorf("embeddings.Client.embed: Gemini returned %d: %s", resp.StatusCode, raw)
	}

	var result geminiEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("embeddings.Client.embed: decode response: %w", err)
	}

	if len(result.Embedding.Values) == 0 {
		return nil, fmt.Errorf("embeddings.Client.embed: empty embedding in response")
	}

	return result.Embedding.Values, nil
}

// geminiBatchRequest is the request body for batchEmbedContents.
type geminiBatchRequest struct {
	Requests []geminiEmbedRequest `json:"requests"`
}

// geminiBatchResponse is the response body for batchEmbedContents.
type geminiBatchResponse struct {
	Embeddings []struct {
		Values []float64 `json:"values"`
	} `json:"embeddings"`
}

// EmbedBatch embeds a slice of text strings in parallel batches (up to 100 per batch)
// using the Gemini batchEmbedContents endpoint, with exponential backoff retries.
func (c *Client) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	results := make([][]float64, len(texts))
	const batchSize = 100

	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		chunk := texts[i:end]

		// Construct requests for this batch
		reqs := make([]geminiEmbedRequest, len(chunk))
		for j, text := range chunk {
			reqs[j] = geminiEmbedRequest{
				Model:                "models/" + c.model,
				Content:              geminiContent{Parts: []geminiPart{{Text: text}}},
				OutputDimensionality: ExpectedDimensions,
			}
		}

		batchReq := geminiBatchRequest{Requests: reqs}
		body, err := json.Marshal(batchReq)
		if err != nil {
			return nil, fmt.Errorf("embeddings.Client.EmbedBatch: marshal request: %w", err)
		}

		var batchResult geminiBatchResponse
		var lastErr error

		// Run request with retries
		for attempt := 0; attempt < maxRetries; attempt++ {
			if attempt > 0 {
				backoff := time.Duration(math.Pow(2, float64(attempt-1))*500) * time.Millisecond
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return nil, fmt.Errorf("embeddings.Client.EmbedBatch: context cancelled: %w", ctx.Err())
				}
			}

			url := fmt.Sprintf("%s/models/%s:batchEmbedContents?key=%s", c.baseURL, c.model, c.apiKey)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
			if err != nil {
				lastErr = fmt.Errorf("create request: %w", err)
				continue
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := c.httpClient.Do(req)
			if err != nil {
				lastErr = fmt.Errorf("http error: %w", err)
				continue
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				raw, _ := io.ReadAll(resp.Body)
				lastErr = fmt.Errorf("Gemini returned %d: %s", resp.StatusCode, raw)
				continue
			}

			if err := json.NewDecoder(resp.Body).Decode(&batchResult); err != nil {
				lastErr = fmt.Errorf("decode response: %w", err)
				continue
			}

			lastErr = nil
			break
		}

		if lastErr != nil {
			return nil, fmt.Errorf("embeddings.Client.EmbedBatch: all %d attempts failed, last error: %w", maxRetries, lastErr)
		}

		if len(batchResult.Embeddings) != len(chunk) {
			return nil, fmt.Errorf("embeddings.Client.EmbedBatch: expected %d embeddings, got %d", len(chunk), len(batchResult.Embeddings))
		}

		// Copy embeddings to result slice
		for j, emb := range batchResult.Embeddings {
			if len(emb.Values) != ExpectedDimensions {
				return nil, fmt.Errorf("embeddings.Client.EmbedBatch: expected %d dimensions, got %d", ExpectedDimensions, len(emb.Values))
			}
			results[i+j] = emb.Values
		}
	}

	return results, nil
}