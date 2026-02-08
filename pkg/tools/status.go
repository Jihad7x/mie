// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Status returns memory graph health and statistics.
func Status(ctx context.Context, client Querier, args map[string]any) (*ToolResult, error) {
	stats, err := client.GetStats(ctx)
	if err != nil {
		return NewError(fmt.Sprintf("Failed to get graph stats: %v", err)), nil
	}

	var sb strings.Builder
	sb.WriteString("## MIE Memory Status\n\n")

	// Graph statistics
	sb.WriteString("### Graph Statistics\n")
	sb.WriteString(fmt.Sprintf("- Facts: %d (%d valid, %d invalidated)\n", stats.TotalFacts, stats.ValidFacts, stats.InvalidatedFacts))
	sb.WriteString(fmt.Sprintf("- Decisions: %d (%d active, %d other)\n", stats.TotalDecisions, stats.ActiveDecisions, stats.TotalDecisions-stats.ActiveDecisions))
	sb.WriteString(fmt.Sprintf("- Entities: %d\n", stats.TotalEntities))
	sb.WriteString(fmt.Sprintf("- Events: %d\n", stats.TotalEvents))
	sb.WriteString(fmt.Sprintf("- Topics: %d\n", stats.TotalTopics))
	sb.WriteString(fmt.Sprintf("- Relationships: %d edges total\n", stats.TotalEdges))

	// Configuration
	sb.WriteString("\n### Configuration\n")
	if stats.StorageEngine != "" {
		sb.WriteString(fmt.Sprintf("- Storage: %s", stats.StorageEngine))
		if stats.StoragePath != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", stats.StoragePath))
		}
		sb.WriteString("\n")
	}
	if client.EmbeddingsEnabled() {
		sb.WriteString("- Embeddings: enabled\n")
	} else {
		sb.WriteString("- Embeddings: disabled\n")
	}
	if stats.SchemaVersion != "" {
		sb.WriteString(fmt.Sprintf("- Schema version: %s\n", stats.SchemaVersion))
	}

	// Health checks
	sb.WriteString("\n### Health\n")
	totalNodes := stats.TotalFacts + stats.TotalDecisions + stats.TotalEntities + stats.TotalEvents + stats.TotalTopics
	if totalNodes > 0 {
		sb.WriteString(fmt.Sprintf("- Database accessible (%d total nodes)\n", totalNodes))
	} else {
		sb.WriteString("- Database accessible (empty graph)\n")
	}
	if client.EmbeddingsEnabled() {
		sb.WriteString("- Embeddings: active\n")
	} else {
		sb.WriteString("- Embeddings: not configured\n")
	}

	// Usage metrics
	if stats.TotalQueries > 0 || stats.TotalStores > 0 {
		sb.WriteString("\n### Usage\n")
		sb.WriteString(fmt.Sprintf("- Total queries: %d\n", stats.TotalQueries))
		sb.WriteString(fmt.Sprintf("- Total stores: %d\n", stats.TotalStores))
		if stats.LastQueryAt > 0 {
			sb.WriteString(fmt.Sprintf("- Last query: %s\n", time.Unix(stats.LastQueryAt, 0).UTC().Format("2006-01-02 15:04:05")))
		}
		if stats.LastStoreAt > 0 {
			sb.WriteString(fmt.Sprintf("- Last store: %s\n", time.Unix(stats.LastStoreAt, 0).UTC().Format("2006-01-02 15:04:05")))
		}
	}

	return NewResult(sb.String()), nil
}
