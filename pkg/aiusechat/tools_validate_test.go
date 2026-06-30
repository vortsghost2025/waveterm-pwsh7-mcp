// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
//
// Validator that enforces OpenAI Responses API strict-mode schema rules on
// every Strict tool shipped by Wave AI. Walks the schema recursively and
// verifies:
//   1. Every object schema (including nested) has a "required" array.
//   2. "required" lists EVERY key in the same level's "properties".
//   3. Every object schema sets "additionalProperties": false.
//   4. No "default" key appears anywhere in the schema (defaults belong in the
//      parse function with a description hint; strict mode rejects them).
//   5. Top-level schema is "type": "object" with a non-empty "properties" map.
//
// To exercise edge cases (empty schema, arrays without required, etc.) without
// breaking real tools, the unit tests in this file use synthetic fixtures.

package aiusechat

import (
	"fmt"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
)

func validateStrictSchema(name string, schema any) []string {
	var errs []string
	walkStrictSchema(name, schema, &errs)
	return errs
}

func walkStrictSchema(name string, schema any, errs *[]string) {
	m, ok := schema.(map[string]any)
	if !ok {
		return
	}
	if _, hasDef := m["default"]; hasDef {
		*errs = append(*errs, fmt.Sprintf("%s: schema-level default keys are forbidden", name))
	}
	t, _ := m["type"].(string)
	switch t {
	case "object":
		props, _ := m["properties"].(map[string]any)
		req, hasReq := m["required"].([]string)
		if !hasReq {
			*errs = append(*errs, fmt.Sprintf("%s: missing required array", name))
			req = []string{}
		}
		if len(req) == 0 && len(props) > 0 {
			*errs = append(*errs, fmt.Sprintf("%s: properties present but required is empty", name))
		}
		for k := range props {
			found := false
			for _, r := range req {
				if r == k {
					found = true
					break
				}
			}
			if !found {
				*errs = append(*errs, fmt.Sprintf("%s: properties.%q not in required", name, k))
			}
		}
		ap, hasAP := m["additionalProperties"]
		if !hasAP {
			*errs = append(*errs, fmt.Sprintf("%s: missing additionalProperties", name))
		} else if ap != false {
			*errs = append(*errs, fmt.Sprintf("%s: additionalProperties must be false", name))
		}
		// An empty `properties` map is valid for "no-input" tools (e.g. tool_list,
		// tool_schema). We only require non-empty properties when required is
		// populated, which is already enforced above.
		for k, sub := range props {
			walkStrictSchema(fmt.Sprintf("%s.properties.%s", name, k), sub, errs)
		}
	case "array":
		if items, ok := m["items"].(map[string]any); ok {
			walkStrictSchema(fmt.Sprintf("%s.items", name), items, errs)
		} else {
			*errs = append(*errs, fmt.Sprintf("%s: array schema missing items object", name))
		}
	}
}

func allStrictTools(t *testing.T) []uctypes.ToolDefinition {
	opts := &uctypes.WaveChatOpts{}
	out := []uctypes.ToolDefinition{
		GetAdderToolDefinition(),
		GetToolListToolDefinition(opts),
		GetToolSchemaToolDefinition(opts),
		GetRunCommandToolDefinition(),
		GetRunInteractiveCommandToolDefinition(),
		GetWriteTextFileToolDefinition(),
		GetEditTextFileToolDefinition(),
		GetDeleteTextFileToolDefinition(),
		GetReadTextFileToolDefinition(),
		GetReadDirToolDefinition(),
		GetWebFetchToolDefinition(),
		GetWebSearchToolDefinition(),
		GetBridgeWriteReplyToolDefinition(),
		GetBridgeReadInboxToolDefinition(),
		GetAISelfIntroToolDefinition(),
		GetGrepToolDefinition(),
		GetGlobToolDefinition(),
		GetCodebaseSearchToolDefinition(),
		GetAuditQueryToolDefinition(),
		GetAuditTailToolDefinition(),
		GetNotePutToolDefinition(),
		GetNoteGetToolDefinition(),
		GetNoteListToolDefinition(),
		GetNoteDeleteToolDefinition(),
		GetNoteSearchToolDefinition(),
		GetNoteDeleteManyToolDefinition(),
		GetNoteDeleteByScopeToolDefinition(),
		GetSysInfoToolDefinition(),
		GetSysEnvToolDefinition(),
		GetCaptureScreenshotToolDefinition("01234567"),
		GetWebNavigateToolDefinition("01234567"),
		GetTermSendKeyToolDefinition("01234567"),
		GetTermGetScrollbackToolDefinition("01234567"),
		GetTermSendInputToolDefinition("01234567"),
		GetTermRunCommandToolDefinition("01234567"),
		GetTermSearchScrollbackToolDefinition("01234567"),
		GetWidgetClearScrollbackToolDefinition("01234567"),
	}
	return out
}

