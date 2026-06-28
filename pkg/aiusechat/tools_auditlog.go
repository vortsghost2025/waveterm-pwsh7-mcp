// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"context"
	"fmt"
	"time"

	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
	"github.com/wavetermdev/waveterm/pkg/aistore"
	"github.com/wavetermdev/waveterm/pkg/util/utilfn"
)

type AuditQueryInput struct {
	ToolName string `json:"toolname,omitempty"`
	Since    string `json:"since,omitempty"`
	Until    string `json:"until,omitempty"`
	Status   string `json:"status,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type AuditQueryOutput struct {
	Count   int                       `json:"count"`
	Entries []aistore.ToolCallLogEntry `json:"entries"`
}

func parseAuditQueryInput(input any) (*AuditQueryInput, error) {
	result := &AuditQueryInput{}
	if input == nil {
		return result, nil
	}
	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}
	return result, nil
}

func GetAuditQueryToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "audit_query",
		DisplayName: "Query Tool Audit Log",
		Description: "Query the persistent tool-call audit log. Returns recently-executed tools, their status (ok|error|started), inputs, outputs, durations, and error messages across every agent in this workspace. Useful for replay, debugging, and self-introspection. Read-only.",
		ToolLogName: "audit:query",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"toolname": map[string]any{
					"type":        "string",
					"description": "Filter by tool name (e.g. 'note_put').",
				},
				"since": map[string]any{
					"type":        "string",
					"description": "ISO-8601 timestamp or relative duration (e.g. '1h', '30m'). Filters to entries after this time.",
				},
				"until": map[string]any{
					"type":        "string",
					"description": "ISO-8601 timestamp. Filters to entries before this time.",
				},
				"status": map[string]any{
					"type":        "string",
					"description": "Filter by status: ok|error|started|approval-required|approval-denied",
				},
				"limit": map[string]any{
					"type":        "integer",
					"maximum":     500,
					"description": "Maximum entries to return (default 50, max 500)",
				},
			},
			"required":             []string{"toolname", "since", "until", "status", "limit"},
			"additionalProperties": false,
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseAuditQueryInput(input)
			if err != nil {
				return nil, err
			}
			q := aistore.ToolCallLogQuery{
				ToolName: parsed.ToolName,
				Status:   parsed.Status,
				Limit:    parsed.Limit,
			}
			if parsed.Since != "" {
				sinceMs, err := parseFlexibleTime(parsed.Since)
				if err != nil {
					return nil, fmt.Errorf("invalid 'since': %w", err)
				}
				q.SinceMs = sinceMs
			}
			if parsed.Until != "" {
				untilMs, err := parseFlexibleTime(parsed.Until)
				if err != nil {
					return nil, fmt.Errorf("invalid 'until': %w", err)
				}
				q.UntilMs = untilMs
			}
			if q.Limit <= 0 {
				q.Limit = 50
			}
			entries, err := aistore.GetAuditLogger().Query(context.Background(), q)
			if err != nil {
				return nil, fmt.Errorf("audit query failed: %w", err)
			}
			return &AuditQueryOutput{Count: len(entries), Entries: entries}, nil
		},
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolVerifyInput: func(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
			_, err := parseAuditQueryInput(input)
			return err
		},
	}
}

type AuditTailInput struct {
	MaxLines int `json:"maxlines,omitempty"`
}

type AuditTailOutput struct {
	Content string `json:"content"`
}

func parseAuditTailInput(input any) (*AuditTailInput, error) {
	result := &AuditTailInput{}
	if input == nil {
		return result, nil
	}
	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}
	if result.MaxLines <= 0 {
		result.MaxLines = 50
	}
	if result.MaxLines > 200 {
		result.MaxLines = 200
	}
	return result, nil
}

func GetAuditTailToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "audit_tail",
		DisplayName: "Tail Recent Audit Entries",
		Description: "Tail the most recent audit log entries for this workspace (raw ndjson lines). Quickest way to see what just happened. Read-only.",
		ToolLogName: "audit:tail",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"maxlines": map[string]any{
					"type":        "integer",
					"description": "Lines to return (default 50, max 200)",
				},
			},
			"required":             []string{"maxlines"},
			"additionalProperties": false,
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseAuditTailInput(input)
			if err != nil {
				return nil, err
			}
			out, err := aistore.GetAuditLogger().Tail(context.Background(), "", "", parsed.MaxLines)
			if err != nil {
				return nil, err
			}
			return &AuditTailOutput{Content: out}, nil
		},
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolVerifyInput: func(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
			_, err := parseAuditTailInput(input)
			return err
		},
	}
}

func parseFlexibleTime(s string) (int64, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UnixMilli(), nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		return time.Now().Add(-d).UnixMilli(), nil
	}
	return 0, fmt.Errorf("expected ISO-8601 timestamp (RFC3339) or duration like '1h'/'30m', got %q", s)
}
