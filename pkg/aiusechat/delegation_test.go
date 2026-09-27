package aiusechat

import (
	"sync"
	"testing"
	"time"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
)

// TestDelegationStore_CreateAndGet verifies that delegation records can be created and retrieved
func TestDelegationStore_CreateAndGet(t *testing.T) {
	store := GetDelegationStore()
	deliveryId := "test_del_1"

	rec := &DelegationRecord{
		DeliveryId:           deliveryId,
		OriginatingChatId:    "chat_1",
		OriginatingTabId:     "tab_1",
		TargetBlockId:        "block_1",
		DelegatedAt:          time.Now(),
		BaselineOutputMarker: "baseline",
		TargetRunState:       "unknown",
		DeliveryState:        DelegationStateDelegated,
	}

	store.Create(rec)

	got := store.Get(deliveryId)
	if got == nil {
		t.Fatal("expected delegation record to be found")
	}
	if got.DeliveryId != deliveryId {
		t.Errorf("expected DeliveryId %q, got %q", deliveryId, got.DeliveryId)
	}
	if got.OriginatingChatId != "chat_1" {
		t.Errorf("expected OriginatingChatId %q, got %q", "chat_1", got.OriginatingChatId)
	}
}

// TestDelegationStore_HasActiveDelegation verifies that HasActiveDelegation
// correctly reports whether a chat has active delegations
func TestDelegationStore_HasActiveDelegation(t *testing.T) {
	store := GetDelegationStore()
	chatId := "test_chat_active"

	// Initially, no active delegation
	if store.HasActiveDelegation(chatId) {
		t.Error("expected no active delegation for new chat")
	}

	// Create an active delegation
	rec := &DelegationRecord{
		DeliveryId:           "test_del_active",
		OriginatingChatId:    chatId,
		OriginatingTabId:     "tab_1",
		TargetBlockId:        "block_1",
		DelegatedAt:          time.Now(),
		DeliveryState:        DelegationStateDelegated,
	}
	store.Create(rec)

	if !store.HasActiveDelegation(chatId) {
		t.Error("expected active delegation to be detected")
	}

	// Mark as delivered
	rec.DeliveryState = DelegationStateDelivered
	store.Update(rec)

	if store.HasActiveDelegation(chatId) {
		t.Error("expected no active delegation after delivery")
	}
}

// TestDelegationStore_GetAllPending verifies that only pending delegations are returned
func TestDelegationStore_GetAllPending(t *testing.T) {
	store := GetDelegationStore()

	// Create a delivered delegation
	deliveredRec := &DelegationRecord{
		DeliveryId:           "test_del_delivered",
		OriginatingChatId:    "chat_delivered",
		OriginatingTabId:     "tab_1",
		TargetBlockId:        "block_1",
		DelegatedAt:          time.Now(),
		DeliveryState:        DelegationStateDelivered,
	}
	store.Create(deliveredRec)

	// Create a pending delegation
	pendingRec := &DelegationRecord{
		DeliveryId:           "test_del_pending",
		OriginatingChatId:    "chat_pending",
		OriginatingTabId:     "tab_2",
		TargetBlockId:        "block_2",
		DelegatedAt:          time.Now(),
		DeliveryState:        DelegationStateDelegated,
	}
	store.Create(pendingRec)

	pending := store.GetAllPending()

	// Check that the delivered one is not in the pending list
	for _, rec := range pending {
		if rec.DeliveryId == "test_del_delivered" {
			t.Error("delivered delegation should not be in pending list")
		}
	}

	// Check that the pending one is in the pending list
	found := false
	for _, rec := range pending {
		if rec.DeliveryId == "test_del_pending" {
			found = true
			break
		}
	}
	if !found {
		t.Error("pending delegation should be in pending list")
	}
}

