// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"runtime"
	"strings"
	"time"

	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
	"github.com/wavetermdev/waveterm/pkg/util/utilfn"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
	"github.com/wavetermdev/waveterm/pkg/wcore"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
)

type SystemInfoToolInput struct {
	Hostname  bool `json:"hostname,omitempty"`
	Env       bool `json:"env,omitempty"`
	Wave      bool `json:"wave,omitempty"`
	Full      bool `json:"full,omitempty"`
}

type SystemInfoToolOutput struct {
	Hostname       string `json:"hostname,omitempty"`
	Username       string `json:"username,omitempty"`
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	CPUCount       int    `json:"cpucount"`
	GoVersion      string `json:"goversion"`
	WaveVersion    string `json:"waveversion,omitempty"`
	WaveDataDir    string `json:"wavedatadir,omitempty"`
	WaveConfigDir  string `json:"waveconfigdir,omitempty"`
}

type SystemEnvToolInput struct {
	Names []string `json:"names,omitempty"`
}

type SystemEnvToolOutput struct {
	Env     map[string]string `json:"env"`
	Count   int               `json:"count"`
	Trunc   bool              `json:"truncated,omitempty"`
}

type TermSearchScrollbackToolInput struct {
	WidgetId   string `json:"widget_id"`
	Pattern    string `json:"pattern"`
	IsRegex    bool   `json:"isregex,omitempty"`
	MaxMatches int    `json:"maxmatches,omitempty"`
}

type TermSearchScrollbackToolOutput struct {
	TotalMatches int              `json:"totalmatches"`
	Matches      []TermSearchMatch `json:"matches"`
}

type TermSearchMatch struct {
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

type WidgetClearScrollbackToolInput struct {
	WidgetId    string                     `json:"widget_id"`
	Operations  []WidgetClearScrollbackOperation `json:"operations,omitempty"`
}

type WidgetClearScrollbackOperation struct {
	WidgetId string `json:"widget_id"`
}

type WidgetClearScrollbackToolOutput struct {
	Cleared bool `json:"cleared"`
}

type WidgetClearScrollbackBatchOutput struct {
	Results []WidgetClearScrollbackBatchResult `json:"results"`
}

type WidgetClearScrollbackBatchResult struct {
	WidgetId string `json:"widget_id"`
	Cleared  bool   `json:"cleared"`
	Err      string `json:"err,omitempty"`
}

func collectSystemInfo(full bool) SystemInfoToolOutput {
	out := SystemInfoToolOutput{}

	if h, err := os.Hostname(); err == nil {
		out.Hostname = h
	}
	if u, err := user.Current(); err == nil {
		if u.Username != "" {
			out.Username = u.Username
		} else if u.Name != "" {
			out.Username = u.Name
		}
	}
	out.OS = runtime.GOOS
	out.Arch = runtime.GOARCH
	out.CPUCount = runtime.NumCPU()
	out.GoVersion = runtime.Version()

	if full {
		out.WaveVersion = wavebase.WaveVersion
		out.WaveDataDir = wavebase.GetWaveDataDir()
		out.WaveConfigDir = wavebase.GetWaveConfigDir()
	}

	return out
}

func getSystemEnvSnapshot(names []string) (map[string]string, bool, error) {
	all := os.Environ()
	if len(names) == 0 {
		out := make(map[string]string, len(all))
		for _, kv := range all {
			if eq := strings.IndexByte(kv, '='); eq > 0 {
				out[kv[:eq]] = kv[eq+1:]
			}
		}
		return out, false, nil
	}
	out := make(map[string]string, len(names))
	for _, name := range names {
		val, ok := os.LookupEnv(name)
		if !ok {
			out[name] = ""
			continue
		}
		out[name] = val
	}
	return out, true, nil
}

func resolveBlockIdOrError(ctx context.Context, tabId, widgetId string) (string, error) {
	if widgetId == "" {
		return "", fmt.Errorf("widget_id is required")
	}
	full, err := wcore.ResolveBlockIdFromPrefix(ctx, tabId, widgetId)
	if err != nil {
		return "", fmt.Errorf("resolving widget_id %q: %w", widgetId, err)
	}
	return full, nil
}

func GetSysInfoToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "sys_info",
		DisplayName: "System Info",
		Description: "Return system info about the Wave server host: hostname, current username, OS, arch, CPU count, Go runtime version, and (when full=true) Wave version + data/config directories. Read-only.",
		ToolLogName: "sys:info",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"hostname": map[string]any{
					"type":        "boolean",
					"description": "Include hostname (default: true)",
				},
				"wave": map[string]any{
					"type":        "boolean",
					"description": "Include Wave version + directories (default: true)",
				},
				"full": map[string]any{
					"type":        "boolean",
					"description": "Include all available fields (default: false)",
				},
			},
			"additionalProperties": false,
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed := &SystemInfoToolInput{Hostname: true, Wave: true}
			if input != nil {
				if err := utilfn.ReUnmarshal(parsed, input); err != nil {
					return nil, fmt.Errorf("invalid input format: %w", err)
				}
			}
			info := collectSystemInfo(parsed.Full || parsed.Wave)
			if !parsed.Hostname {
				info.Hostname = ""
				info.Username = ""
			}
			if !parsed.Wave && !parsed.Full {
				info.WaveVersion = ""
				info.WaveDataDir = ""
				info.WaveConfigDir = ""
			}
			return info, nil
		},
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolVerifyInput: func(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
			if input == nil {
				return nil
			}
			p := &SystemInfoToolInput{}
			return utilfn.ReUnmarshal(p, input)
		},
	}
}

func GetSysEnvToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "sys_env",
		DisplayName: "System Environment",
		Description: "Return environment variables from the Wave server process. By default returns all variables; pass names[] to request specific ones. Sensitive variables (containing TOKEN, SECRET, KEY, PASSWORD) are masked automatically. Read-only.",
		ToolLogName: "sys:env",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"names": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional list of env var names to return. If omitted, returns all.",
				},
			},
			"additionalProperties": false,
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed := &SystemEnvToolInput{}
			if input != nil {
				if err := utilfn.ReUnmarshal(parsed, input); err != nil {
					return nil, fmt.Errorf("invalid input format: %w", err)
				}
			}
			env, truncated, err := getSystemEnvSnapshot(parsed.Names)
			if err != nil {
				return nil, err
			}
			masked := maskSensitiveEnv(env)
			return &SystemEnvToolOutput{Env: masked, Count: len(masked), Trunc: truncated}, nil
		},
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolVerifyInput: func(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
			if input == nil {
				return nil
			}
			p := &SystemEnvToolInput{}
			return utilfn.ReUnmarshal(p, input)
		},
	}
}

func maskSensitiveEnv(env map[string]string) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		upper := strings.ToUpper(k)
		if strings.Contains(upper, "TOKEN") || strings.Contains(upper, "SECRET") || strings.Contains(upper, "KEY") || strings.Contains(upper, "PASSWORD") {
			if len(v) > 8 {
				out[k] = v[:4] + "..." + v[len(v)-4:]
			} else if v != "" {
				out[k] = "***"
			} else {
				out[k] = ""
			}
		} else {
			out[k] = v
		}
	}
	return out
}

func GetTermSearchScrollbackToolDefinition(tabId string) uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "term_search_scrollback",
		DisplayName: "Search Terminal Scrollback",
		Description: "Search a terminal widget's scrollback buffer for a pattern. Returns matching line numbers + snippets (max 200 chars). Set isregex=true to interpret pattern as a regular expression. Read-only.",
		ToolLogName: "term:searchscrollback",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"widget_id": map[string]any{
					"type":        "string",
					"description": "8-character widget ID of the terminal widget",
				},
				"pattern": map[string]any{
					"type":        "string",
					"description": "Pattern to search for (substring by default; regex if isregex=true)",
				},
				"isregex": map[string]any{
					"type":        "boolean",
					"description": "Treat pattern as a regular expression (default false)",
				},
				"maxmatches": map[string]any{
					"type":        "integer",
					"default":     50,
					"description": "Maximum matches to return (default 50, max 200)",
				},
			},
			"required":             []string{"widget_id", "pattern"},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			parsed, err := parseTermSearchScrollbackInput(input)
			if err != nil {
				return fmt.Sprintf("error parsing input: %v", err)
			}
			if parsed.IsRegex {
				return fmt.Sprintf("searching scrollback for /%s/ in %s", parsed.Pattern, parsed.WidgetId)
			}
			return fmt.Sprintf("searching scrollback for %q in %s", parsed.Pattern, parsed.WidgetId)
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseTermSearchScrollbackInput(input)
			if err != nil {
				return nil, err
			}
			ctx, cancelFn := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancelFn()
			fullBlockId, err := resolveBlockIdOrError(ctx, "", parsed.WidgetId)
			if err != nil {
				return nil, err
			}
			rpcClient := wshclient.GetBareRpcClient()
			result, err := wshclient.TermSearchScrollbackCommand(
				rpcClient,
				wshrpc.CommandTermSearchScrollbackData{
					BlockId:    fullBlockId,
					Pattern:    parsed.Pattern,
					IsRegex:    parsed.IsRegex,
					MaxMatches: parsed.MaxMatches,
				},
				&wshrpc.RpcOpts{Route: wshutil.MakeFeBlockRouteId(fullBlockId)},
			)
			if err != nil {
				return nil, fmt.Errorf("scrollback search failed: %w", err)
			}
			matches := make([]TermSearchMatch, 0, len(result.Matches))
			for _, m := range result.Matches {
				matches = append(matches, TermSearchMatch{Line: m.Line, Snippet: m.Snippet})
			}
			return &TermSearchScrollbackToolOutput{
				TotalMatches: result.TotalMatches,
				Matches:      matches,
			}, nil
		},
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolVerifyInput: func(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
			_, err := parseTermSearchScrollbackInput(input)
			return err
		},
	}
}

