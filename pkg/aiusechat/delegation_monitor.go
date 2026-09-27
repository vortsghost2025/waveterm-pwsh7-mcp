package aiusechat

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
)

// DelegationMonitor periodically checks delegations for completion
type DelegationMonitor struct {
	running bool
	cancel  context.CancelFunc
	mu      sync.Mutex
}

var globalDelegationMonitor = &DelegationMonitor{}

// GetDelegationMonitor returns the global delegation monitor
func GetDelegationMonitor() *DelegationMonitor {
	return globalDelegationMonitor
}

// Start begins monitoring delegations in a background goroutine
func (m *DelegationMonitor) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.running = true
	go m.loop(ctx)
}

// Stop stops the monitor background goroutine. Safe to call multiple times.
func (m *DelegationMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return
	}
	m.cancel()
	m.running = false
}

// loop is the main monitoring loop that runs in a goroutine
func (m *DelegationMonitor) loop(ctx context.Context) {
	store := GetDelegationStore()
	store.RestoreFromStore()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.pollDelegations()
			m.pollPendingChatCompletions()
		}
	}
}

// pollDelegations checks all pending delegations for completion
func (m *DelegationMonitor) pollDelegations() {
	store := GetDelegationStore()
	delegations := store.GetAllPending()

	for _, delegation := range delegations {
		m.processDelegation(delegation)
	}
}

// pollPendingChatCompletions checks stored completions whose originating chat may now be active.
func (m *DelegationMonitor) pollPendingChatCompletions() {
	store := GetDelegationStore()
	delegations := store.GetAllPending()

	for _, delegation := range delegations {
		if delegation.DeliveryState != DelegationStatePendingChat {
			continue
		}
		if delegation.NextAttemptAt > 0 && delegation.NextAttemptAt > time.Now().UnixMilli() {
			continue
		}
		if !isChatActive(delegation.OriginatingTabId, delegation.OriginatingChatId) {
			continue
		}
		m.deliverCompletion(delegation)
	}
}

// isChatActive checks if the tab's RTInfo indicates the given chatId is active.
func isChatActive(tabId, chatId string) bool {
	oref := waveobj.MakeORef(waveobj.OType_Tab, tabId)
	rtInfo, err := wshclient.GetRTInfoCommand(
		wshclient.GetBareRpcClient(),
		wshrpc.CommandGetRTInfoData{
			ORef: oref,
		},
		&wshrpc.RpcOpts{
			Route:   wshutil.MakeTabRouteId(tabId),
			Timeout: 5000,
		},
	)
	if err != nil {
		return false
	}
	if rtInfo == nil {
		return false
	}
	return rtInfo.WaveAIChatId == chatId
}

// deliverCompletion delivers a stored completion to the now-active chat.
func (m *DelegationMonitor) deliverCompletion(delegation *DelegationRecord) {
	result := strings.TrimSpace(delegation.ObservedNewOutput)
	if result == "" {
		result = delegation.LastError
		if result == "" {
			result = "delegation completed"
		}
	}

	data := wshrpc.CommandWaveAIAddContextData{
		Text:   result,
		Submit: true,
		ChatId: delegation.OriginatingChatId,
	}

	err := wshclient.WaveAIAddContextCommand(
		wshclient.GetBareRpcClient(),
		data,
		&wshrpc.RpcOpts{
			Route:   wshutil.MakeTabRouteId(delegation.OriginatingTabId),
			Timeout: 10000,
		},
	)

	if err != nil {
		delegation.AttemptCount++
		delegation.LastError = fmt.Sprintf("failed to deliver pending completion: %v", err)
		if hasReachedMaxAttempts(delegation) {
			delegation.DeliveryState = DelegationStateDeliveryFailed
			delegation.NextAttemptAt = -1
		} else {
			delegation.NextAttemptAt = computeNextAttemptAt(delegation.AttemptCount)
		}
		GetDelegationStore().Update(delegation)
		return
	}

	now := time.Now()
	delegation.DeliveredAt = &now
	delegation.DeliveryState = DelegationStateDelivered
	delegation.NextAttemptAt = -1
	delegation.AttemptCount = 0
	GetDelegationStore().Update(delegation)
}

