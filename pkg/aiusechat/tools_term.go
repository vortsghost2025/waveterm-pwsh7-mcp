// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
	"github.com/wavetermdev/waveterm/pkg/util/utilfn"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wcore"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

const (
	AgentModelKey = "agent:model"
	AgentModeKey  = "agent:mode"
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

// ---------- term_send_input ----------

type TermSendInputToolInput struct {
	WidgetId string `json:"widget_id"`
	Text     string `json:"text"`
	Enter    bool   `json:"enter,omitempty"`
}

type TermSendInputToolOutput struct {
	Success bool `json:"success"`
}

func buildTermSendInputPayloads(text string, enter bool) ([][]byte, error) {
	payloads := [][]byte{[]byte(text)}
	if !enter {
		return payloads, nil
	}

	enterSequence, isSignal, _, err := ResolvedSendKeySequence("enter")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve Enter key: %w", err)
	}
	if isSignal || enterSequence == "" {
		return nil, fmt.Errorf("Enter key resolved to an invalid terminal sequence")
	}

	return append(payloads, []byte(enterSequence)), nil
}

func parseTermSendInputInput(input any) (*TermSendInputToolInput, error) {
	result := &TermSendInputToolInput{}

	if input == nil {
		return nil, fmt.Errorf("input is required")
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
	if result.Text == "" {
		return nil, fmt.Errorf("text is required")
	}

	return result, nil
}

func GetTermSendInputToolDefinition(tabId string) uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "term_send_input",
		DisplayName: "Send Input to Terminal",
		Description: "Send text input to a terminal widget as if the user typed it. " +
			`Set "enter": true to press Enter after the text. ` +
			"For a newly spawned Kilo agent, pass the initial task in term_spawn_agent.prompt instead of sending input during startup. " +
			"Use this only for follow-ups after capture_screenshot confirms the Kilo input is ready. " +
			"For simple shell commands on idle terminals, prefer term_run_command instead. " +
			"All tool calls are pre-approved.",
		ToolLogName: "term:sendinput",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"widget_id": map[string]any{
					"type":        "string",
					"description": "8-character widget ID of the terminal widget",
				},
				"text": map[string]any{
					"type":        "string",
					"description": "Text to send to the terminal",
				},
				"enter": map[string]any{
					"type":        "boolean",
					"description": "If true, press Enter after the text. Default: false.",
				},
			},
			"required":             []string{"widget_id", "text"},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			parsed, err := parseTermSendInputInput(input)
			if err != nil {
				return fmt.Sprintf("error parsing input: %v", err)
			}

			textPreview := parsed.Text
			if len(textPreview) > 50 {
				textPreview = textPreview[:47] + "..."
			}
			if parsed.Enter {
				return fmt.Sprintf("sending %q to terminal %s (enter)", textPreview, parsed.WidgetId)
			}
			return fmt.Sprintf("sending %q to terminal %s", textPreview, parsed.WidgetId)
		},
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseTermSendInputInput(input)
			if err != nil {
				return nil, err
			}

			ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelFn()

			fullBlockId, err := wcore.ResolveBlockIdFromPrefix(ctx, tabId, parsed.WidgetId)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve widget %s: %w", parsed.WidgetId, err)
			}

			payloads, err := buildTermSendInputPayloads(parsed.Text, parsed.Enter)
			if err != nil {
				return nil, err
			}

			for idx, payload := range payloads {
				if idx > 0 {
					// Some terminals accept typed text but drop an Enter byte when
					// both arrive in the same controller-input RPC. Give the PTY a
					// brief chance to consume the text, then submit Enter separately.
					time.Sleep(75 * time.Millisecond)
				}

				err = wshclient.ControllerInputCommand(
					wshclient.GetBareRpcClient(),
					wshrpc.CommandBlockInputData{
						BlockId:     fullBlockId,
						InputData64: base64.StdEncoding.EncodeToString(payload),
					},
					nil,
				)
				if err != nil {
					if idx == 0 {
						return nil, fmt.Errorf("failed to send text to terminal: %w", err)
					}
					return nil, fmt.Errorf("failed to send Enter to terminal: %w", err)
				}
			}

			return &TermSendInputToolOutput{Success: true}, nil
		},
	}
}

