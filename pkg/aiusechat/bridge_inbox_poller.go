// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wstore"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
)

const (
	kiloWidgetId = "11c849c6"
)

// BridgeInboxPoller periodically checks the bridge inbox for messages targeting Kilo
type BridgeInboxPoller struct {
	running    bool
	cancel     context.CancelFunc
	mu         sync.Mutex
	lastOffset int64
	inboxPath  string
}

var globalBridgeInboxPoller = &BridgeInboxPoller{
	inboxPath: BridgeInboxDefaultPath,
}

// GetBridgeInboxPoller returns the global bridge inbox poller
func GetBridgeInboxPoller() *BridgeInboxPoller {
	return globalBridgeInboxPoller
}

// Start begins polling the bridge inbox in a background goroutine
func (p *BridgeInboxPoller) Start() {
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
func (p *BridgeInboxPoller) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return
	}
	p.cancel()
	p.running = false
}

// loop is the main polling loop that runs in a goroutine
func (p *BridgeInboxPoller) loop(ctx context.Context) {
	p.initializeOffset()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.pollInbox()
		}
	}
}

// initializeOffset sets the initial read offset to the end of the file
// so we don't inject historical messages on first start
func (p *BridgeInboxPoller) initializeOffset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	info, err := os.Stat(p.inboxPath)
	if err != nil {
		p.lastOffset = 0
		return
	}
	p.lastOffset = info.Size()
}

// pollInbox reads new messages from the bridge inbox and injects them into Kilo's terminal
func (p *BridgeInboxPoller) pollInbox() {
	p.mu.Lock()
	defer p.mu.Unlock()

	file, err := os.Open(p.inboxPath)
	if err != nil {
		return
	}
	defer file.Close()

	if _, err := file.Seek(p.lastOffset, 0); err != nil {
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
			currentOffset += lineLen
			continue
		}

		if p.shouldDeliver(msg) {
			p.deliverToTerminal(msg)
			messagesRead++
		}
		currentOffset += lineLen
	}

	p.lastOffset = currentOffset

	if messagesRead > 0 {
		log.Printf("bridge inbox poller: injected %d messages into Kilo terminal", messagesRead)
	}
}

// shouldDeliver filters messages targeting Kilo
func (p *BridgeInboxPoller) shouldDeliver(msg BridgeMessage) bool {
	if msg.Direction != "wave_to_agent" {
		return false
	}
	if msg.Target == "" {
		return false
	}
	return msg.Target == "janitor-wave-ai" || msg.Target == "kilo-cli"
}

// deliverToTerminal resolves Kilo's block ID and sends the message via AgentSendInputCommand
func (p *BridgeInboxPoller) deliverToTerminal(msg BridgeMessage) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	blockId, err := resolveBlockIdForWidget(ctx, kiloWidgetId)
	if err != nil || blockId == "" {
		log.Printf("bridge inbox poller: failed to resolve block for widget %s: %v", kiloWidgetId, err)
		return
	}

	err = wshclient.AgentSendInputCommand(
		wshclient.GetBareRpcClient(),
		wshrpc.AgentSendInputData{
			BlockId:   blockId,
			InputData: msg.Message,
		},
		&wshrpc.RpcOpts{
			Route:   wshutil.MakeFeBlockRouteId(blockId),
			Timeout: 5000,
		},
	)
	if err != nil {
		log.Printf("bridge inbox poller: AgentSendInputCommand failed for block %s: %v", blockId, err)
		return
	}
	log.Printf("bridge inbox poller: delivered message to block %s", blockId)
}

// resolveBlockIdForWidget scans all blocks across all tabs for one whose OID
// starts with the given widget prefix, and returns the full block OID.
func resolveBlockIdForWidget(ctx context.Context, widgetPrefix string) (string, error) {
	tabs, err := wstore.DBGetAllObjsByType[*waveobj.Tab](ctx, waveobj.OType_Tab)
	if err != nil {
		return "", err
	}
	for _, tab := range tabs {
		if tab == nil {
			continue
		}
		for _, blockId := range tab.BlockIds {
			if strings.HasPrefix(blockId, widgetPrefix) {
				block, err := wstore.DBGet[*waveobj.Block](ctx, blockId)
				if err != nil || block == nil {
					continue
				}
				vt, ok := block.Meta["view"].(string)
				if !ok || vt != "term" {
					continue
				}
				return blockId, nil
			}
		}
	}
	return "", fmt.Errorf("no terminal block found with prefix %s", widgetPrefix)
}