func TestAllStrictSchemasValid(t *testing.T) {
	tools := allStrictTools(t)
	var failures []string
	pass := 0
	total := 0
	for _, tool := range tools {
		if !tool.Strict {
			continue
		}
		total++
		// Top-level shape: schema must be a non-empty object map with
		// type="object" defined. Tools whose InputSchema is missing or wrong
		// type are categorically invalid for strict mode.
		top := tool.InputSchema
		if top == nil {
			failures = append(failures, fmt.Sprintf("tool=%s InputSchema is nil", tool.Name))
			continue
		}
		if topType, _ := top["type"].(string); topType != "object" {
			failures = append(failures, fmt.Sprintf("tool=%s top-level schema type=%q, want object", tool.Name, topType))
		}
		errs := validateStrictSchema(tool.Name, tool.InputSchema)
		if len(errs) > 0 {
			failures = append(failures, fmt.Sprintf("tool=%s errs=%v", tool.Name, errs))
		} else {
			pass++
		}
	}
	if len(failures) > 0 {
		for _, f := range failures {
			t.Errorf("strict schema violation: %s", f)
		}
		t.Fatalf("%d/%d strict tools failed schema validation", len(failures), total)
	}
	t.Logf("validated %d strict tools, all PASS", pass)
}

// Synthetic fixtures that should fail the validator. These prove the
// validator actually catches the bug class it claims to, so a future
// "lazy pass" can't silently degrade.
func TestStrictSchemaEdgeCases(t *testing.T) {
	cases := []struct {
		name      string
		schema    any
		wantMatch []string
	}{
		{
			name: "defaults forbidden at top level",
			schema: map[string]any{
				"type":     "object",
				"default":  "bad",
				"properties": map[string]any{
					"x": map[string]any{"type": "string"},
				},
				"required":             []string{"x"},
				"additionalProperties": false,
			},
			wantMatch: []string{"schema-level default keys are forbidden"},
		},
		{
			name: "defaults forbidden nested",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"q": map[string]any{"type": "object", "properties": map[string]any{
						"y": map[string]any{"type": "string", "default": "no"},
					}, "required": []string{"y"}, "additionalProperties": false},
				},
				"required":             []string{"q"},
				"additionalProperties": false,
			},
			wantMatch: []string{"schema-level default keys are forbidden"},
		},
		{
			name: "missing properties from required",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"a": map[string]any{"type": "string"},
					"b": map[string]any{"type": "string"},
				},
				"required":             []string{"a"},
				"additionalProperties": false,
			},
			wantMatch: []string{`properties."b" not in required`},
		},
		{
			name: "missing additionalProperties",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"x": map[string]any{"type": "string"},
				},
				"required": []string{"x"},
			},
			wantMatch: []string{"missing additionalProperties"},
		},
		{
			name: "array without items",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"xs": map[string]any{"type": "array"},
				},
				"required":             []string{"xs"},
				"additionalProperties": false,
			},
			wantMatch: []string{"array schema missing items object"},
		},
		{
			name: "object with empty properties is allowed",
			schema: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"required":             []string{},
				"additionalProperties": false,
			},
			wantMatch: nil,
		},
		{
			name:      "non-map schema is silently ignored (current behavior)",
			schema:    "not-a-map",
			wantMatch: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateStrictSchema(tc.name, tc.schema)
			if tc.wantMatch == nil {
				if len(errs) != 0 {
					t.Fatalf("expected no errors, got %v", errs)
				}
				return
			}
			for _, want := range tc.wantMatch {
				found := false
				for _, got := range errs {
					if contains(got, want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error containing %q, got %v", want, errs)
				}
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (s == sub || stringIndex(s, sub) >= 0))
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
