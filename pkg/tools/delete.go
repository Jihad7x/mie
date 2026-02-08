// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
)

// Delete removes a memory node or relationship from the graph.
func Delete(ctx context.Context, client Querier, args map[string]any) (*ToolResult, error) {
	action := GetStringArg(args, "action", "")
	if action == "" {
		return NewError("Missing required parameter: action"), nil
	}

	switch action {
	case "delete_node":
		return deleteNode(ctx, client, args)
	case "remove_relationship":
		return removeRelationship(ctx, client, args)
	default:
		return NewError(fmt.Sprintf("Invalid action %q. Must be one of: delete_node, remove_relationship", action)), nil
	}
}

func deleteNode(ctx context.Context, client Querier, args map[string]any) (*ToolResult, error) {
	nodeID := GetStringArg(args, "node_id", "")
	if nodeID == "" {
		return NewError("Missing required parameter: node_id"), nil
	}

	if err := client.DeleteNode(ctx, nodeID); err != nil {
		return NewError(fmt.Sprintf("Failed to delete node: %v", err)), nil
	}

	return NewResult(fmt.Sprintf("Deleted node `%s` and all associated edges.", nodeID)), nil
}

func removeRelationship(ctx context.Context, client Querier, args map[string]any) (*ToolResult, error) {
	edgeType := GetStringArg(args, "edge_type", "")
	if edgeType == "" {
		return NewError("Missing required parameter: edge_type"), nil
	}

	// Collect edge fields from the args.
	fields := make(map[string]string)
	for _, key := range []string{"fact_id", "entity_id", "topic_id", "decision_id", "event_id", "old_fact_id", "new_fact_id", "role"} {
		if v := GetStringArg(args, key, ""); v != "" {
			fields[key] = v
		}
	}

	if len(fields) == 0 {
		return NewError("No edge fields provided. Specify the node IDs that form the edge (e.g., fact_id, entity_id)."), nil
	}

	if err := client.RemoveRelationship(ctx, edgeType, fields); err != nil {
		return NewError(fmt.Sprintf("Failed to remove relationship: %v", err)), nil
	}

	return NewResult(fmt.Sprintf("Removed `%s` relationship.", edgeType)), nil
}