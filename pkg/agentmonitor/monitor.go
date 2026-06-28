package agentmonitor

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type AgentStatus struct {
	WidgetID     string
	AgentType    string
	Model        string
	Mode         string
	Status       string
	ContextPct   int
	ShellState   string
	LastOutput   time.Time
	IsIdle       bool
	IdleDuration time.Duration
}

type MonitorConfig struct {
	CheckInterval   time.Duration
	IdleThreshold   time.Duration
	WaveDataDir     string
	WaveBin         string
}

func DefaultConfig() *MonitorConfig {
	waveBin := "wsh"
	if p, err := exec.LookPath("wsh"); err == nil {
		waveBin = p
	} else if p, err := exec.LookPath("./wsh.exe"); err == nil {
		waveBin = p
	}
	return &MonitorConfig{
		CheckInterval: 30 * time.Second,
		IdleThreshold: 2 * time.Minute,
		WaveDataDir:   "S:\\sean-machine-janitor\\bridge",
		WaveBin:       waveBin,
	}
}

type AgentMonitor struct {
	config   *MonitorConfig
	handlers []IdleHandler
	ctx      context.Context
	cancel   context.CancelFunc
}

type IdleHandler func(agent *AgentStatus) error

func NewMonitor(cfg *MonitorConfig) *AgentMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &AgentMonitor{
		config: cfg,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (m *AgentMonitor) OnIdle(handler IdleHandler) {
	m.handlers = append(m.handlers, handler)
}

func (m *AgentMonitor) Start() error {
	go m.runLoop()
	return nil
}

func (m *AgentMonitor) Stop() {
	m.cancel()
}

func (m *AgentMonitor) runLoop() {
	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkAgents()
		}
	}
}

func (m *AgentMonitor) checkAgents() {
	agents, err := m.DiscoverAgents()
	if err != nil {
		return
	}

	for _, agent := range agents {
		if agent.IsIdle && agent.IdleDuration >= m.config.IdleThreshold {
			for _, handler := range m.handlers {
				if err := handler(agent); err != nil {
					continue
				}
			}
		}
	}
}

func (m *AgentMonitor) DiscoverAgents() ([]*AgentStatus, error) {
	cmd := exec.CommandContext(m.ctx, m.config.WaveBin, "ai", "scan-terminals")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var result struct {
		Terminals []struct {
			WidgetID    string `json:"widget_id"`
			BlockID     string `json:"block_id"`
			ShellType   string `json:"shell_type"`
			ShellState  string `json:"shell_state"`
			LastCmd     string `json:"last_cmd"`
			Integration bool   `json:"integration"`
			HasCurCwd   bool   `json:"has_curcwd"`
			ShortDesc   string `json:"short_desc"`
			WorkspaceID string `json:"workspace_id"`
			TabID       string `json:"tab_id"`
		} `json:"terminals"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, err
	}

	var agents []*AgentStatus
	now := time.Now()

	for _, t := range result.Terminals {
		if !t.Integration {
			continue
		}

		status, err := m.GetAgentStatus(t.WidgetID)
		if err != nil || status == nil {
			continue
		}

		idleDur := now.Sub(status.LastOutput)
		status.IdleDuration = idleDur
		status.IsIdle = status.Status == "idle" || (status.ShellState == "" && idleDur > m.config.IdleThreshold)

		agents = append(agents, status)
	}

	return agents, nil
}

func (m *AgentMonitor) GetAgentStatus(widgetID string) (*AgentStatus, error) {
	cmd := exec.CommandContext(m.ctx, m.config.WaveBin, "ai", "agent-status", widgetID)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var status struct {
		Status        string `json:"status"`
		ContextPct    int    `json:"context_percent"`
		Model         string `json:"model"`
		Mode          string `json:"mode"`
		ShellState    string `json:"shell_state"`
		LastOutputSec int    `json:"last_output_sec"`
	}

	if err := json.Unmarshal(output, &status); err != nil {
		return nil, err
	}

	agentType := "unknown"
	if strings.Contains(strings.ToLower(status.Model), "kilo") {
		agentType = "kilo"
	} else if strings.Contains(strings.ToLower(status.Model), "opencode") || status.Mode != "" {
		agentType = "opencode"
	}

	lastOut := time.Now().Add(-time.Duration(status.LastOutputSec) * time.Second)
	if status.LastOutputSec == 0 {
		lastOut = time.Now()
	}

	return &AgentStatus{
		WidgetID:   widgetID,
		AgentType:  agentType,
		Model:      status.Model,
		Mode:       status.Mode,
		Status:     status.Status,
		ContextPct: status.ContextPct,
		ShellState: status.ShellState,
		LastOutput: lastOut,
	}, nil
}

func (m *AgentMonitor) SendNudge(widgetID, message string) error {
	cmd := exec.CommandContext(m.ctx, m.config.WaveBin, "input", widgetID, message)
	return cmd.Run()
}

func (m *AgentMonitor) SendTask(widgetID, task string) error {
	prompt := fmt.Sprintf("Task: %s\n\nPlease continue working on this. If you're done with current work, acknowledge and proceed.", task)
	return m.SendNudge(widgetID, prompt)
}

func (m *AgentMonitor) GetOutboxPath() string {
	return fmt.Sprintf("%s\\wave-outbox.jsonl", m.config.WaveDataDir)
}

func (m *AgentMonitor) ReadOutbox(widgetID string, count int) ([]map[string]interface{}, error) {
	data, err := exec.CommandContext(m.ctx, "powershell", "-Command",
		fmt.Sprintf("Get-Content '%s' -Tail %d | ConvertFrom-Json", m.GetOutboxPath(), count)).Output()
	if err != nil {
		return nil, err
	}

	var messages []map[string]interface{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var msg map[string]interface{}
		if err := json.Unmarshal([]byte(line), &msg); err == nil {
			if widgetID == "" || msg["widget_id"] == widgetID || msg["block_id"] == widgetID {
				messages = append(messages, msg)
			}
		}
	}
	return messages, nil
}