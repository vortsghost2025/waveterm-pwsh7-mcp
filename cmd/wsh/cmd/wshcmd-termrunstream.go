// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
)

var (
	termRunStreamCwd         string
	termRunStreamJson        bool
	termRunStreamInteractive bool
)

var termRunStreamCmd = &cobra.Command{
	Use:              "run-stream [flags] -- command [args...]",
	Short:            "run a command and stream its output",
	RunE:             termRunStreamRun,
	PreRunE:          preRunSetupRpcClient,
	TraverseChildren: true,
}

func init() {
	flags := termRunStreamCmd.Flags()
	flags.StringVar(&termRunStreamCwd, "cwd", "", "working directory for command")
	flags.BoolVar(&termRunStreamJson, "json", false, "output as line-delimited JSON events")
	flags.BoolVar(&termRunStreamInteractive, "interactive", false, "run in the visible terminal block")
	termCmd.AddCommand(termRunStreamCmd)
}

func termRunStreamRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("termrunstream", rtnErr == nil)
	}()

	fullORef, err := resolveBlockArg()
	if err != nil {
		return err
	}
	if fullORef.OType != "" && fullORef.OType != waveobj.OType_Block {
		return fmt.Errorf("oref %s is not a block", fullORef)
	}
	blockId := fullORef.OID

	// parse command from args (everything after "--")
	var commandParts []string
	for i, arg := range os.Args {
		if arg == "--" {
			if i+1 >= len(os.Args) {
				OutputHelpMessage(cmd)
				return fmt.Errorf("no command provided after --")
			}
			commandParts = os.Args[i+1:]
			break
		}
	}
	if len(commandParts) == 0 {
		OutputHelpMessage(cmd)
		return fmt.Errorf("command must be specified after --")
	}
	command := strings.Join(commandParts, " ")

	req := wshrpc.CommandRunStreamRequest{
		BlockID:     blockId,
		Command:     command,
		Cwd:         termRunStreamCwd,
		Interactive: termRunStreamInteractive,
	}

	ch := wshclient.CommandRunStreamCommand(RpcClient, req, &wshrpc.RpcOpts{Timeout: 0})
	for resp := range ch {
		if resp.Error != nil {
			if termRunStreamJson {
				errEvent := wshrpc.CommandRunStreamEvent{
					EventType: wshrpc.CommandRunStreamEvent_Error,
					Error:     resp.Error.Error(),
				}
				data, _ := json.Marshal(errEvent)
				WriteStdout("%s\n", string(data))
			} else {
				WriteStderr("error: %v\n", resp.Error)
			}
			return resp.Error
		}
		event := resp.Response
		if termRunStreamJson {
			data, _ := json.Marshal(event)
			WriteStdout("%s\n", string(data))
		} else {
			if event.EventType == wshrpc.CommandRunStreamEvent_Stdout || event.EventType == wshrpc.CommandRunStreamEvent_Stderr {
				WriteStdout("%s", event.Data)
			} else if event.EventType == wshrpc.CommandRunStreamEvent_Error {
				WriteStderr("error: %s\n", event.Error)
			} else if event.EventType == wshrpc.CommandRunStreamEvent_Exit {
				if event.ExitCode != nil && *event.ExitCode != 0 {
					WriteStderr("exit code: %d\n", *event.ExitCode)
				}
			}
		}
	}
	return nil
}
