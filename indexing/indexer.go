package indexing

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

type Indexer struct {
	db       *sql.DB
	embedder *Embedder
}

func NewIndexer(dbPath string, embedder *Embedder) (*Indexer, error) {
	db, err := sql.Open("sqlite3", dbPath)
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

	stmt, err := tx.Prepare(`UPDATE chunks SET embedding = ? WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for i, embedding := range embeddings {
		blob := float32SliceToBytes(embedding)
		if _, err := stmt.Exec(blob, chunkIDs[i]); err != nil {
			return fmt.Errorf("failed to store embedding %d: %w", i, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	fmt.Printf("✅ Stored %d embeddings in database\n", len(embeddings))
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
