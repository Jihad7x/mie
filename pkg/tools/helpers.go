// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"fmt"
	"strings"
	"time"
)

// GetStringArg extracts a string argument from the args map, returning defaultVal if missing.
func GetStringArg(args map[string]any, key, defaultVal string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	s, ok := v.(string)
	if !ok {
		return defaultVal
	}
	return s
}

// GetFloat64Arg extracts a float64 argument from the args map, returning defaultVal if missing.
func GetFloat64Arg(args map[string]any, key string, defaultVal float64) float64 {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return defaultVal
	}
}

// GetIntArg extracts an int argument from the args map, returning defaultVal if missing.
func GetIntArg(args map[string]any, key string, defaultVal int) int {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	default:
		return defaultVal
	}
}

// GetBoolArg extracts a bool argument from the args map, returning defaultVal if missing.
func GetBoolArg(args map[string]any, key string, defaultVal bool) bool {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	b, ok := v.(bool)
	if !ok {
		return defaultVal
	}
	return b
}

// GetStringSliceArg extracts a string slice argument from the args map.
func GetStringSliceArg(args map[string]any, key string, defaultVal []string) []string {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	switch val := v.(type) {
	case []any:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		if len(result) == 0 {
			return defaultVal
		}
		return result
	case []string:
		if len(val) == 0 {
			return defaultVal
		}
		return val
	default:
		return defaultVal
	}
}

// SimilarityPercent converts a cosine distance to a similarity percentage.
// Cosine distance: 0.0 = identical, 2.0 = opposite.
// Returns 0-100 where 100 = identical.
func SimilarityPercent(distance float64) int {
	similarity := 1.0 - distance
	if similarity < 0 {
		similarity = 0
	}
	if similarity > 1 {
		similarity = 1
	}
	return int(similarity * 100)
}

// SimilarityIndicator returns an emoji indicator based on similarity percentage.
func SimilarityIndicator(distance float64) string {
	pct := SimilarityPercent(distance)
	switch {
	case pct >= 75:
		return "\U0001f7e2" // green circle
	case pct >= 50:
		return "\U0001f7e1" // yellow circle
	default:
		return "\U0001f534" // red circle
	}
}

// AnyToString converts any value to string.
func AnyToString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int(val)) {
			return fmt.Sprintf("%d", int(val))
		}
		return fmt.Sprintf("%.2f", val)
	case int:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

// AnyToFloat64 converts any numeric value to float64.
func AnyToFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}

// Truncate truncates a string to the specified length.
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// FormatRows formats query result rows for display.
func FormatRows(rows [][]any) string {
	if len(rows) == 0 {
		return "_No results_\n"
	}
	var sb strings.Builder
	for i, row := range rows {
		if i >= 20 {
			sb.WriteString(fmt.Sprintf("_... and %d more_\n", len(rows)-20))
			break
		}
		if len(row) >= 3 {
			sb.WriteString(fmt.Sprintf("- `%s` in `%s:%v`\n", row[0], row[1], row[2]))
		} else if len(row) >= 2 {
			sb.WriteString(fmt.Sprintf("- `%s` in `%s`\n", row[0], row[1]))
		} else if len(row) >= 1 {
			sb.WriteString(fmt.Sprintf("- `%s`\n", row[0]))
		}
	}
	return sb.String()
}

// FormatTime converts a unix timestamp to a human-readable UTC string.
// Returns the raw number as a string if the timestamp is zero or negative.
func FormatTime(ts int64) string {
	if ts <= 0 {
		return fmt.Sprintf("%d", ts)
	}
	return time.Unix(ts, 0).UTC().Format("2006-01-02 15:04:05")
}

// EscapeRegex escapes special regex characters for CozoDB.
func EscapeRegex(s string) string {
	var result []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '.', '(', ')', '[', ']', '{', '}', '+', '*', '?', '^', '$', '|', '\\':
			result = append(result, '[', c, ']')
		default:
			result = append(result, c)
		}
	}
	return string(result)
}

// QuoteCozoPattern wraps a pattern in CozoDB raw string notation.
func QuoteCozoPattern(pattern string) string {
	return `___"` + pattern + `"___`
}