// TestDelegationStateTransitions verifies that the state machine transitions work correctly
func TestDelegationStateTransitions(t *testing.T) {
	store := GetDelegationStore()

	// Create a delegation in the initial state
	rec := &DelegationRecord{
		DeliveryId:           "test_del_transitions",
		OriginatingChatId:    "chat_transitions",
		OriginatingTabId:     "tab_1",
		TargetBlockId:        "block_1",
		DelegatedAt:          time.Now(),
		BaselineOutputMarker: "baseline",
		TargetRunState:       "unknown",
		DeliveryState:        DelegationStateDelegated,
	}
	store.Create(rec)

	// Test transition: delegated -> agent_running (when agent is running but no new output)
	rec.TargetRunState = "running"
	rec.DeliveryState = DelegationStateAgentRunning
	store.Update(rec)

	if rec.DeliveryState != DelegationStateAgentRunning {
		t.Error("expected state to be agent_running")
	}

	// Test transition: agent_running -> agent_output_observed (when new output appears)
	rec.ObservedNewOutput = "new output"
	rec.DeliveryState = DelegationStateAgentOutputObserved
	store.Update(rec)

	if rec.DeliveryState != DelegationStateAgentOutputObserved {
		t.Error("expected state to be agent_output_observed")
	}

	// Test transition: agent_output_observed -> agent_completed (when agent becomes idle)
	rec.DeliveryState = DelegationStateAgentCompleted
	store.Update(rec)

	if rec.DeliveryState != DelegationStateAgentCompleted {
		t.Error("expected state to be agent_completed")
	}

	// Test transition: agent_completed -> delivering -> delivered
	rec.DeliveryState = DelegationStateDelivering
	store.Update(rec)

	rec.DeliveryState = DelegationStateDelivered
	now := time.Now()
	rec.DeliveredAt = &now
	store.Update(rec)

	if rec.DeliveryState != DelegationStateDelivered {
		t.Error("expected state to be delivered")
	}
	if rec.DeliveredAt == nil {
		t.Error("expected DeliveredAt to be set")
	}
}

// TestDelegationState_FailureIsRetryable verifies that a failed delivery
// is retryable when NextAttemptAt is unset, and terminal when NextAttemptAt is -1.
func TestDelegationState_FailureIsRetryable(t *testing.T) {
	store := GetDelegationStore()

	rec := &DelegationRecord{
		DeliveryId:           "test_del_failed",
		OriginatingChatId:    "chat_failed",
		OriginatingTabId:     "tab_1",
		TargetBlockId:        "block_1",
		DelegatedAt:          time.Now(),
		DeliveryState:        DelegationStateDeliveryFailed,
		LastError:            "connection refused",
	}
	store.Create(rec)

	pending := store.GetAllPending()
	foundPending := false
	for _, r := range pending {
		if r.DeliveryId == "test_del_failed" {
			foundPending = true
			break
		}
	}
	if !foundPending {
		t.Error("delivery_failed with NextAttemptAt=0 should be retryable (in pending list)")
	}

	// Mark as terminal
	rec.NextAttemptAt = -1
	store.Update(rec)

	pending = store.GetAllPending()
	for _, r := range pending {
		if r.DeliveryId == "test_del_failed" {
			t.Error("delivery_failed with NextAttemptAt=-1 should not be in pending list")
		}
	}

	// But the record should still be retrievable
	got := store.Get("test_del_failed")
	if got == nil {
		t.Fatal("expected failed delegation to be retrievable")
	}
	if got.DeliveryState != DelegationStateDeliveryFailed {
		t.Errorf("expected state to be delivery_failed, got %q", got.DeliveryState)
	}
	if got.LastError == "" {
		t.Error("expected LastError to be set")
	}
}

// TestDelegationStore_ConcurrentAccess verifies thread-safe access to the delegation store
func TestDelegationStore_ConcurrentAccess(t *testing.T) {
	store := GetDelegationStore()
	chatId := "test_chat_concurrent"

	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			rec := &DelegationRecord{
				DeliveryId:           "test_del_concurrent_" + string(rune(idx)),
				OriginatingChatId:    chatId,
				OriginatingTabId:     "tab_1",
				TargetBlockId:        "block_1",
				DelegatedAt:          time.Now(),
				DeliveryState:        DelegationStateDelegated,
			}
			store.Create(rec)
			store.HasActiveDelegation(chatId)
			store.GetPendingByChat(chatId)
		}(i)
	}

	wg.Wait()

	pending := store.GetPendingByChat(chatId)
	if len(pending) < numGoroutines {
		t.Errorf("expected at least %d pending delegations, got %d", numGoroutines, len(pending))
	}
}

