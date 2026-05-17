package retrieval

import (
	"context"
	"fmt"
	"sort"

	"github.com/Pavan2027/mcu-rag/embeddings"
	"github.com/Pavan2027/mcu-rag/storage"
)

// rrfK is the RRF constant. 60 is the standard value from the original paper.
// Higher values reduce the impact of rank differences between the two lists.
const rrfK = 60

// HybridSearch is the main entry point for the retrieval pipeline.
//
// It runs vector search and FTS search sequentially, then merges the two
// ranked lists using Reciprocal Rank Fusion (RRF). The merged list is
// deduplicated, trimmed to opts.K, and ready to be passed to the reranker.
//
// Each result's FinalScore holds its RRF score at this stage. The reranker
// (Phase 6) will overwrite FinalScore with the weighted additive score.
//
// Note: searches are run sequentially for simplicity. A future optimisation
// can parallelise them with goroutines once the pipeline is stable.
func HybridSearch(
	ctx context.Context,
	db *storage.DB,
	embedder embeddings.Embedder,
	query string,
	opts RetrievalOptions,
) (*RetrievalResult, error) {
	if query == "" {
		return nil, fmt.Errorf("retrieval.HybridSearch: query must not be empty")
	}

	k := opts.K
	if k <= 0 {
		k = 10
	}

	// Pass a larger K to the storage layer so RRF has a wider pool to merge from.
	// Over-fetching here improves recall; RRF trims back to K after merging.
	storageOpts := storage.SearchOptions{
		K:          k * 2,
		ChipFamily: opts.ChipFamily,
		DocTypes:   opts.DocTypes,
	}

	// Step 1: Embed the query.
	vec, err := embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("retrieval.HybridSearch: embed query: %w", err)
	}

	// Step 2: Vector search.
	vectorResults, err := db.VectorSearch(ctx, vec, storageOpts)
	if err != nil {
		return nil, fmt.Errorf("retrieval.HybridSearch: vector search: %w", err)
	}

	// Step 3: FTS search.
	ftsResults, err := db.FTSSearch(ctx, query, storageOpts)
	if err != nil {
		return nil, fmt.Errorf("retrieval.HybridSearch: fts search: %w", err)
	}

	// Step 4: Merge using Reciprocal Rank Fusion.
	merged := mergeRRF(vectorResults, ftsResults)

	// Step 5: Trim to K.
	if len(merged) > k {
		merged = merged[:k]
	}

	return &RetrievalResult{Chunks: merged}, nil
}

// mergeRRF merges two ranked result lists using Reciprocal Rank Fusion.
//
// Formula: RRF(d) = Σ 1 / (rrfK + rank(d))   summed over all lists d appears in.
//
// A chunk appearing in both lists receives contributions from both rankings,
// naturally boosting it above chunks that appear in only one list.
// Chunks from both lists are deduplicated by ChunkID.
//
// SemanticScore and FTSScore from their respective lists are preserved on the
// merged result so the reranker can use them in Phase 6.
func mergeRRF(vectorResults, ftsResults []storage.SearchResult) []storage.SearchResult {
	type entry struct {
		result   storage.SearchResult
		rrfScore float64
	}

	byID := make(map[int]*entry, len(vectorResults)+len(ftsResults))

	// Process vector results (rank is 0-indexed).
	for rank, r := range vectorResults {
		score := 1.0 / float64(rrfK+rank+1)
		if e, ok := byID[r.ChunkID]; ok {
			e.rrfScore += score
		} else {
			rc := r // copy before taking address
			byID[r.ChunkID] = &entry{result: rc, rrfScore: score}
		}
	}

	// Process FTS results.
	for rank, r := range ftsResults {
		score := 1.0 / float64(rrfK+rank+1)
		if e, ok := byID[r.ChunkID]; ok {
			// Chunk appeared in both lists — add FTS contribution and preserve FTSScore.
			e.rrfScore += score
			e.result.FTSScore = r.FTSScore
		} else {
			rc := r
			byID[r.ChunkID] = &entry{result: rc, rrfScore: score}
		}
	}

	// Collect into a slice and set FinalScore = RRF score.
	// The reranker will replace FinalScore in Phase 6.
	merged := make([]storage.SearchResult, 0, len(byID))
	for _, e := range byID {
		r := e.result
		r.FinalScore = e.rrfScore
		merged = append(merged, r)
	}

	// Sort by FinalScore descending (highest RRF score = best combined rank).
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].FinalScore > merged[j].FinalScore
	})

	return merged
}