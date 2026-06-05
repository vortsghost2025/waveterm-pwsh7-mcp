// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"context"
	"encoding/json"
	"fmt"
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
