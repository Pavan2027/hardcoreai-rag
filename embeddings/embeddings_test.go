// Package embeddings_test — black-box tests for the embeddings package.
// Live tests require GEMINI_API_KEY to be set in the environment.
package embeddings_test

import (
	"context"
	"os"
	"testing"

	"hardcoreai-rag/embeddings"
)

// TestEmbedQuery_EmptyInput verifies that an empty query returns an error
// WITHOUT needing GEMINI_API_KEY set.
//
// HOW TO RUN:
//
//	go test ./embeddings/ -run TestEmbedQuery_EmptyInput -v
func TestEmbedQuery_EmptyInput(t *testing.T) {
	client := embeddings.NewClient("", "")
	_, err := client.EmbedQuery(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty query, got nil")
	} else {
		t.Logf("✓ Correctly rejected empty query: %v", err)
	}
}

// TestEmbedQuery_NoAPIKey verifies that a missing API key returns a clear error.
//
// HOW TO RUN:
//
//	go test ./embeddings/ -run TestEmbedQuery_NoAPIKey -v
func TestEmbedQuery_NoAPIKey(t *testing.T) {
	// Temporarily unset the key if it happens to be present.
	orig := os.Getenv("GEMINI_API_KEY")
	os.Unsetenv("GEMINI_API_KEY")
	defer os.Setenv("GEMINI_API_KEY", orig)

	client := embeddings.NewClient("", "")
	_, err := client.EmbedQuery(context.Background(), "USART baud rate")
	if err == nil {
		t.Error("expected error when GEMINI_API_KEY is unset, got nil")
	} else {
		t.Logf("✓ Correctly rejected missing API key: %v", err)
	}
}

// TestEmbedQuery_Dimensions verifies that the embedding returned by Gemini
// has exactly 768 dimensions.
//
// HOW TO RUN (requires GEMINI_API_KEY):
//
//	$env:GEMINI_API_KEY="your-key"; go test ./embeddings/ -run TestEmbedQuery_Dimensions -v
func TestEmbedQuery_Dimensions(t *testing.T) {
	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("GEMINI_API_KEY not set — skipping live API test")
	}

	client := embeddings.NewClient("", "")
	vec, err := client.EmbedQuery(context.Background(), "USART baud rate configuration")
	if err != nil {
		t.Fatalf("EmbedQuery failed: %v", err)
	}
	if len(vec) != embeddings.ExpectedDimensions {
		t.Errorf("expected %d dimensions, got %d", embeddings.ExpectedDimensions, len(vec))
	} else {
		t.Logf("✓ Embedding dimensions: %d", len(vec))
	}
}

// TestEmbedQuery_Deterministic verifies the same input always produces the
// same vector (Gemini embeddings are deterministic).
//
// HOW TO RUN (requires GEMINI_API_KEY):
//
//	$env:GEMINI_API_KEY="your-key"; go test ./embeddings/ -run TestEmbedQuery_Deterministic -v
func TestEmbedQuery_Deterministic(t *testing.T) {
	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("GEMINI_API_KEY not set — skipping live API test")
	}

	client := embeddings.NewClient("", "")
	ctx := context.Background()
	query := "DMA stream configuration"

	vec1, err := client.EmbedQuery(ctx, query)
	if err != nil {
		t.Fatalf("first EmbedQuery: %v", err)
	}
	vec2, err := client.EmbedQuery(ctx, query)
	if err != nil {
		t.Fatalf("second EmbedQuery: %v", err)
	}

	for i := range vec1 {
		if vec1[i] != vec2[i] {
			t.Errorf("dimension %d differs: %f vs %f", i, vec1[i], vec2[i])
			return
		}
	}
	t.Logf("✓ Embeddings are deterministic across two calls")
}