func GetWidgetClearScrollbackToolDefinition(tabId string) uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "widget_clear_scrollback",
		DisplayName: "Clear Widget Scrollback",
		Description: "Clear a terminal widget's scrollback buffer. This is destructive — clears all buffered terminal output for the widget. Use sparingly.",
		ToolLogName: "widget:clearscrollback",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"widget_id": map[string]any{
					"type":        "string",
					"description": "8-character widget ID of the terminal widget",
				},
				"operations": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"widget_id": map[string]any{"type": "string", "description": "8-character widget ID of the terminal widget"},
						},
						"required":             []string{"widget_id"},
						"additionalProperties": false,
					},
					"description": "Optional batch mode: array of widget clear operations. When provided, top-level widget_id is ignored. Each operation returns a result in the same order.",
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			parsed, err := parseWidgetClearScrollbackInput(input)
			if err != nil {
				return fmt.Sprintf("error parsing input: %v", err)
			}
			if len(parsed.Operations) > 0 {
				return fmt.Sprintf("clearing scrollback for %d widgets", len(parsed.Operations))
			}
			return fmt.Sprintf("clearing scrollback of widget %s", parsed.WidgetId)
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseWidgetClearScrollbackInput(input)
			if err != nil {
				return nil, err
			}
			if len(parsed.Operations) > 0 {
				return handleWidgetClearScrollbackBatch(parsed)
			}
			ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelFn()
			fullBlockId, err := resolveBlockIdOrError(ctx, "", parsed.WidgetId)
			if err != nil {
				return nil, err
			}
			rpcClient := wshclient.GetBareRpcClient()
			err = wshclient.WidgetClearScrollbackCommand(
				rpcClient,
				wshrpc.WidgetClearScrollbackData{BlockId: fullBlockId},
				&wshrpc.RpcOpts{Route: wshutil.MakeFeBlockRouteId(fullBlockId)},
			)
			if err != nil {
				return nil, fmt.Errorf("clear scrollback failed: %w", err)
			}
			return &WidgetClearScrollbackToolOutput{Cleared: true}, nil
		},
		ToolApproval: func(input any) string {
			return uctypes.ApprovalNeedsApproval
		},
		ToolVerifyInput: func(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
			_, err := parseWidgetClearScrollbackInput(input)
			return err
		},
	}
}

func parseTermSearchScrollbackInput(input any) (*TermSearchScrollbackToolInput, error) {
	result := &TermSearchScrollbackToolInput{MaxMatches: 50}
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}
	if result.WidgetId == "" {
		return nil, fmt.Errorf("widget_id is required")
	}
	if result.Pattern == "" {
		return nil, fmt.Errorf("pattern is required")
	}
	if result.MaxMatches <= 0 || result.MaxMatches > 200 {
		result.MaxMatches = 50
	}
	return result, nil
}

func parseWidgetClearScrollbackInput(input any) (*WidgetClearScrollbackToolInput, error) {
	result := &WidgetClearScrollbackToolInput{}
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}
	if len(result.Operations) == 0 {
		if result.WidgetId == "" {
			return nil, fmt.Errorf("widget_id is required (or provide operations array for batch mode)")
		}
	}
	return result, nil
}

func handleWidgetClearScrollbackBatch(parsed *WidgetClearScrollbackToolInput) (*WidgetClearScrollbackBatchOutput, error) {
	results := make([]WidgetClearScrollbackBatchResult, 0, len(parsed.Operations))
	for _, op := range parsed.Operations {
		if op.WidgetId == "" {
			results = append(results, WidgetClearScrollbackBatchResult{Err: "widget_id is required"})
			continue
		}
		ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelFn()
		fullBlockId, err := resolveBlockIdOrError(ctx, "", op.WidgetId)
		if err != nil {
			results = append(results, WidgetClearScrollbackBatchResult{WidgetId: op.WidgetId, Err: fmt.Sprintf("resolve failed: %v", err)})
			continue
		}
		rpcClient := wshclient.GetBareRpcClient()
		err = wshclient.WidgetClearScrollbackCommand(
			rpcClient,
			wshrpc.WidgetClearScrollbackData{BlockId: fullBlockId},
			&wshrpc.RpcOpts{Route: wshutil.MakeFeBlockRouteId(fullBlockId)},
		)
		if err != nil {
			results = append(results, WidgetClearScrollbackBatchResult{WidgetId: op.WidgetId, Err: fmt.Sprintf("clear failed: %v", err)})
		} else {
			results = append(results, WidgetClearScrollbackBatchResult{WidgetId: op.WidgetId, Cleared: true})
		}
	}
	return &WidgetClearScrollbackBatchOutput{Results: results}, nil
}
