package aiusechat

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
	"github.com/wavetermdev/waveterm/pkg/util/utilfn"
)

const (
	RunCmdTimeout       = 30 * time.Second
	RunCmdMaxOutputSize = 100 * 1024
)

type runCommandParams struct {
	Command string `json:"command"`
}

type allowedCommand struct {
	Pattern *regexp.Regexp
	Label   string
}

var cmdAllowlist = []allowedCommand{
	// Navigation / identity
	{regexp.MustCompile(`^pwd$`), "pwd"},
	{regexp.MustCompile(`^whoami$`), "whoami"},
	{regexp.MustCompile(`^hostname$`), "hostname"},
	{regexp.MustCompile(`^date$`), "date"},
	{regexp.MustCompile(`^uname .*$`), "uname"},
	{regexp.MustCompile(`^echo .+$`), "echo"},

	// Directory listing
	{regexp.MustCompile(`^dir$`), "dir"},
	{regexp.MustCompile(`^dir .+$`), "dir"},
	{regexp.MustCompile(`^ls$`), "ls"},
	{regexp.MustCompile(`^ls .+$`), "ls"},
	{regexp.MustCompile(`^Get-ChildItem .*$`), "Get-ChildItem"},
	{regexp.MustCompile(`^Get-Location$`), "Get-Location"},

	// File reading
	{regexp.MustCompile(`^cat .+$`), "cat"},
	{regexp.MustCompile(`^type .+$`), "type"},
	{regexp.MustCompile(`^Get-Content .+$`), "Get-Content"},
	{regexp.MustCompile(`^head .+$`), "head"},
	{regexp.MustCompile(`^tail .+$`), "tail"},
	{regexp.MustCompile(`^wc .+$`), "wc"},
	{regexp.MustCompile(`^Select-String .+$`), "Select-String"},

	// Find / search
	{regexp.MustCompile(`^find .+$`), "find"},
	{regexp.MustCompile(`^grep .+$`), "grep"},
	{regexp.MustCompile(`^rg .+$`), "rg"},
	{regexp.MustCompile(`^fd .+$`), "fd"},

	// Environment / process info
	{regexp.MustCompile(`^env$`), "env"},
	{regexp.MustCompile(`^printenv .*$`), "printenv"},
	{regexp.MustCompile(`^Get-ChildItem Env:.*$`), "Get-Env"},
	{regexp.MustCompile(`^\$PSVersionTable\.PSVersion$`), "PSVersion"},
	{regexp.MustCompile(`^Get-Process .*$`), "Get-Process"},
	{regexp.MustCompile(`^ps .*$`), "ps"},

	// Git (all read-only subcommands)
	{regexp.MustCompile(`^git status$`), "git status"},
	{regexp.MustCompile(`^git status --short$`), "git status"},
	{regexp.MustCompile(`^git status .*$`), "git status"},
	{regexp.MustCompile(`^git branch$`), "git branch"},
	{regexp.MustCompile(`^git branch --show-current$`), "git branch"},
	{regexp.MustCompile(`^git branch .*$`), "git branch"},
	{regexp.MustCompile(`^git log .*$`), "git log"},
	{regexp.MustCompile(`^git diff .*$`), "git diff"},
	{regexp.MustCompile(`^git show .*$`), "git show"},
	{regexp.MustCompile(`^git remote .*$`), "git remote"},
	{regexp.MustCompile(`^git stash list$`), "git stash list"},
	{regexp.MustCompile(`^git tag .*$`), "git tag"},
	{regexp.MustCompile(`^git describe .*$`), "git describe"},
	{regexp.MustCompile(`^git rev-parse .*$`), "git rev-parse"},
	{regexp.MustCompile(`^git config --get .*$`), "git config"},
	{regexp.MustCompile(`^git config --list$`), "git config list"},
	{regexp.MustCompile(`^git blame .*$`), "git blame"},

	// Dev tool version checks
	{regexp.MustCompile(`^go version$`), "go version"},
	{regexp.MustCompile(`^go env .*$`), "go env"},
	{regexp.MustCompile(`^go list .*$`), "go list"},
	{regexp.MustCompile(`^go vet .*$`), "go vet"},
	{regexp.MustCompile(`^node --version$`), "node version"},
	{regexp.MustCompile(`^npm --version$`), "npm version"},
	{regexp.MustCompile(`^npm list .*$`), "npm list"},
	{regexp.MustCompile(`^npx --version$`), "npx version"},
	{regexp.MustCompile(`^python --version$`), "python version"},
	{regexp.MustCompile(`^python -m py_compile .+$`), "python compile check"},
	{regexp.MustCompile(`^pip --version$`), "pip version"},
	{regexp.MustCompile(`^pip list .*$`), "pip list"},
	{regexp.MustCompile(`^task --list$`), "task list"},
	{regexp.MustCompile(`^task --list-all$`), "task list"},
	{regexp.MustCompile(`^which .+$`), "which"},
	{regexp.MustCompile(`^Get-Command .+$`), "Get-Command"},
	{regexp.MustCompile(`^where .+$`), "where"},
	{regexp.MustCompile(`^command -v .+$`), "command"},

	// WSH (Wave Shell) commands
	{regexp.MustCompile(`^wsh$`), "wsh"},
	{regexp.MustCompile(`^wsh --help$`), "wsh help"},
	{regexp.MustCompile(`^wsh version$`), "wsh version"},
	{regexp.MustCompile(`^wsh getvar .+$`), "wsh getvar"},
	{regexp.MustCompile(`^wsh blocks$`), "wsh blocks"},
	{regexp.MustCompile(`^wsh status$`), "wsh status"},
	{regexp.MustCompile(`^wsh chatstatus$`), "wsh chatstatus"},

	// Disk usage (read-only)
	{regexp.MustCompile(`^df .*$`), "df"},
	{regexp.MustCompile(`^du .*$`), "du"},
	{regexp.MustCompile(`^Get-PSDrive .*$`), "Get-PSDrive"},

	// Network info (read-only, no outbound)
	{regexp.MustCompile(`^ipconfig$`), "ipconfig"},
	{regexp.MustCompile(`^ifconfig$`), "ifconfig"},
	{regexp.MustCompile(`^ip addr show$`), "ip addr"},
	{regexp.MustCompile(`^netstat .*$`), "netstat"},
	{regexp.MustCompile(`^ss .*$`), "ss"},

	// File info
	{regexp.MustCompile(`^file .+$`), "file"},
	{regexp.MustCompile(`^stat .+$`), "stat"},
	{regexp.MustCompile(`^Get-Item .+$`), "Get-Item"},
	{regexp.MustCompile(`^Get-FileHash .+$`), "Get-FileHash"},
}

var cmdBlockedPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(rm|del|erase|remove|rmdir|rd).*(/s|/f|/r|-r|-rf|-f|--recursive|--force)`),
	regexp.MustCompile(`(?i)(shutdown|restart-computer|stop-computer)`),
	regexp.MustCompile(`(?i)format.*`),
	regexp.MustCompile(`(?i)net\s+user`),
}

func parseRunCommandInput(input any) (*runCommandParams, error) {
	result := &runCommandParams{}
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}
	if result.Command == "" {
		return nil, fmt.Errorf("missing command parameter")
	}
	return result, nil
}

func checkCommand(cmd string) error {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return fmt.Errorf("empty command")
	}
	if strings.ContainsAny(cmd, ";&|><`\n") {
		return fmt.Errorf("command contains shell metacharacters: ; & | > < ` \\n")
	}
	for _, bp := range cmdBlockedPatterns {
		if bp.MatchString(cmd) {
			return fmt.Errorf("command blocked by security policy")
		}
	}
	for _, ac := range cmdAllowlist {
		if ac.Pattern.MatchString(cmd) {
			return nil
		}
	}
	return fmt.Errorf("command not in allowlist")
}

func verifyRunCommandInput(input any, toolUseData *uctypes.UIMessageDataToolUse) error {
	params, err := parseRunCommandInput(input)
	if err != nil {
		return err
	}
	return checkCommand(params.Command)
}

func runCommandCallback(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
	params, err := parseRunCommandInput(input)
	if err != nil {
		return nil, err
	}
	if err := checkCommand(params.Command); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), RunCmdTimeout)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		shell := "powershell"
		if pwshPath, err := exec.LookPath("pwsh"); err == nil {
			shell = pwshPath
		}
		cmd = exec.CommandContext(ctx, shell, "-NoProfile", "-Command", params.Command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", params.Command)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	result := stdout.String()
	if stderr.Len() > 0 {
		if result != "" {
			result += "\n"
		}
		result += stderr.String()
	}
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("command timed out after %v", RunCmdTimeout)
		}
		if result != "" {
			result += "\n"
		}
		result += fmt.Sprintf("error: %v", err)
	}
	if len(result) > RunCmdMaxOutputSize {
		result = result[:RunCmdMaxOutputSize] + "\n... (truncated)"
	}
	return map[string]any{
		"result": result,
	}, nil
}

func GetRunCommandToolDefinition() uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "run_command",
		DisplayName: "Run Command",
		Description: "Run a shell command with read-only access. Only allowlisted commands are permitted. Destructive operations are blocked. Requires user approval before execution.",
		ToolLogName: "gen:runcommand",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The command to run. Must be on the allowlist (e.g., pwd, whoami, ls, echo, git status, cat/type, Get-Content). Destructive patterns (rm -rf, del, shutdown, format) are blocked. Shell metacharacters (;&|><`) are rejected.",
				},
			},
			"required":             []string{"command"},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			params, err := parseRunCommandInput(input)
			if err != nil {
				return fmt.Sprintf("error parsing input: %v", err)
			}
			return fmt.Sprintf("running %q", params.Command)
		},
		ToolAnyCallback: runCommandCallback,
		ToolApproval: func(input any) string {
			return uctypes.ApprovalNeedsApproval
		},
		ToolVerifyInput: verifyRunCommandInput,
	}
}
