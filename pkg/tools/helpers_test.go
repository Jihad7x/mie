// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import "testing"

func TestGetStringArg(t *testing.T) {
	args := map[string]any{
		"name":  "test",
		"empty": "",
		"num":   42,
	}

	if got := GetStringArg(args, "name", "default"); got != "test" {
		t.Errorf("GetStringArg(name) = %q, want %q", got, "test")
	}
	if got := GetStringArg(args, "missing", "default"); got != "default" {
		t.Errorf("GetStringArg(missing) = %q, want %q", got, "default")
	}
	if got := GetStringArg(args, "empty", "default"); got != "" {
		t.Errorf("GetStringArg(empty) = %q, want %q", got, "")
	}
	if got := GetStringArg(args, "num", "default"); got != "default" {
		t.Errorf("GetStringArg(num) = %q, want %q", got, "default")
	}
}

func TestGetFloat64Arg(t *testing.T) {
	args := map[string]any{
		"float": 0.85,
		"int":   42,
		"int64": int64(100),
		"str":   "not a number",
	}

	if got := GetFloat64Arg(args, "float", 0.5); got != 0.85 {
		t.Errorf("GetFloat64Arg(float) = %f, want 0.85", got)
	}
	if got := GetFloat64Arg(args, "int", 0.5); got != 42.0 {
		t.Errorf("GetFloat64Arg(int) = %f, want 42.0", got)
	}
	if got := GetFloat64Arg(args, "missing", 0.5); got != 0.5 {
		t.Errorf("GetFloat64Arg(missing) = %f, want 0.5", got)
	}
	if got := GetFloat64Arg(args, "str", 0.5); got != 0.5 {
		t.Errorf("GetFloat64Arg(str) = %f, want 0.5", got)
	}
}

func TestGetIntArg(t *testing.T) {
	args := map[string]any{
		"float": float64(10),
		"int":   42,
	}

	if got := GetIntArg(args, "float", 0); got != 10 {
		t.Errorf("GetIntArg(float) = %d, want 10", got)
	}
	if got := GetIntArg(args, "int", 0); got != 42 {
		t.Errorf("GetIntArg(int) = %d, want 42", got)
	}
	if got := GetIntArg(args, "missing", 5); got != 5 {
		t.Errorf("GetIntArg(missing) = %d, want 5", got)
	}
}

func TestGetBoolArg(t *testing.T) {
	args := map[string]any{
		"yes": true,
		"no":  false,
		"str": "true",
	}

	if got := GetBoolArg(args, "yes", false); got != true {
		t.Errorf("GetBoolArg(yes) = %v, want true", got)
	}
	if got := GetBoolArg(args, "no", true); got != false {
		t.Errorf("GetBoolArg(no) = %v, want false", got)
	}
	if got := GetBoolArg(args, "missing", true); got != true {
		t.Errorf("GetBoolArg(missing) = %v, want true", got)
	}
	if got := GetBoolArg(args, "str", false); got != false {
		t.Errorf("GetBoolArg(str) = %v, want false", got)
	}
}

func TestGetStringSliceArg(t *testing.T) {
	args := map[string]any{
		"types": []any{"fact", "decision"},
		"strs":  []string{"a", "b"},
		"empty": []any{},
		"bad":   "not a slice",
	}

	got := GetStringSliceArg(args, "types", nil)
	if len(got) != 2 || got[0] != "fact" || got[1] != "decision" {
		t.Errorf("GetStringSliceArg(types) = %v, want [fact decision]", got)
	}

	got = GetStringSliceArg(args, "strs", nil)
	if len(got) != 2 || got[0] != "a" {
		t.Errorf("GetStringSliceArg(strs) = %v, want [a b]", got)
	}

	got = GetStringSliceArg(args, "empty", []string{"default"})
	if len(got) != 1 || got[0] != "default" {
		t.Errorf("GetStringSliceArg(empty) = %v, want [default]", got)
	}

	got = GetStringSliceArg(args, "missing", []string{"x"})
	if len(got) != 1 || got[0] != "x" {
		t.Errorf("GetStringSliceArg(missing) = %v, want [x]", got)
	}
}

func TestSimilarityPercent(t *testing.T) {
	tests := []struct {
		distance float64
		want     int
	}{
		{0.0, 100},
		{0.1, 90},
		{0.25, 75},
		{0.5, 50},
		{1.0, 0},
	}
	for _, tt := range tests {
		got := SimilarityPercent(tt.distance)
		if got != tt.want {
			t.Errorf("SimilarityPercent(%f) = %d, want %d", tt.distance, got, tt.want)
		}
	}
}

func TestSimilarityIndicator(t *testing.T) {
	// Green for >= 75%
	got := SimilarityIndicator(0.1) // 90%
	if got != "\U0001f7e2" {
		t.Errorf("SimilarityIndicator(0.1) = %q, want green circle", got)
	}

	// Yellow for 50-74%
	got = SimilarityIndicator(0.4) // 60%
	if got != "\U0001f7e1" {
		t.Errorf("SimilarityIndicator(0.4) = %q, want yellow circle", got)
	}

	// Red for < 50%
	got = SimilarityIndicator(0.6) // 40%
	if got != "\U0001f534" {
		t.Errorf("SimilarityIndicator(0.6) = %q, want red circle", got)
	}
}

func TestAnyToString(t *testing.T) {
	tests := []struct {
		input any
		want  string
	}{
		{"hello", "hello"},
		{float64(42), "42"},
		{float64(3.14), "3.14"},
		{int(7), "7"},
		{true, "true"},
		{false, "false"},
		{nil, ""},
	}
	for _, tt := range tests {
		got := AnyToString(tt.input)
		if got != tt.want {
			t.Errorf("AnyToString(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := Truncate("short", 10); got != "short" {
		t.Errorf("Truncate(short, 10) = %q", got)
	}
	if got := Truncate("this is a long string", 10); got != "this is a ..." {
		t.Errorf("Truncate(long, 10) = %q", got)
	}
}

func TestEscapeRegex(t *testing.T) {
	got := EscapeRegex("func.test()")
	want := "func[.]test[(][)]"
	if got != want {
		t.Errorf("EscapeRegex = %q, want %q", got, want)
	}
}
