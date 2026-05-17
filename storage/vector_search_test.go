// Package storage_test — vector search tests (Phase 3).
// Run without Ollama. Uses in-memory SQLite and synthetic embeddings.
//
// HOW TO RUN:
//
//	go test ./storage/ -run TestVectorSearch -v
//	VEC0_PATH=../bin/vec0 go test ./storage/ -run TestVectorSearch -v
package storage_test

import (
	"context"
	"math"
	"testing"

	"github.com/Pavan2027/mcu-rag/storage"
)

// --- Seed helpers -----------------------------------------------------------

// unitVec returns a 768-dimensional zero vector with index i set to 1.0.
// Use different indices to create orthogonal test embeddings.
func unitVec(i int) []float64 {
	v := make([]float64, 768)
	v[i] = 1.0
	return v
}

// seedVectorDB sets up an in-memory DB with two documents and three chunks,
// each with a distinct unit-vector embedding in vec_chunks.
//
// Chunk layout:
//
//	chunkID 1 — USART_BRR / USART / STM32F4 / reference_manual — embedding: unitVec(0)
//	chunkID 2 — CFSR / FaultHandling / STM32F4 / reference_manual — embedding: unitVec(1)
//	chunkID 3 — DMA_SxCR / DMA / STM32H7 / app_note — embedding: unitVec(2)
func seedVectorDB(t *testing.T) *storage.DB {
	t.Helper()

	db, err := storage.Open(":memory:", vecPath())
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := db.ApplySchema(schemaSQL(t)); err != nil {
		t.Fatalf("ApplySchema: %v", err)
	}

	// Document 1 — STM32F4 Reference Manual
	res, err := db.Exec(`
		INSERT INTO documents (filename, doc_type, chip_family, chip_model)
		VALUES ('stm32f4_rm.pdf', 'reference_manual', 'STM32F4', 'STM32F429')
	`)
	if err != nil {
		t.Fatalf("insert doc1: %v", err)
	}
	doc1ID, _ := res.LastInsertId()

	// Document 2 — STM32H7 App Note
	res, err = db.Exec(`
		INSERT INTO documents (filename, doc_type, chip_family, chip_model)
		VALUES ('stm32h7_dma_an.pdf', 'app_note', 'STM32H7', 'STM32H743')
	`)
	if err != nil {
		t.Fatalf("insert doc2: %v", err)
	}
	doc2ID, _ := res.LastInsertId()

	// Chunks
	chunks := []struct {
		docID        int64
		text         string
		section      string
		peripheral   string
		registerName string
		page         int
		vecIndex     int // which unit-vector dimension to set to 1.0
	}{
		{doc1ID, "USART_BRR controls baud rate divider.", "USART Baud Rate", "USART", "USART_BRR", 842, 0},
		{doc1ID, "CFSR contains precise and imprecise fault status bits.", "Fault Handling", "CoreDebug", "CFSR", 210, 1},
		{doc2ID, "DMA_SxCR configures DMA stream direction and priority.", "DMA Stream Config", "DMA", "DMA_SxCR", 330, 2},
	}

	for _, c := range chunks {
		res, err = db.Exec(`
			INSERT INTO chunks (document_id, chunk_text, section_title, peripheral, register_name, page_number, chunk_index)
			VALUES (?, ?, ?, ?, ?, ?, 0)
		`, c.docID, c.text, c.section, c.peripheral, c.registerName, c.page)
		if err != nil {
			t.Fatalf("insert chunk: %v", err)
		}
		chunkID, _ := res.LastInsertId()

		blob := storage.SerializeEmbedding(unitVec(c.vecIndex))
		if _, err := db.Exec(`INSERT INTO vec_chunks (chunk_id, embedding) VALUES (?, ?)`, chunkID, blob); err != nil {
			t.Fatalf("insert vec_chunks chunkID=%d: %v", chunkID, err)
		}
	}

	return db
}

// --- Tests ------------------------------------------------------------------

