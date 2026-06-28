// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wstore"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
)

var aiScanCmd = &cobra.Command{
	Use:     "scan-terminals",
	Short:   "scan all terminal widgets across workspaces",
	Args:    cobra.NoArgs,
	RunE:    aiScanTerminalsRun,
	PreRunE: preRunSetupRpcClient,
}

func init() {
	aiCmd.AddCommand(aiScanCmd)
}

func aiScanTerminalsRun(cmd *cobra.Command, args []string) error {
	entries, err := wshclient.BlocksListCommand(RpcClient, wshrpc.BlocksListRequest{}, &wshrpc.RpcOpts{Timeout: 10000})
	if err != nil {
		return fmt.Errorf("failed to list blocks: %w", err)
	}

	type terminalInfo struct {
		WidgetID    string `json:"widget_id"`
		BlockID     string `json:"block_id"`
		ShellType   string `json:"shell_type,omitempty"`
		ShellState  string `json:"shell_state,omitempty"`
		LastCmd     string `json:"last_cmd,omitempty"`
		Integration bool   `json:"integration,omitempty"`
		HasCurCwd   bool   `json:"has_curcwd,omitempty"`
		ShortDesc   string `json:"short_desc,omitempty"`
		WorkspaceID string `json:"workspace_id,omitempty"`
		TabID       string `json:"tab_id,omitempty"`
	}

	result := struct {
		Terminals []terminalInfo `json:"terminals"`
	}{Terminals: []terminalInfo{}}

	for _, b := range entries {
		if b.Meta == nil {
			continue
		}
		vt, ok := b.Meta[waveobj.MetaKey_View].(string)
		if !ok || vt != "term" {
			continue
		}

		oref := waveobj.MakeORef(waveobj.OType_Block, b.BlockId)
		var shellType, shellState, lastCmd string
		var integration, hasCurCwd bool

		if rtInfo := wstore.GetRTInfo(oref); rtInfo != nil {
			shellType = rtInfo.ShellType
			shellState = rtInfo.ShellState
			lastCmd = rtInfo.ShellLastCmd
			integration = rtInfo.ShellIntegration
			hasCurCwd = rtInfo.ShellHasCurCwd
		}

		ti := terminalInfo{
			WidgetID:    b.BlockId[:min(8, len(b.BlockId))],
			BlockID:     b.BlockId,
			ShellType:   shellType,
			ShellState:  shellState,
			LastCmd:     lastCmd,
			Integration: integration,
			HasCurCwd:   hasCurCwd,
			ShortDesc:   makeTerminalShortDesc(b.Meta),
			WorkspaceID: b.WorkspaceId,
			TabID:       b.TabId,
		}
		result.Terminals = append(result.Terminals, ti)
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(out))
	return nil
}

func makeTerminalShortDesc(meta waveobj.MetaMapType) string {
	name, _ := meta["name"].(string)
	if name == "" {
		title, _ := meta["title"].(string)
		name = title
	}
	cwd, _ := meta[waveobj.MetaKey_CmdCwd].(string)
	if name != "" && cwd != "" {
		return fmt.Sprintf("shell %q in %s", name, cwd)
	}
	if name != "" {
		return fmt.Sprintf("shell %q", name)
	}
	if cwd != "" {
		return fmt.Sprintf("shell in %s", cwd)
	}
	return "terminal widget"
}