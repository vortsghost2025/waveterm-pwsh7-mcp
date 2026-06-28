// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"context"
	"fmt"
	"log"
	"os/user"
	"strings"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/aiusechat/aiutil"
	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
	"github.com/wavetermdev/waveterm/pkg/blockcontroller"
	"github.com/wavetermdev/waveterm/pkg/util/utilfn"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

func makeTerminalBlockDesc(block *waveobj.Block) string {
	connection, hasConnection := block.Meta["connection"].(string)
	cwd, hasCwd := block.Meta["cmd:cwd"].(string)

	blockORef := waveobj.MakeORef(waveobj.OType_Block, block.OID)
	rtInfo := wstore.GetRTInfo(blockORef)
	hasCurCwd := rtInfo != nil && rtInfo.ShellHasCurCwd

	var desc string
	if hasConnection && connection != "" {
		desc = fmt.Sprintf("CLI terminal connected to %q", connection)
	} else {
		desc = "local CLI terminal"
	}

	if rtInfo != nil && rtInfo.ShellType != "" {
		desc += fmt.Sprintf(" (%s", rtInfo.ShellType)
		if rtInfo.ShellVersion != "" {
			desc += fmt.Sprintf(" %s", rtInfo.ShellVersion)
		}
		desc += ")"
	}

	if rtInfo != nil {
		if rtInfo.ShellIntegration {
			var stateStr string
			switch rtInfo.ShellState {
			case "ready":
				stateStr = "waiting for input"
			case "running-command":
				stateStr = "running command"
				if rtInfo.ShellLastCmd != "" {
					cmdStr := rtInfo.ShellLastCmd
					if len(cmdStr) > 30 {
						cmdStr = cmdStr[:27] + "..."
					}
					cmdJSON := utilfn.MarshalJSONString(cmdStr)
					stateStr = fmt.Sprintf("running command %s", cmdJSON)
				}
			default:
				stateStr = "state unknown"
			}
			desc += fmt.Sprintf(", %s", stateStr)
		} else {
			desc += ", no shell integration"
		}
	}

	if hasCurCwd && hasCwd && cwd != "" {
		desc += fmt.Sprintf(", in directory %q", cwd)
	}

	return desc
}

func MakeBlockShortDesc(block *waveobj.Block) string {
	if block.Meta == nil {
		return ""
	}

	viewType, ok := block.Meta["view"].(string)
	if !ok {
		return ""
	}

	switch viewType {
	case "term":
		return makeTerminalBlockDesc(block)
	case "preview":
		file, hasFile := block.Meta["file"].(string)
		connection, hasConnection := block.Meta["connection"].(string)

		if hasConnection && connection != "" {
			if hasFile && file != "" {
				return fmt.Sprintf("preview widget viewing %q on %q", file, connection)
			}
			return fmt.Sprintf("preview widget viewing files on %q", connection)
		}
		if hasFile && file != "" {
			return fmt.Sprintf("preview widget viewing %q", file)
		}
		return "file and directory preview widget"
	case "web":
		if url, hasUrl := block.Meta["url"].(string); hasUrl && url != "" {
			return fmt.Sprintf("web browser widget pointing at %q", url)
		}
		return "web browser widget"
	case "waveai":
		return "AI chat widget"
	case "cpuplot":
		if connection, hasConnection := block.Meta["connection"].(string); hasConnection && connection != "" {
			return fmt.Sprintf("cpu graph for %q", connection)
		}
		return "cpu graph"
	case "tips":
		return "Wave quick tips widget"
	case "help":
		return "Wave documentation widget"
	case "launcher":
		return "placeholder widget used to launch other widgets"
	case "tsunami":
		return handleTsunamiBlockDesc(block)
	case "aifilediff":
		return "" // AI doesn't need to see these
	case "waveconfig":
		if file, hasFile := block.Meta["file"].(string); hasFile && file != "" {
			return fmt.Sprintf("wave config editor for %q", file)
		}
		return "wave config editor"
	default:
		return fmt.Sprintf("unknown widget with type %q", viewType)
	}
}

