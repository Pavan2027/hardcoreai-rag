-- Documents table
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

-- Chunks table
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

-- Vector table (sqlite-vec, 768 dimensions for nomic-embed-text)
CREATE VIRTUAL TABLE IF NOT EXISTS vec_chunks USING vec0(
    chunk_id INTEGER PRIMARY KEY,
    embedding FLOAT[768]
);

-- Full-text search (FTS5)
CREATE VIRTUAL TABLE IF NOT EXISTS chunk_fts USING fts5(
    chunk_text,
    section_title,
    peripheral,
    register_name,
    content='chunks',
    content_rowid='id'
);

-- Performance indexes
CREATE INDEX IF NOT EXISTS idx_documents_chip_family ON documents(chip_family);
CREATE INDEX IF NOT EXISTS idx_documents_status ON documents(processing_status);
CREATE INDEX IF NOT EXISTS idx_chunks_document_id ON chunks(document_id);
CREATE INDEX IF NOT EXISTS idx_chunks_peripheral ON chunks(peripheral);
CREATE INDEX IF NOT EXISTS idx_chunks_register ON chunks(register_name);