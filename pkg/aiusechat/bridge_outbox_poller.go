// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wavetermdev/waveterm/pkg/wconfig"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
)

// BridgeOutboxPoller periodically checks the bridge outbox for agent completions
type BridgeOutboxPoller struct {
	running    bool
	cancel     context.CancelFunc
	mu         sync.Mutex
	lastOffset int64
	outboxPath string
}

var globalBridgeOutboxPoller = &BridgeOutboxPoller{
	outboxPath: BridgeOutboxDefaultPath,
}

// GetBridgeOutboxPoller returns the global bridge outbox poller
func GetBridgeOutboxPoller() *BridgeOutboxPoller {
	return globalBridgeOutboxPoller
}

// Start begins polling the bridge outbox in a background goroutine
func (p *BridgeOutboxPoller) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.running = true
	go p.loop(ctx)
}

// Stop stops the poller background goroutine. Safe to call multiple times.
func (p *BridgeOutboxPoller) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return
	}
	p.cancel()
	p.running = false
}

// loop is the main polling loop that runs in a goroutine
func (p *BridgeOutboxPoller) loop(ctx context.Context) {
	// Initialize offset to current file size
	p.initializeOffset()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.pollOutbox()
		}
	}
}

// initializeOffset sets the initial read offset to 0 to ensure no messages are missed
func (p *BridgeOutboxPoller) initializeOffset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastOffset = 0
}

// pollOutbox reads new messages from the bridge outbox and routes them to Wave AI chat
func (p *BridgeOutboxPoller) pollOutbox() {
	p.mu.Lock()
	defer p.mu.Unlock()

	file, err := os.Open(p.outboxPath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		log.Printf("bridge outbox poller: failed to open outbox: %v", err)
		return
	}
	defer file.Close()

	if _, err := file.Seek(p.lastOffset, 0); err != nil {
		log.Printf("bridge outbox poller: failed to seek: %v", err)
		return
	}

	scanner := bufio.NewScanner(file)
	var currentOffset int64 = p.lastOffset
	messagesRead := 0
	for scanner.Scan() {
		line := scanner.Text()
		lineLen := int64(len(line) + 1)
		if strings.TrimSpace(line) == "" {
			currentOffset += lineLen
			continue
		}

		var msg BridgeMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			log.Printf("bridge outbox poller: failed to parse message: %v", err)
			currentOffset += lineLen
			continue
		}

		p.routeCompletionToChat(msg)
		messagesRead++
		currentOffset += lineLen
	}

	p.lastOffset = currentOffset

	if messagesRead > 0 {
		log.Printf("bridge outbox poller: read and routed %d messages", messagesRead)
	}
}

// routeCompletionToChat routes an agent completion to the active Wave AI chat
func (p *BridgeOutboxPoller) routeCompletionToChat(msg BridgeMessage) {
	// Only route completions and replies from agents
	if msg.Direction != "agent_to_wave" && msg.Direction != "assistant_reply" && msg.Direction != "opencode_to_waveai" {
		return
	}

	chatId, tabId := getActiveWaveAIChatInfo()

	data := wshrpc.CommandWaveAIAddContextData{
		Text:   msg.Message,
		Submit: true,
	}

	if chatId != "" && tabId != "" {
		data.ChatId = chatId
	} else if tabId != "" {
		data.NewChat = true
		log.Printf("bridge outbox poller: no active chat, creating new chat for message from %s", msg.Source)
	} else {
		log.Printf("bridge outbox poller: no active tab found, dropping message from %s", msg.Source)
		return
	}

	// Route directly to the frontend via tab route (bypasses server-side sourceRoute check)
	err := wshclient.WaveAIAddContextCommand(
		wshclient.GetBareRpcClient(),
		data,
		&wshrpc.RpcOpts{
			Route:   wshutil.MakeTabRouteId(tabId),
			Timeout: 10000,
		},
	)

	if err != nil {
		log.Printf("bridge outbox poller: failed to route message from %s: %v", msg.Source, err)
	} else {
		log.Printf("bridge outbox poller: routed message from %s (chat=%s new=%v)", msg.Source, chatId, data.NewChat)
	}
}

var (
	lastActiveChatId string
	lastActiveTabId  string
	lastActiveMu     sync.RWMutex
)

// SetLastActiveChatInfo stores the chat ID and tab ID for the poller to use
func SetLastActiveChatInfo(chatId, tabId string) {
	lastActiveMu.Lock()
	defer lastActiveMu.Unlock()
	lastActiveChatId = chatId
	lastActiveTabId = tabId
}

// ClearLastActiveChatInfo clears the chat ID when the chat ends, but keeps the tab ID
// so the poller can continue routing messages to new chats in the same tab
func ClearLastActiveChatInfo(chatId string) {
	lastActiveMu.Lock()
	defer lastActiveMu.Unlock()
	if lastActiveChatId == chatId {
		lastActiveChatId = ""
	}
}

// getActiveWaveAIChatInfo returns the active chat ID and tab ID.
// The tab ID is always returned (kept from last known value) so the poller
// can always route messages even when no chat is currently active.
func getActiveWaveAIChatInfo() (string, string) {
	lastActiveMu.RLock()
	chatId := lastActiveChatId
	tabId := lastActiveTabId
	lastActiveMu.RUnlock()

	if tabId == "" {
		return "", ""
	}

	if chatId == "" {
		return "", tabId
	}

	if _, ok := activeChats.GetEx(chatId); ok {
		return chatId, tabId
	}

	return "", tabId
}

// isAgentPairingEnabled checks if the ai:agentpairing setting is on
func isAgentPairingEnabled() bool {
	fullConfig := wconfig.GetWatcher().GetFullConfig()
	if fullConfig.Settings.AiAgentPairing == nil {
		return false
	}
	return *fullConfig.Settings.AiAgentPairing
}

// writePairingTextToBridge writes assistant text to the bridge outbox
// when agent pairing mode is enabled
func writePairingTextToBridge(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	path := bridgeOutboxPath()
	dirPath := filepath.Dir(path)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		log.Printf("pairing: failed to create bridge directory: %v", err)
		return
	}

	msg := BridgeMessage{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Type:      "pairing",
		Direction: "wave_to_agent",
		Source:    "wave-ai-pairing",
		Target:    "janitor-wave-ai",
		Message:   text,
		Content:   text,
	}
	line, err := json.Marshal(msg)
	if err != nil {
		log.Printf("pairing: failed to marshal message: %v", err)
		return
	}
	line = append(line, '\n')

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("pairing: failed to open bridge outbox: %v", err)
		return
	}
	defer file.Close()

	if _, err := file.Write(line); err != nil {
		log.Printf("pairing: failed to write bridge outbox: %v", err)
		return
	}
	log.Printf("pairing: wrote %d bytes to bridge outbox", len(line))
}