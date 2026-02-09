// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package memory

import (
	"fmt"
	"strings"
)

// Batcher splits Datalog mutations into batches targeting a specific mutation count.
type Batcher struct {
	targetMutations int
	maxScriptSize   int
}

// NewBatcher creates a new batcher.
func NewBatcher(targetMutations int, maxScriptSize int) *Batcher {
	return &Batcher{
		targetMutations: targetMutations,
		maxScriptSize:   maxScriptSize,
	}
}

// Batch splits a Datalog script into multiple batches.
// Each batch targets targetMutations mutations and stays under maxScriptSize bytes.
func (b *Batcher) Batch(script string) ([]string, error) {
	if script == "" {
		return nil, nil
	}

	statements := b.splitStatements(script)
	if len(statements) == 0 {
		return nil, nil
	}

	var batches []string
	var currentBatch []string
	currentSize := 0
	currentMutations := 0

	separatorSize := len("\n\n")

	for _, stmt := range statements {
		stmtSize := len(stmt)

		additionalSize := stmtSize
		if len(currentBatch) > 0 {
			additionalSize += separatorSize
		}

		wouldExceedSize := currentSize+additionalSize > b.maxScriptSize
		wouldExceedTarget := currentMutations >= b.targetMutations

		if len(currentBatch) > 0 && (wouldExceedSize || wouldExceedTarget) {
			batch := strings.Join(currentBatch, "\n\n")
			if !strings.HasSuffix(batch, "\n") {
				batch += "\n"
			}
			batches = append(batches, batch)
			currentBatch = nil
			currentSize = 0
			currentMutations = 0
		}

		if stmtSize > b.maxScriptSize {
			stmtPreview := stmt
			if len(stmtPreview) > 200 {
				stmtPreview = stmtPreview[:200] + "..."
			}
			return nil, fmt.Errorf("mutation statement exceeds max size: %d bytes (limit: %d). Statement preview: %s", stmtSize, b.maxScriptSize, stmtPreview)
		}

		currentBatch = append(currentBatch, stmt)
		if len(currentBatch) == 1 {
			currentSize = stmtSize
		} else {
			currentSize += separatorSize + stmtSize
		}
		currentMutations++
	}

	if len(currentBatch) > 0 {
		batch := strings.Join(currentBatch, "\n\n")
		if !strings.HasSuffix(batch, "\n") {
			batch += "\n"
		}
		batches = append(batches, batch)
	}

	return batches, nil
}

type statementParser struct {
	braceDepth, bracketDepth int
	inString                 bool
	stringChar               rune
	escapeNext               bool
}

func (b *Batcher) splitStatements(script string) []string {
	var statements []string
	var current strings.Builder
	parser := &statementParser{}

	for _, line := range strings.Split(script, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}

		parser.processLine(line)

		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(line)

		if parser.isStatementComplete() && current.Len() > 0 {
			if stmt := extractValidStatement(current.String()); stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
		}
	}

	if stmt := extractValidStatement(current.String()); stmt != "" {
		statements = append(statements, stmt)
	}
	return statements
}

func (p *statementParser) processLine(line string) {
	for _, char := range line {
		p.processChar(char)
	}
}

func (p *statementParser) processChar(char rune) {
	if p.escapeNext {
		p.escapeNext = false
		return
	}

	if char == '\\' {
		p.escapeNext = true
		return
	}

	p.handleStringState(char)
	if !p.inString {
		p.updateBracketDepth(char)
	}
}

func (p *statementParser) handleStringState(char rune) {
	if !p.inString && (char == '"' || char == '\'') {
		p.inString = true
		p.stringChar = char
	} else if p.inString && char == p.stringChar {
		p.inString = false
		p.stringChar = 0
	}
}

func (p *statementParser) updateBracketDepth(char rune) {
	switch char {
	case '{':
		p.braceDepth++
	case '}':
		p.braceDepth--
	case '[':
		p.bracketDepth++
	case ']':
		p.bracketDepth--
	}
}

func (p *statementParser) isStatementComplete() bool {
	return p.braceDepth == 0 && p.bracketDepth == 0 && !p.inString
}

func extractValidStatement(s string) string {
	stmt := strings.TrimSpace(s)
	if stmt == "" || stmt == "//" || strings.HasPrefix(stmt, "//") {
		return ""
	}
	return stmt
}
