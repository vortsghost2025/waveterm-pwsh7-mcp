// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"testing"
)

// ---------- list_workspaces ----------

func TestParseListWorkspacesInput(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		wantErr bool
	}{
		{name: "nil input", input: nil, wantErr: false},
		{name: "empty map", input: map[string]any{}, wantErr: false},
		{name: "with extra field (allowed)", input: map[string]any{"ignored": "x"}, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseListWorkspacesInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseListWorkspacesInput: err=%v wantErr=%v", err, tt.wantErr)
			}
			if !tt.wantErr && got == nil {
				t.Fatal("got nil result")
			}
		})
	}
}

func TestGetListWorkspacesToolDefinition(t *testing.T) {
	def := GetListWorkspacesToolDefinition()
	if def.Name != "list_workspaces" {
		t.Errorf("Name: got %q, want %q", def.Name, "list_workspaces")
	}
	if def.Description == "" {
		t.Error("Description is empty")
	}
	if def.InputSchema == nil {
		t.Fatal("InputSchema is nil")
	}
	schema := def.InputSchema
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("InputSchema.properties is not a map")
	}
	if len(props) != 0 {
		t.Errorf("list_workspaces should have no properties, got %d", len(props))
	}
	if ap, ok := schema["additionalProperties"].(bool); !ok || ap {
		t.Errorf("InputSchema.additionalProperties should be false, got %v", schema["additionalProperties"])
	}
	if def.ToolLogName == "" {
		t.Error("ToolLogName is empty")
	}
	if def.ToolAnyCallback == nil {
		t.Error("ToolAnyCallback is nil")
	}
}

// ---------- list_tabs ----------

func TestParseListTabsInput(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		wantErr bool
		wsId    string
	}{
		{name: "nil input", input: nil, wantErr: false, wsId: ""},
		{name: "empty map", input: map[string]any{}, wantErr: false, wsId: ""},
		{name: "with workspace_id", input: map[string]any{"workspace_id": "ws_abc12345"}, wantErr: false, wsId: "ws_abc12345"},
		{name: "workspace_id wrong type", input: map[string]any{"workspace_id": 12345}, wantErr: true, wsId: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseListTabsInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseListTabsInput: err=%v wantErr=%v", err, tt.wantErr)
			}
			if !tt.wantErr && got.WorkspaceId != tt.wsId {
				t.Errorf("WorkspaceId: got %q, want %q", got.WorkspaceId, tt.wsId)
			}
		})
	}
}

func TestGetListTabsToolDefinition(t *testing.T) {
	def := GetListTabsToolDefinition()
	if def.Name != "list_tabs" {
		t.Errorf("Name: got %q, want %q", def.Name, "list_tabs")
	}
	schema := def.InputSchema
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("InputSchema.properties is not a map")
	}
	if _, ok := props["workspace_id"]; !ok {
		t.Error("InputSchema.properties.workspace_id missing")
	}
	if ap, ok := schema["additionalProperties"].(bool); !ok || ap {
		t.Errorf("InputSchema.additionalProperties should be false, got %v", schema["additionalProperties"])
	}
	if def.ToolAnyCallback == nil {
		t.Error("ToolAnyCallback is nil")
	}
	if def.ToolCallDesc == nil {
		t.Error("ToolCallDesc is nil")
	}
}

// ---------- get_widget ----------

func TestParseGetWidgetInput(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		wantErr bool
	}{
		{name: "nil input", input: nil, wantErr: true}, // neither widget_id nor block_id
		{name: "empty map", input: map[string]any{}, wantErr: true},
		{name: "widget_id set", input: map[string]any{"widget_id": "abc12345"}, wantErr: false},
		{name: "block_id set", input: map[string]any{"block_id": "blk_full_oid_xxxxxxxx"}, wantErr: false},
		{name: "both set", input: map[string]any{"widget_id": "a", "block_id": "b"}, wantErr: true},
		{name: "widget_id wrong type", input: map[string]any{"widget_id": 123}, wantErr: true},
		{name: "block_id wrong type", input: map[string]any{"block_id": 123}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseGetWidgetInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseGetWidgetInput: err=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestGetGetWidgetToolDefinition(t *testing.T) {
	def := GetGetWidgetToolDefinition()
	if def.Name != "get_widget" {
		t.Errorf("Name: got %q, want %q", def.Name, "get_widget")
	}
	schema := def.InputSchema
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("InputSchema.properties is not a map")
	}
	if _, ok := props["widget_id"]; !ok {
		t.Error("InputSchema.properties.widget_id missing")
	}
	if _, ok := props["block_id"]; !ok {
		t.Error("InputSchema.properties.block_id missing")
	}
	// Neither field should be in 'required' since either is acceptable.
	if req, exists := schema["required"]; exists {
		t.Errorf("get_widget should not require either field (either is valid), got required=%v", req)
	}
	if ap, ok := schema["additionalProperties"].(bool); !ok || ap {
		t.Errorf("InputSchema.additionalProperties should be false, got %v", schema["additionalProperties"])
	}
	if def.ToolAnyCallback == nil {
		t.Error("ToolAnyCallback is nil")
	}
}
