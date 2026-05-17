package storage

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
)

// SerializeEmbedding converts a []float64 into a little-endian float32 byte
// blob that sqlite-vec's vec0 virtual table accepts as a KNN query vector.
//
// Exported so the indexing pipeline uses the same serialization when
// inserting embeddings into vec_chunks.
func SerializeEmbedding(vec []float64) []byte {
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		bits := math.Float32bits(float32(v))
		binary.LittleEndian.PutUint32(buf[i*4:], bits)
	}
	return buf
}

// VectorSearch performs a KNN search against vec_chunks using sqlite-vec.
//
// Filters in opts are applied at the SQL level on the outer JOIN — not
// in-memory. When filters are active the KNN subquery over-fetches by 3×
// to compensate for candidates eliminated by the outer WHERE clause.
//
// SemanticScore on each result is normalised to [0,1]: distance=0 → 1.0.
func (db *DB) VectorSearch(ctx context.Context, embedding []float64, opts SearchOptions) ([]SearchResult, error) {
	if len(embedding) == 0 {
		return nil, fmt.Errorf("storage.VectorSearch: embedding must not be empty")
	}

	knnLimit := opts.K
	if knnLimit <= 0 {
		knnLimit = 10
	}

	filterClause, filterArgs := BuildFilterSQL(opts)
	if filterClause != "" {
		// Over-fetch so the outer WHERE doesn't starve the result set.
		knnLimit *= 3
	}

	blob := SerializeEmbedding(embedding)

	// The KNN LIMIT must sit directly on the vec0 virtual table scan.
	// Filter conditions go on the outer query against the joined document/chunk
	// tables. ORDER BY knn.distance is always the final clause.
	const selectPart = `
		SELECT
			c.id, c.document_id, c.chunk_text, c.section_title,
			c.peripheral, c.register_name, c.page_number, c.chunk_index,
			d.filename, d.doc_type, d.chip_family, d.chip_model,
			knn.distance
		FROM (
			SELECT chunk_id, distance
			FROM vec_chunks
			WHERE embedding MATCH ?
			ORDER BY distance
			LIMIT ?
		) knn
		JOIN chunks c ON c.id = knn.chunk_id
		JOIN documents d ON d.id = c.document_id
	`

	query := selectPart
	if filterClause != "" {
		query += "WHERE " + filterClause + "\n"
	}
	query += "ORDER BY knn.distance"

	// Arg order: blob + knnLimit for the subquery, then filter args for outer WHERE.
	args := make([]interface{}, 0, 2+len(filterArgs))
	args = append(args, blob, knnLimit)
	args = append(args, filterArgs...)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("storage.VectorSearch: query failed: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	var distances []float64

	for rows.Next() {
		var r SearchResult
		var dist float64
		if err := rows.Scan(
			&r.ChunkID, &r.DocumentID, &r.ChunkText, &r.SectionTitle,
			&r.Peripheral, &r.RegisterName, &r.PageNumber, &r.ChunkIndex,
			&r.Filename, &r.DocType, &r.ChipFamily, &r.ChipModel,
			&dist,
		); err != nil {
			return nil, fmt.Errorf("storage.VectorSearch: scan row: %w", err)
		}
		results = append(results, r)
		distances = append(distances, dist)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage.VectorSearch: rows error: %w", err)
	}

	for i := range results {
		results[i].SemanticScore = 1.0 / (1.0 + distances[i])
	}

	if opts.K > 0 && len(results) > opts.K {
		results = results[:opts.K]
	}

	return results, nil
}