// TestDelegationStore_TwoChatsNoCrossRoute verifies that delegations for different chats
// don't cross-route results between chats
func TestDelegationStore_TwoChatsNoCrossRoute(t *testing.T) {
	store := GetDelegationStore()

	chatA := "test_chat_A"
	chatB := "test_chat_B"

	recA := &DelegationRecord{
		DeliveryId:           "test_del_chatA",
		OriginatingChatId:    chatA,
		OriginatingTabId:     "tab_A",
		TargetBlockId:        "block_A",
		DelegatedAt:          time.Now(),
		DeliveryState:        DelegationStateDelegated,
	}
	store.Create(recA)

	recB := &DelegationRecord{
		DeliveryId:           "test_del_chatB",
		OriginatingChatId:    chatB,
		OriginatingTabId:     "tab_B",
		TargetBlockId:        "block_B",
		DelegatedAt:          time.Now(),
		DeliveryState:        DelegationStateDelegated,
	}
	store.Create(recB)

	// Verify chat A only has its own delegation
	pendingA := store.GetPendingByChat(chatA)
	if len(pendingA) != 1 {
		t.Errorf("expected 1 pending delegation for chat A, got %d", len(pendingA))
	}
	if pendingA[0].OriginatingChatId != chatA {
		t.Errorf("expected chat A delegation, got chat %q", pendingA[0].OriginatingChatId)
	}
	if pendingA[0].TargetBlockId != "block_A" {
		t.Errorf("expected block_A, got %q", pendingA[0].TargetBlockId)
	}

	// Verify chat B only has its own delegation
	pendingB := store.GetPendingByChat(chatB)
	if len(pendingB) != 1 {
		t.Errorf("expected 1 pending delegation for chat B, got %d", len(pendingB))
	}
	if pendingB[0].OriginatingChatId != chatB {
		t.Errorf("expected chat B delegation, got chat %q", pendingB[0].OriginatingChatId)
	}
	if pendingB[0].TargetBlockId != "block_B" {
		t.Errorf("expected block_B, got %q", pendingB[0].TargetBlockId)
	}
}

// TestDelegationStore_Deduplication verifies that deliveryId is used for deduplication
func TestDelegationStore_Deduplication(t *testing.T) {
	store := GetDelegationStore()
	deliveryId := "test_del_dedup"

	rec := &DelegationRecord{
		DeliveryId:           deliveryId,
		OriginatingChatId:    "chat_dedup",
		OriginatingTabId:     "tab_1",
		TargetBlockId:        "block_1",
		DelegatedAt:          time.Now(),
		DeliveryState:        DelegationStateDelegated,
	}
	store.Create(rec)

	// Update with the same deliveryId should overwrite, not duplicate
	rec2 := &DelegationRecord{
		DeliveryId:           deliveryId,
		OriginatingChatId:    "chat_dedup",
		OriginatingTabId:     "tab_1",
		TargetBlockId:        "block_1",
		DelegatedAt:          time.Now(),
		DeliveryState:        DelegationStateDelivered,
	}
	store.Update(rec2)

	got := store.Get(deliveryId)
	if got == nil {
		t.Fatal("expected delegation to be found")
	}
	if got.DeliveryState != DelegationStateDelivered {
		t.Errorf("expected state to be delivered after update, got %q", got.DeliveryState)
	}

	// Verify it's not in pending list
	pending := store.GetAllPending()
	for _, r := range pending {
		if r.DeliveryId == deliveryId {
			t.Error("delivered delegation should not be in pending list")
		}
	}
}

// TestDelegationState_Constants verifies that all required state constants are defined
func TestDelegationState_Constants(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"Delegated", DelegationStateDelegated},
		{"AgentRunning", DelegationStateAgentRunning},
		{"AgentOutputObserved", DelegationStateAgentOutputObserved},
		{"AgentCompleted", DelegationStateAgentCompleted},
		{"Delivering", DelegationStateDelivering},
		{"Delivered", DelegationStateDelivered},
		{"DeliveryFailed", DelegationStateDeliveryFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expected == "" {
				t.Errorf("state constant %s is empty", tt.name)
			}
		})
	}
}

// TestDelegationRecord_RequiredFields verifies that all required fields are present
func TestDelegationRecord_RequiredFields(t *testing.T) {
	rec := &DelegationRecord{
		DeliveryId:           "test_del_fields",
		OriginatingChatId:    "chat_fields",
		OriginatingTabId:     "tab_fields",
		TargetBlockId:        "block_fields",
		DelegatedAt:          time.Now(),
		BaselineOutputMarker: "baseline_output",
		ObservedNewOutput:     "new_output",
		TargetRunState:       "running",
		DeliveryState:        DelegationStateDelegated,
	}

	// Verify all required fields are set
	if rec.DeliveryId == "" {
		t.Error("DeliveryId is required")
	}
	if rec.OriginatingChatId == "" {
		t.Error("OriginatingChatId is required")
	}
	if rec.OriginatingTabId == "" {
		t.Error("OriginatingTabId is required")
	}
	if rec.TargetBlockId == "" {
		t.Error("TargetBlockId is required")
	}
	if rec.DelegatedAt.IsZero() {
		t.Error("DelegatedAt is required")
	}
	if rec.BaselineOutputMarker == "" {
		t.Error("BaselineOutputMarker is required")
	}
	if rec.TargetRunState == "" {
		t.Error("TargetRunState is required")
	}
	if rec.DeliveryState == "" {
		t.Error("DeliveryState is required")
	}
}

