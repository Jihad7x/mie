// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"strings"
)

// Conflicts detects potentially contradicting facts in the memory graph.
func Conflicts(ctx context.Context, client Querier, args map[string]any) (*ToolResult, error) {
	if !client.EmbeddingsEnabled() {
		return NewError("Conflict detection requires embeddings to be enabled."), nil
	}

	category := GetStringArg(args, "category", "")
	threshold := GetFloat64Arg(args, "threshold", 0.85)
	if threshold <= 0 || threshold > 1.0 {
		threshold = 0.85
	}
	limit := GetIntArg(args, "limit", 10)
	if limit < 1 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}

	conflicts, err := client.DetectConflicts(ctx, ConflictOptions{
		Category:  category,
		Threshold: threshold,
		Limit:     limit,
	})
	if err != nil {
		return NewError(fmt.Sprintf("Failed to detect conflicts: %v", err)), nil
	}

	var sb strings.Builder

	if len(conflicts) == 0 {
		sb.WriteString("## Conflict Scan Results\n\n")
		sb.WriteString("_No potential conflicts found._\n")
		if category != "" {
			sb.WriteString(fmt.Sprintf("\nScanned category: %s (threshold: %.0f%%)\n", category, threshold*100))
		} else {
			sb.WriteString(fmt.Sprintf("\nScanned all categories (threshold: %.0f%%)\n", threshold*100))
		}
		return NewResult(sb.String()), nil
	}

	sb.WriteString(fmt.Sprintf("## Potential Conflicts Found (%d)\n\n", len(conflicts)))

	for i, c := range conflicts {
		sb.WriteString(fmt.Sprintf("### Conflict %d (similarity: %.0f%%)\n", i+1, c.Similarity*100))
		sb.WriteString(fmt.Sprintf("- [%s] %q (%s, confidence: %.1f)\n",
			c.FactA.ID, Truncate(c.FactA.Content, 80), c.FactA.Category, c.FactA.Confidence))
		sb.WriteString(fmt.Sprintf("- [%s] %q (%s, confidence: %.1f)\n",
			c.FactB.ID, Truncate(c.FactB.Content, 80), c.FactB.Category, c.FactB.Confidence))

		// Recommendation based on similarity tier
		similarityPct := c.Similarity * 100
		if similarityPct >= 90 && c.FactA.Category == c.FactB.Category {
			// High similarity with same category: suggest supersedence
			if c.FactA.CreatedAt < c.FactB.CreatedAt {
				sb.WriteString(fmt.Sprintf("  Recommendation: The newer fact [%s] likely supersedes the older one [%s].\n\n",
					c.FactB.ID, c.FactA.ID))
			} else if c.FactB.CreatedAt < c.FactA.CreatedAt {
				sb.WriteString(fmt.Sprintf("  Recommendation: The newer fact [%s] likely supersedes the older one [%s].\n\n",
					c.FactA.ID, c.FactB.ID))
			} else {
				sb.WriteString("  Recommendation: Review both facts and invalidate the incorrect one.\n\n")
			}
		} else if similarityPct >= 75 {
			sb.WriteString("  Recommendation: These facts may be related or contradictory. Review and invalidate if needed.\n\n")
		} else {
			sb.WriteString("  Recommendation: These facts are semantically similar but may not be contradictory.\n\n")
		}
	}

	sb.WriteString("To resolve: call mie_update with action=\"invalidate\" on the outdated fact.\n")

	return NewResult(sb.String()), nil
}