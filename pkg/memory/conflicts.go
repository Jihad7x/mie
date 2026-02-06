// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build cozodb

package memory

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kraklabs/mie/pkg/storage"
	"github.com/kraklabs/mie/pkg/tools"
)

// ConflictDetector identifies facts that may contradict each other.
type ConflictDetector struct {
	backend  storage.Backend
	embedder *EmbeddingGenerator
	logger   *slog.Logger
}

// NewConflictDetector creates a new ConflictDetector.
func NewConflictDetector(backend storage.Backend, embedder *EmbeddingGenerator, logger *slog.Logger) *ConflictDetector {
	if logger == nil {
		logger = slog.Default()
	}
	return &ConflictDetector{backend: backend, embedder: embedder, logger: logger}
}

// DetectConflicts scans for potentially contradicting facts using HNSW neighbor search.
func (cd *ConflictDetector) DetectConflicts(ctx context.Context, opts tools.ConflictOptions) ([]tools.Conflict, error) {
	if cd.embedder == nil {
		return nil, fmt.Errorf("conflict detection requires embeddings to be enabled")
	}

	threshold := opts.Threshold
	if threshold <= 0 {
		threshold = 0.15 // Default: cosine distance < 0.15 means ~85% similarity
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	// Get all valid facts
	categoryFilter := ""
	if opts.Category != "" {
		categoryFilter = fmt.Sprintf(`, category = '%s'`, escapeDatalog(opts.Category))
	}

	factsQuery := fmt.Sprintf(
		`?[id, content, category, confidence, source_agent, source_conversation, created_at, updated_at] :=
    *mie_fact { id, content, category, confidence, source_agent, source_conversation, valid, created_at, updated_at },
    valid = true%s`, categoryFilter,
	)

	qr, err := cd.backend.Query(ctx, factsQuery)
	if err != nil {
		return nil, fmt.Errorf("query facts: %w", err)
	}

	if len(qr.Rows) < 2 {
		return nil, nil // Need at least 2 facts to find conflicts
	}

	// For each fact, find its nearest neighbors via HNSW
	var conflicts []tools.Conflict
	seen := make(map[string]bool) // Track pairs to avoid duplicates

	for _, row := range qr.Rows {
		factID := toString(row[0])
		factContent := toString(row[1])

		// Generate embedding for this fact's content
		queryEmb, err := cd.embedder.GenerateQuery(ctx, factContent)
		if err != nil {
			cd.logger.Warn("failed to generate embedding for conflict check", "fact_id", factID, "error", err)
			continue
		}

		vecStr := formatVector(queryEmb)

		// Search for nearest neighbors
		script := fmt.Sprintf(
			`?[neighbor_id, content, category, confidence, source_agent, source_conversation, created_at, updated_at, distance] :=
    ~mie_fact_embedding:fact_embedding_idx { fact_id | query: q, k: 10, ef: 200, bind_distance: distance },
    q = vec(%s),
    *mie_fact { id: fact_id, content, category, confidence, source_agent, source_conversation, valid, created_at, updated_at },
    valid = true,
    neighbor_id = fact_id,
    neighbor_id != '%s',
    distance < %f
    :order distance
    :limit 5`, vecStr, escapeDatalog(factID), threshold,
		)

		neighbors, err := cd.backend.Query(ctx, script)
		if err != nil {
			cd.logger.Warn("hnsw neighbor search failed", "fact_id", factID, "error", err)
			continue
		}

		factA := tools.Fact{
			ID:                 factID,
			Content:            factContent,
			Category:           toString(row[2]),
			Confidence:         toFloat64(row[3]),
			SourceAgent:        toString(row[4]),
			SourceConversation: toString(row[5]),
			Valid:              true,
			CreatedAt:          toInt64(row[6]),
			UpdatedAt:          toInt64(row[7]),
		}

		for _, nRow := range neighbors.Rows {
			neighborID := toString(nRow[0])

			// Avoid duplicate pairs
			pairKey := factID + "|" + neighborID
			reversePairKey := neighborID + "|" + factID
			if seen[pairKey] || seen[reversePairKey] {
				continue
			}
			seen[pairKey] = true

			distance := toFloat64(nRow[8])
			similarity := 1.0 - distance // Convert cosine distance to similarity

			factB := tools.Fact{
				ID:                 neighborID,
				Content:            toString(nRow[1]),
				Category:           toString(nRow[2]),
				Confidence:         toFloat64(nRow[3]),
				SourceAgent:        toString(nRow[4]),
				SourceConversation: toString(nRow[5]),
				Valid:              true,
				CreatedAt:          toInt64(nRow[6]),
				UpdatedAt:          toInt64(nRow[7]),
			}

			conflicts = append(conflicts, tools.Conflict{
				FactA:      factA,
				FactB:      factB,
				Similarity: similarity,
			})
		}

		if len(conflicts) >= limit {
			break
		}
	}

	// Sort by similarity (highest first)
	for i := 0; i < len(conflicts); i++ {
		for j := i + 1; j < len(conflicts); j++ {
			if conflicts[j].Similarity > conflicts[i].Similarity {
				conflicts[i], conflicts[j] = conflicts[j], conflicts[i]
			}
		}
	}

	if len(conflicts) > limit {
		conflicts = conflicts[:limit]
	}

	return conflicts, nil
}

// CheckNewFactConflicts checks if new content conflicts with existing facts.
func (cd *ConflictDetector) CheckNewFactConflicts(ctx context.Context, content, category string) ([]tools.Conflict, error) {
	if cd.embedder == nil {
		return nil, fmt.Errorf("conflict detection requires embeddings to be enabled")
	}

	// Generate embedding for the proposed content
	queryEmb, err := cd.embedder.GenerateQuery(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("generate query embedding: %w", err)
	}

	vecStr := formatVector(queryEmb)
	threshold := 0.15 // cosine distance threshold

	categoryFilter := ""
	if category != "" {
		categoryFilter = fmt.Sprintf(`,
    category = '%s'`, escapeDatalog(category))
	}

	script := fmt.Sprintf(
		`?[id, fact_content, category, confidence, source_agent, source_conversation, created_at, updated_at, distance] :=
    ~mie_fact_embedding:fact_embedding_idx { fact_id | query: q, k: 10, ef: 200, bind_distance: distance },
    q = vec(%s),
    *mie_fact { id: fact_id, content: fact_content, category, confidence, source_agent, source_conversation, valid, created_at, updated_at },
    valid = true,
    id = fact_id,
    distance < %f%s
    :order distance
    :limit 10`, vecStr, threshold, categoryFilter,
	)

	qr, err := cd.backend.Query(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("check conflicts: %w", err)
	}

	proposedFact := tools.Fact{
		Content:  content,
		Category: category,
		Valid:    true,
	}

	var conflicts []tools.Conflict
	for _, row := range qr.Rows {
		distance := toFloat64(row[8])
		similarity := 1.0 - distance

		existingFact := tools.Fact{
			ID:                 toString(row[0]),
			Content:            toString(row[1]),
			Category:           toString(row[2]),
			Confidence:         toFloat64(row[3]),
			SourceAgent:        toString(row[4]),
			SourceConversation: toString(row[5]),
			Valid:              true,
			CreatedAt:          toInt64(row[6]),
			UpdatedAt:          toInt64(row[7]),
		}

		conflicts = append(conflicts, tools.Conflict{
			FactA:      proposedFact,
			FactB:      existingFact,
			Similarity: similarity,
		})
	}

	return conflicts, nil
}