func GetTermSendKeyToolDefinition(tabId string) uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "term_send_key",
		DisplayName: "Send Special Key to Terminal",
		Description: "Send a special key press (Tab, Escape, arrows, Home/End, PageUp/PageDown, Backspace, Delete, or signal keys). " +
			"Useful for interacting with TUI apps that don't accept raw text input. " +
			"Enter and signal keys (ctrlc, ctrlz, ctrld, ctrlbackslash, sigterm, sigkill) require user approval because they can execute pending commands or interrupt processes. " +
			"To send text with Enter (e.g., submitting a message or command), use term_send_input with \"enter\": true instead.",
		ToolLogName: "term:sendkey",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"widget_id": map[string]any{
					"type":        "string",
					"description": "8-character widget ID of the terminal widget",
				},
				"key": map[string]any{
					"type":        "string",
					"description": "Key to send. Safe keys: tab, escape/esc, space, backspace, delete, home, end, pageup, pagedown, arrowup/up, arrowdown/down, arrowleft/left, arrowright/right. Dangerous (approval required): enter, ctrlc, ctrlz, ctrld, ctrlbackslash, sigterm, sigkill.",
				},
			},
			"required":             []string{"widget_id", "key"},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			parsed, err := parseTermSendKeyInput(input)
			if err != nil {
				return fmt.Sprintf("error parsing input: %v", err)
			}
			return fmt.Sprintf("sending key %q to terminal %s", parsed.Key, parsed.WidgetId)
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseTermSendKeyInput(input)
			if err != nil {
				return nil, err
			}
			ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelFn()
			fullBlockId, err := wcore.ResolveBlockIdFromPrefix(ctx, tabId, parsed.WidgetId)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve widget %s: %w", parsed.WidgetId, err)
			}
			sequence, isSignal, sigName, err := ResolvedSendKeySequence(parsed.Key)
			if err != nil {
				return nil, err
			}
			req := wshrpc.CommandBlockInputData{BlockId: fullBlockId}
			if isSignal {
				req.SigName = sigName
			} else if sequence != "" {
				req.InputData64 = base64.StdEncoding.EncodeToString([]byte(sequence))
			} else {
				return nil, fmt.Errorf("key %q resolved to empty sequence", parsed.Key)
			}
			err = wshclient.ControllerInputCommand(
				wshclient.GetBareRpcClient(),
				req,
				nil,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to send key to terminal: %w", err)
			}
			return &SendKeyOutput{Sent: true, Sequence: sequence}, nil
		},
		ToolApproval: func(input any) string {
			if input == nil {
				return uctypes.ApprovalAutoApproved
			}
			parsed, err := parseTermSendKeyInput(input)
			if err != nil {
				return uctypes.ApprovalAutoApproved
			}
			if IsDangerousKey(parsed.Key) {
				return uctypes.ApprovalNeedsApproval
			}
			return uctypes.ApprovalAutoApproved
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
		Description: "Retrieve output from the most recent command in a terminal widget. Uses shell integration for exact command boundaries when available, otherwise falls back to recent scrollback. Returns the command text, exit code, and up to 1000 lines of output.",
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

// ---------- term_spawn_agent ----------

type TermSpawnAgentToolInput struct {
	CLI         string `json:"cli"`
	Model       string `json:"model"`
	Mode        string `json:"mode"`
	WorkingDir  string `json:"working_dir"`
	ProjectFile string `json:"project_file"`
	Prompt      string `json:"prompt"`
}

type TermSpawnAgentToolOutput struct {
	WidgetId string `json:"widget_id"`
	Status   string `json:"status"`
	Model    string `json:"model"`
	Mode     string `json:"mode"`
	CLI      string `json:"cli"`
}

func parseTermSpawnAgentInput(input any) (*TermSpawnAgentToolInput, error) {
	result := &TermSpawnAgentToolInput{CLI: "opencode"}
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	inputBytes, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}
	if err := json.Unmarshal(inputBytes, result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal input: %w", err)
	}
	if result.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	if result.CLI == "" {
		result.CLI = "opencode"
	}
	if result.WorkingDir == "" {
		return nil, fmt.Errorf("working_dir is required")
	}
	return result, nil
}

