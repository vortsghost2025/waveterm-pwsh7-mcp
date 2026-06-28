package agentmonitor

import (
	"context"
	"fmt"
	"time"
)

func DefaultIdleHandlers(m *AgentMonitor) []IdleHandler {
	return []IdleHandler{
		func(agent *AgentStatus) error {
			if agent.Status == "compacting" {
				return nil
			}

			nudgeMsg := fmt.Sprintf(
				"[AgentMonitor] You appear idle (status: %s, shell: %s, idle: %v). "+
					"Resuming work — if you have a pending task, continue. "+
					"If you need direction, say 'ready for task'.",
				agent.Status, agent.ShellState, agent.IdleDuration.Round(time.Second))
			return m.SendNudge(agent.WidgetID, nudgeMsg)
		},
		func(agent *AgentStatus) error {
			if agent.ContextPct > 85 {
				return nil
			}

			if agent.Status == "idle" || agent.ShellState == "" {
				taskMsg := "[AgentMonitor] Idle detected. Ready for next task. Awaiting instructions."
				return m.SendNudge(agent.WidgetID, taskMsg)
			}
			return nil
		},
	}
}

func ReactivateHandler(m *AgentMonitor, taskQueue chan string) IdleHandler {
	return func(agent *AgentStatus) error {
		select {
		case task := <-taskQueue:
			return m.SendTask(agent.WidgetID, task)
		default:
			return m.SendNudge(agent.WidgetID,
				"[AgentMonitor] No tasks queued. Status: "+agent.Status+
					". Reply 'ready' when you can take work.")
		}
	}
}

func BridgeOutboxHandler(m *AgentMonitor) IdleHandler {
	return func(agent *AgentStatus) error {
		msgs, err := m.ReadOutbox(agent.WidgetID, 5)
		if err != nil || len(msgs) == 0 {
			return nil
		}

		for _, msg := range msgs {
			if msg["direction"] == "assistant_reply" || msg["type"] == "reply" {
				content := ""
				if c, ok := msg["message"].(string); ok {
					content = c
				} else if c, ok := msg["content"].(string); ok {
					content = c
				}

				if content != "" {
					nudge := fmt.Sprintf("[AgentMonitor] Bridge message received:\n%s\n\nRespond if needed.", content)
					return m.SendNudge(agent.WidgetID, nudge)
				}
			}
		}
		return nil
	}
}

type Orchestrator struct {
	monitor      *AgentMonitor
	taskQueue    chan string
	agentTargets []string
	ctx          context.Context
	cancel       context.CancelFunc
}

func NewOrchestrator(cfg *MonitorConfig) *Orchestrator {
	ctx, cancel := context.WithCancel(context.Background())
	m := NewMonitor(cfg)
	o := &Orchestrator{
		monitor:   m,
		taskQueue: make(chan string, 100),
		ctx:       ctx,
		cancel:    cancel,
	}

	for _, h := range DefaultIdleHandlers(m) {
		m.OnIdle(h)
	}
	m.OnIdle(BridgeOutboxHandler(m))
	m.OnIdle(ReactivateHandler(m, o.taskQueue))

	return o
}

func (o *Orchestrator) Start() error {
	return o.monitor.Start()
}

func (o *Orchestrator) Stop() {
	o.cancel()
	o.monitor.Stop()
}

func (o *Orchestrator) QueueTask(task string) {
	select {
	case o.taskQueue <- task:
	case <-o.ctx.Done():
	}
}

func (o *Orchestrator) SetTargets(widgetIDs []string) {
	o.agentTargets = widgetIDs
}

func (o *Orchestrator) GetMonitor() *AgentMonitor {
	return o.monitor
}

func (o *Orchestrator) RunOnce() error {
	agents, err := o.monitor.DiscoverAgents()
	if err != nil {
		return err
	}

	for _, agent := range agents {
		if agent.IsIdle && agent.IdleDuration >= o.monitor.config.IdleThreshold {
			for _, handler := range o.monitor.handlers {
				_ = handler(agent)
			}
		}
	}
	return nil
}