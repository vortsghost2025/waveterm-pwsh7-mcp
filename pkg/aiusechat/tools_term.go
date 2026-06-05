// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wcore"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

type TermGetScrollbackToolInput struct {
	WidgetId  string `json:"widget_id"`
	LineStart int    `json:"line_start,omitempty"`
	Count     int    `json:"count,omitempty"`
}

type CommandInfo struct {
	Command  string `json:"command"`
	Status   string `json:"status"`
	ExitCode *int   `json:"exitcode,omitempty"`
}

type TermGetScrollbackToolOutput struct {
	TotalLines         int          `json:"totallines"`
	LineStart          int          `json:"linestart"`
	LineEnd            int          `json:"lineend"`
	ReturnedLines      int          `json:"returnedlines"`
	Content            string       `json:"content"`
	SinceLastOutputSec *int         `json:"sincelastoutputsec,omitempty"`
	HasMore            bool         `json:"hasmore"`
	NextStart          *int         `json:"nextstart"`
	LastCommand        *CommandInfo `json:"lastcommand,omitempty"`
}

func parseTermGetScrollbackInput(input any) (*TermGetScrollbackToolInput, error) {
	const (
		DefaultCount = 200
		MaxCount     = 1000
	)

	result := &TermGetScrollbackToolInput{
		LineStart: 0,
		Count:     0,
	}

	if input == nil {
		result.Count = DefaultCount
		return result, nil
	}

	inputBytes, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}

	if err := json.Unmarshal(inputBytes, result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal input: %w", err)
	}

	if result.Count == 0 {
		result.Count = DefaultCount
	}

	if result.Count < 0 {
		return nil, fmt.Errorf("count must be positive")
	}

	result.Count = min(result.Count, MaxCount)

	return result, nil
}

func getTermScrollbackOutput(tabId string, widgetId string, rpcData wshrpc.CommandTermGetScrollbackLinesData) (*TermGetScrollbackToolOutput, error) {
	ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFn()

	fullBlockId, err := wcore.ResolveBlockIdFromPrefix(ctx, tabId, widgetId)
	if err != nil {
		return nil, err
	}

	rpcClient := wshclient.GetBareRpcClient()
	result, err := wshclient.TermGetScrollbackLinesCommand(
		rpcClient,
		rpcData,
		&wshrpc.RpcOpts{Route: wshutil.MakeFeBlockRouteId(fullBlockId)},
	)
	if err != nil {
		return nil, err
	}

	content := strings.Join(result.Lines, "\n")
	var effectiveLineEnd int
	if rpcData.LastCommand {
		effectiveLineEnd = result.LineStart + len(result.Lines)
	} else {
		effectiveLineEnd = min(rpcData.LineEnd, result.TotalLines)
	}
	hasMore := effectiveLineEnd < result.TotalLines

	var sinceLastOutputSec *int
	if result.LastUpdated > 0 {
		sec := max(0, int((time.Now().UnixMilli()-result.LastUpdated)/1000))
		sinceLastOutputSec = &sec
	}

	var nextStart *int
	if hasMore {
		nextStart = &effectiveLineEnd
	}

	blockORef := waveobj.MakeORef(waveobj.OType_Block, fullBlockId)
	rtInfo := wstore.GetRTInfo(blockORef)

	var lastCommand *CommandInfo
	if rtInfo != nil && rtInfo.ShellIntegration && rtInfo.ShellLastCmd != "" {
		cmdInfo := &CommandInfo{
			Command: rtInfo.ShellLastCmd,
		}
		if rtInfo.ShellState == "running-command" {
			cmdInfo.Status = "running"
		} else if rtInfo.ShellState == "ready" {
			cmdInfo.Status = "completed"
			exitCode := rtInfo.ShellLastCmdExitCode
			cmdInfo.ExitCode = &exitCode
		}
		lastCommand = cmdInfo
	}

	return &TermGetScrollbackToolOutput{
		TotalLines:         result.TotalLines,
		LineStart:          result.LineStart,
		LineEnd:            effectiveLineEnd,
		ReturnedLines:      len(result.Lines),
		Content:            content,
		SinceLastOutputSec: sinceLastOutputSec,
		HasMore:            hasMore,
		NextStart:          nextStart,
		LastCommand:        lastCommand,
	}, nil
}

