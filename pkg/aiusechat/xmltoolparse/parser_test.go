// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package xmltoolparse

import (
	"testing"
)

func TestParseXMLToolCalls(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedText   string
		expectedCalls  int
		expectedTool   string
		expectedArgKey string
		expectedArgVal any
	}{
		{
			name: "single tool call with string arg",
			input: "Here is a result.\n\n" + "```" + "toolrun_readonly_command\n<arg_key>command</arg_key>\n<arg_value>whoami</arg_value>\n" + "```" + "\n",
			expectedText:   "Here is a result.",
			expectedCalls:  1,
			expectedTool:   "run_readonly_command",
			expectedArgKey: "command",
			expectedArgVal: "whoami",
		},
		{
			name: "multiple tool calls",
			input: "Let me search and then run.\n\n" + "```" + "toolgrep\n<arg_key>pattern</arg_key>\n<arg_value>func main</arg_value>\n<arg_key>path</arg_key>\n<arg_value>.</arg_value>\n" + "```" + "toolglob\n<arg_key>pattern</arg_key>\n<arg_value>*.go</arg_value>\n" + "```" + "\n",
			expectedText:   "Let me search and then run.",
			expectedCalls:  2,
			expectedTool:   "grep",
			expectedArgKey: "pattern",
			expectedArgVal: "func main",
		},
		{
			name: "no tool calls - plain text",
			input: "Hello, how can I help you?",
			expectedText: "Hello, how can I help you?",
			expectedCalls: 0,
		},
		{
			name: "integer arg",
			input: "Setting limit.\n\n" + "```" + "toolsearch\n<arg_key>limit</arg_key>\n<arg_value>100</arg_value>\n" + "```" + "\n",
			expectedText:   "Setting limit.",
			expectedCalls:  1,
			expectedTool:   "search",
			expectedArgKey: "limit",
			expectedArgVal: float64(100),
		},
		{
			name: "boolean arg",
			input: "Configuring.\n\n" + "```" + "toolset_option\n<arg_key>enabled</arg_key>\n<arg_value>true</arg_value>\n" + "```" + "\n",
			expectedText:   "Configuring.",
			expectedCalls:  1,
			expectedTool:   "set_option",
			expectedArgKey: "enabled",
			expectedArgVal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseXMLToolCalls(tt.input)
			if result.Text != tt.expectedText {
				t.Errorf("Text: got %q, want %q", result.Text, tt.expectedText)
			}
			if len(result.ToolCalls) != tt.expectedCalls {
				t.Fatalf("ToolCalls: got %d, want %d", len(result.ToolCalls), tt.expectedCalls)
			}
			if tt.expectedCalls > 0 {
				first := result.ToolCalls[0]
				if first.ToolName != tt.expectedTool {
					t.Errorf("ToolName: got %q, want %q", first.ToolName, tt.expectedTool)
				}
				val, ok := first.Args[tt.expectedArgKey]
				if !ok {
					t.Errorf("Arg key %q not found", tt.expectedArgKey)
				} else if val != tt.expectedArgVal {
					t.Errorf("Arg[%q]: got %v (%T), want %v (%T)",
						tt.expectedArgKey, val, val, tt.expectedArgVal, tt.expectedArgVal)
				}
			}
		})
	}
}

func TestHasXMLToolCall(t *testing.T) {
	if !HasXMLToolCall("hello `n" + "```" + "tooltest`n") {
		t.Error("should detect XML tool call")
	}
	if HasXMLToolCall("plain text") {
		t.Error("should not detect tool call in plain text")
	}
}

func TestToolCallID(t *testing.T) {
	if got := ToolCallID(0); got != "xml_call_0" {
		t.Errorf("got %q", got)
	}
}