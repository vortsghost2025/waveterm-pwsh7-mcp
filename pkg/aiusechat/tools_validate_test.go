// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
//
// Validator that enforces OpenAI Responses API strict-mode schema rules on
// every Strict tool shipped by Wave AI. Walks the schema recursively and
// verifies:
//   1. Every object schema (including nested) has a "required" array.
//   2. "required" lists EVERY key in the same level's "properties".
//   3. Every object schema sets "additionalProperties": false.

package aiusechat

import (
	"fmt"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
)

func validateStrictSchema(name string, schema any) []string {
	var errs []string
	switch s := schema.(type) {
	case map[string]any:
		t, _ := s["type"].(string)
		switch t {
		case "object":
			props, _ := s["properties"].(map[string]any)
			req, hasReq := s["required"].([]string)
			if !hasReq {
				errs = append(errs, fmt.Sprintf("%s: missing required array", name))
				req = []string{}
			}
			if !hasReq || len(req) == 0 {
				if props == nil {
					props = map[string]any{}
				}
				if len(props) > 0 {
					errs = append(errs, fmt.Sprintf("%s: properties present but required is empty", name))
				}
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
					errs = append(errs, fmt.Sprintf("%s: properties.%q not in required", name, k))
				}
			}
			ap, hasAP := s["additionalProperties"]
			if !hasAP {
				errs = append(errs, fmt.Sprintf("%s: missing additionalProperties", name))
			} else if ap != false {
				errs = append(errs, fmt.Sprintf("%s: additionalProperties must be false", name))
			}
			for k, sub := range props {
				childErrs := validateStrictSchema(fmt.Sprintf("%s.properties.%s", name, k), sub)
				errs = append(errs, childErrs...)
			}
		case "array":
			if items, ok := s["items"].(map[string]any); ok {
				childErrs := validateStrictSchema(fmt.Sprintf("%s.items", name), items)
				errs = append(errs, childErrs...)
			}
		}
	}
	return errs
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
