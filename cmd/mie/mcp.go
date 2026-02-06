//go:build cozodb

// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/kraklabs/mie/pkg/memory"
	"github.com/kraklabs/mie/pkg/tools"
)

const (
	mcpVersion    = "0.1.0"
	mcpServerName = "mie"
)

// JSON-RPC 2.0 types for MCP protocol.

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type mcpServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type mcpCapabilities struct {
	Tools map[string]any `json:"tools,omitempty"`
}

type mcpInitializeResult struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    mcpCapabilities `json:"capabilities"`
	ServerInfo      mcpServerInfo   `json:"serverInfo"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type mcpToolsListResult struct {
	Tools []mcpTool `json:"tools"`
}

type mcpToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type mcpToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// mcpServer maintains state for the running MCP server instance.
type mcpServer struct {
	client tools.Querier
	config *Config
}

// toolHandler is the signature for MCP tool handlers.
type toolHandler func(ctx context.Context, s *mcpServer, args map[string]any) (*tools.ToolResult, error)

// toolHandlers maps tool names to their handler functions.
var toolHandlers = map[string]toolHandler{
	"mie_analyze":   handleAnalyze,
	"mie_store":     handleStore,
	"mie_query":     handleQuery,
	"mie_update":    handleUpdate,
	"mie_list":      handleList,
	"mie_conflicts": handleConflicts,
	"mie_export":    handleExport,
	"mie_status":    handleMIEStatus,
}

// runMCPServer starts the MIE MCP server on stdin/stdout.
func runMCPServer(configPath string) {
	var cfg *Config
	var err error

	cfg, err = LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		fmt.Fprintf(os.Stderr, "Using default configuration with environment variable overrides\n")
		cfg = DefaultConfig()
		cfg.applyEnvOverrides()
	}

	if cfg.Storage.Engine == "sqlite" {
		fmt.Fprintf(os.Stderr, "Warning: sqlite engine may not be available in pre-built binaries; consider using \"rocksdb\"\n")
	}

	// Resolve storage path
	dataDir, err := ResolveDataDir(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitConfig)
	}

	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot create data directory %s: %v\n", dataDir, err)
		os.Exit(ExitDatabase)
	}

	// Create the memory client (implements tools.Querier)
	// This opens CozoDB, ensures schema, and sets up embeddings.
	client, err := memory.NewClient(memory.ClientConfig{
		DataDir:            dataDir,
		StorageEngine:      cfg.Storage.Engine,
		EmbeddingEnabled:   cfg.Embedding.Enabled,
		EmbeddingProvider:  cfg.Embedding.Provider,
		EmbeddingBaseURL:   cfg.Embedding.BaseURL,
		EmbeddingModel:     cfg.Embedding.Model,
		EmbeddingAPIKey:    cfg.Embedding.APIKey,
		EmbeddingDimensions: cfg.Embedding.Dimensions,
		EmbeddingWorkers:   cfg.Embedding.Workers,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot initialize MIE: %v\n", err)
		os.Exit(ExitDatabase)
	}
	defer func() { _ = client.Close() }()

	server := &mcpServer{
		client: client,
		config: cfg,
	}

	fmt.Fprintf(os.Stderr, "MIE MCP Server v%s starting...\n", mcpVersion)
	fmt.Fprintf(os.Stderr, "  Storage: %s (%s)\n", cfg.Storage.Engine, dataDir)
	if cfg.Embedding.Enabled {
		fmt.Fprintf(os.Stderr, "  Embeddings: %s (%s, %dd)\n", cfg.Embedding.Provider, cfg.Embedding.Model, cfg.Embedding.Dimensions)
	}

	if err := server.serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Error: stdin read error: %v\n", err)
		os.Exit(ExitGeneral)
	}
}

// serve runs the JSON-RPC read loop, reading requests from r and writing responses to w.
func (s *mcpServer) serve(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: invalid JSON-RPC request: %v\n", err)
			continue
		}

		fmt.Fprintf(os.Stderr, "-> %s\n", req.Method)

		ctx := context.Background()
		resp := s.handleRequest(ctx, req)

		if resp.ID == nil && resp.Result == nil && resp.Error == nil {
			continue
		}

		respBytes, err := json.Marshal(resp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot encode response: %v\n", err)
			continue
		}

		_, _ = fmt.Fprintf(w, "%s\n", respBytes)

		fmt.Fprintf(os.Stderr, "<- response sent for %s\n", req.Method)
	}

	return scanner.Err()
}

// handleRequest dispatches a JSON-RPC request to the appropriate handler.
func (s *mcpServer) handleRequest(ctx context.Context, req jsonRPCRequest) jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: mcpInitializeResult{
				ProtocolVersion: "2024-11-05",
				Capabilities: mcpCapabilities{
					Tools: map[string]any{"listChanged": true},
				},
				ServerInfo: mcpServerInfo{
					Name:    mcpServerName,
					Version: mcpVersion,
				},
			},
		}

	case "notifications/initialized":
		return jsonRPCResponse{}

	case "tools/list":
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: mcpToolsListResult{
				Tools: s.getTools(),
			},
		}

	case "tools/call":
		var params mcpToolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &rpcError{
					Code:    -32602,
					Message: "Invalid params",
					Data:    err.Error(),
				},
			}
		}

		result, err := s.handleToolCall(ctx, params)
		if err != nil {
			return jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &rpcError{
					Code:    -32603,
					Message: "Internal error",
					Data:    err.Error(),
				},
			}
		}

		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  result,
		}

	default:
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &rpcError{
				Code:    -32601,
				Message: "Method not found",
				Data:    req.Method,
			},
		}
	}
}

// handleToolCall dispatches a tool call to the registered handler.
func (s *mcpServer) handleToolCall(ctx context.Context, params mcpToolCallParams) (*mcpToolResult, error) {
	handler, ok := toolHandlers[params.Name]
	if !ok {
		return &mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: fmt.Sprintf("Unknown tool: %s", params.Name)}},
			IsError: true,
		}, nil
	}

	result, err := handler(ctx, s, params.Arguments)
	if err != nil {
		return &mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: fmt.Sprintf("Error in %s: %v", params.Name, err)}},
			IsError: true,
		}, nil
	}

	return &mcpToolResult{
		Content: []mcpContent{{Type: "text", Text: result.Text}},
		IsError: result.IsError,
	}, nil
}

// getTools returns the list of all MIE MCP tool definitions.
func (s *mcpServer) getTools() []mcpTool {
	return []mcpTool{
		{
			Name:        "mie_analyze",
			Description: "Analyze a conversation fragment for potential memory storage. Returns related existing memory and an evaluation guide for the agent to decide what to persist. Call this at the end of meaningful conversations or when noticing something worth remembering.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{
						"type":        "string",
						"description": "Conversation fragment or information to analyze for potential memory storage",
					},
					"content_type": map[string]any{
						"type":        "string",
						"enum":        []string{"conversation", "statement", "decision", "event"},
						"description": "Type of content being analyzed. Helps focus the search.",
						"default":     "conversation",
					},
				},
				"required": []string{"content"},
			},
		},
		{
			Name:        "mie_store",
			Description: "Store a new memory node (fact, decision, entity, event, or topic) in the memory graph. Use after mie_analyze confirms something is worth persisting.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type": map[string]any{
						"type":        "string",
						"enum":        []string{"fact", "decision", "entity", "event", "topic"},
						"description": "Type of memory node to store",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Fact content text (required for type=fact)",
					},
					"category": map[string]any{
						"type":        "string",
						"enum":        []string{"personal", "professional", "preference", "technical", "relationship", "general"},
						"description": "Fact category",
						"default":     "general",
					},
					"confidence": map[string]any{
						"type":        "number",
						"minimum":     0,
						"maximum":     1,
						"description": "Confidence level (0.0-1.0)",
						"default":     0.8,
					},
					"title": map[string]any{
						"type":        "string",
						"description": "Decision or event title (required for type=decision, type=event)",
					},
					"rationale": map[string]any{
						"type":        "string",
						"description": "Decision rationale (required for type=decision)",
					},
					"alternatives": map[string]any{
						"type":        "string",
						"description": "JSON array of alternatives considered (for decisions)",
					},
					"context": map[string]any{
						"type":        "string",
						"description": "Decision context",
					},
					"name": map[string]any{
						"type":        "string",
						"description": "Entity or topic name (required for type=entity, type=topic)",
					},
					"kind": map[string]any{
						"type":        "string",
						"enum":        []string{"person", "company", "project", "product", "technology", "place", "other"},
						"description": "Entity kind (required for type=entity)",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "Description for entity, event, or topic",
					},
					"event_date": map[string]any{
						"type":        "string",
						"description": "Event date in ISO format (e.g., 2026-02-05). Required for type=event.",
					},
					"source_agent": map[string]any{
						"type":        "string",
						"description": "Agent identifier (e.g., 'claude', 'cursor')",
						"default":     "unknown",
					},
					"source_conversation": map[string]any{
						"type":        "string",
						"description": "Conversation reference or identifier",
					},
					"relationships": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"edge": map[string]any{
									"type":        "string",
									"enum":        []string{"fact_entity", "fact_topic", "decision_topic", "decision_entity", "event_decision", "entity_topic"},
									"description": "Relationship type",
								},
								"target_id": map[string]any{
									"type":        "string",
									"description": "Target node ID",
								},
								"role": map[string]any{
									"type":        "string",
									"description": "Role description (for decision_entity edges)",
								},
							},
							"required": []string{"edge", "target_id"},
						},
						"description": "Relationships to create after storing",
					},
					"invalidates": map[string]any{
						"type":        "string",
						"description": "ID of a fact to invalidate (marks it as invalid and creates invalidation edge)",
					},
				},
				"required": []string{"type"},
			},
		},
		{
			Name:        "mie_query",
			Description: "Search the memory graph. Supports three modes: 'semantic' (natural language similarity search), 'exact' (substring match), and 'graph' (traverse relationships from a node).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Search query. Natural language for semantic mode, exact text for exact mode, or node ID for graph mode.",
					},
					"mode": map[string]any{
						"type":        "string",
						"enum":        []string{"semantic", "exact", "graph"},
						"description": "Search mode",
						"default":     "semantic",
					},
					"node_types": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string", "enum": []string{"fact", "decision", "entity", "event"}},
						"description": "Node types to search (default: all)",
					},
					"limit": map[string]any{
						"type":    "number",
						"minimum": 1,
						"maximum": 50,
						"default": 10,
					},
					"category": map[string]any{
						"type":        "string",
						"description": "Filter facts by category",
					},
					"kind": map[string]any{
						"type":        "string",
						"description": "Filter entities by kind",
					},
					"valid_only": map[string]any{
						"type":    "boolean",
						"default": true,
					},
					"node_id": map[string]any{
						"type":        "string",
						"description": "Node ID for graph traversal mode",
					},
					"traversal": map[string]any{
						"type":        "string",
						"enum":        []string{"related_entities", "related_facts", "invalidation_chain", "decision_entities", "facts_about_entity", "entity_decisions"},
						"description": "Traversal type for graph mode",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "mie_update",
			Description: "Update or invalidate existing memory nodes. For facts, invalidation creates a chain (old fact marked invalid, linked to new). For entities, update description. For decisions, change status.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"node_id": map[string]any{
						"type":        "string",
						"description": "ID of the node to modify",
					},
					"action": map[string]any{
						"type":        "string",
						"enum":        []string{"invalidate", "update_description", "update_status"},
						"description": "Action: invalidate a fact, update an entity description, or change a decision status",
					},
					"reason": map[string]any{
						"type":        "string",
						"description": "Why this change is being made (required for invalidation)",
					},
					"replacement_id": map[string]any{
						"type":        "string",
						"description": "ID of the new fact that replaces the invalidated one",
					},
					"new_value": map[string]any{
						"type":        "string",
						"description": "New value for update_description or update_status actions",
					},
				},
				"required": []string{"node_id", "action"},
			},
		},
		{
			Name:        "mie_list",
			Description: "List memory nodes with filtering, pagination, and sorting. Returns a formatted table of results.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"node_type": map[string]any{
						"type":        "string",
						"enum":        []string{"fact", "decision", "entity", "event", "topic"},
						"description": "Type of memory nodes to list",
					},
					"category": map[string]any{
						"type":        "string",
						"description": "Filter facts by category",
					},
					"kind": map[string]any{
						"type":        "string",
						"description": "Filter entities by kind",
					},
					"status": map[string]any{
						"type":        "string",
						"description": "Filter decisions by status (active, superseded, reversed)",
					},
					"topic": map[string]any{
						"type":        "string",
						"description": "Filter by topic name",
					},
					"valid_only": map[string]any{
						"type":    "boolean",
						"default": true,
					},
					"limit": map[string]any{
						"type":    "number",
						"minimum": 1,
						"maximum": 100,
						"default": 20,
					},
					"offset": map[string]any{
						"type":    "number",
						"minimum": 0,
						"default": 0,
					},
					"sort_by": map[string]any{
						"type":        "string",
						"description": "Sort field (created_at, updated_at, name)",
						"default":     "created_at",
					},
					"sort_order": map[string]any{
						"type":        "string",
						"enum":        []string{"asc", "desc"},
						"default":     "desc",
					},
				},
				"required": []string{"node_type"},
			},
		},
		{
			Name:        "mie_conflicts",
			Description: "Detect potentially contradicting facts in the memory graph. Returns pairs of facts that are semantically similar but may contain conflicting information. Use this to maintain memory consistency.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"category": map[string]any{
						"type":        "string",
						"description": "Limit conflict scan to a specific category",
					},
					"threshold": map[string]any{
						"type":        "number",
						"minimum":     0,
						"maximum":     1,
						"description": "Similarity threshold (0.0-1.0). Higher = stricter matching.",
						"default":     0.85,
					},
					"limit": map[string]any{
						"type":    "number",
						"minimum": 1,
						"maximum": 50,
						"default": 10,
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        "mie_export",
			Description: "Export the complete memory graph for backup or migration. Returns all nodes and relationships in structured format.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"format": map[string]any{
						"type":        "string",
						"enum":        []string{"json", "datalog"},
						"description": "Export format",
						"default":     "json",
					},
					"include_embeddings": map[string]any{
						"type":        "boolean",
						"description": "Include embedding vectors (can be very large)",
						"default":     false,
					},
					"node_types": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string", "enum": []string{"fact", "decision", "entity", "event", "topic"}},
						"description": "Types to export (default: all)",
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        "mie_status",
			Description: "Display memory graph health and statistics. Shows counts of all node types, configuration details, and health checks.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []string{},
			},
		},
	}
}

// Tool handler implementations — each delegates to the corresponding pkg/tools function
// passing the Querier client and the raw arguments map.

func handleAnalyze(ctx context.Context, s *mcpServer, args map[string]any) (*tools.ToolResult, error) {
	return tools.Analyze(ctx, s.client, args)
}

func handleStore(ctx context.Context, s *mcpServer, args map[string]any) (*tools.ToolResult, error) {
	return tools.Store(ctx, s.client, args)
}

func handleQuery(ctx context.Context, s *mcpServer, args map[string]any) (*tools.ToolResult, error) {
	return tools.Query(ctx, s.client, args)
}

func handleUpdate(ctx context.Context, s *mcpServer, args map[string]any) (*tools.ToolResult, error) {
	return tools.Update(ctx, s.client, args)
}

func handleList(ctx context.Context, s *mcpServer, args map[string]any) (*tools.ToolResult, error) {
	return tools.List(ctx, s.client, args)
}

func handleConflicts(ctx context.Context, s *mcpServer, args map[string]any) (*tools.ToolResult, error) {
	return tools.Conflicts(ctx, s.client, args)
}

func handleExport(ctx context.Context, s *mcpServer, args map[string]any) (*tools.ToolResult, error) {
	return tools.Export(ctx, s.client, args)
}

func handleMIEStatus(ctx context.Context, s *mcpServer, args map[string]any) (*tools.ToolResult, error) {
	return tools.Status(ctx, s.client, args)
}