func GetTermGetScrollbackToolDefinition(tabId string) uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "term_get_scrollback",
		DisplayName: "Get Terminal Scrollback",
		Description: "Fetch terminal scrollback from a widget as plain text. Index 0 is the most recent line; indices increase going upward (older lines). Also returns last command and exit code if shell integration is enabled.",
		ToolLogName: "term:getscrollback",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"widget_id": map[string]any{
					"type":        "string",
					"description": "8-character widget ID of the terminal widget",
				},
				"line_start": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"description": "Logical start index where 0 = most recent line (default: 0).",
				},
				"count": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"description": "Number of lines to return from line_start (default: 200).",
				},
			},
			"required":             []string{"widget_id"},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			parsed, err := parseTermGetScrollbackInput(input)
			if err != nil {
				return fmt.Sprintf("error parsing input: %v", err)
			}

			if parsed.LineStart == 0 && parsed.Count == 200 {
				return fmt.Sprintf("reading terminal output from %s (most recent %d lines)", parsed.WidgetId, parsed.Count)
			}
			lineEnd := parsed.LineStart + parsed.Count
			return fmt.Sprintf("reading terminal output from %s (lines %d-%d)", parsed.WidgetId, parsed.LineStart, lineEnd)
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseTermGetScrollbackInput(input)
			if err != nil {
				return nil, err
			}

			lineEnd := parsed.LineStart + parsed.Count
			output, err := getTermScrollbackOutput(
				tabId,
				parsed.WidgetId,
				wshrpc.CommandTermGetScrollbackLinesData{
					LineStart:   parsed.LineStart,
					LineEnd:     lineEnd,
					LastCommand: false,
				},
			)
			if err != nil {
				return nil, fmt.Errorf("failed to get terminal scrollback: %w", err)
			}
			return output, nil
		},
	}
}

type TermCommandOutputToolInput struct {
	WidgetId string `json:"widget_id"`
}

func parseTermCommandOutputInput(input any) (*TermCommandOutputToolInput, error) {
	result := &TermCommandOutputToolInput{}

	if input == nil {
		return nil, fmt.Errorf("widget_id is required")
	}

	inputBytes, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}

	if err := json.Unmarshal(inputBytes, result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal input: %w", err)
	}

	if result.WidgetId == "" {
		return nil, fmt.Errorf("widget_id is required")
	}

	return result, nil
}

type TermListWidgetsToolInput struct {
	ViewType string `json:"view_type,omitempty"`
}

type WidgetInfo struct {
	WidgetId   string `json:"widget_id"`
	BlockId    string `json:"block_id"`
	ViewType   string `json:"view_type"`
	ShortDesc  string `json:"short_desc"`
	ShellType  string `json:"shell_type,omitempty"`
	ShellState string `json:"shell_state,omitempty"`
}

type TermListWidgetsToolOutput struct {
	TabId   string       `json:"tab_id"`
	Count   int          `json:"count"`
	Widgets []WidgetInfo `json:"widgets"`
}

func parseTermListWidgetsInput(input any) (*TermListWidgetsToolInput, error) {
	result := &TermListWidgetsToolInput{}
	if input == nil {
		return result, nil
	}
	inputBytes, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}
	if err := json.Unmarshal(inputBytes, result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal input: %w", err)
	}
	return result, nil
}

