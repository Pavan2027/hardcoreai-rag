// Package embeddings_test contains black-box tests for the embeddings package.
// Most tests require Ollama to be running locally (ollama serve).
package embeddings_test

import (
	"context"
	"testing"

	"github.com/Pavan2027/mcu-rag/embeddings"
)

// TestEmbedQuery_EmptyInput verifies that an empty query returns an error
// WITHOUT needing Ollama running.
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

// TestEmbedQuery_Dimensions verifies that the embedding returned by Ollama
// has exactly 768 dimensions (matching nomic-embed-text + our DB schema).
//
// HOW TO RUN (requires Ollama running):
//
//	go test ./embeddings/ -run TestEmbedQuery_Dimensions -v
//
// PREREQUISITE: ollama serve  (runs as background service after install)
func TestEmbedQuery_Dimensions(t *testing.T) {
	client := embeddings.NewClient("", "")
	vec, err := client.EmbedQuery(context.Background(), "USART baud rate configuration")
	if err != nil {
		t.Fatalf("EmbedQuery failed: %v\n\nIs Ollama running? Try: ollama serve", err)
	}
	if len(vec) != embeddings.ExpectedDimensions {
		t.Errorf("expected %d dimensions, got %d", embeddings.ExpectedDimensions, len(vec))
	} else {
		t.Logf("✓ Embedding dimensions: %d", len(vec))
	}
}

// TestEmbedQuery_Deterministic verifies the same input always produces the same vector.
//
// HOW TO RUN (requires Ollama running):
//
//	go test ./embeddings/ -run TestEmbedQuery_Deterministic -v
func TestEmbedQuery_Deterministic(t *testing.T) {
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
			t.Errorf("dimension %d differs: %f vs %f — model is non-deterministic", i, vec1[i], vec2[i])
			return
		}
	}
	t.Logf("✓ Embeddings are deterministic across two calls")
}