func GetTermSpawnAgentToolDefinition(tabId string) uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "term_spawn_agent",
		DisplayName: "Spawn Agent Terminal",
		Description: "Spawn a new AI coding agent in a terminal widget. For Kilo, provide prompt so the initial task is passed with --prompt at process launch instead of racing TUI startup. Returns a widget_id for screenshot monitoring and later term_send_input follow-ups.",
		ToolLogName: "term:spawnagent",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"model": map[string]any{
					"type":        "string",
					"description": "Model identifier, e.g. 'glm-5.1', 'nemotron-3-ultra'",
				},
				"cli": map[string]any{
					"type":        "string",
					"description": "Agent CLI to use: 'opencode' (default) or 'kilo'",
				},
				"mode": map[string]any{
					"type":        "string",
					"description": "Agent mode: 'plan' (architecture/research) or 'build' (implementation)",
				},
				"working_dir": map[string]any{
					"type":        "string",
					"description": "Working directory for the agent (project root)",
				},
				"project_file": map[string]any{
					"type":        "string",
					"description": "Optional path to AGENTS.md or project context file",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "Initial task. For Kilo this is passed atomically with --prompt so input cannot be lost during TUI startup.",
				},
			},
			"required":             []string{"model", "working_dir"},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			parsed, err := parseTermSpawnAgentInput(input)
			if err != nil {
				return fmt.Sprintf("error parsing input: %v", err)
			}
			return fmt.Sprintf("spawning %s agent (model=%s, mode=%s) in %s",
				parsed.CLI, parsed.Model, parsed.Mode, parsed.WorkingDir)
		},
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseTermSpawnAgentInput(input)
			if err != nil {
				return nil, err
			}

			modelFlag := "--model"
			cmdArgs := []string{modelFlag, parsed.Model}
			isKilo := strings.EqualFold(parsed.CLI, "kilo")

			if isKilo && parsed.Prompt != "" {
				cmdArgs = append(cmdArgs, "--prompt", parsed.Prompt)
			}
			if !isKilo && parsed.ProjectFile != "" {
				cmdArgs = append(cmdArgs, "--project", parsed.ProjectFile)
			}

			cmdEnv := map[string]string{}
			if parsed.Mode != "" {
				cmdEnv["AGENT_MODE"] = parsed.Mode
			}

			createMeta := map[string]any{
				waveobj.MetaKey_View:          "term",
				waveobj.MetaKey_CmdCwd:        parsed.WorkingDir,
				waveobj.MetaKey_Controller:    "cmd",
				waveobj.MetaKey_Cmd:           parsed.CLI,
				waveobj.MetaKey_CmdArgs:       cmdArgs,
				waveobj.MetaKey_CmdShell:      false,
				waveobj.MetaKey_CmdRunOnStart: true,
				waveobj.MetaKey_CmdRunOnce:    true,
				waveobj.MetaKey_CmdEnv:        cmdEnv,
				AgentModelKey:                 parsed.Model,
				AgentModeKey:                  parsed.Mode,
			}

			createBlockData := wshrpc.CommandCreateBlockData{
				TabId: tabId,
				BlockDef: &waveobj.BlockDef{
					Meta: createMeta,
				},
				Focused: true,
			}

			oref, err := wshclient.CreateBlockCommand(
				wshclient.GetBareRpcClient(), createBlockData, nil,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to create agent terminal: %w", err)
			}

			widgetId := oref.OID[:8]

			return &TermSpawnAgentToolOutput{
				WidgetId: widgetId,
				Status:   "spawned",
				Model:    parsed.Model,
				Mode:     parsed.Mode,
				CLI:      parsed.CLI,
			}, nil
		},
	}
}

// ---------- term_get_agent_status ----------

type TermGetAgentStatusToolInput struct {
	WidgetId string `json:"widget_id"`
}

