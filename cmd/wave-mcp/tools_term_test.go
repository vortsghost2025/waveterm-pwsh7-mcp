package main

import (
	"encoding/json"
	"regexp"
	"sync"
	"testing"
)

var ansiStripRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func TestAnsiStripRe(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello world", "hello world"},
		{"color code", "\x1b[32mhello\x1b[0m", "hello"},
		{"bold", "\x1b[1mbold\x1b[0m text", "bold text"},
		{"cursor move", "\x1b[2J\x1b[Hclear", "clear"},
		{"256 color", "\x1b[38;5;196mred\x1b[0m", "red"},
		{"rgb color", "\x1b[38;2;255;0;0mred\x1b[0m", "red"},
		{"mixed ANSI", "\x1b[1;32mOK\x1b[0m: \x1b[33mwarn\x1b[0m", "OK: warn"},
		{"no escape", "no escapes here", "no escapes here"},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ansiStripRe.ReplaceAllString(tc.input, "")
			if got != tc.want {
				t.Fatalf("stripANSI(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCallTermGetScrollbackValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
	}{
		{"missing widget_id", map[string]any{}, true},
		{"empty widget_id", map[string]any{"widget_id": ""}, true},
		{"widget_id non-string", map[string]any{"widget_id": 123}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := callTermGetScrollback(tc.args)
			if tc.wantErr {
				var m map[string]any
				json.Unmarshal([]byte(result.Content[0].Text), &m)
				if _, ok := m["error"]; !ok {
					t.Fatalf("expected error result, got: %s", result.Content[0].Text)
				}
			} else if result.IsError {
				t.Fatalf("unexpected error: %s", result.Content[0].Text)
			}
		})
	}
}

func TestCallTermGetScrollbackCountClamping(t *testing.T) {
	tests := []struct {
		name      string
		countArg  float64
		wantCount int
	}{
		{"zero clamped to 1", 0, 1},
		{"negative clamped to 1", -5, 1},
		{"over max clamped", 5000, termScrollbackMaxCount},
		{"valid count", 100, 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_ = callTermGetScrollback(map[string]any{"widget_id": "test", "count": tc.countArg})
		})
	}
}

func TestCallTermSendInputValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
	}{
		{"missing widget_id", map[string]any{"text": "hi"}, true},
		{"missing text", map[string]any{"widget_id": "abc"}, true},
		{"empty text", map[string]any{"widget_id": "abc", "text": ""}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := callTermSendInput(tc.args)
			if tc.wantErr && !result.IsError {
				t.Fatalf("expected error, got: %s", result.Content[0].Text)
			}
		})
	}
}

func TestRpcClientMissingJWT(t *testing.T) {
	t.Setenv("WAVETERM_JWT", "")
	rpcClientOnce = sync.Once{}
	rpcClient = nil
	rpcClientErr = nil
	_, err := getRpcClient()
	if err == nil {
		t.Fatal("expected error when WAVETERM_JWT is empty")
	}
}
