package main

import (
	"fmt"
	"log"
	"os"

	"hardcoreai-rag/indexing"
	"hardcoreai-rag/ingestion"
	"hardcoreai-rag/storage"
	"hardcoreai-rag/utils"
)

type DocConfig struct {
	Filename   string
	LocalPath  string
	DocType    string
	ChipFamily string
	ChipModel  string
	Version    string
}

var docsToIngest = []DocConfig{
	{
		Filename:   "stm32f407vg.pdf",
		LocalPath:  "testdata/stm_docs/stm32f407vg.pdf",
		DocType:    "datasheet",
		ChipFamily: "STM32F4",
		ChipModel:  "STM32F407VG",
		Version:    "Rev12",
	},
}

func main() {
	// Load environment variables from .env file (if present)
	_ = utils.LoadEnv(".env")

	const dbPath = "testdata/test.db"

	// Step 1: Setup storage DB
	db, err := storage.NewDB(dbPath)
	if err != nil {
		log.Fatalf("DB failed: %v", err)
	}
	defer db.Close()

	// Step 2: Setup indexer with appropriate embedder
	var embedder *indexing.Embedder
	if os.Getenv("GEMINI_API_KEY") == "" {
		fmt.Println("⚠️  Warning: GEMINI_API_KEY environment variable is not set. Falling back to MockEmbedder.")
		embedder = indexing.NewMockEmbedder()
	} else {
		fmt.Println("🚀 Using live Gemini Embedder (gemini-embedding-001)")
		embedder = indexing.NewEmbedder("", "")
	}
	indexer, err := indexing.NewIndexer(dbPath, embedder)
	if err != nil {
		log.Fatalf("Indexer failed: %v", err)
	}
	defer indexer.Close()

	parser := ingestion.NewPDFParser()
	chunker := ingestion.NewChunker()

	for _, docConfig := range docsToIngest {
		fmt.Printf("\n=============================\n")
		fmt.Printf("Processing: %s\n", docConfig.Filename)
		fmt.Printf("=============================\n")

		// Parse
		doc, err := parser.ParsePDF(docConfig.LocalPath)
		if err != nil {
			fmt.Printf("❌ Failed to parse: %v\n", err)
			continue
		}

		// Insert document record
		docID, err := db.InsertDocument(storage.Document{
			Filename:   docConfig.Filename,
			LocalPath:  docConfig.LocalPath,
			DocType:    docConfig.DocType,
			ChipFamily: docConfig.ChipFamily,
			ChipModel:  docConfig.ChipModel,
			Version:    docConfig.Version,
		})
		if err != nil {
			fmt.Printf("❌ Failed to insert document: %v\n", err)
			continue
		}

		// Chunk
		ingestionChunks := chunker.ChunkDocument(doc)

		// Convert to storage chunks
		storageChunks := make([]storage.Chunk, len(ingestionChunks))
		for i, c := range ingestionChunks {
			storageChunks[i] = storage.Chunk{
				DocumentID:   int(docID),
				ChunkText:    c.ChunkText,
				SectionTitle: c.SectionTitle,
				Peripheral:   c.Peripheral,
				RegisterName: c.RegisterName,
				PageNumber:   c.PageNumber,
				TokenCount:   c.TokenCount,
				ChunkIndex:   c.ChunkIndex,
			}
		}

		// Insert chunks and get their IDs back (only call this, not InsertChunks)
		chunkIDs, err := db.InsertChunksAndReturnIDs(storageChunks)
		if err != nil {
			db.UpdateDocumentStatus(docID, "failed", err.Error())
			fmt.Printf("❌ Failed to insert chunks: %v\n", err)
			continue
		}

		// Extract texts for embedding
		chunkTexts := make([]string, len(ingestionChunks))
		for i, c := range ingestionChunks {
			chunkTexts[i] = c.ChunkText
		}

		// Generate and store embeddings
		if err := indexer.IndexChunks(chunkIDs, chunkTexts); err != nil {
			db.UpdateDocumentStatus(docID, "failed", err.Error())
			fmt.Printf("❌ Failed to index embeddings: %v\n", err)
			continue
		}

		db.UpdateDocumentStatus(docID, "indexed", "")
		fmt.Printf("✅ Fully indexed: %s\n", docConfig.Filename)
	}

	fmt.Printf("\n=============================\n")
	fmt.Printf("PIPELINE COMPLETE\n")
	fmt.Printf("=============================\n")
}