// processDelegation checks a single delegation for state transitions and completion
func (m *DelegationMonitor) processDelegation(delegation *DelegationRecord) {
	if delegation.DeliveryState == DelegationStateDelivered {
		return
	}
	if delegation.DeliveryState == DelegationStateDeliveryFailed {
		return
	}
	if delegation.DeliveryState == DelegationStatePendingChat {
		return
	}

	if time.Since(delegation.DelegatedAt) < 1*time.Second {
		return
	}

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	scrollbackResp, err := wshclient.TermGetScrollbackLinesCommand(
		wshclient.GetBareRpcClient(),
		wshrpc.CommandTermGetScrollbackLinesData{
			LineStart:   0,
			LineEnd:     2000,
			LastCommand: false,
		},
		&wshrpc.RpcOpts{
			Route:   wshutil.MakeFeBlockRouteId(delegation.TargetBlockId),
			Timeout: 5000,
		},
	)
	if err != nil {
		delegation.LastError = fmt.Sprintf("failed to get scrollback: %v", err)
		GetDelegationStore().Update(delegation)
		return
	}

	blockORef := waveobj.MakeORef(waveobj.OType_Block, delegation.TargetBlockId)
	rtInfoResp, err := wshclient.GetRTInfoCommand(
		wshclient.GetBareRpcClient(),
		wshrpc.CommandGetRTInfoData{
			ORef: blockORef,
		},
		&wshrpc.RpcOpts{
			Route:   wshutil.MakeFeBlockRouteId(delegation.TargetBlockId),
			Timeout: 5000,
		},
	)
	if err != nil {
		delegation.LastError = fmt.Sprintf("failed to get RTInfo: %v", err)
		GetDelegationStore().Update(delegation)
		return
	}

	currentOutput := strings.Join(scrollbackResp.Lines, "\n")
	currentState := ""
	if rtInfoResp != nil {
		currentState = rtInfoResp.ShellState
	}

	var newOutput string
	if len(currentOutput) > len(delegation.BaselineOutputMarker) {
		newOutput = currentOutput[len(delegation.BaselineOutputMarker):]
	}

	delegation.LastPolledAt = time.Now()

	switch delegation.DeliveryState {
	case DelegationStateDelegated:
		if newOutput != "" {
			delegation.ObservedNewOutput = newOutput
			delegation.DeliveryState = DelegationStateAgentOutputObserved
			if currentState == "running-command" {
				delegation.TargetRunState = "running"
				delegation.DeliveryState = DelegationStateAgentRunning
			}
		} else if currentState == "running-command" {
			delegation.TargetRunState = "running"
			delegation.DeliveryState = DelegationStateAgentRunning
		}

	case DelegationStateAgentRunning:
		if newOutput != "" && newOutput != delegation.ObservedNewOutput {
			delegation.ObservedNewOutput = newOutput
			delegation.DeliveryState = DelegationStateAgentOutputObserved
		}
		if currentState == "ready" {
			delegation.DeliveryState = DelegationStateAgentCompleted
			delegation.TargetRunState = "completed"
		}

	case DelegationStateAgentOutputObserved:
		if newOutput != "" && newOutput != delegation.ObservedNewOutput {
			delegation.ObservedNewOutput = newOutput
		}
		if currentState == "ready" {
			delegation.DeliveryState = DelegationStateAgentCompleted
			delegation.TargetRunState = "completed"
		}

	case DelegationStateAgentCompleted:
		if newOutput != "" && newOutput != delegation.ObservedNewOutput {
			delegation.ObservedNewOutput = newOutput
		}

		delegation.DeliveryState = DelegationStateDelivering

		result := strings.TrimSpace(delegation.ObservedNewOutput)
		if result == "" {
			result = strings.TrimSpace(currentOutput)
		}

		data := wshrpc.CommandWaveAIAddContextData{
			Text:   result,
			Submit: true,
			ChatId: delegation.OriginatingChatId,
		}

		err := wshclient.WaveAIAddContextCommand(
			wshclient.GetBareRpcClient(),
			data,
			&wshrpc.RpcOpts{
				Route:   wshutil.MakeTabRouteId(delegation.OriginatingTabId),
				Timeout: 10000,
			},
		)

		if err != nil {
			delegation.AttemptCount++
			delegation.LastError = fmt.Sprintf("failed to deliver result: %v", err)

			if strings.Contains(err.Error(), "chat-not-active") {
				storeCompletion := &CompletionRecord{
					DeliveryId:        delegation.DeliveryId,
					OriginatingChatId: delegation.OriginatingChatId,
					OriginatingTabId:  delegation.OriginatingTabId,
					TargetBlockId:     delegation.TargetBlockId,
					CompletedAt:       time.Now().UnixMilli(),
					Result:            result,
					DeliveryState:     DelegationStatePendingChat,
					AttemptCount:      delegation.AttemptCount,
					NextAttemptAt:     computeNextAttemptAt(delegation.AttemptCount),
					LastError:         delegation.LastError,
				}
				GetDelegationStore().StoreCompletion(storeCompletion)
				delegation.DeliveryState = DelegationStatePendingChat
				delegation.NextAttemptAt = computeNextAttemptAt(delegation.AttemptCount)
			} else if hasReachedMaxAttempts(delegation) {
				delegation.DeliveryState = DelegationStateDeliveryFailed
				delegation.NextAttemptAt = -1
			} else {
				delegation.NextAttemptAt = computeNextAttemptAt(delegation.AttemptCount)
			}
			GetDelegationStore().Update(delegation)
			return
		}

		now := time.Now()
		delegation.DeliveredAt = &now
		delegation.DeliveryState = DelegationStateDelivered
		delegation.NextAttemptAt = -1
		delegation.AttemptCount = 0
	}

	GetDelegationStore().Update(delegation)
}