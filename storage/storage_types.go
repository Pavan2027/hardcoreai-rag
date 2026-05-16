package storage

import "time"

// Document represents a hardware documentation file
type Document struct {
	ID               int       `db:"id"`
	MongoID          string    `db:"mongo_id"`
	Filename         string    `db:"filename"`
	LocalPath        string    `db:"local_path"`
	SourceURL        string    `db:"source_url"`
	DocType          string    `db:"doc_type"`    // reference_manual, datasheet, app_note
	ChipFamily       string    `db:"chip_family"` // STM32F4, STM32H7
	ChipModel        string    `db:"chip_model"`  // STM32F407VG
	Version          string    `db:"version"`
	ProcessingStatus string    `db:"processing_status"` // pending, processing, indexed, failed
	ErrorMessage     string    `db:"error_message"`
	CreatedAt        time.Time `db:"created_at"`
}

// Chunk represents a piece of text from a document
type Chunk struct {
	ID              int       `db:"id"`
	DocumentID      int       `db:"document_id"`
	ChunkText       string    `db:"chunk_text"`
	SectionTitle    string    `db:"section_title"`
	SubsectionTitle string    `db:"subsection_title"`
	Peripheral      string    `db:"peripheral"`    // USART, GPIO, SPI, RCC
	RegisterName    string    `db:"register_name"` // USART_BRR, RCC_CFGR
	PageNumber      int       `db:"page_number"`
	TokenCount      int       `db:"token_count"`
	ChunkIndex      int       `db:"chunk_index"`
	Metadata        string    `db:"metadata"` // JSON blob
	Embedding       []float32 // Vector embedding (from vec_chunks table)
}

// ChunkWithDistance is used for retrieval results
type ChunkWithDistance struct {
	Chunk
	Distance float32 `db:"distance"` // Similarity score
}
