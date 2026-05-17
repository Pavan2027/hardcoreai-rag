package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mattn/go-sqlite3"
)

// DB wraps the standard sql.DB with project-specific helpers.
type DB struct {
	*sql.DB
}

// Open loads the sqlite-vec extension and returns a ready DB handle.
//
// vecExtPath is the path to the vec0 shared library (vec0.dll on Windows,
// vec0.so on Linux). If empty, it defaults to looking for the library in a
// "bin/" directory relative to the current working directory.
func Open(dbPath string, vecExtPath string) (*DB, error) {
	if vecExtPath == "" {
		// Default: look for vec0 in bin/ next to the binary.
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("storage.Open: could not determine working directory: %w", err)
		}
		vecExtPath = filepath.Join(cwd, "bin", "vec0")
	}

	// Register a custom sqlite3 driver that loads the vec0 extension.
	// The driver name is unique so it can be registered once per process.
	driverName := "sqlite3_with_vec"
	// Guard against duplicate registration (e.g., in tests).
	for _, d := range sql.Drivers() {
		if d == driverName {
			goto openDB
		}
	}
	sql.Register(driverName, &sqlite3.SQLiteDriver{
		Extensions: []string{vecExtPath},
	})

openDB:
	db, err := sql.Open(driverName, dbPath)
	if err != nil {
		return nil, fmt.Errorf("storage.Open: failed to open db at %q: %w", dbPath, err)
	}

	// Verify the connection and that vec0 was loaded correctly.
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("storage.Open: ping failed (check vec0 extension path %q): %w", vecExtPath, err)
	}

	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return nil, fmt.Errorf("storage.Open: failed to set WAL mode: %w", err)
	}

	// Enable foreign key enforcement.
	if _, err := db.Exec("PRAGMA foreign_keys=ON;"); err != nil {
		return nil, fmt.Errorf("storage.Open: failed to enable foreign keys: %w", err)
	}

	return &DB{db}, nil
}

