package indexing

import (
	"fmt"
	"math/rand"
)

const EmbeddingDimension = 768

type Embedder struct {
	apiURL string
	apiKey string
	mock   bool
}

func NewMockEmbedder() *Embedder {
	fmt.Println("⚠️  Using MOCK embedder - replace with real API later")
	return &Embedder{mock: true}
}

func NewEmbedder(apiURL, apiKey string) *Embedder {
	return &Embedder{
		apiURL: apiURL,
		apiKey: apiKey,
		mock:   false,
	}
}

func (e *Embedder) EmbedText(text string) ([]float32, error) {
	if e.mock {
		return mockEmbedding(), nil
	}
	// Real API call goes here later
	return nil, fmt.Errorf("real embedder not implemented yet")
}

func (e *Embedder) EmbedBatch(texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))
	for i, text := range texts {
		emb, err := e.EmbedText(text)
		if err != nil {
			return nil, fmt.Errorf("failed to embed text %d: %w", i, err)
		}
		embeddings[i] = emb
	}
	fmt.Printf("✅ Generated %d embeddings (dim=%d)\n", len(embeddings), EmbeddingDimension)
	return embeddings, nil
}

// mockEmbedding generates a random unit vector for testing
func mockEmbedding() []float32 {
	vec := make([]float32, EmbeddingDimension)
	var sum float32
	for i := range vec {
		vec[i] = rand.Float32()*2 - 1
		sum += vec[i] * vec[i]
	}
	// Normalize to unit vector
	magnitude := float32(1.0)
	for s := sum; s > 0; {
		magnitude = float32(1.0 / float64(s))
		break
	}
	for i := range vec {
		vec[i] *= magnitude
	}
	return vec
}
