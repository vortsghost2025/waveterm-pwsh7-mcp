// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
)

var (
	agentStaleHours    int
	agentRecentMinutes int
	agentJSON          bool
	agentForce         bool
)

// AgentRow is one entry reported by `wsh agent list` / `status` / `stale`.
// Mirrors the JSON shape of wave-collisions.ps1 so we can pipe either path.
type AgentRow struct {
	Kind      string  `json:"kind"`
	Pid       int     `json:"pid"`
	ParentPid int     `json:"parent"`
	Started   string  `json:"started,omitempty"`
	AgeHours  float64 `json:"age_hours,omitempty"`
	Profile   string  `json:"profile,omitempty"`
	Stale     bool    `json:"stale,omitempty"`
	ExePath   string  `json:"exe_path,omitempty"`
	CmdLine   string  `json:"cmd_line,omitempty"`
}

// CollisionReport is the JSON envelope emitted by wave-collisions.ps1 -Json.
type CollisionReport struct {
	GeneratedAt         string      `json:"generated_at,omitempty"`
	StaleThresholdHours int         `json:"stale_threshold_hours,omitempty"`
	RecentTouchMinutes  int         `json:"recent_touch_minutes,omitempty"`
	Agents              []AgentRow  `json:"agents"`
	ActivePids          int         `json:"active_pids,omitempty"`
	StalePids           int         `json:"stale_pids,omitempty"`
	RecentTouches       []any       `json:"recent_touches,omitempty"`
}

// resolveCollisionScript returns the absolute path to wave-collisions.ps1.
// Looks adjacent to the running wsh binary first, then falls back to
// S:\\waveterm\\wave-collisions.ps1 (the user's repo root).
func resolveCollisionScript() (string, error) {
	exe, err := os.Executable()
	if err == nil {
		adj := filepath.Join(filepath.Dir(exe), "wave-collisions.ps1")
		if _, statErr := os.Stat(adj); statErr == nil {
			return adj, nil
		}
	}
	candidates := []string{
		`S:\waveterm\wave-collisions.ps1`,
		`S:\\waveterm\\wave-collisions.ps1`,
	}
	for _, p := range candidates {
		if _, statErr := os.Stat(p); statErr == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("could not locate wave-collisions.ps1 next to wsh.exe or at S:\\waveterm\\wave-collisions.ps1")
}

// runCollisionScript invokes wave-collisions.ps1 -Json and parses the result.
func runCollisionScript(extraArgs ...string) (*CollisionReport, error) {
	scriptPath, err := resolveCollisionScript()
	if err != nil {
		return nil, err
	}
	args := []string{"-NoLogo", "-NoProfile", "-File", scriptPath, "-Json"}
	args = append(args, extraArgs...)
	cmd := exec.Command("pwsh", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("wave-collisions.ps1 failed: %w", err)
	}
	var rep CollisionReport
	if err := json.Unmarshal(out, &rep); err != nil {
		return nil, fmt.Errorf("invalid JSON from wave-collisions.ps1: %w", err)
	}
	return &rep, nil
}

// printRows renders AgentRow slice as a small aligned table.
func printRows(rows []AgentRow) {
	if len(rows) == 0 {
		fmt.Println("(no rows)")
		return
	}
	headers := []string{"KIND", "PID", "PARENT", "STARTED", "HOURS", "PROFILE", "STALE", "EXE"}
	cellFor := func(r AgentRow) []string {
		return []string{
			r.Kind,
			strconv.Itoa(r.Pid),
			strconv.Itoa(r.ParentPid),
			r.Started,
			strconv.FormatFloat(r.AgeHours, 'f', 1, 64),
			r.Profile,
			boolStr(r.Stale, "YES", "-"),
			shortenExe(r.ExePath),
		}
	}
	// Compute widest cell per column across header + every row.
	numCols := len(headers)
	widths := make([]int, numCols)
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, r := range rows {
		c := cellFor(r)
		for i, cell := range c {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	formatRow := func(cells []string) {
		for i, cell := range cells {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Printf("%-*s", widths[i], cell)
		}
		fmt.Println()
	}
	formatRow(headers)
	for _, r := range rows {
		formatRow(cellFor(r))
	}
}

func boolStr(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}

func shortenExe(p string) string {
	if len(p) <= 48 {
		return p
	}
	return "..." + p[len(p)-45:]
}

func joinCmdline(r AgentRow) string {
	if r.CmdLine != "" {
		return r.CmdLine
	}
	return r.ExePath
}

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage external agent processes (kilo/opencode) coexisting with Wave",
	Long: `Surface stale, conflicting, or runaway kilo / opencode / wave-mcp processes that are
side-by-side with Wave Terminal. This is observational and stop-only by design; it does
not edit agent configs or env vars.

Subcommands:
  list               Print all detected agent processes with age + profile inference
  status <pid>       Print detailed info for one agent (pid, exe, cmdline, parent, age)
  stale              List only agents older than --stale-hours (default 4)
  stop <pid>         Gracefully shut down an agent (Stop-Process). Refuses without --force.`,
}

var agentListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all agent processes",
	RunE: func(cmd *cobra.Command, args []string) error {
		rep, err := runCollisionScript("-StaleHours", strconv.Itoa(agentStaleHours), "-RecentTouchMinutes", strconv.Itoa(agentRecentMinutes))
		if err != nil {
			return err
		}
		if agentJSON {
			out, _ := json.MarshalIndent(rep, "", "  ")
			fmt.Println(string(out))
			return nil
		}
		printRows(rep.Agents)
		fmt.Printf("\nTotals: active=%d  stale=%d  (threshold=%dh)\n", rep.ActivePids, rep.StalePids, agentStaleHours)
		return nil
	},
}

var agentStatusCmd = &cobra.Command{
	Use:   "status <pid>",
	Short: "Print detailed status for one agent process",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("pid must be an integer, got %q", args[0])
		}
		rep, err := runCollisionScript()
		if err != nil {
			return err
		}
		var hit *AgentRow
		for i, r := range rep.Agents {
			if r.Pid == target {
				hit = &rep.Agents[i]
				break
			}
		}
		if hit == nil {
			return fmt.Errorf("pid %d not in agent inventory (seen=%d)", target, len(rep.Agents))
		}
		if agentJSON {
			out, _ := json.MarshalIndent(hit, "", "  ")
			fmt.Println(string(out))
			return nil
		}
		fmt.Printf("Kind       %s\n", hit.Kind)
		fmt.Printf("Pid        %d\n", hit.Pid)
		fmt.Printf("Parent     %d\n", hit.ParentPid)
		fmt.Printf("Started    %s\n", hit.Started)
		fmt.Printf("AgeHours   %s\n", strconv.FormatFloat(hit.AgeHours, 'f', 1, 64))
		fmt.Printf("Profile    %s\n", hit.Profile)
		fmt.Printf("Stale      %s\n", boolStr(hit.Stale, "YES", "-"))
		fmt.Printf("Exe        %s\n", hit.ExePath)
		fmt.Printf("Cmdline    %s\n", joinCmdline(*hit))
		return nil
	},
}

