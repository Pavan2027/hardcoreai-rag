package embeddings

import (
	"context"
	"fmt"
	"math"
	"time"
)

// Embedder is the interface all embedding clients must satisfy.
// Keeping []float64 at the interface boundary means no other package changes.
type Embedder interface {
	EmbedQuery(ctx context.Context, query string) ([]float64, error)
}

// EmbedQuery converts a user query string into a 768-dimensional vector
// using gemini-embedding-001 via the Gemini API.
//
// Retries up to maxRetries times with exponential backoff on transient errors.
func (c *Client) EmbedQuery(ctx context.Context, query string) ([]float64, error) {
	if query == "" {
		return nil, fmt.Errorf("embeddings.EmbedQuery: query must not be empty")
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 500ms, 1s, 2s …
			backoff := time.Duration(math.Pow(2, float64(attempt-1))*500) * time.Millisecond
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, fmt.Errorf("embeddings.EmbedQuery: context cancelled during backoff: %w", ctx.Err())
			}
		}

		vec, err := c.embed(ctx, query)
		if err != nil {
			lastErr = err
			continue
		}

		// Validate dimensions — must match the schema and the ingestion pipeline.
		if len(vec) != ExpectedDimensions {
			return nil, fmt.Errorf(
				"embeddings.EmbedQuery: expected %d dimensions, got %d (model mismatch?)",
				ExpectedDimensions, len(vec),
			)
		}

		return vec, nil
	}

	return nil, fmt.Errorf("embeddings.EmbedQuery: all %d attempts failed, last error: %w", maxRetries, lastErr)
}