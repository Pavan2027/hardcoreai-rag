package storage

import "time"

type Document struct {
	ID               int
	MongoID          string
	Filename         string
	LocalPath        string
	SourceURL        string
	DocType          string
	ChipFamily       string
	ChipModel        string
	Version          string
	ProcessingStatus string
	ErrorMessage     string
	CreatedAt        time.Time
}

type Chunk struct {
	ID              int
	DocumentID      int
	ChunkText       string
	SectionTitle    string
	SubsectionTitle string
	Peripheral      string
	RegisterName    string
	PageNumber      int
	TokenCount      int
	ChunkIndex      int
	Metadata        string
	Embedding       []float32
}
