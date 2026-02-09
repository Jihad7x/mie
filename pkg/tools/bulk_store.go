// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const maxBulkItems = 50

// bulkItem tracks the result of storing a single item in a bulk operation.
type bulkItem struct {
	nodeID   string
	nodeType string
	summary  string
}

// BulkStore writes multiple nodes and optional relationships to the memory graph in a single call.
func BulkStore(ctx context.Context, client Querier, args map[string]any) (*ToolResult, error) {
	rawItems, ok := args["items"]
	if !ok || rawItems == nil {
		return NewError("Missing required parameter: items"), nil
	}
	itemSlice, ok := rawItems.([]any)
	if !ok || len(itemSlice) == 0 {
		return NewError("items must be a non-empty array"), nil
	}
	if len(itemSlice) > maxBulkItems {
		return NewError(fmt.Sprintf("Too many items: %d (max %d)", len(itemSlice), maxBulkItems)), nil
	}

	// Pre-validate all items before storing any.
	if validationErrors := bulkPreValidate(itemSlice); len(validationErrors) > 0 {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Validation failed for %d item(s). Nothing was stored.\n\n", len(validationErrors)))
		for _, e := range validationErrors {
			sb.WriteString(fmt.Sprintf("  - %s\n", e))
		}
		return NewError(sb.String()), nil
	}

	stored, typeCounts, errors := bulkStoreNodes(ctx, client, itemSlice)
	relMessages, relErrors := bulkProcessRelationships(ctx, client, itemSlice, stored)
	errors = append(errors, relErrors...)

	// Increment usage counters (never fail the main operation).
	totalStored := 0
	for _, c := range typeCounts {
		totalStored += c
	}
	_ = client.IncrementCounterBy(ctx, "total_stores", totalStored)

	return NewResult(bulkFormatOutput(stored, typeCounts, totalStored, relMessages, errors)), nil
}

// bulkStoreNodes stores all items and returns their results, type counts, and any errors.
func bulkStoreNodes(ctx context.Context, client Querier, itemSlice []any) ([]bulkItem, map[string]int, []string) {
	stored := make([]bulkItem, len(itemSlice))
	var errors []string
	typeCounts := map[string]int{}

	for i, raw := range itemSlice {
		itemArgs, ok := raw.(map[string]any)
		if !ok {
			errors = append(errors, fmt.Sprintf("item[%d]: not a valid object", i))
			continue
		}
		nodeType := GetStringArg(itemArgs, "type", "")
		if nodeType == "" {
			errors = append(errors, fmt.Sprintf("item[%d]: missing required parameter: type", i))
			continue
		}

		nodeID, summary, err := storeNode(ctx, client, itemArgs, nodeType)
		if err != nil {
			errors = append(errors, fmt.Sprintf("item[%d] (%s): %v", i, nodeType, err))
			continue
		}
		if nodeID == "" {
			errors = append(errors, fmt.Sprintf("item[%d]: invalid type %q", i, nodeType))
			continue
		}

		stored[i] = bulkItem{nodeID: nodeID, nodeType: nodeType, summary: summary}
		typeCounts[nodeType]++
	}

	return stored, typeCounts, errors
}

// bulkProcessRelationships handles invalidations and relationships for stored items.
func bulkProcessRelationships(ctx context.Context, client Querier, itemSlice []any, stored []bulkItem) ([]string, []string) {
	var relMessages []string
	var errors []string

	for i, item := range stored {
		if item.nodeID == "" {
			continue
		}
		itemArgs, _ := itemSlice[i].(map[string]any)

		toolErr, invalidationMsg := handleInvalidation(ctx, client, itemArgs, item.nodeID)
		if toolErr != nil {
			errors = append(errors, fmt.Sprintf("item[%d] invalidation: %s", i, toolErr.Text))
		} else if invalidationMsg != "" {
			relMessages = append(relMessages, fmt.Sprintf("item[%d]%s", i, invalidationMsg))
		}

		if rels, ok := itemArgs["relationships"]; ok && rels != nil {
			resolved := resolveBatchRefs(rels, stored, &errors, i)
			if msg := storeRelationships(ctx, client, item.nodeID, resolved); msg != "" {
				relMessages = append(relMessages, fmt.Sprintf("item[%d]:\n%s", i, msg))
			}
		}
	}

	return relMessages, errors
}

