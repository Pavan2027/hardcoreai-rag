-- Documents Table
CREATE TABLE IF NOT EXISTS documents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    mongo_id TEXT UNIQUE,
    filename TEXT NOT NULL,
    doc_type TEXT,
    chip_family TEXT,
    chip_model TEXT,
    version TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Chunks Table
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
    FOREIGN KEY(document_id) REFERENCES documents(id)
);

-- Vector Chunks Table (Using 768 dimensions for nomic-embed-text)
CREATE VIRTUAL TABLE IF NOT EXISTS vec_chunks USING vec0(
    chunk_id INTEGER PRIMARY KEY,
    embedding FLOAT[768]
);

-- FTS5 Table for Keyword Search
CREATE VIRTUAL TABLE IF NOT EXISTS chunk_fts USING fts5(
    chunk_text,
    section_title,
    peripheral,
    register_name,
    content='chunks',
    content_rowid='id'
);
