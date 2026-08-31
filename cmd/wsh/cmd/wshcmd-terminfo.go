// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
)

var termInfoJson bool

var termInfoCmd = &cobra.Command{
	Use:     "info",
	Short:   "get terminal block information",
	RunE:    termInfoRun,
	PreRunE: preRunSetupRpcClient,
}

func init() {
	termInfoCmd.Flags().BoolVar(&termInfoJson, "json", false, "output as JSON")
	termCmd.AddCommand(termInfoCmd)
}

func termInfoRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("terminfo", rtnErr == nil)
	}()

	fullORef, err := resolveBlockArg()
	if err != nil {
		return err
	}
	if fullORef.OType != "" && fullORef.OType != waveobj.OType_Block {
		return fmt.Errorf("oref %s is not a block", fullORef)
	}
	blockId := fullORef.OID

	info, err := wshclient.TermInfoCommand(RpcClient, wshrpc.TermInfoRequest{BlockID: blockId}, &wshrpc.RpcOpts{Timeout: 5000})
	if err != nil {
		return fmt.Errorf("getting term info: %w", err)
	}

	if termInfoJson {
		data, _ := json.MarshalIndent(info, "", "  ")
		WriteStdout("%s\n", string(data))
		return nil
	}

	WriteStdout("block: %s\n", info.BlockID)
	if info.Cwd != "" {
		WriteStdout("cwd:   %s\n", info.Cwd)
	}
	if info.Shell != "" {
		WriteStdout("shell: %s\n", info.Shell)
	}
	if info.Pid != 0 {
		WriteStdout("pid:   %d\n", info.Pid)
	}
	return nil
}