// bulkFormatOutput builds the formatted output string for a bulk store operation.
func bulkFormatOutput(stored []bulkItem, typeCounts map[string]int, totalStored int, relMessages, errors []string) string {
	var sb strings.Builder

	var parts []string
	for _, nt := range []string{"fact", "decision", "entity", "event", "topic"} {
		if c := typeCounts[nt]; c > 0 {
			parts = append(parts, pluralizeNodeType(nt, c))
		}
	}
	sb.WriteString(fmt.Sprintf("Stored %d items: %s\n", totalStored, strings.Join(parts, ", ")))

	sb.WriteString("\nIDs:\n")
	for i, item := range stored {
		if item.nodeID != "" {
			sb.WriteString(fmt.Sprintf("  [%d] %s [%s]\n", i, item.nodeType, item.nodeID))
		}
	}

	if len(relMessages) > 0 {
		sb.WriteString("\nRelationships:\n")
		for _, msg := range relMessages {
			sb.WriteString(msg)
		}
	}

	if len(errors) > 0 {
		sb.WriteString(fmt.Sprintf("\nErrors (%d):\n", len(errors)))
		for _, e := range errors {
			sb.WriteString(fmt.Sprintf("  - %s\n", e))
		}
	}

	return sb.String()
}

// resolveBatchRefs replaces target_ref index references in relationships with actual IDs
// from previously stored items in the same batch. Errors for invalid refs are appended to errs.
func resolveBatchRefs(rels any, stored []bulkItem, errs *[]string, itemIdx int) []any {
	relSlice, ok := rels.([]any)
	if !ok {
		return nil
	}
	resolved := make([]any, 0, len(relSlice))
	for _, rel := range relSlice {
		relMap, ok := rel.(map[string]any)
		if !ok {
			continue
		}
		// Check for target_ref (cross-batch index reference).
		if refIdx, hasRef := relMap["target_ref"]; hasRef {
			idx := toInt(refIdx)
			if idx < 0 || idx >= len(stored) {
				*errs = append(*errs, fmt.Sprintf("item[%d]: target_ref %d is out of bounds (batch has %d items)", itemIdx, idx, len(stored)))
				continue
			}
			if stored[idx].nodeID == "" {
				*errs = append(*errs, fmt.Sprintf("item[%d]: target_ref %d references a failed item", itemIdx, idx))
				continue
			}
			// Validate edge/target type compatibility.
			edge := ""
			if e, ok := relMap["edge"].(string); ok {
				edge = e
			}
			if endpoints, ok := validEdgeEndpoints[edge]; ok {
				if !strings.HasPrefix(stored[idx].nodeID, endpoints[1]) {
					*errs = append(*errs, fmt.Sprintf("item[%d]: target_ref %d resolves to [%s] but edge %q requires target prefix %q", itemIdx, idx, stored[idx].nodeID, edge, endpoints[1]))
					continue
				}
			}
			// Copy the map and replace target_ref with the resolved target_id.
			resolved = append(resolved, map[string]any{
				"edge":      relMap["edge"],
				"target_id": stored[idx].nodeID,
				"role":      relMap["role"],
			})
		} else {
			resolved = append(resolved, relMap)
		}
	}
	return resolved
}

// pluralizeNodeType returns a count + pluralized node type string.
func pluralizeNodeType(nt string, count int) string {
	if count == 1 {
		return "1 " + nt
	}
	if nt == "entity" {
		return fmt.Sprintf("%d entities", count)
	}
	return fmt.Sprintf("%d %ss", count, nt)
}