// NewDB is a convenience wrapper (matching the ingestion pipeline entry point)
// that loads the sqlite-vec extension and initializes all tables using the unified schema.
func NewDB(dbPath string) (*DB, error) {
	// Create directory if it doesn't exist
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	db, err := Open(dbPath, "")
	if err != nil {
		return nil, fmt.Errorf("NewDB failed: %w", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS documents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		mongo_id TEXT UNIQUE,
		filename TEXT NOT NULL,
		local_path TEXT,
		source_url TEXT,
		doc_type TEXT,
		chip_family TEXT,
		chip_model TEXT,
		version TEXT,
		processing_status TEXT DEFAULT 'pending',
		error_message TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS chunks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		document_id INTEGER NOT NULL,
		chunk_text TEXT NOT NULL,
		section_title TEXT,
		subsection_title TEXT,
		peripheral TEXT,
		register_name TEXT,
		page_number INTEGER,
		token_count INTEGER,
		chunk_index INTEGER,
		metadata TEXT,
		FOREIGN KEY(document_id) REFERENCES documents(id)
	);

	CREATE VIRTUAL TABLE IF NOT EXISTS vec_chunks USING vec0(
		chunk_id INTEGER PRIMARY KEY,
		embedding FLOAT[768]
	);

	CREATE VIRTUAL TABLE IF NOT EXISTS chunk_fts USING fts5(
		chunk_text,
		section_title,
		peripheral,
		register_name,
		content='chunks',
		content_rowid='id'
	);

	CREATE INDEX IF NOT EXISTS idx_documents_chip_family ON documents(chip_family);
	CREATE INDEX IF NOT EXISTS idx_documents_status ON documents(processing_status);
	CREATE INDEX IF NOT EXISTS idx_chunks_document_id ON chunks(document_id);
	CREATE INDEX IF NOT EXISTS idx_chunks_peripheral ON chunks(peripheral);
	CREATE INDEX IF NOT EXISTS idx_chunks_register ON chunks(register_name);
	`

	if err := db.ApplySchema(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("NewDB: failed to apply schema: %w", err)
	}

	fmt.Println("✅ Database connected, vec0 loaded, and tables ready")
	return db, nil
}

// Close closes the underlying database connection.
func (db *DB) Close() error {
	return db.DB.Close()
}

// ApplySchema runs the contents of schemaSQL against the database.
// Typically used during seed DB setup and tests.
func (db *DB) ApplySchema(schemaSQL string) error {
	if _, err := db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("storage.ApplySchema: %w", err)
	}
	return nil
}

// InsertDocument inserts a new document record.
func (db *DB) InsertDocument(doc Document) (int64, error) {
	result, err := db.Exec(`
		INSERT INTO documents 
			(mongo_id, filename, local_path, source_url, doc_type, chip_family, chip_model, version, processing_status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		doc.MongoID, doc.Filename, doc.LocalPath, doc.SourceURL,
		doc.DocType, doc.ChipFamily, doc.ChipModel, doc.Version, "processing",
	)
	if err != nil {
		return 0, fmt.Errorf("failed to insert document: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	fmt.Printf("📄 Inserted document: %s (id=%d)\n", doc.Filename, id)
	return id, nil
}

// InsertChunk inserts a single chunk record.
func (db *DB) InsertChunk(chunk Chunk) (int64, error) {
	result, err := db.Exec(`
		INSERT INTO chunks
			(document_id, chunk_text, section_title, subsection_title, peripheral, register_name, page_number, token_count, chunk_index, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		chunk.DocumentID, chunk.ChunkText, chunk.SectionTitle, chunk.SubsectionTitle,
		chunk.Peripheral, chunk.RegisterName, chunk.PageNumber, chunk.TokenCount,
		chunk.ChunkIndex, chunk.Metadata,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to insert chunk: %w", err)
	}

	return result.LastInsertId()
}

// InsertChunks inserts a batch of chunk records.
func (db *DB) InsertChunks(chunks []Chunk) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO chunks
			(document_id, chunk_text, section_title, subsection_title, peripheral, register_name, page_number, token_count, chunk_index, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, chunk := range chunks {
		_, err := stmt.Exec(
			chunk.DocumentID, chunk.ChunkText, chunk.SectionTitle, chunk.SubsectionTitle,
			chunk.Peripheral, chunk.RegisterName, chunk.PageNumber, chunk.TokenCount,
			chunk.ChunkIndex, chunk.Metadata,
		)
		if err != nil {
			return fmt.Errorf("failed to insert chunk %d: %w", chunk.ChunkIndex, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	fmt.Printf("✅ Inserted %d chunks into database\n", len(chunks))
	return nil
}

// InsertChunksAndReturnIDs inserts a batch of chunks and returns their auto-generated IDs.
func (db *DB) InsertChunksAndReturnIDs(chunks []Chunk) ([]int, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO chunks
			(document_id, chunk_text, section_title, subsection_title, peripheral, register_name, page_number, token_count, chunk_index, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	ids := make([]int, len(chunks))
	for i, chunk := range chunks {
		result, err := stmt.Exec(
			chunk.DocumentID, chunk.ChunkText, chunk.SectionTitle, chunk.SubsectionTitle,
			chunk.Peripheral, chunk.RegisterName, chunk.PageNumber, chunk.TokenCount,
			chunk.ChunkIndex, chunk.Metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to insert chunk %d: %w", chunk.ChunkIndex, err)
		}
		id, _ := result.LastInsertId()
		ids[i] = int(id)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	fmt.Printf("✅ Inserted %d chunks into database\n", len(chunks))
	return ids, nil
}

// UpdateDocumentStatus updates the processing status and error message of a document.
func (db *DB) UpdateDocumentStatus(id int64, status, errMsg string) error {
	_, err := db.Exec(`
		UPDATE documents SET processing_status = ?, error_message = ? WHERE id = ?`,
		status, errMsg, id,
	)
	return err
}

// GetChunksByPeripheral retrieves all chunks belonging to a specific peripheral tag.
func (db *DB) GetChunksByPeripheral(peripheral string) ([]Chunk, error) {
	rows, err := db.Query(`
		SELECT id, document_id, chunk_text, section_title, peripheral, register_name, page_number, token_count, chunk_index
		FROM chunks WHERE peripheral = ?`, peripheral)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []Chunk
	for rows.Next() {
		var c Chunk
		err := rows.Scan(&c.ID, &c.DocumentID, &c.ChunkText, &c.SectionTitle,
			&c.Peripheral, &c.RegisterName, &c.PageNumber, &c.TokenCount, &c.ChunkIndex)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, c)
	}
	return chunks, nil
}
