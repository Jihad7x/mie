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

// validFactCategories enumerates allowed fact categories.
var validFactCategories = map[string]bool{
	"personal": true, "professional": true, "preference": true,
	"technical": true, "relationship": true, "general": true,
}

// validEntityKinds enumerates allowed entity kinds.
var validEntityKinds = map[string]bool{
	"person": true, "company": true, "project": true, "product": true,
	"technology": true, "place": true, "other": true,
}

const maxContentLength = 10000 // 10KB max for any text field

// validEdgeTypes enumerates allowed relationship edge types.
var validEdgeTypes = map[string]bool{
	"fact_entity": true, "fact_topic": true, "decision_topic": true,
	"decision_entity": true, "event_decision": true, "entity_topic": true,
}

// validEdgeEndpoints maps edge types to their required source and target node ID prefixes.
var validEdgeEndpoints = map[string][2]string{
	"fact_entity":     {"fact:", "ent:"},
	"fact_topic":      {"fact:", "top:"},
	"decision_topic":  {"dec:", "top:"},
	"decision_entity": {"dec:", "ent:"},
	"event_decision":  {"evt:", "dec:"},
	"entity_topic":    {"ent:", "top:"},
}

// Store writes a new node and optional relationships to the memory graph.
func Store(ctx context.Context, client Querier, args map[string]any) (*ToolResult, error) {
	nodeType := GetStringArg(args, "type", "")
	if nodeType == "" {
		return NewError("Missing required parameter: type"), nil
	}

	nodeID, summary, err := storeNode(ctx, client, args, nodeType)
	if err != nil {
		return NewError(fmt.Sprintf("Failed to store %s: %v", nodeType, err)), nil
	}
	if nodeID == "" {
		return NewError(fmt.Sprintf("Invalid type %q. Must be one of: fact, decision, entity, event, topic", nodeType)), nil
	}

	// Handle invalidation
	toolErr, invalidationMsg := handleInvalidation(ctx, client, args, nodeID)
	if toolErr != nil {
		return toolErr, nil
	}

	// Handle relationships
	var relMsg string
	if rels, ok := args["relationships"]; ok && rels != nil {
		relMsg = storeRelationships(ctx, client, nodeID, rels)
	}

	// Increment usage counter (never fail the main operation).
	_ = client.IncrementCounter(ctx, "total_stores")

	output := fmt.Sprintf("Stored %s [%s]\n%s", nodeType, nodeID, summary)
	if relMsg != "" {
		output += "\n\nRelationships created:\n" + relMsg
	}
	if invalidationMsg != "" {
		output += "\n" + invalidationMsg
	}

	return NewResult(output), nil
}

func storeNode(ctx context.Context, client Querier, args map[string]any, nodeType string) (string, string, error) {
	sourceAgent := GetStringArg(args, "source_agent", "unknown")
	sourceConversation := GetStringArg(args, "source_conversation", "")

	switch nodeType {
	case "fact":
		result, err := storeFact(ctx, client, args, sourceAgent, sourceConversation)
		if err != nil {
			return "", "", err
		}
		return result.ID, fmt.Sprintf("Content: %q\nCategory: %s | Confidence: %g | Source: %s",
			Truncate(result.Content, 100), result.Category, result.Confidence, result.SourceAgent), nil

	case "decision":
		result, err := storeDecision(ctx, client, args, sourceAgent, sourceConversation)
		if err != nil {
			return "", "", err
		}
		return result.ID, fmt.Sprintf("Title: %q\nRationale: %s\nStatus: %s | Source: %s",
			Truncate(result.Title, 100), Truncate(result.Rationale, 100), result.Status, result.SourceAgent), nil

	case "entity":
		result, err := storeEntity(ctx, client, args, sourceAgent)
		if err != nil {
			return "", "", err
		}
		summary := fmt.Sprintf("Name: %q\nKind: %s | Source: %s",
			result.Name, result.Kind, result.SourceAgent)
		if result.Description != "" {
			summary += fmt.Sprintf("\nDescription: %s", Truncate(result.Description, 100))
		}
		return result.ID, summary, nil

	case "event":
		result, err := storeEvent(ctx, client, args, sourceAgent, sourceConversation)
		if err != nil {
			return "", "", err
		}
		return result.ID, fmt.Sprintf("Title: %q\nDate: %s | Source: %s",
			Truncate(result.Title, 100), result.EventDate, result.SourceAgent), nil

	case "topic":
		result, err := storeTopic(ctx, client, args)
		if err != nil {
			return "", "", err
		}
		summary := fmt.Sprintf("Name: %q", result.Name)
		if result.Description != "" {
			summary += fmt.Sprintf("\nDescription: %s", Truncate(result.Description, 100))
		}
		return result.ID, summary, nil

	default:
		return "", "", nil
	}
}