// bulkPreValidate checks all items for required fields and valid enum values
// before any storage occurs. Returns a list of validation errors (empty = all valid).
func bulkPreValidate(items []any) []string {
	validTypes := map[string]bool{"fact": true, "decision": true, "entity": true, "event": true, "topic": true}
	var errors []string
	for i, raw := range items {
		itemArgs, ok := raw.(map[string]any)
		if !ok {
			errors = append(errors, fmt.Sprintf("item[%d]: not a valid object", i))
			continue
		}
		nodeType := GetStringArg(itemArgs, "type", "")
		if nodeType == "" {
			errors = append(errors, fmt.Sprintf("item[%d]: missing required parameter: type", i))
			continue
		}
		if !validTypes[nodeType] {
			errors = append(errors, fmt.Sprintf("item[%d]: invalid type %q", i, nodeType))
			continue
		}

		switch nodeType {
		case "fact":
			content := GetStringArg(itemArgs, "content", "")
			if content == "" {
				errors = append(errors, fmt.Sprintf("item[%d] (fact): content is required", i))
			} else if len(content) > maxContentLength {
				errors = append(errors, fmt.Sprintf("item[%d] (fact): content exceeds maximum length", i))
			}
			category := GetStringArg(itemArgs, "category", "general")
			if !validFactCategories[category] {
				errors = append(errors, fmt.Sprintf("item[%d] (fact): invalid category %q", i, category))
			}
			confidence := GetFloat64Arg(itemArgs, "confidence", 0.8)
			if confidence < 0 || confidence > 1.0 {
				errors = append(errors, fmt.Sprintf("item[%d] (fact): confidence must be between 0.0 and 1.0", i))
			}
		case "decision":
			if GetStringArg(itemArgs, "title", "") == "" {
				errors = append(errors, fmt.Sprintf("item[%d] (decision): title is required", i))
			}
			if GetStringArg(itemArgs, "rationale", "") == "" {
				errors = append(errors, fmt.Sprintf("item[%d] (decision): rationale is required", i))
			}
			alternatives := GetStringArg(itemArgs, "alternatives", "[]")
			var altJSON []any
			if err := json.Unmarshal([]byte(alternatives), &altJSON); err != nil {
				errors = append(errors, fmt.Sprintf("item[%d] (decision): alternatives must be a valid JSON array", i))
			}
		case "entity":
			if GetStringArg(itemArgs, "name", "") == "" {
				errors = append(errors, fmt.Sprintf("item[%d] (entity): name is required", i))
			}
			kind := GetStringArg(itemArgs, "kind", "")
			if kind == "" {
				errors = append(errors, fmt.Sprintf("item[%d] (entity): kind is required", i))
			} else if !validEntityKinds[kind] {
				errors = append(errors, fmt.Sprintf("item[%d] (entity): invalid kind %q", i, kind))
			}
		case "event":
			if GetStringArg(itemArgs, "title", "") == "" {
				errors = append(errors, fmt.Sprintf("item[%d] (event): title is required", i))
			}
			eventDate := GetStringArg(itemArgs, "event_date", "")
			if eventDate == "" {
				errors = append(errors, fmt.Sprintf("item[%d] (event): event_date is required", i))
			} else if _, err := time.Parse("2006-01-02", eventDate); err != nil {
				errors = append(errors, fmt.Sprintf("item[%d] (event): invalid event_date format %q", i, eventDate))
			}
		case "topic":
			if GetStringArg(itemArgs, "name", "") == "" {
				errors = append(errors, fmt.Sprintf("item[%d] (topic): name is required", i))
			}
		}
	}
	return errors
}

// toInt converts a JSON number to int. JSON numbers from map[string]any are float64.
func toInt(v any) int {
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	default:
		return -1
	}
}
