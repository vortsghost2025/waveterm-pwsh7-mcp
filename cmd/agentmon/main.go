// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/agentmonitor"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
)

var (
	rootCmd = &cobra.Command{
		Use:   "agentmon",
		Short: "Agent monitoring daemon for Wave Terminal",
	}
	daemonCmd = &cobra.Command{
		Use:   "daemon",
		Short: "Run the agent monitoring daemon (continuous background service)",
		RunE:  daemonRun,
	}
	onceCmd = &cobra.Command{
		Use:   "once",
		Short: "Run a single check cycle (alias for check)",
		RunE:  checkRun,
	}
	statusCmd = &cobra.Command{
		Use:   "status [widget-id]",
		Short: "Check agent status (all agents or specific widget)",
		Args:  cobra.MaximumNArgs(1),
		RunE:  statusRun,
	}
	nudgeCmd = &cobra.Command{
		Use:   "nudge <widget-id> <message>",
		Short: "Send a nudge message to an agent terminal",
		Args:  cobra.ExactArgs(2),
		RunE:  nudgeRun,
	}
	checkCmd = &cobra.Command{
		Use:   "check",
		Short: "Run a single check cycle and exit",
		RunE:  checkRun,
	}
)

func init() {
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(onceCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(nudgeCmd)
	rootCmd.AddCommand(checkCmd)
}

func main() {
	wavebase.WaveVersion = "dev"
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func daemonRun(cmd *cobra.Command, args []string) error {
	log.SetFlags(0)
	log.SetPrefix("[agentmon] ")

	cfg := agentmonitor.DefaultConfig()
	cfg.CheckInterval = 15 * time.Second
	cfg.IdleThreshold = 45 * time.Second

	orc := agentmonitor.NewOrchestrator(cfg)

	go watchWorkQueue(orc)
	go orc.Start()
	log.Printf("Agent monitor daemon started (check: %v, idle: %v)", cfg.CheckInterval, cfg.IdleThreshold)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sigChan
	log.Printf("Shutting down...")
	orc.Stop()
	return nil
}

func checkRun(cmd *cobra.Command, args []string) error {
	cfg := agentmonitor.DefaultConfig()
	if wshPath := os.Getenv("WSH_PATH"); wshPath != "" {
		cfg.WaveBin = wshPath
	}
	mon := agentmonitor.NewMonitor(cfg)

	_ = os.Getenv("WAVETERM_JWT")
	_ = os.Getenv("WAVETERM_SWAPTOKEN")

	agents, err := mon.DiscoverAgents()
	if err != nil {
		log.Printf("discover error: %v", err)
		return err
	}

	log.Printf("Found %d integrated terminal(s)", len(agents))
	for _, a := range agents {
		log.Printf("  - %s: status=%s, shell=%s, idle=%v, context=%d%%",
			a.WidgetID, a.Status, a.ShellState, a.IdleDuration.Round(time.Second), a.ContextPct)
	}
	return nil
}

func statusRun(cmd *cobra.Command, args []string) error {
	cfg := agentmonitor.DefaultConfig()
	if wshPath := os.Getenv("WSH_PATH"); wshPath != "" {
		cfg.WaveBin = wshPath
	}
	mon := agentmonitor.NewMonitor(cfg)

	if len(args) > 0 {
		// Check specific widget
		agent, err := mon.GetAgentStatus(args[0])
		if err != nil {
			return fmt.Errorf("status error: %w", err)
		}
		if agent == nil {
			fmt.Printf("Agent %s not found or not integrated\n", args[0])
			return nil
		}
		out, _ := json.MarshalIndent(agent, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	// Check all agents
	agents, err := mon.DiscoverAgents()
	if err != nil {
		return fmt.Errorf("discover error: %w", err)
	}

	fmt.Printf("Integrated terminals: %d\n", len(agents))
	for _, a := range agents {
		fmt.Printf("  - %s: %s (shell: %s, idle: %v)\n",
			a.WidgetID, a.Status, a.ShellState, a.IdleDuration.Round(time.Second))
	}
	return nil
}

func nudgeRun(cmd *cobra.Command, args []string) error {
	widgetID := args[0]
	message := args[1]

	cfg := agentmonitor.DefaultConfig()
	if wshPath := os.Getenv("WSH_PATH"); wshPath != "" {
		cfg.WaveBin = wshPath
	}
	mon := agentmonitor.NewMonitor(cfg)

	err := mon.SendNudge(widgetID, message)
	if err != nil {
		return fmt.Errorf("nudge failed: %w", err)
	}
	fmt.Printf("Sent nudge to %s\n", widgetID)
	return nil
}

var lastQueueSize int64
var queueMu sync.Mutex

func watchWorkQueue(orc *agentmonitor.Orchestrator) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		items, err := readWorkQueue()
		if err == nil {
			queueMu.Lock()
			for i := range items {
				orc.QueueTask(items[i].Task)
			}
			queueMu.Unlock()
		}
		<-ticker.C
	}
}

func readWorkQueue() ([]WorkQueueItem, error) {
	path := os.Getenv("AGENT_WORKQUEUE_PATH")
	if path == "" {
		path = "S:\\sean-machine-janitor\\bridge\\agent-workqueue.jsonl"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var items []WorkQueueItem
	for _, line := range splitLines(string(data)) {
		if line == "" {
			continue
		}
		var item WorkQueueItem
		if err := json.Unmarshal([]byte(line), &item); err == nil && item.Task != "" {
			items = append(items, item)
		}
	}
	return items, nil
}

type WorkQueueItem struct {
	TargetWidget string `json:"target_widget,omitempty"`
	Task         string `json:"task"`
	Priority     int    `json:"priority,omitempty"`
	CreatedAt    int64  `json:"created_at"`
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}