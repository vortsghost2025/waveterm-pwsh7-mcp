// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
)

func TestParseRunInteractiveCommandInput(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		wantCmd   string
		wantMs    *int
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "valid - no timeout",
			input:   map[string]any{"command": "echo hello"},
			wantCmd: "echo hello",
			wantMs:  nil,
		},
		{
			name:    "valid - with timeout",
			input:   map[string]any{"command": "npm install", "timeout_ms": 5000},
			wantCmd: "npm install",
			wantMs:  intPtr(5000),
		},
		{
			name:    "valid - max timeout",
			input:   map[string]any{"command": "go build ./...", "timeout_ms": 600000},
			wantCmd: "go build ./...",
			wantMs:  intPtr(600000),
		},
		{
			name:    "valid - min timeout",
			input:   map[string]any{"command": "echo hi", "timeout_ms": 1000},
			wantCmd: "echo hi",
			wantMs:  intPtr(1000),
		},
		{
			name:      "rejects nil input",
			input:     nil,
			wantErr:   true,
			errSubstr: "input is required",
		},
		{
			name:      "rejects empty command",
			input:     map[string]any{"command": ""},
			wantErr:   true,
			errSubstr: "missing command",
		},
		{
			name:      "rejects too-small timeout",
			input:     map[string]any{"command": "ls", "timeout_ms": 500},
			wantErr:   true,
			errSubstr: "timeout_ms must be >=",
		},
		{
			name:      "rejects too-large timeout",
			input:     map[string]any{"command": "ls", "timeout_ms": 999999},
			wantErr:   true,
			errSubstr: "timeout_ms must be <=",
		},
		{
			name:    "rejects wrong type - non-int timeout",
			input:   map[string]any{"command": "ls", "timeout_ms": "5000"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			params, err := parseRunInteractiveCommandInput(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Fatalf("expected error containing %q, got %q", tc.errSubstr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if params.Command != tc.wantCmd {
				t.Errorf("Command: got %q, want %q", params.Command, tc.wantCmd)
			}
			if tc.wantMs == nil {
				if params.TimeoutMs != nil {
					t.Errorf("TimeoutMs: got %v, want nil", *params.TimeoutMs)
				}
			} else {
				if params.TimeoutMs == nil {
					t.Errorf("TimeoutMs: got nil, want %d", *tc.wantMs)
				} else if *params.TimeoutMs != *tc.wantMs {
					t.Errorf("TimeoutMs: got %d, want %d", *params.TimeoutMs, *tc.wantMs)
				}
			}
		})
	}
}

func TestVerifyRunInteractiveCommandInput(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		wantErr bool
	}{
		{name: "allowed", cmd: "echo hi"},
		{name: "rejected - destructive", cmd: "rm -rf /", wantErr: true},
		{name: "rejected - metachar", cmd: "echo hi && whoami", wantErr: true},
		{name: "rejected - shutdown", cmd: "shutdown /s", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := map[string]any{"command": tc.cmd}
			err := verifyRunInteractiveCommandInput(input, &uctypes.UIMessageDataToolUse{})
			if tc.wantErr && err == nil {
				t.Errorf("expected error for %q, got nil", tc.cmd)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for %q: %v", tc.cmd, err)
			}
		})
	}
}

func TestGetRunInteractiveCommandToolDefinition(t *testing.T) {
	def := GetRunInteractiveCommandToolDefinition()
	if def.Name != "run_interactive_command" {
		t.Errorf("Name: got %q, want run_interactive_command", def.Name)
	}
	if def.ToolLogName != "gen:runinteractivecommand" {
		t.Errorf("ToolLogName: got %q", def.ToolLogName)
	}
	if def.ToolAnyCallback == nil {
		t.Error("ToolAnyCallback should not be nil")
	}
	if def.ToolApproval == nil {
		t.Error("ToolApproval should not be nil")
	}
	if def.ToolVerifyInput == nil {
		t.Error("ToolVerifyInput should not be nil")
	}
	if def.ToolCallDesc == nil {
		t.Error("ToolCallDesc should not be nil")
	}
	if !def.Strict {
		t.Error("Strict should be true")
	}
	schema := def.InputSchema
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("InputSchema.properties is not a map")
	}
	if _, ok := props["command"]; !ok {
		t.Error("InputSchema.properties.command missing")
	}
	if _, ok := props["timeout_ms"]; !ok {
		t.Error("InputSchema.properties.timeout_ms missing")
	}
	req, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("InputSchema.required is not []string")
	}
	if len(req) != 1 || req[0] != "command" {
		t.Errorf("InputSchema.required: got %v, want [command]", req)
	}
}

func TestParseTermListWidgetsInput(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		wantType  string
		wantErr   bool
		errSubstr string
	}{
		{
			name:     "nil input",
			input:    nil,
			wantType: "",
		},
		{
			name:     "empty map",
			input:    map[string]any{},
			wantType: "",
		},
		{
			name:     "view_type set",
			input:    map[string]any{"view_type": "term"},
			wantType: "term",
		},
		{
			name:    "invalid type",
			input:   "not a map",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			params, err := parseTermListWidgetsInput(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if params.ViewType != tc.wantType {
				t.Errorf("ViewType: got %q, want %q", params.ViewType, tc.wantType)
			}
		})
	}
}

func TestGetTermListWidgetsToolDefinition(t *testing.T) {
	def := GetTermListWidgetsToolDefinition("test-tab-id")
	if def.Name != "term_list_widgets" {
		t.Errorf("Name: got %q, want term_list_widgets", def.Name)
	}
	if def.ToolLogName != "term:listwidgets" {
		t.Errorf("ToolLogName: got %q", def.ToolLogName)
	}
	if def.ToolAnyCallback == nil {
		t.Error("ToolAnyCallback should not be nil")
	}
	if def.ToolCallDesc == nil {
		t.Error("ToolCallDesc should not be nil")
	}
	schema := def.InputSchema
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("InputSchema.properties is not a map")
	}
	if _, ok := props["view_type"]; !ok {
		t.Error("InputSchema.properties.view_type missing")
	}
	if req, exists := schema["required"]; exists {
		t.Errorf("InputSchema should not require view_type (it's optional), got %v", req)
	}
}

func intPtr(v int) *int { return &v }