func handleInvalidation(ctx context.Context, client Querier, args map[string]any, nodeID string) (*ToolResult, string) {
	invalidates := GetStringArg(args, "invalidates", "")
	if invalidates == "" {
		return nil, ""
	}
	if !strings.HasPrefix(invalidates, "fact:") {
		return NewError(fmt.Sprintf("invalidates must reference a fact ID (got %q)", invalidates)), ""
	}
	reason := fmt.Sprintf("Replaced by %s", nodeID)
	if err := client.InvalidateFact(ctx, invalidates, nodeID, reason); err != nil {
		return NewError(fmt.Sprintf("Failed to invalidate fact %s: %v", invalidates, err)), ""
	}
	return nil, fmt.Sprintf("\nInvalidated: [%s]\nReason: %s", invalidates, reason)
}

func storeFact(ctx context.Context, client Querier, args map[string]any, sourceAgent, sourceConversation string) (*Fact, error) {
	content := GetStringArg(args, "content", "")
	if content == "" {
		return nil, fmt.Errorf("content is required for fact type")
	}
	if len(content) > maxContentLength {
		return nil, fmt.Errorf("content exceeds maximum length (%d > %d bytes)", len(content), maxContentLength)
	}
	category := GetStringArg(args, "category", "general")
	if !validFactCategories[category] {
		return nil, fmt.Errorf("invalid category %q. Must be one of: personal, professional, preference, technical, relationship, general", category)
	}
	confidence := GetFloat64Arg(args, "confidence", 0.8)
	if confidence < 0 || confidence > 1.0 {
		return nil, fmt.Errorf("confidence must be between 0.0 and 1.0 (got %g)", confidence)
	}
	return client.StoreFact(ctx, StoreFactRequest{
		Content:            content,
		Category:           category,
		Confidence:         confidence,
		SourceAgent:        sourceAgent,
		SourceConversation: sourceConversation,
	})
}

func storeDecision(ctx context.Context, client Querier, args map[string]any, sourceAgent, sourceConversation string) (*Decision, error) {
	title := GetStringArg(args, "title", "")
	if title == "" {
		return nil, fmt.Errorf("title is required for decision type")
	}
	if len(title) > maxContentLength {
		return nil, fmt.Errorf("title exceeds maximum length (%d > %d bytes)", len(title), maxContentLength)
	}
	rationale := GetStringArg(args, "rationale", "")
	if rationale == "" {
		return nil, fmt.Errorf("rationale is required for decision type")
	}
	if len(rationale) > maxContentLength {
		return nil, fmt.Errorf("rationale exceeds maximum length (%d > %d bytes)", len(rationale), maxContentLength)
	}
	return client.StoreDecision(ctx, StoreDecisionRequest{
		Title:              title,
		Rationale:          rationale,
		Alternatives:       GetStringArg(args, "alternatives", "[]"),
		Context:            GetStringArg(args, "context", ""),
		SourceAgent:        sourceAgent,
		SourceConversation: sourceConversation,
	})
}

func storeEntity(ctx context.Context, client Querier, args map[string]any, sourceAgent string) (*Entity, error) {
	name := GetStringArg(args, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name is required for entity type")
	}
	if len(name) > maxContentLength {
		return nil, fmt.Errorf("name exceeds maximum length (%d > %d bytes)", len(name), maxContentLength)
	}
	description := GetStringArg(args, "description", "")
	if len(description) > maxContentLength {
		return nil, fmt.Errorf("description exceeds maximum length (%d > %d bytes)", len(description), maxContentLength)
	}
	kind := GetStringArg(args, "kind", "")
	if kind == "" {
		return nil, fmt.Errorf("kind is required for entity type")
	}
	if !validEntityKinds[kind] {
		return nil, fmt.Errorf("invalid entity kind %q. Must be one of: person, company, project, product, technology, place, other", kind)
	}
	return client.StoreEntity(ctx, StoreEntityRequest{
		Name:        name,
		Kind:        kind,
		Description: description,
		SourceAgent: sourceAgent,
	})
}

