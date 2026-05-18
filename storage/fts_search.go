package storage

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode"
)

// sanitizeFTSQuery strips characters that FTS5 treats as syntax operators
// while preserving tokens common in STM32 docs: letters, digits, underscores
// (USART_BRR), hyphens (chip names), and spaces.
func sanitizeFTSQuery(query string) string {
	var b strings.Builder
	for _, r := range query {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == ' ' {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// FTSSearch performs BM25-ranked full-text search against chunk_fts.
//
// Filters in opts are ANDed into the WHERE clause at the SQL level.
// When filters are active the query over-fetches by 3× before applying LIMIT,
// since filter conditions narrow the FTS candidates after the MATCH.
//
// FTSScore on each result is normalised to [0,1]: best BM25 rank → 1.0.
func (db *DB) FTSSearch(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
	if query == "" {
		return nil, fmt.Errorf("storage.FTSSearch: query must not be empty")
	}

	clean := sanitizeFTSQuery(query)
	if clean == "" {
		return []SearchResult{}, nil
	}

	fetchK := opts.K
	if fetchK <= 0 {
		fetchK = 10
	}

	filterClause, filterArgs := BuildFilterSQL(opts)
	if filterClause != "" {
		fetchK *= 3
	}

	// Base query: FTS MATCH, JOINs to chunks and documents.
	// Filter clause is ANDed after MATCH. ORDER BY bm25 and LIMIT are appended last.
	const baseQuery = `
		SELECT
			c.id, c.document_id, c.chunk_text, c.section_title,
			c.peripheral, c.register_name, c.page_number, c.chunk_index,
			d.filename, d.doc_type, d.chip_family, d.chip_model,
			bm25(chunk_fts) AS bm25_score
		FROM chunk_fts
		JOIN chunks c ON c.id = chunk_fts.rowid
		JOIN documents d ON d.id = c.document_id
		WHERE chunk_fts MATCH ?
	`

	q := baseQuery
	if filterClause != "" {
		q += "AND " + filterClause + "\n"
	}
	q += "ORDER BY bm25_score\nLIMIT ?"

	// Arg order: MATCH string, filter args (if any), then LIMIT.
	args := make([]interface{}, 0, 2+len(filterArgs))
	args = append(args, clean)
	args = append(args, filterArgs...)
	args = append(args, fetchK)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("storage.FTSSearch: query failed: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	var rawScores []float64

	for rows.Next() {
		var r SearchResult
		var bm25 float64
		if err := rows.Scan(
			&r.ChunkID, &r.DocumentID, &r.ChunkText, &r.SectionTitle,
			&r.Peripheral, &r.RegisterName, &r.PageNumber, &r.ChunkIndex,
			&r.Filename, &r.DocType, &r.ChipFamily, &r.ChipModel,
			&bm25,
		); err != nil {
			return nil, fmt.Errorf("storage.FTSSearch: scan row: %w", err)
		}
		results = append(results, r)
		rawScores = append(rawScores, bm25)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage.FTSSearch: rows error: %w", err)
	}

	if len(results) == 0 {
		return []SearchResult{}, nil
	}

	// BM25 from FTS5 is negative: most negative = best match.
	// Normalise: best (minScore) → 1.0, worst (maxScore) → 0.0.
	minScore, maxScore := rawScores[0], rawScores[0]
	for _, s := range rawScores[1:] {
		if s < minScore {
			minScore = s
		}
		if s > maxScore {
			maxScore = s
		}
	}

	spread := maxScore - minScore
	for i := range results {
		if math.Abs(spread) < 1e-9 {
			results[i].FTSScore = 1.0
		} else {
			results[i].FTSScore = (maxScore - rawScores[i]) / spread
		}
	}

	if opts.K > 0 && len(results) > opts.K {
		results = results[:opts.K]
	}

	return results, nil
}