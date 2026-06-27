// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/base64"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
)

var inputCmd = &cobra.Command{
	Use:   "input [flags] <widget-id> <text>",
	Short: "send text input to a terminal widget",
	Long: `Send text input to a terminal widget as if the user typed it.
Use --enter to press Enter after the text.
The widget-id is the 8-character ID shown in the block header.`,
	Example: "  wsh input 8d4a72e2 \"ls -la\" --enter\n  wsh input 8d4a72e2 \"hello\"",
	Args:    cobra.ExactArgs(2),
	RunE:    inputRun,
	PreRunE: preRunSetupRpcClient,
	DisableFlagsInUseLine: true,
}

var (
	inputEnter bool
)

func init() {
	rootCmd.AddCommand(inputCmd)
	inputCmd.Flags().BoolVarP(&inputEnter, "enter", "e", false, "press Enter after the text")
}

func inputRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("input", rtnErr == nil)
	}()

	widgetId := args[0]
	text := args[1]

	if inputEnter {
		text += "\r"
	}

	fullORef, err := resolveSimpleId(widgetId)
	if err != nil {
		return fmt.Errorf("resolving widget id %s: %w", widgetId, err)
	}

	blockId := fullORef.OID

	err = wshclient.ControllerInputCommand(
		RpcClient,
		wshrpc.CommandBlockInputData{
			BlockId:      blockId,
			InputData64:  base64.StdEncoding.EncodeToString([]byte(text)),
		},
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to send input to terminal: %w", err)
	}

	fmt.Printf("sent input to %s\n", widgetId)
	return nil
}