func GenerateTabStateAndTools(ctx context.Context, tabid string, widgetAccess bool, chatOpts *uctypes.WaveChatOpts) (string, []uctypes.ToolDefinition, error) {
	if tabid == "" {
		return "", nil, nil
	}
	var blocks []*waveobj.Block
	if widgetAccess {
		if _, err := uuid.Parse(tabid); err != nil {
			return "", nil, fmt.Errorf("tabid must be a valid UUID")
		}

		tabObj, err := wstore.DBMustGet[*waveobj.Tab](ctx, tabid)
		if err != nil {
			return "", nil, fmt.Errorf("error getting tab: %v", err)
		}

		for _, blockId := range tabObj.BlockIds {
			block, err := wstore.DBGet[*waveobj.Block](ctx, blockId)
			if err != nil {
				continue
			}
			blocks = append(blocks, block)
		}
	}
	tabState := GenerateCurrentTabStatePrompt(blocks, widgetAccess)
	// for debugging
	// log.Printf("TABPROMPT %s\n", tabState)
	var tools []uctypes.ToolDefinition
	if widgetAccess {
		// Only add screenshot tool for:
		// - openai-responses API type
		// - google-gemini API type with Gemini 3+ models
		if chatOpts.Config.APIType == uctypes.APIType_OpenAIResponses ||
			(chatOpts.Config.APIType == uctypes.APIType_GoogleGemini && aiutil.GeminiSupportsImageToolResults(chatOpts.Config.Model)) {
			tools = append(tools, GetCaptureScreenshotToolDefinition(tabid))
		}
		tools = append(tools, GetReadTextFileToolDefinition())
		tools = append(tools, GetReadDirToolDefinition())
		tools = append(tools, GetWriteTextFileToolDefinition())
		tools = append(tools, GetEditTextFileToolDefinition())
		tools = append(tools, GetWebFetchToolDefinition())
		tools = append(tools, GetWebSearchToolDefinition())
		tools = append(tools, GetDeleteTextFileToolDefinition())
		tools = append(tools, GetBridgeReadInboxToolDefinition())
		tools = append(tools, GetBridgeWriteReplyToolDefinition())
		tools = append(tools, GetAISelfIntroToolDefinition())
		tools = append(tools, GetRunCommandToolDefinition())
		tools = append(tools, GetRunInteractiveCommandToolDefinition())
		tools = append(tools, GetGrepToolDefinition())
		tools = append(tools, GetGlobToolDefinition())
		tools = append(tools, GetCodebaseSearchToolDefinition())
		tools = append(tools, GetTermListWidgetsToolDefinition(tabid))
		tools = append(tools, GetListWorkspacesToolDefinition())
		tools = append(tools, GetListTabsToolDefinition())
		tools = append(tools, GetNotePutToolDefinition())
		tools = append(tools, GetNoteGetToolDefinition())
		tools = append(tools, GetNoteListToolDefinition())
		tools = append(tools, GetNoteDeleteToolDefinition())
		tools = append(tools, GetNoteSearchToolDefinition())
		tools = append(tools, GetNoteDeleteManyToolDefinition())
		tools = append(tools, GetNoteDeleteByScopeToolDefinition())
		tools = append(tools, GetSysInfoToolDefinition())
		tools = append(tools, GetSysEnvToolDefinition())
		tools = append(tools, GetGetWidgetToolDefinition())
		tools = append(tools, GetScanTerminalsToolDefinition())
		viewTypes := make(map[string]bool)
		for _, block := range blocks {
			if block.Meta == nil {
				continue
			}
			viewType, ok := block.Meta["view"].(string)
			if !ok {
				continue
			}
			viewTypes[viewType] = true
			if viewType == "tsunami" {
				blockTools := generateToolsForTsunamiBlock(block)
				tools = append(tools, blockTools...)
			}
		}
		if viewTypes["term"] {
			tools = append(tools, GetTermGetScrollbackToolDefinition(tabid))
			tools = append(tools, GetTermSendInputToolDefinition(tabid))
		tools = append(tools, GetTermSendKeyToolDefinition(tabid))
			tools = append(tools, GetTermRunCommandToolDefinition(tabid))
			tools = append(tools, GetTermSpawnAgentToolDefinition(tabid))
			tools = append(tools, GetTermGetAgentStatusToolDefinition(tabid))
			tools = append(tools, GetTermSearchScrollbackToolDefinition(tabid))
			tools = append(tools, GetWidgetClearScrollbackToolDefinition(tabid))
			// tools = append(tools, GetTermCommandOutputToolDefinition(tabid))
		}

		hasScrollback := false
		hasSendInput := false
		for _, tool := range tools {
			if tool.Name == "term_get_scrollback" {
				hasScrollback = true
			}
			if tool.Name == "term_send_input" {
				hasSendInput = true
			}
		}
		log.Printf("[AIUSECHAT TERMTOOLS CHECK] tab=%s term_get_scrollback=%v term_send_input=%v total_tools=%d",
			tabid,
			hasScrollback,
			hasSendInput,
			len(tools),
		)
		if viewTypes["web"] {
			tools = append(tools, GetWebNavigateToolDefinition(tabid))
		}
	}
	tools = append(tools, GetToolListToolDefinition(chatOpts))
	tools = append(tools, GetToolSchemaToolDefinition(chatOpts))
	tools = append(tools, GetAuditQueryToolDefinition())
	tools = append(tools, GetAuditTailToolDefinition())
	return tabState, tools, nil
}