type TermGetAgentStatusToolOutput struct {
	Status         string `json:"status"`
	ContextPercent int    `json:"context_percent"`
	Model          string `json:"model"`
	Mode           string `json:"mode"`
	ShellState     string `json:"shell_state"`
	LastOutputSec  *int   `json:"last_output_sec"`
}

func parseTermGetAgentStatusInput(input any) (*TermGetAgentStatusToolInput, error) {
	result := &TermGetAgentStatusToolInput{}
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

func GetTermGetAgentStatusToolDefinition(tabId string) uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "term_get_agent_status",
		DisplayName: "Get Agent Status",
		Description: "Check agent metadata and advisory status. For Kilo full-screen TUIs, scrollback may be blank; use capture_screenshot as the source of truth for visible readiness, responses, and completion.",
		ToolLogName: "term:agentstatus",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"widget_id": map[string]any{
					"type":        "string",
					"description": "8-character widget ID of the agent terminal",
				},
			},
			"required":             []string{"widget_id"},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			parsed, err := parseTermGetAgentStatusInput(input)
			if err != nil {
				return fmt.Sprintf("error parsing input: %v", err)
			}
			return fmt.Sprintf("checking agent status for %s", parsed.WidgetId)
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseTermGetAgentStatusInput(input)
			if err != nil {
				return nil, err
			}

			ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelFn()

			fullBlockId, err := wcore.ResolveBlockIdFromPrefix(ctx, tabId, parsed.WidgetId)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve widget %s: %w", parsed.WidgetId, err)
			}

			blockORef := waveobj.MakeORef(waveobj.OType_Block, fullBlockId)
			rtInfo := wstore.GetRTInfo(blockORef)

			blockData, err := wstore.DBGet[*waveobj.Block](ctx, fullBlockId)
			var agentModel, agentMode string
			if err == nil && blockData.Meta != nil {
				if m, ok := blockData.Meta[AgentModelKey].(string); ok {
					agentModel = m
				}
				if m, ok := blockData.Meta[AgentModeKey].(string); ok {
					agentMode = m
				}
				if agentModel == "" {
					if args := blockData.Meta.GetStringList(waveobj.MetaKey_CmdArgs); args != nil {
						for i, arg := range args {
							if arg == "--model" && i+1 < len(args) {
								agentModel = args[i+1]
							}
						}
					}
				}
				if agentMode == "" {
					if envMap := blockData.Meta.GetStringMap(waveobj.MetaKey_CmdEnv, false); envMap != nil {
						agentMode = envMap["AGENT_MODE"]
					}
				}
			}

			scrollback, err := getTermScrollbackOutput(
				tabId,
				parsed.WidgetId,
				wshrpc.CommandTermGetScrollbackLinesData{
					LineStart:   0,
					LineEnd:     100,
					LastCommand: false,
				},
			)
			if err != nil {
				return nil, fmt.Errorf("failed to read agent scrollback: %w", err)
			}

			status := "unknown"
			contextPct := 0
			lines := strings.Split(scrollback.Content, "\n")

			for i := len(lines) - 1; i >= 0; i-- {
				line := strings.ToLower(lines[i])
				if strings.TrimSpace(line) == "" {
					continue
				}
				if strings.Contains(line, "compacting") ||
					strings.Contains(line, "summarizing") {
					status = "compacting"
					if idx := strings.Index(line, "%"); idx > 0 {
						numStart := idx - 1
						for numStart >= 0 && numStart < len(line) && line[numStart] >= '0' && line[numStart] <= '9' {
							numStart--
						}
						if numStart >= 0 && numStart+1 < len(line) {
							if pct, err := strconv.Atoi(line[numStart+1 : idx]); err == nil {
								contextPct = pct
							}
						}
					}
					break
				}
				if strings.Contains(line, "error:") ||
					strings.Contains(line, "failed:") ||
					strings.Contains(line, "panic:") ||
					strings.Contains(line, "fatal:") {
					status = "error"
					break
				}
				if strings.HasSuffix(strings.TrimSpace(lines[i]), ">") ||
					strings.Contains(line, "waiting for input") ||
					strings.Contains(line, "ask anything") ||
					strings.Contains(line, "ctrl+p") {
					status = "idle"
					break
				}
				status = "active"
				break
			}

			shellState := ""
			if rtInfo != nil {
				shellState = rtInfo.ShellState
				if status == "unknown" {
					if shellState == "ready" {
						status = "idle"
					} else if shellState == "running-command" {
						status = "active"
					}
				}
			}

			if status == "unknown" && blockData != nil && blockData.Meta != nil {
				if cmd, ok := blockData.Meta[waveobj.MetaKey_Cmd].(string); ok &&
					strings.EqualFold(cmd, "kilo") {
					// Kilo's alternate-screen TUI can legitimately expose only
					// whitespace to scrollback. The process exists; visual state
					// must be checked with capture_screenshot.
					status = "active"
				}
			}

			return &TermGetAgentStatusToolOutput{
				Status:         status,
				ContextPercent: contextPct,
				Model:          agentModel,
				Mode:           agentMode,
				ShellState:     shellState,
				LastOutputSec:  scrollback.SinceLastOutputSec,
			}, nil
		},
	}
}

