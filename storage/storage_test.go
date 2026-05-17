// Package storage_test contains black-box tests for the storage package.
// Tests run against an in-memory SQLite DB so no files are created on disk.
package storage_test

import (
	"os"
	"testing"

	"github.com/Pavan2027/mcu-rag/storage"
)

// schemaSQL reads the schema file from disk.
func schemaSQL(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatalf("could not read schema.sql: %v", err)
	}
	return string(data)
}

// vecPath returns the vec0 extension path from env or the default bin location.
func vecPath() string {
	if p := os.Getenv("VEC0_PATH"); p != "" {
		return p
	}
	return "../bin/vec0"
}

// TestOpen_InMemory verifies that the DB opens with the vec0 extension loaded
// and that the schema can be applied cleanly.
//
// HOW TO RUN:
//
//	go test ./storage/ -run TestOpen_InMemory -v
func TestOpen_InMemory(t *testing.T) {
	db, err := storage.Open(":memory:", vecPath())
	if err != nil {
		t.Fatalf("storage.Open failed: %v\n\nMake sure vec0.dll is in bin/ or set VEC0_PATH.", err)
	}
	defer db.Close()

	if err := db.ApplySchema(schemaSQL(t)); err != nil {
		t.Fatalf("ApplySchema failed: %v", err)
	}

	// Verify all four tables were created.
	for _, tbl := range []string{"documents", "chunks", "vec_chunks", "chunk_fts"} {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE name = ?", tbl,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found after schema apply: %v", tbl, err)
		} else {
			t.Logf("✓ Table exists: %s", name)
		}
	}
}

// TestBasicInsertAndQuery verifies we can insert and retrieve a document + chunk.
//
// HOW TO RUN:
//
//	go test ./storage/ -run TestBasicInsertAndQuery -v
func TestBasicInsertAndQuery(t *testing.T) {
	db, err := storage.Open(":memory:", vecPath())
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	defer db.Close()

	if err := db.ApplySchema(schemaSQL(t)); err != nil {
		t.Fatalf("ApplySchema: %v", err)
	}

	// Insert a synthetic document.
	res, err := db.Exec(`
		INSERT INTO documents (filename, doc_type, chip_family, chip_model)
		VALUES ('stm32f4_rm.pdf', 'reference_manual', 'STM32F4', 'STM32F429')
	`)
	if err != nil {
		t.Fatalf("insert document: %v", err)
	}
	docID, _ := res.LastInsertId()

	// Insert a synthetic chunk.
	_, err = db.Exec(`
		INSERT INTO chunks (document_id, chunk_text, section_title, peripheral, register_name, page_number, chunk_index)
		VALUES (?, 'The USART_BRR register controls baud rate.', 'USART Baud Rate', 'USART', 'USART_BRR', 842, 0)
	`, docID)
	if err != nil {
		t.Fatalf("insert chunk: %v", err)
	}

	// Read it back.
	var chunkText, sectionTitle string
	err = db.QueryRow(
		"SELECT chunk_text, section_title FROM chunks WHERE document_id = ?", docID,
	).Scan(&chunkText, &sectionTitle)
	if err != nil {
		t.Fatalf("query chunk: %v", err)
	}

	t.Logf("✓ Retrieved chunk: section=%q text=%q", sectionTitle, chunkText)
}