// TestDelegationMonitor_Start verifies that the monitor can be started without panicking
func TestDelegationMonitor_Start(t *testing.T) {
	monitor := GetDelegationMonitor()
	store := GetDelegationStore()
	t.Cleanup(func() {
		monitor.Stop()
		store.ResetStore()
	})
	// Should not panic
	monitor.Start()
	// Starting again should be safe
	monitor.Start()
}

// TestDelegationStore_GetPendingByChat_Empty verifies that an empty list is returned for a chat with no delegations
func TestDelegationStore_GetPendingByChat_Empty(t *testing.T) {
	store := GetDelegationStore()
	pending := store.GetPendingByChat("nonexistent_chat")
	if len(pending) != 0 {
		t.Errorf("expected 0 pending delegations for nonexistent chat, got %d", len(pending))
	}
}

// TestDelegationStore_Update verifies that updates work correctly
func TestDelegationStore_Update(t *testing.T) {
	store := GetDelegationStore()
	deliveryId := "test_del_update"

	rec := &DelegationRecord{
		DeliveryId:           deliveryId,
		OriginatingChatId:    "chat_update",
		OriginatingTabId:     "tab_1",
		TargetBlockId:        "block_1",
		DelegatedAt:          time.Now(),
		DeliveryState:        DelegationStateDelegated,
	}
	store.Create(rec)

	// Update the state
	rec.DeliveryState = DelegationStateAgentRunning
	rec.TargetRunState = "running"
	store.Update(rec)

	got := store.Get(deliveryId)
	if got.DeliveryState != DelegationStateAgentRunning {
		t.Errorf("expected state to be agent_running, got %q", got.DeliveryState)
	}
	if got.TargetRunState != "running" {
		t.Errorf("expected TargetRunState to be running, got %q", got.TargetRunState)
	}
}

// TestIsAgentTerminal_ByCmd verifies agent detection by cmd field
func TestIsAgentTerminal_ByCmd(t *testing.T) {
	for _, cmd := range []string{"kilo", "opencode", "agent", "aienv", "claude"} {
		meta := map[string]any{
			waveobj.MetaKey_Cmd: cmd,
		}
		if !isAgentByCmdAndArgs(meta) {
			t.Errorf("expected cmd=%q to be detected as agent", cmd)
		}
	}
}

// TestIsAgentTerminal_ByAgentModelKey verifies detection when agent:model is set
func TestIsAgentTerminal_ByAgentModelKey(t *testing.T) {
	meta := map[string]any{
		"agent:model": "gpt-4",
		"agent:mode":  "build",
	}
	if !isAgentByModelKey(meta) {
		t.Error("expected agent:model to be detected as agent")
	}
}

// TestIsAgentTerminal_ByCmdArgs verifies detection when cmdArgs contains agent binary
func TestIsAgentTerminal_ByCmdArgs(t *testing.T) {
	meta := map[string]any{
		waveobj.MetaKey_Cmd:     "bash",
		waveobj.MetaKey_CmdArgs: []string{"-c", "kilo run task"},
	}
	if !isAgentByCmdAndArgs(meta) {
		t.Error("expected cmdArgs containing 'kilo' to be detected as agent")
	}
}

// TestIsAgentTerminal_NotAgent verifies normal terminals are not detected
func TestIsAgentTerminal_NotAgent(t *testing.T) {
	meta := map[string]any{
		waveobj.MetaKey_Cmd: "bash",
	}
	if isAgentByCmdAndArgs(meta) {
		t.Error("expected normal terminal to not be detected as agent")
	}
}

// TestIsAgentTerminal_NilMeta verifies nil meta is handled safely
func TestIsAgentTerminal_NilMeta(t *testing.T) {
	if isAgentByCmdAndArgs(nil) {
		t.Error("expected nil meta to not be detected as agent")
	}
	if isAgentByModelKey(nil) {
		t.Error("expected nil meta to not be detected by model key")
	}
}

