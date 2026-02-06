// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"strings"
)

// validDecisionStatuses enumerates allowed decision status transitions.
var validDecisionStatuses = map[string]bool{
	"active": true, "superseded": true, "reversed": true,
}

// Update modifies existing nodes or invalidates facts.
func Update(ctx context.Context, client Querier, args map[string]any) (*ToolResult, error) {
	nodeID := GetStringArg(args, "node_id", "")
	if nodeID == "" {
		return NewError("Missing required parameter: node_id"), nil
	}

	action := GetStringArg(args, "action", "")
	if action == "" {
		return NewError("Missing required parameter: action"), nil
	}

	switch action {
	case "invalidate":
		return updateInvalidate(ctx, client, nodeID, args)
	case "update_description":
		return updateDescription(ctx, client, nodeID, args)
	case "update_status":
		return updateStatus(ctx, client, nodeID, args)
	default:
		return NewError(fmt.Sprintf("Invalid action %q. Must be one of: invalidate, update_description, update_status", action)), nil
	}
}

func updateInvalidate(ctx context.Context, client Querier, nodeID string, args map[string]any) (*ToolResult, error) {
	if !strings.HasPrefix(nodeID, "fact:") {
		return NewError(fmt.Sprintf("invalidate action requires a fact ID (prefix 'fact:'), got %q", nodeID)), nil
	}

	reason := GetStringArg(args, "reason", "")
	if reason == "" {
		return NewError("reason is required for invalidate action"), nil
	}

	replacementID := GetStringArg(args, "replacement_id", "")
	if replacementID != "" && !strings.HasPrefix(replacementID, "fact:") {
		return NewError(fmt.Sprintf("replacement_id must be a fact ID (prefix 'fact:'), got %q", replacementID)), nil
	}

	err := client.InvalidateFact(ctx, nodeID, replacementID, reason)
	if err != nil {
		return NewError(fmt.Sprintf("Failed to invalidate fact: %v", err)), nil
	}

	output := fmt.Sprintf("Invalidated [%s]\nReason: %s", nodeID, reason)
	if replacementID != "" {
		output += fmt.Sprintf("\nReplaced by: [%s]", replacementID)
	}

	return NewResult(output), nil
}

func updateDescription(ctx context.Context, client Querier, nodeID string, args map[string]any) (*ToolResult, error) {
	newValue := GetStringArg(args, "new_value", "")
	if newValue == "" {
		return NewError("new_value is required for update_description action"), nil
	}

	err := client.UpdateDescription(ctx, nodeID, newValue)
	if err != nil {
		return NewError(fmt.Sprintf("Failed to update description: %v", err)), nil
	}

	return NewResult(fmt.Sprintf("Updated description for [%s]\nNew description: %s", nodeID, Truncate(newValue, 200))), nil
}

func updateStatus(ctx context.Context, client Querier, nodeID string, args map[string]any) (*ToolResult, error) {
	if !strings.HasPrefix(nodeID, "dec:") {
		return NewError(fmt.Sprintf("update_status action requires a decision ID (prefix 'dec:'), got %q", nodeID)), nil
	}

	newValue := GetStringArg(args, "new_value", "")
	if newValue == "" {
		return NewError("new_value is required for update_status action"), nil
	}

	if !validDecisionStatuses[newValue] {
		return NewError(fmt.Sprintf("Invalid status %q. Must be one of: active, superseded, reversed", newValue)), nil
	}

	err := client.UpdateStatus(ctx, nodeID, newValue)
	if err != nil {
		return NewError(fmt.Sprintf("Failed to update status: %v", err)), nil
	}

	return NewResult(fmt.Sprintf("Updated status for [%s]\nNew status: %s", nodeID, newValue)), nil
}