// 100ms keeps the wait responsive without hammering the wstore cache, which is
// already fronted by an in-memory map per-oref.
const TermRunCommandPollInterval = 100 * time.Millisecond

type TermRunCommandToolInput struct {
	WidgetId      string `json:"widget_id"`
	Command       string `json:"command"`
	WaitTimeoutMs int    `json:"waittimeoutms,omitempty"`
}

type TermRunCommandToolOutput struct {
	Success  bool   `json:"success"`
	Status   string `json:"status"`
	Output   string `json:"output,omitempty"`
	ExitCode *int   `json:"exitcode,omitempty"`
	TimedOut bool   `json:"timedout"`
	WaitedMs int    `json:"waitedms"`
}

func parseTermRunCommandInput(input any) (*TermRunCommandToolInput, error) {
	result := &TermRunCommandToolInput{}

	if input == nil {
		return nil, fmt.Errorf("input is required")
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
	if result.Command == "" {
		return nil, fmt.Errorf("command is required")
	}

	if result.WaitTimeoutMs == 0 {
		result.WaitTimeoutMs = 30000
	}
	if result.WaitTimeoutMs < 1000 {
		result.WaitTimeoutMs = 1000
	}
	if result.WaitTimeoutMs > 120000 {
		result.WaitTimeoutMs = 120000
	}

	return result, nil
}

func waitForShellReady(ctx context.Context, fullBlockId string, timeoutMs int) (status string, exitCode *int, lastCmd string, waitedMs int, err error) {
	start := time.Now()
	deadline := start.Add(time.Duration(timeoutMs) * time.Millisecond)
	blockORef := waveobj.MakeORef(waveobj.OType_Block, fullBlockId)

	sawNewCommand := false
	initialLastCmd := ""
	if rtInfo := wstore.GetRTInfo(blockORef); rtInfo != nil {
		initialLastCmd = rtInfo.ShellLastCmd
	}

	for {
		rtInfo := wstore.GetRTInfo(blockORef)
		if rtInfo != nil {
			switch rtInfo.ShellState {
			case "ready":
				lastCmd = rtInfo.ShellLastCmd
				if !sawNewCommand {
					return "completed", nil, lastCmd, int(time.Since(start) / time.Millisecond), nil
				}
				if rtInfo.ShellLastCmdExitCode != 0 || rtInfo.ShellLastCmd != initialLastCmd {
					ec := rtInfo.ShellLastCmdExitCode
					return "completed", &ec, lastCmd, int(time.Since(start) / time.Millisecond), nil
				}
			case "running-command":
				sawNewCommand = true
			}
		}

		if time.Now().After(deadline) {
			waited := int(time.Since(start) / time.Millisecond)
			if rtInfo := wstore.GetRTInfo(blockORef); rtInfo != nil {
				lastCmd = rtInfo.ShellLastCmd
				if rtInfo.ShellState == "running-command" {
					return "running", nil, lastCmd, waited, nil
				}
			}
			return "timeout", nil, lastCmd, waited, nil
		}

		select {
		case <-ctx.Done():
			return "timeout", nil, lastCmd, int(time.Since(start) / time.Millisecond), ctx.Err()
		case <-time.After(TermRunCommandPollInterval):
		}
	}
}

func findIdleTerminalBlock(tabId string, excludeBlockId string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	tab, err := wstore.DBGet[*waveobj.Tab](ctx, tabId)
	if err != nil || tab == nil {
		return ""
	}
	for _, blockId := range tab.BlockIds {
		if blockId == excludeBlockId {
			continue
		}
		blockORef := waveobj.MakeORef(waveobj.OType_Block, blockId)
		rtInfo := wstore.GetRTInfo(blockORef)
		if rtInfo != nil && rtInfo.ShellIntegration && rtInfo.ShellState == "ready" {
			return blockId
		}
	}
	return ""
}

func createTerminalBlock(tabId string) (string, error) {
	createBlockData := wshrpc.CommandCreateBlockData{
		TabId: tabId,
		BlockDef: &waveobj.BlockDef{
			Meta: map[string]any{
				waveobj.MetaKey_View:          "term",
				waveobj.MetaKey_Controller:    "cmd",
				waveobj.MetaKey_Cmd:           "pwsh",
				waveobj.MetaKey_CmdShell:      true,
				waveobj.MetaKey_CmdRunOnStart: true,
				waveobj.MetaKey_CmdRunOnce:    true,
			},
		},
		Focused: false,
	}

	oref, err := wshclient.CreateBlockCommand(
		wshclient.GetBareRpcClient(), createBlockData, nil,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create terminal block: %w", err)
	}
	return oref.OID, nil
}

func isTerminalReadyForCommand(_ bool, shellState string) bool {
	return shellState == "ready"
}

func executeTermRunCommand(tabId string, parsed *TermRunCommandToolInput) (*TermRunCommandToolOutput, error) {
	resolveCtx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFn()

	fullBlockId, err := wcore.ResolveBlockIdFromPrefix(resolveCtx, tabId, parsed.WidgetId)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve widget %s: %w", parsed.WidgetId, err)
	}

	blockORef := waveobj.MakeORef(waveobj.OType_Block, fullBlockId)
	rtInfo := wstore.GetRTInfo(blockORef)
	targetBlockId := fullBlockId

	if rtInfo == nil || !isTerminalReadyForCommand(rtInfo.ShellIntegration, rtInfo.ShellState) {
		idleBlockId := findIdleTerminalBlock(tabId, fullBlockId)
		if idleBlockId != "" {
			targetBlockId = idleBlockId
		} else {
			newBlockId, createErr := createTerminalBlock(tabId)
			if createErr != nil {
				return &TermRunCommandToolOutput{
					Success: false,
					Status:  "shell-busy-no-alternate",
					Output:  fmt.Sprintf("Widget %s is busy (shell state: running-command) and failed to create a new terminal: %v", parsed.WidgetId, createErr),
				}, nil
			}
			targetBlockId = newBlockId
			waitReadyCtx, waitReadyCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer waitReadyCancel()
			for {
				newORef := waveobj.MakeORef(waveobj.OType_Block, newBlockId)
				newRtInfo := wstore.GetRTInfo(newORef)
				if newRtInfo != nil && isTerminalReadyForCommand(newRtInfo.ShellIntegration, newRtInfo.ShellState) {
					break
				}
				select {
				case <-waitReadyCtx.Done():
					return &TermRunCommandToolOutput{
						Success: false,
						Status:  "shell-busy-new-terminal-not-ready",
						Output:  fmt.Sprintf("Widget %s is busy. Created terminal %s, but that same terminal did not report ready within 30 seconds. Retry against widget %s instead of creating another terminal.", parsed.WidgetId, newBlockId[:8], newBlockId[:8]),
					}, nil
				case <-time.After(250 * time.Millisecond):
				}
			}
		}
	}

	payloads, err := buildTermSendInputPayloads(parsed.Command, true)
	if err != nil {
		return nil, err
	}

	for idx, payload := range payloads {
		if idx > 0 {
			time.Sleep(75 * time.Millisecond)
		}

		err = wshclient.ControllerInputCommand(
			wshclient.GetBareRpcClient(),
			wshrpc.CommandBlockInputData{
				BlockId:     targetBlockId,
				InputData64: base64.StdEncoding.EncodeToString(payload),
			},
			nil,
		)
		if err != nil {
			if idx == 0 {
				return nil, fmt.Errorf("failed to send command text to terminal: %w", err)
			}
			return nil, fmt.Errorf("failed to send command Enter to terminal: %w", err)
		}
	}

	// Let shell integration observe the transition away from ready before the
	// completion waiter begins, otherwise an instant pre-command ready sample
	// can be mistaken for command completion.
	time.Sleep(150 * time.Millisecond)

	waitCtx, cancelWait := context.WithTimeout(context.Background(), time.Duration(parsed.WaitTimeoutMs)*time.Millisecond)
	defer cancelWait()

	status, exitCode, _, waitedMs, _ := waitForShellReady(waitCtx, targetBlockId, parsed.WaitTimeoutMs)

	usedWidget := targetBlockId[:8]
	out := &TermRunCommandToolOutput{
		Status:   status,
		Output:   usedWidget,
		ExitCode: exitCode,
		TimedOut: status == "timeout",
		WaitedMs: waitedMs,
		Success:  status == "completed",
	}
	return out, nil
}