func executeTermListWidgets(tabId string, params *TermListWidgetsToolInput) (*TermListWidgetsToolOutput, error) {
	ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFn()

	tabObj, err := wstore.DBMustGet[*waveobj.Tab](ctx, tabId)
	if err != nil {
		return nil, fmt.Errorf("failed to get tab %q: %w", tabId, err)
	}

	viewTypeFilter := params.ViewType
	out := &TermListWidgetsToolOutput{
		TabId:   tabId,
		Widgets: []WidgetInfo{},
	}

	for _, blockId := range tabObj.BlockIds {
		block, err := wstore.DBGet[*waveobj.Block](ctx, blockId)
		if err != nil {
			continue
		}
		if block.Meta == nil {
			continue
		}
		viewType, ok := block.Meta["view"].(string)
		if !ok {
			continue
		}
		if viewTypeFilter != "" && viewType != viewTypeFilter {
			continue
		}

		info := WidgetInfo{
			WidgetId:  block.OID[:8],
			BlockId:   block.OID,
			ViewType:  viewType,
			ShortDesc: MakeBlockShortDesc(block),
		}

		blockORef := waveobj.MakeORef(waveobj.OType_Block, block.OID)
		if rtInfo := wstore.GetRTInfo(blockORef); rtInfo != nil {
			info.ShellType = rtInfo.ShellType
			info.ShellState = rtInfo.ShellState
		}

		out.Widgets = append(out.Widgets, info)
	}
	out.Count = len(out.Widgets)
	return out, nil
}

func GetTermListWidgetsToolDefinition(tabId string) uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "term_list_widgets",
		DisplayName: "List Widgets in Tab",
		Description: "Enumerate all widgets (blocks) open in the current tab. Returns the 8-character widget ID, view type (e.g. 'term', 'web', 'preview', 'waveai'), and a short description for each. Optionally filter by view_type.",
		ToolLogName: "term:listwidgets",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"view_type": map[string]any{
					"type":        "string",
					"description": "Optional filter. Only return widgets with this view type (e.g. 'term', 'web', 'preview'). Omit to return all.",
				},
			},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			parsed, err := parseTermListWidgetsInput(input)
			if err != nil {
				return fmt.Sprintf("error parsing input: %v", err)
			}
			if parsed.ViewType != "" {
				return fmt.Sprintf("listing widgets with view_type=%q", parsed.ViewType)
			}
			return "listing all widgets in tab"
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseTermListWidgetsInput(input)
			if err != nil {
				return nil, err
			}
			out, err := executeTermListWidgets(tabId, parsed)
			if err != nil {
				return nil, err
			}
			return out, nil
		},
	}
}

func GetTermCommandOutputToolDefinition(tabId string) uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "term_command_output",
		DisplayName: "Get Last Command Output",
		Description: "Retrieve output from the most recent command in a terminal widget. Requires shell integration to be enabled. Returns the command text, exit code, and up to 1000 lines of output.",
		ToolLogName: "term:commandoutput",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"widget_id": map[string]any{
					"type":        "string",
					"description": "8-character widget ID of the terminal widget",
				},
			},
			"required":             []string{"widget_id"},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			parsed, err := parseTermCommandOutputInput(input)
			if err != nil {
				return fmt.Sprintf("error parsing input: %v", err)
			}
			return fmt.Sprintf("reading last command output from %s", parsed.WidgetId)
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseTermCommandOutputInput(input)
			if err != nil {
				return nil, err
			}

			ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelFn()

			fullBlockId, err := wcore.ResolveBlockIdFromPrefix(ctx, tabId, parsed.WidgetId)
			if err != nil {
				return nil, err
			}

			blockORef := waveobj.MakeORef(waveobj.OType_Block, fullBlockId)
			rtInfo := wstore.GetRTInfo(blockORef)
			if rtInfo == nil || !rtInfo.ShellIntegration {
				return nil, fmt.Errorf("shell integration is not enabled for this terminal")
			}

			output, err := getTermScrollbackOutput(
				tabId,
				parsed.WidgetId,
				wshrpc.CommandTermGetScrollbackLinesData{
					LastCommand: true,
				},
			)
			if err != nil {
				return nil, fmt.Errorf("failed to get command output: %w", err)
			}
			return output, nil
		},
	}
}