// TestVectorSearch_TopResult verifies the nearest chunk is returned first
// when querying with a vector identical to chunk 1's embedding.
//
// HOW TO RUN:
//
//	go test ./storage/ -run TestVectorSearch_TopResult -v
func TestVectorSearch_TopResult(t *testing.T) {
	db := seedVectorDB(t)
	ctx := context.Background()

	// Query with unitVec(0) — should match chunk 1 (USART_BRR) at distance 0.
	results, err := db.VectorSearch(ctx, unitVec(0), storage.SearchOptions{K: 3})
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected results, got 0")
	}

	top := results[0]
	if top.RegisterName != "USART_BRR" {
		t.Errorf("expected top result USART_BRR, got %q", top.RegisterName)
	}
	t.Logf("✓ Top result: register=%q section=%q score=%.4f", top.RegisterName, top.SectionTitle, top.SemanticScore)
}

// TestVectorSearch_ScoreNormalised verifies SemanticScore is in [0,1].
//
// HOW TO RUN:
//
//	go test ./storage/ -run TestVectorSearch_ScoreNormalised -v
func TestVectorSearch_ScoreNormalised(t *testing.T) {
	db := seedVectorDB(t)
	ctx := context.Background()

	results, err := db.VectorSearch(ctx, unitVec(0), storage.SearchOptions{K: 3})
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}

	for _, r := range results {
		if r.SemanticScore < 0 || r.SemanticScore > 1 {
			t.Errorf("SemanticScore out of [0,1]: %f (chunkID=%d)", r.SemanticScore, r.ChunkID)
		}
	}
	t.Logf("✓ All %d scores in [0,1]", len(results))
}

// TestVectorSearch_ScoreOrdering verifies results are ordered best-first.
//
// HOW TO RUN:
//
//	go test ./storage/ -run TestVectorSearch_ScoreOrdering -v
func TestVectorSearch_ScoreOrdering(t *testing.T) {
	db := seedVectorDB(t)
	ctx := context.Background()

	results, err := db.VectorSearch(ctx, unitVec(0), storage.SearchOptions{K: 3})
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}

	for i := 1; i < len(results); i++ {
		if results[i].SemanticScore > results[i-1].SemanticScore {
			t.Errorf("results not ordered: index %d (%.4f) > index %d (%.4f)",
				i, results[i].SemanticScore, i-1, results[i-1].SemanticScore)
		}
	}
	t.Logf("✓ Scores are monotonically non-increasing across %d results", len(results))
}

// TestVectorSearch_PerfectMatchScore verifies a zero-distance result scores ~1.0.
//
// HOW TO RUN:
//
//	go test ./storage/ -run TestVectorSearch_PerfectMatchScore -v
func TestVectorSearch_PerfectMatchScore(t *testing.T) {
	db := seedVectorDB(t)
	ctx := context.Background()

	results, err := db.VectorSearch(ctx, unitVec(0), storage.SearchOptions{K: 1})
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	// distance=0 → score = 1/(1+0) = 1.0. Allow float32 rounding tolerance.
	got := results[0].SemanticScore
	if math.Abs(got-1.0) > 0.01 {
		t.Errorf("expected score ~1.0 for identical vector, got %.6f", got)
	}
	t.Logf("✓ Perfect-match SemanticScore = %.6f", got)
}

// TestVectorSearch_FilterByChipFamily verifies that ChipFamily filter excludes
// chunks from non-matching families.
//
// HOW TO RUN:
//
//	go test ./storage/ -run TestVectorSearch_FilterByChipFamily -v
func TestVectorSearch_FilterByChipFamily(t *testing.T) {
	db := seedVectorDB(t)
	ctx := context.Background()

	// unitVec(2) is the DMA chunk (STM32H7). Filtering for STM32F4 should exclude it.
	results, err := db.VectorSearch(ctx, unitVec(2), storage.SearchOptions{
		K:          3,
		ChipFamily: "STM32F4",
	})
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}

	for _, r := range results {
		if r.ChipFamily != "STM32F4" {
			t.Errorf("filter failed: got result with ChipFamily=%q", r.ChipFamily)
		}
	}
	t.Logf("✓ All %d results have ChipFamily=STM32F4", len(results))
}

// TestVectorSearch_EmptyEmbedding verifies an error is returned for empty input.
//
// HOW TO RUN:
//
//	go test ./storage/ -run TestVectorSearch_EmptyEmbedding -v
func TestVectorSearch_EmptyEmbedding(t *testing.T) {
	db := seedVectorDB(t)
	_, err := db.VectorSearch(context.Background(), []float64{}, storage.SearchOptions{K: 5})
	if err == nil {
		t.Error("expected error for empty embedding, got nil")
	} else {
		t.Logf("✓ Correctly rejected empty embedding: %v", err)
	}
}