func GetTermRunCommandToolDefinition(tabId string) uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "term_run_command",
		DisplayName: "Run Command in Terminal",
		Description: "Run a command in a terminal widget and wait for it to finish. " +
			"Sends command text and Enter as separate terminal-controller inputs, then polls ShellState " +
			"until the shell returns to the \"ready\" state (prompt visible). " +
			"If the target terminal is busy, automatically finds an idle terminal in the same tab or creates exactly one terminal " +
			"and waits up to 30 seconds for that same widget. The 'output' field contains the 8-char widget_id used. " +
			"Use this for non-interactive commands on idle shells. For interacting with running TUI apps, use term_send_input instead. " +
			"All tool calls are pre-approved.",
		ToolLogName: "term:runcommand",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"widget_id": map[string]any{
					"type":        "string",
					"description": "8-character widget ID of the terminal widget",
				},
				"command": map[string]any{
					"type":        "string",
					"description": "Command to execute in the terminal. Will be terminated with a newline.",
				},
				"waittimeoutms": map[string]any{
					"type":        "integer",
					"description": "Maximum number of milliseconds to wait for the command to finish. Default 30000, min 1000, max 120000.",
				},
			},
			"required":             []string{"widget_id", "command"},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			parsed, err := parseTermRunCommandInput(input)
			if err != nil {
				return fmt.Sprintf("error parsing input: %v", err)
			}

			cmdPreview := parsed.Command
			if len(cmdPreview) > 50 {
				cmdPreview = cmdPreview[:47] + "..."
			}
			return fmt.Sprintf("running %q in terminal %s (timeout %dms)", cmdPreview, parsed.WidgetId, parsed.WaitTimeoutMs)
		},
		ToolProgressDesc: func(input any) ([]string, error) {
			parsed, err := parseTermRunCommandInput(input)
			if err != nil {
				return nil, err
			}
			cmdPreview := parsed.Command
			if len(cmdPreview) > 40 {
				cmdPreview = cmdPreview[:37] + "..."
			}
			return []string{fmt.Sprintf("running %q in %s", cmdPreview, parsed.WidgetId)}, nil
		},
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseTermRunCommandInput(input)
			if err != nil {
				return nil, err
			}
			return executeTermRunCommand(tabId, parsed)
		},
	}
}

func parseTermSendKeyInput(input any) (*SendKeyInput, error) {
	result := &SendKeyInput{}
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}
	if result.WidgetId == "" {
		return nil, fmt.Errorf("widget_id is required")
	}
	if result.Key == "" {
		return nil, fmt.Errorf("key is required")
	}
	if _, _, _, err := ResolvedSendKeySequence(result.Key); err != nil {
		return nil, err
	}
	return result, nil
}
