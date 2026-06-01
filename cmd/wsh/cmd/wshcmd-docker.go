// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
)

var dockerCmd = &cobra.Command{
	Use:   "docker",
	Short: "manage Docker containers and images",
	Long: `Execute Docker commands locally via the Docker CLI,
or remotely inside an SSH session by spawning a terminal block.

Subcommands pass arguments through to the Docker CLI.
When connected to a remote session (SSH), a terminal block is
created on the remote connection to run the docker command.`,
}

var dockerFollow bool

func init() {
	rootCmd.AddCommand(dockerCmd)

	dockerLogsCmd.Flags().BoolVarP(&dockerFollow, "follow", "f", false, "follow log output")

	dockerCmd.AddCommand(dockerPsCmd)
	dockerCmd.AddCommand(dockerLogsCmd)
	dockerCmd.AddCommand(dockerExecCmd)
	dockerCmd.AddCommand(dockerStopCmd)
	dockerCmd.AddCommand(dockerStartCmd)
	dockerCmd.AddCommand(dockerRestartCmd)
	dockerCmd.AddCommand(dockerImagesCmd)
	dockerCmd.AddCommand(dockerRmCmd)
	dockerCmd.AddCommand(dockerRunCmd)
	dockerCmd.AddCommand(dockerComposeCmd)
}

var dockerPsCmd = &cobra.Command{
	Use:                "ps [args...]",
	Short:              "list containers",
	Args:               cobra.ArbitraryArgs,
	RunE:               dockerPassthruRun("ps"),
	DisableFlagParsing: true,
}

var dockerLogsCmd = &cobra.Command{
	Use:   "logs <container> [args...]",
	Short: "fetch container logs",
	Args:  cobra.MinimumNArgs(1),
	RunE:  dockerLogsRun,
}

func dockerLogsRun(cmd *cobra.Command, args []string) error {
	dockerArgs := append([]string{"logs"}, args...)
	if dockerFollow {
		return runDockerPassthru(dockerArgs)
	}
	return runDockerCapture(dockerArgs)
}

var dockerExecCmd = &cobra.Command{
	Use:                "exec <container> [args...]",
	Short:              "execute command in container",
	Args:               cobra.MinimumNArgs(2),
	RunE:               dockerPassthruRun("exec"),
	DisableFlagParsing: true,
}

var dockerStopCmd = &cobra.Command{
	Use:                "stop [args...]",
	Short:              "stop containers",
	Args:               cobra.MinimumNArgs(1),
	RunE:               dockerPassthruRun("stop"),
	DisableFlagParsing: true,
}

var dockerStartCmd = &cobra.Command{
	Use:                "start [args...]",
	Short:              "start containers",
	Args:               cobra.MinimumNArgs(1),
	RunE:               dockerPassthruRun("start"),
	DisableFlagParsing: true,
}

var dockerRestartCmd = &cobra.Command{
	Use:                "restart [args...]",
	Short:              "restart containers",
	Args:               cobra.MinimumNArgs(1),
	RunE:               dockerPassthruRun("restart"),
	DisableFlagParsing: true,
}

var dockerImagesCmd = &cobra.Command{
	Use:                "images [args...]",
	Short:              "list images",
	Args:               cobra.ArbitraryArgs,
	RunE:               dockerPassthruRun("images"),
	DisableFlagParsing: true,
}

var dockerRmCmd = &cobra.Command{
	Use:                "rm [args...]",
	Short:              "remove containers",
	Args:               cobra.MinimumNArgs(1),
	RunE:               dockerPassthruRun("rm"),
	DisableFlagParsing: true,
}

var dockerRunCmd = &cobra.Command{
	Use:                "run [args...]",
	Short:              "run a new container",
	Args:               cobra.MinimumNArgs(1),
	RunE:               dockerPassthruRun("run"),
	DisableFlagParsing: true,
}

var dockerComposeCmd = &cobra.Command{
	Use:                "compose [args...]",
	Short:              "run docker compose commands",
	Args:               cobra.ArbitraryArgs,
	RunE:               dockerPassthruRun("compose"),
	DisableFlagParsing: true,
}

func dockerPassthruRun(subcmd string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		dockerArgs := append([]string{subcmd}, args...)
		return runDockerPassthru(dockerArgs)
	}
}

func runDockerPassthru(dockerArgs []string) error {
	if RpcContext.Conn != "" {
		return runDockerRemote(dockerArgs)
	}
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return fmt.Errorf("docker CLI not found in PATH")
	}
	ecmd := exec.Command(dockerPath, dockerArgs...)
	ecmd.Stdin = os.Stdin
	ecmd.Stdout = os.Stdout
	ecmd.Stderr = os.Stderr
	return ecmd.Run()
}

func runDockerCapture(dockerArgs []string) error {
	if RpcContext.Conn != "" {
		return runDockerRemote(dockerArgs)
	}
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return fmt.Errorf("docker CLI not found in PATH")
	}
	ecmd := exec.Command(dockerPath, dockerArgs...)
	output, err := ecmd.CombinedOutput()
	if len(output) > 0 {
		WriteStdout("%s", output)
	}
	if err != nil {
		return fmt.Errorf("docker %s: %w", dockerArgs[0], err)
	}
	return nil
}

func runDockerRemote(dockerArgs []string) error {
	shellCmd := "docker " + strings.Join(dockerArgs, " ")
	tabId := getTabIdFromEnv()
	if tabId == "" {
		return fmt.Errorf("no WAVETERM_TABID env var set")
	}
	createMeta := map[string]any{
		waveobj.MetaKey_View:            "term",
		waveobj.MetaKey_Controller:      "cmd",
		waveobj.MetaKey_Connection:      RpcContext.Conn,
		waveobj.MetaKey_Cmd:             shellCmd,
		waveobj.MetaKey_CmdShell:        true,
		waveobj.MetaKey_CmdRunOnce:      true,
		waveobj.MetaKey_CmdRunOnStart:   true,
		waveobj.MetaKey_CmdClearOnStart: true,
	}
	createBlockData := wshrpc.CommandCreateBlockData{
		TabId: tabId,
		BlockDef: &waveobj.BlockDef{
			Meta: createMeta,
		},
		Focused: true,
	}
	_, err := wshclient.CreateBlockCommand(RpcClient, createBlockData, &wshrpc.RpcOpts{Timeout: 60000})
	if err != nil {
		return fmt.Errorf("creating remote docker block: %w", err)
	}
	return nil
}