func storeEvent(ctx context.Context, client Querier, args map[string]any, sourceAgent, sourceConversation string) (*Event, error) {
	title := GetStringArg(args, "title", "")
	if title == "" {
		return nil, fmt.Errorf("title is required for event type")
	}
	if len(title) > maxContentLength {
		return nil, fmt.Errorf("title exceeds maximum length (%d > %d bytes)", len(title), maxContentLength)
	}
	eventDescription := GetStringArg(args, "description", "")
	if len(eventDescription) > maxContentLength {
		return nil, fmt.Errorf("description exceeds maximum length (%d > %d bytes)", len(eventDescription), maxContentLength)
	}
	eventDate := GetStringArg(args, "event_date", "")
	if eventDate == "" {
		return nil, fmt.Errorf("event_date is required for event type")
	}
	// Validate date format (ISO 8601 date).
	if _, err := time.Parse("2006-01-02", eventDate); err != nil {
		return nil, fmt.Errorf("invalid event_date format %q: expected ISO date (YYYY-MM-DD)", eventDate)
	}
	return client.StoreEvent(ctx, StoreEventRequest{
		Title:              title,
		Description:        eventDescription,
		EventDate:          eventDate,
		SourceAgent:        sourceAgent,
		SourceConversation: sourceConversation,
	})
}

func storeTopic(ctx context.Context, client Querier, args map[string]any) (*Topic, error) {
	name := GetStringArg(args, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name is required for topic type")
	}
	return client.StoreTopic(ctx, StoreTopicRequest{
		Name:        strings.ToLower(name),
		Description: GetStringArg(args, "description", ""),
	})
}

func storeRelationships(ctx context.Context, client Querier, sourceNodeID string, rels any) string {
	relSlice, ok := rels.([]any)
	if !ok {
		return ""
	}
	var sb strings.Builder
	for _, rel := range relSlice {
		relMap, ok := rel.(map[string]any)
		if !ok {
			continue
		}
		edgeType := GetStringArg(relMap, "edge", "")
		targetID := GetStringArg(relMap, "target_id", "")
		if edgeType == "" || targetID == "" {
			if edgeType == "" {
				sb.WriteString("- Skipped relationship: missing edge type\n")
			}
			if targetID == "" {
				sb.WriteString(fmt.Sprintf("- Skipped %s: missing target_id\n", edgeType))
			}
			continue
		}
		if !validEdgeTypes[edgeType] {
			sb.WriteString(fmt.Sprintf("- Skipped invalid edge type: %s\n", edgeType))
			continue
		}

		// Validate target node exists before creating edge.
		if target, err := client.GetNodeByID(ctx, targetID); err != nil || target == nil {
			sb.WriteString(fmt.Sprintf("- Skipped %s -> [%s]: target node not found\n", edgeType, targetID))
			continue
		}

		// Validate source and target node types match the edge semantics.
		if endpoints, ok := validEdgeEndpoints[edgeType]; ok {
			if !strings.HasPrefix(sourceNodeID, endpoints[0]) {
				sb.WriteString(fmt.Sprintf("- Skipped %s: source node [%s] must be a %s node\n", edgeType, sourceNodeID, strings.TrimSuffix(endpoints[0], ":")))
				continue
			}
			if !strings.HasPrefix(targetID, endpoints[1]) {
				sb.WriteString(fmt.Sprintf("- Skipped %s: target node [%s] must be a %s node\n", edgeType, targetID, strings.TrimSuffix(endpoints[1], ":")))
				continue
			}
		}

		fields := buildEdgeFields(edgeType, sourceNodeID, targetID, relMap)
		tableName := "mie_" + edgeType
		if err := client.AddRelationship(ctx, tableName, fields); err != nil {
			sb.WriteString(fmt.Sprintf("- Failed %s -> [%s]: %v\n", edgeType, targetID, err))
		} else {
			sb.WriteString(fmt.Sprintf("- %s -> [%s]\n", edgeType, targetID))
		}
	}
	return sb.String()
}

func buildEdgeFields(edgeType, sourceNodeID, targetID string, relMap map[string]any) map[string]string {
	fields := map[string]string{}
	switch edgeType {
	case "fact_entity":
		fields["fact_id"] = sourceNodeID
		fields["entity_id"] = targetID
	case "fact_topic":
		fields["fact_id"] = sourceNodeID
		fields["topic_id"] = targetID
	case "decision_topic":
		fields["decision_id"] = sourceNodeID
		fields["topic_id"] = targetID
	case "decision_entity":
		fields["decision_id"] = sourceNodeID
		fields["entity_id"] = targetID
		fields["role"] = GetStringArg(relMap, "role", "")
	case "event_decision":
		fields["event_id"] = sourceNodeID
		fields["decision_id"] = targetID
	case "entity_topic":
		fields["entity_id"] = sourceNodeID
		fields["topic_id"] = targetID
	}
	return fields
}
