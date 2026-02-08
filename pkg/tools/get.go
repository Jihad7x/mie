// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"strings"
)

// Get retrieves a single node by its ID and returns its full details.
func Get(ctx context.Context, client Querier, args map[string]any) (*ToolResult, error) {
	nodeID := GetStringArg(args, "node_id", "")
	if nodeID == "" {
		return NewError("Missing required parameter: node_id"), nil
	}

	node, err := client.GetNodeByID(ctx, nodeID)
	if err != nil {
		return NewError(fmt.Sprintf("Node not found: %v", err)), nil
	}

	return NewResult(formatNode(nodeID, node)), nil
}

func formatNode(nodeID string, node any) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Node [%s]\n\n", nodeID))

	switch n := node.(type) {
	case *Fact:
		sb.WriteString("**Type:** fact\n")
		sb.WriteString(fmt.Sprintf("**Content:** %s\n", n.Content))
		sb.WriteString(fmt.Sprintf("**Category:** %s\n", n.Category))
		sb.WriteString(fmt.Sprintf("**Confidence:** %g\n", n.Confidence))
		sb.WriteString(fmt.Sprintf("**Valid:** %v\n", n.Valid))
		sb.WriteString(fmt.Sprintf("**Source:** %s\n", n.SourceAgent))
		if n.SourceConversation != "" {
			sb.WriteString(fmt.Sprintf("**Conversation:** %s\n", n.SourceConversation))
		}
		sb.WriteString(fmt.Sprintf("**Created:** %d\n", n.CreatedAt))
		sb.WriteString(fmt.Sprintf("**Updated:** %d\n", n.UpdatedAt))
	case *Decision:
		sb.WriteString("**Type:** decision\n")
		sb.WriteString(fmt.Sprintf("**Title:** %s\n", n.Title))
		sb.WriteString(fmt.Sprintf("**Rationale:** %s\n", n.Rationale))
		if n.Alternatives != "" {
			sb.WriteString(fmt.Sprintf("**Alternatives:** %s\n", n.Alternatives))
		}
		if n.Context != "" {
			sb.WriteString(fmt.Sprintf("**Context:** %s\n", n.Context))
		}
		sb.WriteString(fmt.Sprintf("**Status:** %s\n", n.Status))
		sb.WriteString(fmt.Sprintf("**Source:** %s\n", n.SourceAgent))
		sb.WriteString(fmt.Sprintf("**Created:** %d\n", n.CreatedAt))
		sb.WriteString(fmt.Sprintf("**Updated:** %d\n", n.UpdatedAt))
	case *Entity:
		sb.WriteString("**Type:** entity\n")
		sb.WriteString(fmt.Sprintf("**Name:** %s\n", n.Name))
		sb.WriteString(fmt.Sprintf("**Kind:** %s\n", n.Kind))
		if n.Description != "" {
			sb.WriteString(fmt.Sprintf("**Description:** %s\n", n.Description))
		}
		sb.WriteString(fmt.Sprintf("**Source:** %s\n", n.SourceAgent))
		sb.WriteString(fmt.Sprintf("**Created:** %d\n", n.CreatedAt))
		sb.WriteString(fmt.Sprintf("**Updated:** %d\n", n.UpdatedAt))
	case *Event:
		sb.WriteString("**Type:** event\n")
		sb.WriteString(fmt.Sprintf("**Title:** %s\n", n.Title))
		if n.Description != "" {
			sb.WriteString(fmt.Sprintf("**Description:** %s\n", n.Description))
		}
		sb.WriteString(fmt.Sprintf("**Date:** %s\n", n.EventDate))
		sb.WriteString(fmt.Sprintf("**Source:** %s\n", n.SourceAgent))
		sb.WriteString(fmt.Sprintf("**Created:** %d\n", n.CreatedAt))
		sb.WriteString(fmt.Sprintf("**Updated:** %d\n", n.UpdatedAt))
	case *Topic:
		sb.WriteString("**Type:** topic\n")
		sb.WriteString(fmt.Sprintf("**Name:** %s\n", n.Name))
		if n.Description != "" {
			sb.WriteString(fmt.Sprintf("**Description:** %s\n", n.Description))
		}
		sb.WriteString(fmt.Sprintf("**Created:** %d\n", n.CreatedAt))
		sb.WriteString(fmt.Sprintf("**Updated:** %d\n", n.UpdatedAt))
	default:
		sb.WriteString(fmt.Sprintf("%v\n", node))
	}

	return sb.String()
}