func GenerateCurrentTabStatePrompt(blocks []*waveobj.Block, widgetAccess bool) string {
	if !widgetAccess {
		return `<current_tab_state>The user has chosen not to share widget context with you</current_tab_state>`
	}
	var widgetDescriptions []string
	for _, block := range blocks {
		desc := MakeBlockShortDesc(block)
		if desc == "" {
			continue
		}
		blockIdPrefix := block.OID[:8]
		fullDesc := fmt.Sprintf("(%s) %s", blockIdPrefix, desc)
		widgetDescriptions = append(widgetDescriptions, fullDesc)
	}

	var prompt strings.Builder
	prompt.WriteString("<current_tab_state>\n")
	systemInfo := wavebase.GetSystemSummary()
	if currentUser, err := user.Current(); err == nil && currentUser.Username != "" {
		prompt.WriteString(fmt.Sprintf("Local Machine: %s, User: %s\n", systemInfo, currentUser.Username))
	} else {
		prompt.WriteString(fmt.Sprintf("Local Machine: %s\n", systemInfo))
	}
	if len(widgetDescriptions) == 0 {
		prompt.WriteString("No widgets open\n")
	} else {
		prompt.WriteString("Open Widgets:\n")
		for _, desc := range widgetDescriptions {
			prompt.WriteString("* ")
			prompt.WriteString(desc)
			prompt.WriteString("\n")
		}
	}
	prompt.WriteString("</current_tab_state>")
	rtn := prompt.String()
	return rtn
}

func generateToolsForTsunamiBlock(block *waveobj.Block) []uctypes.ToolDefinition {
	var tools []uctypes.ToolDefinition

	status := blockcontroller.GetBlockControllerRuntimeStatus(block.OID)
	if status == nil || status.ShellProcStatus != blockcontroller.Status_Running || status.TsunamiPort <= 0 {
		return nil
	}

	blockORef := waveobj.MakeORef(waveobj.OType_Block, block.OID)
	rtInfo := wstore.GetRTInfo(blockORef)

	if tool := GetTsunamiGetDataToolDefinition(block, rtInfo, status); tool != nil {
		tools = append(tools, *tool)
	}
	if tool := GetTsunamiGetConfigToolDefinition(block, rtInfo, status); tool != nil {
		tools = append(tools, *tool)
	}
	if tool := GetTsunamiSetConfigToolDefinition(block, rtInfo, status); tool != nil {
		tools = append(tools, *tool)
	}

	return tools
}

// Used for internal testing of tool loops
func GetAdderToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "adder",
		DisplayName: "Adder",
		Description: "Add an array of numbers together and return their sum",
		ToolLogName: "gen:adder",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"values": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "integer",
					},
					"description": "Array of numbers to add together",
				},
			},
			"required":             []string{"values"},
			"additionalProperties": false,
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			inputMap, ok := input.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid input format")
			}

			valuesInterface, ok := inputMap["values"]
			if !ok {
				return nil, fmt.Errorf("missing values parameter")
			}

			valuesSlice, ok := valuesInterface.([]any)
			if !ok {
				return nil, fmt.Errorf("values must be an array")
			}

			if len(valuesSlice) == 0 {
				return 0, nil
			}

			sum := 0
			for i, val := range valuesSlice {
				floatVal, ok := val.(float64)
				if !ok {
					return nil, fmt.Errorf("value at index %d is not a number", i)
				}
				sum += int(floatVal)
			}

			return sum, nil
		},
	}
}

func GetToolListToolDefinition(chatOpts *uctypes.WaveChatOpts) uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "tool_list",
		DisplayName: "List Available Tools",
		Description: "Return a list of all available tools with their names and descriptions. Use this to discover what tools you can call.",
		ToolLogName: "gen:tool_list",
		Strict:      true,
		InputSchema: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"required":             []string{},
			"additionalProperties": false,
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			tools := chatOpts.Tools
			if chatOpts.TabTools != nil {
				tools = append(tools, chatOpts.TabTools...)
			}
			list := make([]map[string]any, 0, len(tools))
			for _, tool := range tools {
				if tool.Name == "tool_list" || tool.Name == "tool_schema" {
					continue
				}
				list = append(list, map[string]any{
					"name":        tool.Name,
					"description": tool.Description,
				})
			}
			return map[string]any{"tools": list}, nil
		},
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolVerifyInput: func(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
			return nil
		},
	}
}

func GetToolSchemaToolDefinition(chatOpts *uctypes.WaveChatOpts) uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "tool_schema",
		DisplayName: "Get Tool Schema",
		Description: "Return the full input schema for a named tool. Use tool_list first to discover available tools.",
		ToolLogName: "gen:tool_schema",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Name of the tool to get the schema for",
				},
			},
			"required":             []string{"name"},
			"additionalProperties": false,
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseToolSchemaInput(input)
			if err != nil {
				return nil, err
			}
			tools := chatOpts.Tools
			if chatOpts.TabTools != nil {
				tools = append(tools, chatOpts.TabTools...)
			}
			for _, tool := range tools {
				if tool.Name == parsed.Name {
					return map[string]any{
						"name":        tool.Name,
						"description": tool.Description,
						"input_schema": tool.InputSchema,
					}, nil
				}
			}
			return nil, fmt.Errorf("tool %q not found", parsed.Name)
		},
		ToolApproval: func(input any) string {
			return uctypes.ApprovalAutoApproved
		},
		ToolVerifyInput: func(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
			_, err := parseToolSchemaInput(input)
			return err
		},
	}
}

type toolSchemaInput struct {
	Name string `json:"name"`
}

func parseToolSchemaInput(input any) (*toolSchemaInput, error) {
	result := &toolSchemaInput{}
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}
	if result.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	return result, nil
}