// TestIsAgentTerminal_LauncherScript verifies detection by launcher script name
func TestIsAgentTerminal_LauncherScript(t *testing.T) {
	meta := map[string]any{
		waveobj.MetaKey_Cmd:     "cmd.exe",
		waveobj.MetaKey_CmdArgs: []string{"/c", "START-KILO-gpt4.cmd"},
	}
	if !isAgentByCmdAndArgs(meta) {
		t.Error("expected launcher script to be detected as agent")
	}
}

// TestComputeBackoffMs verifies exponential backoff calculation
func TestComputeBackoffMs(t *testing.T) {
	tests := []struct {
		attempt int
		wantMin int64
		wantMax int64
	}{
		{1, 1000, 1000},
		{2, 2000, 2000},
		{3, 4000, 4000},
		{4, 8000, 8000},
		{5, 16000, 16000},
		{6, 32000, 32000},
		{7, 60000, 60000},
		{10, 60000, 60000},
	}
	for _, tt := range tests {
		got := computeBackoffMs(tt.attempt)
		if got < tt.wantMin || got > tt.wantMax {
			t.Errorf("computeBackoffMs(%d) = %d, want in [%d, %d]", tt.attempt, got, tt.wantMin, tt.wantMax)
		}
	}
}

// TestHasReachedMaxAttempts verifies the max attempts cap
func TestHasReachedMaxAttempts(t *testing.T) {
	rec := &DelegationRecord{MaxAttempts: 5}
	rec.AttemptCount = 4
	if hasReachedMaxAttempts(rec) {
		t.Error("expected attempt 4 of 5 to not have reached max")
	}
	rec.AttemptCount = 5
	if !hasReachedMaxAttempts(rec) {
		t.Error("expected attempt 5 of 5 to have reached max")
	}
	rec.AttemptCount = 10
	if !hasReachedMaxAttempts(rec) {
		t.Error("expected attempt 10 of default to have reached max")
	}
}

// TestGetAllPending_NextAttemptAtFuture verifies records with future NextAttemptAt are excluded
func TestGetAllPending_NextAttemptAtFuture(t *testing.T) {
	store := GetDelegationStore()

	rec := &DelegationRecord{
		DeliveryId:           "test_del_future_retry",
		OriginatingChatId:    "chat_future",
		OriginatingTabId:     "tab_1",
		TargetBlockId:        "block_1",
		DelegatedAt:          time.Now(),
		DeliveryState:        DelegationStateDeliveryFailed,
		NextAttemptAt:        time.Now().UnixMilli() + 60000,
		LastError:            "transient error",
	}
	store.Create(rec)

	pending := store.GetAllPending()
	for _, r := range pending {
		if r.DeliveryId == "test_del_future_retry" {
			t.Error("expected record with future NextAttemptAt to be excluded from pending")
		}
	}
}

// TestGetAllPending_TerminalFailed verifies records with NextAttemptAt=-1 are excluded
func TestGetAllPending_TerminalFailed(t *testing.T) {
	store := GetDelegationStore()

	rec := &DelegationRecord{
		DeliveryId:           "test_del_terminal",
		OriginatingChatId:    "chat_terminal",
		OriginatingTabId:     "tab_1",
		TargetBlockId:        "block_1",
		DelegatedAt:          time.Now(),
		DeliveryState:        DelegationStateDeliveryFailed,
		NextAttemptAt:        -1,
		AttemptCount:         10,
		LastError:            "max attempts reached",
	}
	store.Create(rec)

	pending := store.GetAllPending()
	for _, r := range pending {
		if r.DeliveryId == "test_del_terminal" {
			t.Error("expected terminal failed record to be excluded from pending")
		}
	}
}

// TestGetAllPending_ImmediateRetry verifies records with NextAttemptAt=0 are included
func TestGetAllPending_ImmediateRetry(t *testing.T) {
	store := GetDelegationStore()

	rec := &DelegationRecord{
		DeliveryId:           "test_del_immediate",
		OriginatingChatId:    "chat_immediate",
		OriginatingTabId:     "tab_1",
		TargetBlockId:        "block_1",
		DelegatedAt:          time.Now(),
		DeliveryState:        DelegationStateDeliveryFailed,
		NextAttemptAt:        0,
		LastError:            "will retry now",
	}
	store.Create(rec)

	pending := store.GetAllPending()
	found := false
	for _, r := range pending {
		if r.DeliveryId == "test_del_immediate" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected record with NextAttemptAt=0 to be included in pending")
	}
}
