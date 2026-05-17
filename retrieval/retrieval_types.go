package retrieval

import "github.com/Pavan2027/mcu-rag/storage"

// RetrievalOptions controls the full hybrid retrieval pipeline.
type RetrievalOptions struct {
	// K is the maximum number of results returned after merging.
	// Defaults to 10 if zero.
	K int

	// ChipFamily restricts results to a specific STM32 family (e.g. "STM32F4").
	// Empty means no restriction.
	ChipFamily string

	// DocTypes restricts results to specific document types
	// (e.g. []string{"reference_manual"}). Empty means no restriction.
	DocTypes []string
}

// RetrievalResult is the output of the hybrid retrieval pipeline.
// FinalScore on each chunk is the RRF score at this stage; the reranker
// (Phase 6) will replace it with the weighted additive score.
type RetrievalResult struct {
	// Chunks holds the merged, deduplicated results ordered by FinalScore descending.
	Chunks []storage.SearchResult

	// ChunksDropped is reserved for the context builder (Phase 7) to report
	// how many chunks were trimmed due to token budget.
	ChunksDropped int
}