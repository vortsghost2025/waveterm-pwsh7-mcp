// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wstore"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
)

var aiStatusCmd = &cobra.Command{
	Use:     "agent-status <widget-id>",
	Short:   "check agent terminal status",
	Args:    cobra.ExactArgs(1),
	RunE:    aiAgentStatusRun,
	PreRunE: preRunSetupRpcClient,
}

func init() {
	aiCmd.AddCommand(aiStatusCmd)
}

type agentStatusResult struct {
	Status         string `json:"status"`
	ContextPercent int    `json:"context_percent"`
	Model          string `json:"model"`
	Mode           string `json:"mode"`
	ShellState     string `json:"shell_state,omitempty"`
	LastOutputSec  int    `json:"last_output_sec"`
}

func aiAgentStatusRun(cmd *cobra.Command, args []string) error {
	widgetID := args[0]

	fullORef, err := resolveSimpleId(widgetID)
	if err != nil {
		return fmt.Errorf("resolving widget id: %w", err)
	}

	info, err := wshclient.BlockInfoCommand(RpcClient, fullORef.OID, &wshrpc.RpcOpts{Timeout: 10000})
	if err != nil {
		return fmt.Errorf("blockinfo failed: %w", err)
	}
	if info == nil || info.Block == nil {
		return fmt.Errorf("block not found: %s", widgetID)
	}
	block := info.Block

	status := "unknown"
	contextPct := 0
	shellState := ""

	oref := waveobj.MakeORef(waveobj.OType_Block, block.OID)
	rtInfo := wstore.GetRTInfo(oref)
	if rtInfo != nil {
		shellState = rtInfo.ShellState
		if shellState == "ready" {
			status = "idle"
		} else if shellState == "running-command" {
			status = "active"
		}
	}

	scrollback, err := wshclient.TermGetScrollbackLinesCommand(RpcClient, wshrpc.CommandTermGetScrollbackLinesData{
		BlockId:   fullORef.OID,
		LineStart: 0,
		LineEnd:   100,
	}, &wshrpc.RpcOpts{Timeout: 10000})
	if err == nil && scrollback != nil {
		lines := scrollback.Lines
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.ToLower(lines[i])
			if strings.TrimSpace(line) == "" {
				continue
			}
			if strings.Contains(line, "compacting") || strings.Contains(line, "summarizing") {
				status = "compacting"
				break
			}
			if strings.Contains(line, "error:") || strings.Contains(line, "failed:") {
				status = "error"
				break
			}
			if strings.HasSuffix(strings.TrimSpace(lines[i]), ">") || strings.Contains(line, "waiting") {
				status = "idle"
				break
			}
			status = "active"
			break
		}

		lastOutputSec := int(time.Since(time.UnixMilli(scrollback.LastUpdated)).Seconds())
		result := agentStatusResult{
			Status:         status,
			ContextPercent: contextPct,
			Model:          getAgentModel(block),
			Mode:           getAgentMode(block),
			ShellState:     shellState,
			LastOutputSec:  lastOutputSec,
		}
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	result := agentStatusResult{
		Status:         status,
		ContextPercent: contextPct,
		Model:          getAgentModel(block),
		Mode:           getAgentMode(block),
		ShellState:     shellState,
		LastOutputSec:  0,
	}
	out, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(out))
	return nil
}

func getAgentModel(block *waveobj.Block) string {
	if block.Meta == nil {
		return ""
	}
	if m, ok := block.Meta["agent:model"].(string); ok {
		return m
	}
	if args := block.Meta.GetStringList(waveobj.MetaKey_CmdArgs); args != nil {
		for i, arg := range args {
			if arg == "--model" && i+1 < len(args) {
				return args[i+1]
			}
		}
	}
	return ""
}

func getAgentMode(block *waveobj.Block) string {
	if block.Meta == nil {
		return ""
	}
	if m, ok := block.Meta["agent:mode"].(string); ok {
		return m
	}
	envMap := block.Meta.GetStringMap(waveobj.MetaKey_CmdEnv, false)
	if envMap != nil {
		return envMap["AGENT_MODE"]
	}
	return ""
}