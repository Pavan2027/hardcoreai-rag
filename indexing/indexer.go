package indexing

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
)

type Indexer struct {
	db       *sql.DB
	embedder *Embedder
}

func NewIndexer(dbPath string, embedder *Embedder) (*Indexer, error) {
	// The DB is opened standardly using the CGO driver registered by storage.
	// Since we are running in rag-cli or test_parser, we can use "sqlite3_with_vec" as the driver.
	// We'll check if "sqlite3_with_vec" is registered, otherwise fall back to standard "sqlite3".
	driver := "sqlite3"
	for _, d := range sql.Drivers() {
		if d == "sqlite3_with_vec" {
			driver = "sqlite3_with_vec"
			break
		}
	}

	db, err := sql.Open(driver, dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open db: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	fmt.Println("✅ Indexer ready")
	return &Indexer{db: db, embedder: embedder}, nil
}

func (idx *Indexer) IndexChunks(chunkIDs []int, chunkTexts []string) error {
	fmt.Printf("🔢 Generating embeddings for %d chunks...\n", len(chunkTexts))

	embeddings, err := idx.embedder.EmbedBatch(chunkTexts)
	if err != nil {
		return fmt.Errorf("embedding failed: %w", err)
	}

	tx, err := idx.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Store embeddings in the vec_chunks virtual table for vector search
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO vec_chunks (chunk_id, embedding) VALUES (?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for i, embedding := range embeddings {
		blob := float32SliceToBytes(embedding)
		if _, err := stmt.Exec(chunkIDs[i], blob); err != nil {
			return fmt.Errorf("failed to store embedding %d: %w", i, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	fmt.Printf("✅ Stored %d embeddings in vec_chunks\n", len(embeddings))
	return nil
}

func (idx *Indexer) Close() error {
	return idx.db.Close()
}

func float32SliceToBytes(floats []float32) []byte {
	b := make([]byte, len(floats)*4)
	for i, f := range floats {
		bits := math.Float32bits(f)
		binary.LittleEndian.PutUint32(b[i*4:], bits)
	}
	return b
}