var agentStaleCmd = &cobra.Command{
	Use:   "stale",
	Short: "List stale agents (older than --stale-hours, default 4)",
	RunE: func(cmd *cobra.Command, args []string) error {
		rep, err := runCollisionScript("-StaleHours", strconv.Itoa(agentStaleHours), "-RecentTouchMinutes", strconv.Itoa(agentRecentMinutes))
		if err != nil {
			return err
		}
		var out []AgentRow
		for _, r := range rep.Agents {
			if r.Stale {
				out = append(out, r)
			}
		}
		if agentJSON {
			payload := struct {
				StaleThresholdHours int         `json:"stale_threshold_hours"`
				StaleCount          int         `json:"stale_count"`
				Agents              []AgentRow  `json:"agents"`
			}{agentStaleHours, len(out), out}
			b, _ := json.MarshalIndent(payload, "", "  ")
			fmt.Println(string(b))
			return nil
		}
		if len(out) == 0 {
			fmt.Printf("No agents older than %dh. (active=%d total)\n", agentStaleHours, rep.ActivePids)
			return nil
		}
		printRows(out)
		fmt.Printf("\nStale (%dh+): %d\n", agentStaleHours, len(out))
		return nil
	},
}

var agentStopCmd = &cobra.Command{
	Use:   "stop <pid>",
	Short: "Stop an agent process (requires --force)",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("pid must be an integer, got %q", args[0])
		}
		if !agentForce {
			return fmt.Errorf("refusing to stop pid %d without --force (destructive). Re-run with --force if intentional.", target)
		}
		// Confirm the pid is in the agent inventory first — protects against
		// the user accidentally killing their own shell or an unrelated PID.
		rep, err := runCollisionScript()
		if err != nil {
			return err
		}
		var hit *AgentRow
		for i, r := range rep.Agents {
			if r.Pid == target {
				hit = &rep.Agents[i]
				break
			}
		}
		if hit == nil {
			return fmt.Errorf("pid %d is not a detected agent pid; refusing to kill", target)
		}
		scriptPath, _ := resolveCollisionScript()
		psArgs := []string{
			"-NoLogo", "-NoProfile",
			"-Command",
			fmt.Sprintf("Stop-Process -Id %d -Force; exit $LASTEXITCODE", target),
		}
		psCmd := exec.Command("pwsh", psArgs...)
		psCmd.Stdout = os.Stdout
		psCmd.Stderr = os.Stderr
		if err := psCmd.Run(); err != nil {
			return fmt.Errorf("Stop-Process failed for pid %d: %w", target, err)
		}
		fmt.Fprintf(os.Stderr, "stopped pid=%d kind=%s (script=%s)\n", target, hit.Kind, scriptPath)
		return nil
	},
}

func init() {
	agentListCmd.Flags().IntVar(&agentStaleHours, "stale-hours", 4, "Age in hours at which an agent is flagged stale")
	agentListCmd.Flags().IntVar(&agentRecentMinutes, "recent-touch-min", 30, "Minutes back to scan for config-file touches")
	agentListCmd.Flags().BoolVar(&agentJSON, "json", false, "Emit JSON instead of a table")

	agentStatusCmd.Flags().BoolVar(&agentJSON, "json", false, "Emit JSON")
	agentStaleCmd.Flags().IntVar(&agentStaleHours, "stale-hours", 4, "Age in hours at which an agent is flagged stale")
	agentStaleCmd.Flags().IntVar(&agentRecentMinutes, "recent-touch-min", 30, "Minutes back to scan for config-file touches")
	agentStaleCmd.Flags().BoolVar(&agentJSON, "json", false, "Emit JSON")
	agentStopCmd.Flags().BoolVar(&agentForce, "force", false, "Required to actually stop the process (safety gate)")

	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentStatusCmd)
	agentCmd.AddCommand(agentStaleCmd)
	agentCmd.AddCommand(agentStopCmd)
	rootCmd.AddCommand(agentCmd)
}
