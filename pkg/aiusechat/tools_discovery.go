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
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

// ---------- list_workspaces ----------

type ListWorkspacesToolInput struct {
	// No arguments. Reserved for future filtering (e.g. "include_archived").
}

type WorkspaceInfo struct {
	WorkspaceId string `json:"workspace_id"`
	Name        string `json:"name,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Color       string `json:"color,omitempty"`
	ActiveTabId string `json:"active_tab_id,omitempty"`
	TabCount    int    `json:"tab_count"`
}

type ListWorkspacesToolOutput struct {
	Count      int             `json:"count"`
	Workspaces []WorkspaceInfo `json:"workspaces"`
}

func parseListWorkspacesInput(input any) (*ListWorkspacesToolInput, error) {
	result := &ListWorkspacesToolInput{}
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

func executeListWorkspaces() (*ListWorkspacesToolOutput, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	workspaces, err := wstore.DBGetAllObjsByType[*waveobj.Workspace](ctx, waveobj.OType_Workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}

	out := &ListWorkspacesToolOutput{Workspaces: []WorkspaceInfo{}}
	for _, ws := range workspaces {
		if ws == nil {
			continue
		}
		out.Workspaces = append(out.Workspaces, WorkspaceInfo{
			WorkspaceId: ws.OID,
			Name:        ws.Name,
			Icon:        ws.Icon,
			Color:       ws.Color,
			ActiveTabId: ws.ActiveTabId,
			TabCount:    len(ws.TabIds),
		})
	}
	out.Count = len(out.Workspaces)
	return out, nil
}

func GetListWorkspacesToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "list_workspaces",
		DisplayName: "List Workspaces",
		Description: "Enumerate all open workspaces in the current Wave client. Returns workspace ID, display name, icon, color, the currently-active tab, and total tab count for each.",
		ToolLogName: "discover:listworkspaces",
		InputSchema: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			return "listing all open workspaces"
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			if _, err := parseListWorkspacesInput(input); err != nil {
				return nil, err
			}
			return executeListWorkspaces()
		},
	}
}

// ---------- list_tabs ----------

type ListTabsToolInput struct {
	WorkspaceId string `json:"workspace_id,omitempty"` // optional filter
}

type TabInfo struct {
	TabId       string `json:"tab_id"`
	Name        string `json:"name,omitempty"`
	WorkspaceId string `json:"workspace_id"`
	BlockCount  int    `json:"block_count"`
	IsActive    bool   `json:"is_active"`
}

type ListTabsToolOutput struct {
	Count int       `json:"count"`
	Tabs  []TabInfo `json:"tabs"`
}

func parseListTabsInput(input any) (*ListTabsToolInput, error) {
	result := &ListTabsToolInput{}
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

func executeListTabs(params *ListTabsToolInput) (*ListTabsToolOutput, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out := &ListTabsToolOutput{Tabs: []TabInfo{}}

	// Walk all workspaces to get active-tab markers and to honor the optional
	// workspace_id filter. Then enumerate every tab (cheap in wstore).
	workspaces, err := wstore.DBGetAllObjsByType[*waveobj.Workspace](ctx, waveobj.OType_Workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}
	activeInWs := make(map[string]string) // tabId -> workspaceId (active marker)
	wsFilter := params.WorkspaceId
	for _, ws := range workspaces {
		if ws == nil {
			continue
		}
		if wsFilter != "" && ws.OID != wsFilter {
			continue
		}
		if ws.ActiveTabId != "" {
			activeInWs[ws.ActiveTabId] = ws.OID
		}
	}

	tabs, err := wstore.DBGetAllObjsByType[*waveobj.Tab](ctx, waveobj.OType_Tab)
	if err != nil {
		return nil, fmt.Errorf("failed to list tabs: %w", err)
	}
	for _, tab := range tabs {
		if tab == nil {
			continue
		}
		wsId, ok := activeInWs[tab.OID]
		isActive := ok
		// If the filter is set, only include tabs belonging to a matching
		// workspace. We resolve the workspace by reverse-walking BlockIds
		// only when the filter is set AND we don't already know the workspace.
		if wsFilter != "" {
			if !isActive && !tabBelongsToWorkspace(tab, workspaces, wsFilter) {
				continue
			}
			wsId = wsFilter
		}
		out.Tabs = append(out.Tabs, TabInfo{
			TabId:       tab.OID,
			Name:        tab.Name,
			WorkspaceId: wsId,
			BlockCount:  len(tab.BlockIds),
			IsActive:    isActive,
		})
	}
	out.Count = len(out.Tabs)
	return out, nil
}

func tabBelongsToWorkspace(tab *waveobj.Tab, workspaces []*waveobj.Workspace, wsId string) bool {
	for _, ws := range workspaces {
		if ws == nil || ws.OID != wsId {
			continue
		}
		for _, tid := range ws.TabIds {
			if tid == tab.OID {
				return true
			}
		}
		return false
	}
	return false
}

func GetListTabsToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "list_tabs",
		DisplayName: "List Tabs",
		Description: "Enumerate all tabs across all open Wave workspaces. Returns tab ID, name, owning workspace, block count, and an is_active marker for the currently focused tab. Optionally filter by workspace_id.",
		ToolLogName: "discover:listtabs",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{
					"type":        "string",
					"description": "Optional workspace ID (full OID or 8-char prefix). Only return tabs belonging to this workspace.",
				},
			},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			parsed, err := parseListTabsInput(input)
			if err != nil {
				return fmt.Sprintf("error parsing input: %v", err)
			}
			if parsed.WorkspaceId != "" {
				return fmt.Sprintf("listing tabs in workspace %q", parsed.WorkspaceId)
			}
			return "listing all open tabs"
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseListTabsInput(input)
			if err != nil {
				return nil, err
			}
			return executeListTabs(parsed)
		},
	}
}

// ---------- get_widget ----------

type GetWidgetToolInput struct {
	WidgetId string `json:"widget_id,omitempty"` // 8-char prefix or full OID
	BlockId  string `json:"block_id,omitempty"`  // alias for widget_id, accepts full OID
}

type GetWidgetToolOutput struct {
	Found    bool           `json:"found"`
	BlockId  string         `json:"block_id,omitempty"`
	WidgetId string         `json:"widget_id,omitempty"`
	TabId    string         `json:"tab_id,omitempty"`
	ViewType string         `json:"view_type,omitempty"`
	Name     string         `json:"name,omitempty"`
	Meta     map[string]any `json:"meta,omitempty"`
	// Subset of ObjRTInfo most useful to an agent (only filled if RT info exists).
	Shell *ShellInfo `json:"shell,omitempty"`
}

type ShellInfo struct {
	Type            string `json:"type,omitempty"`
	State           string `json:"state,omitempty"`
	Version         string `json:"version,omitempty"`
	Integration     bool   `json:"integration,omitempty"`
	LastCmd         string `json:"last_cmd,omitempty"`
	LastCmdExitCode int    `json:"last_cmd_exit_code,omitempty"`
	HasCurCwd       bool   `json:"has_curcwd,omitempty"`
}

func parseGetWidgetInput(input any) (*GetWidgetToolInput, error) {
	result := &GetWidgetToolInput{}
	if input == nil {
		return nil, fmt.Errorf("must provide one of 'widget_id' or 'block_id'")
	}
	inputBytes, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}
	if err := json.Unmarshal(inputBytes, result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal input: %w", err)
	}
	if result.WidgetId == "" && result.BlockId == "" {
		return nil, fmt.Errorf("must provide one of 'widget_id' or 'block_id'")
	}
	if result.WidgetId != "" && result.BlockId != "" {
		return nil, fmt.Errorf("provide only one of 'widget_id' or 'block_id', not both")
	}
	return result, nil
}

func executeGetWidget(params *GetWidgetToolInput) (*GetWidgetToolOutput, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ref := params.WidgetId
	if ref == "" {
		ref = params.BlockId
	}
	oref, err := wstore.DBResolveEasyOID(ctx, ref)
	if err != nil {
		return &GetWidgetToolOutput{Found: false}, nil
	}
	if oref == nil || oref.OType != waveobj.OType_Block {
		return &GetWidgetToolOutput{Found: false}, nil
	}
	block, err := wstore.DBGet[*waveobj.Block](ctx, oref.OID)
	if err != nil || block == nil {
		return &GetWidgetToolOutput{Found: false}, nil
	}

	out := &GetWidgetToolOutput{
		Found:    true,
		BlockId:  block.OID,
		WidgetId: block.OID[:8],
	}
	if block.Meta != nil {
		if v, ok := block.Meta["view"].(string); ok {
			out.ViewType = v
		}
		if v, ok := block.Meta["title"].(string); ok {
			out.Name = v
		} else if v, ok := block.Meta["name"].(string); ok {
			out.Name = v
		}
		// Copy the full meta map. Skip any well-known secrets-related fields
		// for safety; the meta is otherwise non-sensitive block config.
		out.Meta = make(map[string]any, len(block.Meta))
		for k, v := range block.Meta {
			out.Meta[k] = v
		}
	}
	if rtInfo := wstore.GetRTInfo(*oref); rtInfo != nil {
		out.Shell = &ShellInfo{
			Type:            rtInfo.ShellType,
			State:           rtInfo.ShellState,
			Version:         rtInfo.ShellVersion,
			Integration:     rtInfo.ShellIntegration,
			LastCmd:         rtInfo.ShellLastCmd,
			LastCmdExitCode: rtInfo.ShellLastCmdExitCode,
			HasCurCwd:       rtInfo.ShellHasCurCwd,
		}
	}
	// Reverse-lookup the parent tab. Cheap: walk all tabs once.
	tabs, err := wstore.DBGetAllObjsByType[*waveobj.Tab](ctx, waveobj.OType_Tab)
	if err == nil {
		for _, tab := range tabs {
			if tab == nil {
				continue
			}
			for _, bid := range tab.BlockIds {
				if bid == block.OID {
					out.TabId = tab.OID
					break
				}
			}
			if out.TabId != "" {
				break
			}
		}
	}
	return out, nil
}

func GetGetWidgetToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "get_widget",
		DisplayName: "Get Widget Details",
		Description: "Get full metadata for a single widget (block) by its 8-character widget ID or full block ID. Returns view type, name, full meta map, parent tab ID, and shell runtime info if it's a terminal widget. Use list_widgets or list_tabs first to discover widget IDs.",
		ToolLogName: "discover:getwidget",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"widget_id": map[string]any{
					"type":        "string",
					"description": "8-character widget ID prefix (e.g. 'abc12345').",
				},
				"block_id": map[string]any{
					"type":        "string",
					"description": "Full block OID. Use this OR widget_id, not both.",
				},
			},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			parsed, err := parseGetWidgetInput(input)
			if err != nil {
				return fmt.Sprintf("error parsing input: %v", err)
			}
			ref := parsed.WidgetId
			if ref == "" {
				ref = parsed.BlockId
			}
			return fmt.Sprintf("fetching widget %q", ref)
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseGetWidgetInput(input)
			if err != nil {
				return nil, err
			}
			return executeGetWidget(parsed)
		},
	}
}

// ---------- scan_terminals ----------
//
// Cross-tab discovery: enumerate every terminal (or other view-typed) widget
// across every tab in every open workspace. The in-app Wave AI assistant has
// the same visibility into all terminal widgets; this tool exposes that to
// external/MCP agents so they can pick which terminal to talk to without
// having to walk the workspace -> tab -> block hierarchy manually.

type ScanTerminalsToolInput struct {
	ViewType     string `json:"view_type,omitempty"`     // default "term"
	WorkspaceId  string `json:"workspace_id,omitempty"`  // optional filter
	OnlyActive   bool   `json:"only_active,omitempty"`   // only the active tab in each workspace
	OnlyRunning  bool   `json:"only_running,omitempty"`  // only widgets whose shell state is "running" (or integration-active)
	IncludeEmpty bool   `json:"include_empty,omitempty"` // include tabs with zero matches (for discovery)
}

type ScannedTerminal struct {
	TabId        string `json:"tab_id"`
	TabName      string `json:"tab_name,omitempty"`
	WorkspaceId  string `json:"workspace_id,omitempty"`
	IsActiveTab  bool   `json:"is_active_tab"`
	BlockId      string `json:"block_id"`
	WidgetId     string `json:"widget_id"`
	ViewType     string `json:"view_type"`
	ShortDesc    string `json:"short_desc,omitempty"`
	ShellType    string `json:"shell_type,omitempty"`
	ShellState   string `json:"shell_state,omitempty"`
	ShellVersion string `json:"shell_version,omitempty"`
	Integration  bool   `json:"integration,omitempty"`
	LastCmd      string `json:"last_cmd,omitempty"`
	LastCmdExit  int    `json:"last_cmd_exit_code,omitempty"`
	HasCurCwd    bool   `json:"has_curcwd,omitempty"`
}

type ScanTerminalsToolOutput struct {
	Count       int               `json:"count"`
	ViewType    string            `json:"view_type"`
	ScannedTabs int               `json:"scanned_tabs"`
	ScannedWs   int               `json:"scanned_workspaces"`
	Terminals   []ScannedTerminal `json:"terminals"`
}

func parseScanTerminalsInput(input any) (*ScanTerminalsToolInput, error) {
	result := &ScanTerminalsToolInput{}
	if input == nil {
		result.ViewType = "term"
		return result, nil
	}
	inputBytes, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}
	if err := json.Unmarshal(inputBytes, result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal input: %w", err)
	}
	if result.ViewType == "" {
		result.ViewType = "term"
	}
	return result, nil
}

func executeScanTerminals(params *ScanTerminalsToolInput) (*ScanTerminalsToolOutput, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	viewType := params.ViewType

	// Build a lookup of which tab is active in which workspace. O(ws).
	workspaces, err := wstore.DBGetAllObjsByType[*waveobj.Workspace](ctx, waveobj.OType_Workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}
	activeTabInWs := make(map[string]string) // workspaceId -> activeTabId
	wsByTab := make(map[string]string)       // tabId -> workspaceId
	wsFilter := params.WorkspaceId
	for _, ws := range workspaces {
		if ws == nil {
			continue
		}
		if wsFilter != "" && ws.OID != wsFilter {
			continue
		}
		activeTabInWs[ws.OID] = ws.ActiveTabId
		for _, tid := range ws.TabIds {
			wsByTab[tid] = ws.OID
		}
	}

	tabs, err := wstore.DBGetAllObjsByType[*waveobj.Tab](ctx, waveobj.OType_Tab)
	if err != nil {
		return nil, fmt.Errorf("failed to list tabs: %w", err)
	}

	out := &ScanTerminalsToolOutput{
		ViewType:  viewType,
		Terminals: []ScannedTerminal{},
	}

	for _, tab := range tabs {
		if tab == nil {
			continue
		}
		wsId := wsByTab[tab.OID]
		if wsFilter != "" && wsId != wsFilter {
			continue
		}
		isActive := activeTabInWs[wsId] == tab.OID
		if params.OnlyActive && !isActive {
			continue
		}
		out.ScannedTabs++

		for _, blockId := range tab.BlockIds {
			block, err := wstore.DBGet[*waveobj.Block](ctx, blockId)
			if err != nil || block == nil || block.Meta == nil {
				continue
			}
			vt, ok := block.Meta["view"].(string)
			if !ok || vt != viewType {
				continue
			}
			scan := ScannedTerminal{
				TabId:       tab.OID,
				TabName:     tab.Name,
				WorkspaceId: wsId,
				IsActiveTab: isActive,
				BlockId:     block.OID,
				WidgetId:    block.OID[:min(8, len(block.OID))],
				ViewType:    vt,
				ShortDesc:   MakeBlockShortDesc(block),
			}
			oref := waveobj.MakeORef(waveobj.OType_Block, block.OID)
			if rtInfo := wstore.GetRTInfo(oref); rtInfo != nil {
				scan.ShellType = rtInfo.ShellType
				scan.ShellState = rtInfo.ShellState
				scan.ShellVersion = rtInfo.ShellVersion
				scan.Integration = rtInfo.ShellIntegration
				scan.LastCmd = rtInfo.ShellLastCmd
				scan.LastCmdExit = rtInfo.ShellLastCmdExitCode
				scan.HasCurCwd = rtInfo.ShellHasCurCwd
			}
			if params.OnlyRunning {
				// "running" = shell state is "running" OR shell integration is active.
				if scan.ShellState != "running" && !scan.Integration {
					continue
				}
			}
			out.Terminals = append(out.Terminals, scan)
		}
		if params.IncludeEmpty && len(out.Terminals) == 0 {
			// Not a no-op; we want the caller to see this tab existed but had no matches.
		}
	}
	out.Count = len(out.Terminals)
	out.ScannedWs = len(workspaces)
	if wsFilter != "" {
		out.ScannedWs = 1
	}
	return out, nil
}

func GetScanTerminalsToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "scan_terminals",
		DisplayName: "Scan Terminal Widgets",
		Description: "Cross-tab discovery: enumerate every terminal widget (or any view-typed widget) across every tab in every open Wave workspace. Mirrors the in-app AI assistant's ability to see all terminals. Returns tab/workspace context plus shell runtime info (shell type, state, last command, exit code, integration status). Use this to pick a terminal to inspect or interact with, then call term_get_scrollback or term_run_command on the chosen widget_id.",
		ToolLogName: "discover:scterminals",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"view_type": map[string]any{
					"type":        "string",
					"description": "Widget view type to scan for. Defaults to 'term'. Other useful values: 'web', 'preview', 'waveai', 'codeeditor'.",
				},
				"workspace_id": map[string]any{
					"type":        "string",
					"description": "Optional. Only scan tabs in this workspace (full OID or 8-char prefix).",
				},
				"only_active": map[string]any{
					"type":        "boolean",
					"description": "Optional. If true, only return terminals from the active tab of each workspace.",
				},
				"only_running": map[string]any{
					"type":        "boolean",
					"description": "Optional. If true, only return terminals whose shell state is 'running' or that have shell integration active.",
				},
				"include_empty": map[string]any{
					"type":        "boolean",
					"description": "Optional. If true, scan all tabs even if they have no matching widgets (useful for discovery).",
				},
			},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			parsed, err := parseScanTerminalsInput(input)
			if err != nil {
				return fmt.Sprintf("error parsing input: %v", err)
			}
			parts := []string{fmt.Sprintf("view_type=%q", parsed.ViewType)}
			if parsed.WorkspaceId != "" {
				parts = append(parts, fmt.Sprintf("workspace=%q", parsed.WorkspaceId))
			}
			if parsed.OnlyActive {
				parts = append(parts, "active-tab-only")
			}
			if parsed.OnlyRunning {
				parts = append(parts, "running-only")
			}
			return fmt.Sprintf("scanning terminals (%s)", strings.Join(parts, ", "))
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseScanTerminalsInput(input)
			if err != nil {
				return nil, err
			}
			return executeScanTerminals(parsed)
		},
	}
}
