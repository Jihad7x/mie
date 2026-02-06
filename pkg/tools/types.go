// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

// ToolResult represents the result of a tool execution.
type ToolResult struct {
	Text    string
	IsError bool
}

// NewResult creates a successful tool result.
func NewResult(text string) *ToolResult {
	return &ToolResult{Text: text}
}

// NewError creates an error tool result.
func NewError(text string) *ToolResult {
	return &ToolResult{Text: text, IsError: true}
}
