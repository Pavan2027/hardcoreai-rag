package storage

// Document maps to the `documents` table.
type Document struct {
	ID         int    `db:"id"`
	MongoID    string `db:"mongo_id"`
	Filename   string `db:"filename"`
	DocType    string `db:"doc_type"`
	ChipFamily string `db:"chip_family"`
	ChipModel  string `db:"chip_model"`
	Version    string `db:"version"`
	CreatedAt  string `db:"created_at"`
}

// Chunk maps to the `chunks` table.
type Chunk struct {
	ID              int       `db:"id"`
	DocumentID      int       `db:"document_id"`
	ChunkText       string    `db:"chunk_text"`
	SectionTitle    string    `db:"section_title"`
	SubsectionTitle string    `db:"subsection_title"`
	Peripheral      string    `db:"peripheral"`
	RegisterName    string    `db:"register_name"`
	PageNumber      int       `db:"page_number"`
	TokenCount      int       `db:"token_count"`
	ChunkIndex      int       `db:"chunk_index"`
	Embedding       []float64 `db:"embedding"`
}

// SearchResult is the unified result type returned by all search functions.
// It combines fields from chunks and documents plus scoring information.
type SearchResult struct {
	// From chunks table
	ChunkID      int
	ChunkText    string
	SectionTitle string
	Peripheral   string
	RegisterName string
	PageNumber   int
	DocumentID   int
	ChunkIndex   int

	// From documents table (populated via JOIN)
	Filename   string
	DocType    string
	ChipFamily string
	ChipModel  string

	// Scoring — populated by the retrieval layer
	SemanticScore float64
	FTSScore      float64
	MetadataBoost float64
	FinalScore    float64
}

// SearchOptions contains query parameters for VectorSearch and FTSSearch.
// All filter fields except K are passed to BuildFilterSQL for SQL-level
// pre-filtering. Adding fields here automatically applies them in queries
// without any changes to the function signatures.
type SearchOptions struct {
	// K is the maximum number of results to return.
	K int

	// Filter fields — all optional. Empty/nil means no restriction.
	ChipFamily string   // e.g. "STM32F4"
	ChipModel  string   // e.g. "STM32F429"
	Peripheral string   // e.g. "USART"
	DocTypes   []string // e.g. []string{"reference_manual"}
}