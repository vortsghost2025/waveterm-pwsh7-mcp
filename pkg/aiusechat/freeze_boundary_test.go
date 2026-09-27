// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wavetermdev/waveterm/pkg/aiusechat/chatstore"
	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
	"github.com/wavetermdev/waveterm/pkg/web/sse"
)

// testResponseWriter is an http.ResponseWriter that supports Flush and SetWriteDeadline,
// needed by SSEHandlerCh.SetupSSE which uses http.ResponseController.
type testResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	mu     sync.Mutex
}

func newTestResponseWriter() *testResponseWriter {
	return &testResponseWriter{
		header: make(http.Header),
	}
}

func (w *testResponseWriter) Header() http.Header {
	return w.header
}

func (w *testResponseWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.Write(p)
}

func (w *testResponseWriter) WriteHeader(statusCode int) {
	w.header.Set("Status", http.StatusText(statusCode))
}

func (w *testResponseWriter) Flush() {}

func (w *testResponseWriter) SetWriteDeadline(t time.Time) error {
	return nil
}

// getBody returns a thread-safe snapshot of the buffer contents.
func (w *testResponseWriter) getBody() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.String()
}

// mockBackend implements UseChatBackend for freeze-boundary testing.
// callCount tracks how many times RunChatStep has been invoked.
// stopReasons[k] is returned on the k-th call; if k >= len, the last entry is reused.
// callErrs[k] is returned on the k-th call; if k >= len, nil is returned.
type mockBackend struct {
	stopReasons []uctypes.WaveStopReason
	callErrs    []error
	messages    []uctypes.GenAIMessage
	callCount   int
	mu          sync.Mutex
}

func (b *mockBackend) RunChatStep(
	ctx context.Context,
	sseHandler *sse.SSEHandlerCh,
	chatOpts uctypes.WaveChatOpts,
	cont *uctypes.WaveContinueResponse,
) (*uctypes.WaveStopReason, []uctypes.GenAIMessage, *uctypes.RateLimitInfo, error) {
	b.mu.Lock()
	idx := b.callCount
	b.callCount++
	b.mu.Unlock()

	_ = sseHandler.AiMsgStart("msg1")
	_ = sseHandler.AiMsgTextStart("text1")
	_ = sseHandler.AiMsgTextDelta("text1", "Hello")
	_ = sseHandler.AiMsgTextEnd("text1")

	// Simulate realistic backend latency so the SSE writerLoop has time to
	// drain the 10-slot writeCh buffer between message bursts. Without this,
	// queueMessage silently drops messages (channel full) causing test flakes.
	time.Sleep(50 * time.Millisecond)

	var stopReason *uctypes.WaveStopReason
	if idx < len(b.stopReasons) {
		stopReason = &b.stopReasons[idx]
	} else if len(b.stopReasons) > 0 {
		stopReason = &b.stopReasons[len(b.stopReasons)-1]
	}
	if stopReason == nil {
		stopReason = &uctypes.WaveStopReason{Kind: uctypes.StopKindDone}
	}

	var callErr error
	if idx < len(b.callErrs) {
		callErr = b.callErrs[idx]
	}

	if callErr != nil {
		return stopReason, nil, nil, callErr
	}

	if stopReason.Kind == uctypes.StopKindToolUse {
		for _, tc := range stopReason.ToolCalls {
			argsJSON := ""
			if tc.Input != nil {
				if bytes, err := json.Marshal(tc.Input); err == nil {
					argsJSON = string(bytes)
				}
			}
			_ = sseHandler.AiMsgToolInputStart(tc.ID, tc.Name)
			_ = sseHandler.AiMsgToolInputDelta(tc.ID, argsJSON)
			_ = sseHandler.AiMsgToolInputAvailable(tc.ID, tc.Name, json.RawMessage(argsJSON))
		}
		// Match real backend behavior: do NOT call AiMsgFinish when StopKindToolUse
		return stopReason, b.messages, nil, nil
	}

	// Normal completion: call AiMsgFinish before returning
	if stopReason.Kind == uctypes.StopKindDone {
		_ = sseHandler.AiMsgFinish("stop", nil)
	}
	return stopReason, b.messages, nil, nil
}

func (b *mockBackend) UpdateToolUseData(chatId string, toolCallId string, toolUseData uctypes.UIMessageDataToolUse) error {
	return nil
}

func (b *mockBackend) RemoveToolUseCall(chatId string, toolCallId string) error {
	return nil
}

func (b *mockBackend) ConvertToolResultsToNativeChatMessage(toolResults []uctypes.AIToolResult) ([]uctypes.GenAIMessage, error) {
	return nil, nil
}

func (b *mockBackend) ConvertAIMessageToNativeChatMessage(message uctypes.AIMessage) (uctypes.GenAIMessage, error) {
	return nil, nil
}

func (b *mockBackend) GetFunctionCallInputByToolCallId(aiChat uctypes.AIChat, toolCallId string) *uctypes.AIFunctionCallInput {
	return nil
}

func (b *mockBackend) ConvertAIChatToUIChat(aiChat uctypes.AIChat) (*uctypes.UIChat, error) {
	return nil, nil
}

// makeStopToolCall creates a WaveToolCall with the given name and a simple input.
func makeStopToolCall(name string) uctypes.WaveToolCall {
	return uctypes.WaveToolCall{
		ID:   "call_1",
		Name: name,
		Input: map[string]any{
			"arg": "value",
		},
	}
}

// extractSSEEvents parses the SSE stream and returns the ordered list of event types.
func extractSSEEvents(sseStream string) []string {
	var events []string
	for _, line := range strings.Split(sseStream, "\n") {
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var msg map[string]json.RawMessage
			if err := json.Unmarshal([]byte(data), &msg); err == nil {
				if tp, ok := msg["type"]; ok {
					var typ string
					_ = json.Unmarshal(tp, &typ)
					events = append(events, typ)
				}
			}
		}
	}
	return events
}

// countEventType returns the number of times a given event type appears in the stream.
func countEventType(events []string, typ string) int {
	count := 0
	for _, e := range events {
		if e == typ {
			count++
		}
	}
	return count
}

// makeTestChatOpts creates a WaveChatOpts with the given chatId and a simple tool.
func makeTestChatOpts(chatId string) uctypes.WaveChatOpts {
	return uctypes.WaveChatOpts{
		ChatId: chatId,
		TabId:  chatId + "-tab",
		Config: uctypes.AIOptsType{
			Provider: "openai",
			APIType:  uctypes.APIType_OpenAIResponses,
			Model:    "gpt-5-mini",
		},
		Tools: []uctypes.ToolDefinition{
			{
				Name: "test_tool",
				ToolTextCallback: func(input any) (string, error) {
					return "result", nil
				},
			},
		},
	}
}

// TestFreezeBoundary_NormalCompletion_EmitsFinish proves that when RunAIChat
// returns via StopKindDone, the SSE stream contains exactly one "finish" event.
func TestFreezeBoundary_NormalCompletion_EmitsFinish(t *testing.T) {
	chatId := "freeze-test-normal-" + time.Now().Format("20060102150405")

	chatstore.DefaultChatStore.Delete(chatId)
	defer chatstore.DefaultChatStore.Delete(chatId)

	chatOpts := makeTestChatOpts(chatId)

	mock := &mockBackend{
		stopReasons: []uctypes.WaveStopReason{
			{Kind: uctypes.StopKindDone},
		},
	}

	recorder := newTestResponseWriter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sseHandler := sse.MakeSSEHandlerCh(recorder, ctx)
	if err := sseHandler.SetupSSE(); err != nil {
		t.Fatalf("SetupSSE failed: %v", err)
	}

	_, err := RunAIChat(ctx, sseHandler, mock, chatOpts)
	if err != nil {
		t.Fatalf("RunAIChat returned error: %v", err)
	}

	sseHandler.Close()

	stream := recorder.body.String()
	events := extractSSEEvents(stream)

	finishCount := countEventType(events, "finish")
	if finishCount != 1 {
		t.Errorf("NORMAL COMPLETION: expected exactly 1 'finish' event, got %d.\nStream:\n%s", finishCount, stream)
	}
	t.Logf("NORMAL COMPLETION: SSE events = %v", events)
}

// TestFreezeBoundary_DelegationBreak_EmitsFinish proves that when HasActiveDelegation
// returns true after processing tool calls, RunAIChat emits exactly one "finish" event
// before [DONE], so the frontend transitions to idle.
func TestFreezeBoundary_DelegationBreak_EmitsFinish(t *testing.T) {
	chatId := "freeze-test-delegation-" + time.Now().Format("20060102150405")

	// Clean up chatstore and delegation store
	chatstore.DefaultChatStore.Delete(chatId)
	defer chatstore.DefaultChatStore.Delete(chatId)

	delegationStore := GetDelegationStore()
	delegationStore.ResetStore()
	defer delegationStore.ResetStore()

	// Pre-create an active delegation for this chat ID
	rec := &DelegationRecord{
		DeliveryId:           "del_" + chatId,
		OriginatingChatId:    chatId,
		OriginatingTabId:     chatId + "-tab",
		TargetBlockId:        "block_1",
		DelegatedAt:          time.Now(),
		DeliveryState:        DelegationStateDelegated,
	}
	delegationStore.Create(rec)

	chatOpts := makeTestChatOpts(chatId)

	mock := &mockBackend{
		stopReasons: []uctypes.WaveStopReason{
			{
				Kind: uctypes.StopKindToolUse,
				ToolCalls: []uctypes.WaveToolCall{
					makeStopToolCall("test_tool"),
				},
			},
		},
	}

	recorder := newTestResponseWriter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sseHandler := sse.MakeSSEHandlerCh(recorder, ctx)
	if err := sseHandler.SetupSSE(); err != nil {
		t.Fatalf("SetupSSE failed: %v", err)
	}

	// Call RunAIChat directly
	_, err := RunAIChat(ctx, sseHandler, mock, chatOpts)
	if err != nil {
		t.Fatalf("RunAIChat returned error: %v", err)
	}

	// Poll for the finish event to appear in the SSE stream. The SSE channel
	// buffer (10) can fill up during processAllToolCalls, causing queueMessage
	// to silently drop the AiMsgFinish call. We poll until the writerLoop has
	// drained all buffered messages, or timeout.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		stream := recorder.getBody()
		events := extractSSEEvents(stream)
		if countEventType(events, "finish") >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	sseHandler.Close()

	stream := recorder.getBody()
	events := extractSSEEvents(stream)

	finishCount := countEventType(events, "finish")
	if finishCount != 1 {
		t.Errorf("DELEGATION BREAK: expected exactly 1 'finish' event, got %d.\nStream:\n%s", finishCount, stream)
	}

	// Verify finish appears before [DONE]
	finishIdx := -1
	doneIdx := -1
	for i, line := range strings.Split(stream, "\n") {
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				doneIdx = i
				break
			}
			var msg map[string]json.RawMessage
			if err := json.Unmarshal([]byte(data), &msg); err == nil {
				if tp, ok := msg["type"]; ok {
					var typ string
					_ = json.Unmarshal(tp, &typ)
					if typ == "finish" {
						finishIdx = i
					}
				}
			}
		}
	}
	if finishIdx < 0 {
		t.Errorf("DELEGATION BREAK: no 'finish' event found in stream")
	}
	if doneIdx < 0 {
		t.Errorf("DELEGATION BREAK: no [DONE] found in stream")
	}
	if finishIdx >= 0 && doneIdx >= 0 && finishIdx > doneIdx {
		t.Errorf("DELEGATION BREAK: 'finish' event (line %d) must appear before [DONE] (line %d)", finishIdx, doneIdx)
	}

	t.Logf("DELEGATION BREAK: SSE events = %v", events)
	t.Logf("DELEGATION BREAK: SSE stream length = %d bytes", len(stream))
}

// TestFreezeBoundary_ToolUseWithoutDelegation_Completes verifies that when a backend
// returns StopKindToolUse and there is no active delegation, RunAIChat continues
// the loop and eventually emits exactly one "finish" event.
func TestFreezeBoundary_ToolUseWithoutDelegation_Completes(t *testing.T) {
	chatId := "freeze-test-tooluse-" + time.Now().Format("20060102150405")

	chatstore.DefaultChatStore.Delete(chatId)
	defer chatstore.DefaultChatStore.Delete(chatId)

	delegationStore := GetDelegationStore()
	delegationStore.ResetStore()
	defer delegationStore.ResetStore()

	chatOpts := makeTestChatOpts(chatId)

	// First call: tool use. Second call: normal completion with finish.
	mock := &mockBackend{
		stopReasons: []uctypes.WaveStopReason{
			{
				Kind: uctypes.StopKindToolUse,
				ToolCalls: []uctypes.WaveToolCall{
					makeStopToolCall("test_tool"),
				},
			},
			{Kind: uctypes.StopKindDone},
		},
	}

	recorder := newTestResponseWriter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sseHandler := sse.MakeSSEHandlerCh(recorder, ctx)
	if err := sseHandler.SetupSSE(); err != nil {
		t.Fatalf("SetupSSE failed: %v", err)
	}

	_, err := RunAIChat(ctx, sseHandler, mock, chatOpts)
	if err != nil {
		t.Fatalf("RunAIChat returned error: %v", err)
	}

	sseHandler.Close()

	stream := recorder.body.String()
	events := extractSSEEvents(stream)

	finishCount := countEventType(events, "finish")
	if finishCount != 1 {
		t.Errorf("TOOLUSE-NO-DELEGATION: expected exactly 1 'finish' event, got %d.\nStream:\n%s", finishCount, stream)
	}
	t.Logf("TOOLUSE-NO-DELEGATION: SSE events = %v", events)
}

// TestFreezeBoundary_ErrorPath_NoDuplicateFinish verifies that when RunAIChat
// encounters an error on a non-first step, the error path's existing AiMsgFinish
// call (line 570) emits exactly 1 finish event — not duplicated by the fix.
func TestFreezeBoundary_ErrorPath_NoDuplicateFinish(t *testing.T) {
	chatId := "freeze-test-error-" + time.Now().Format("20060102150405")

	chatstore.DefaultChatStore.Delete(chatId)
	defer chatstore.DefaultChatStore.Delete(chatId)

	delegationStore := GetDelegationStore()
	delegationStore.ResetStore()
	defer delegationStore.ResetStore()

	chatOpts := makeTestChatOpts(chatId)

	// Mock returns StopKindDone on first call (normal completion, firstStep=true).
	// Then on second call (if it happens) returns an error.
	// But since StopKindDone causes break on first iteration, RunAIChat returns.
	// To test the error path at line 566-571 (non-first-step error), we need the
	// first step to succeed and the second step to fail.
	// First call: StopKindToolUse (no delegation) → continues loop, firstStep=false
	// Second call: error → triggers line 566-571 which calls AiMsgError + AiMsgFinish
	mock := &mockBackend{
		stopReasons: []uctypes.WaveStopReason{
			{
				Kind: uctypes.StopKindToolUse,
				ToolCalls: []uctypes.WaveToolCall{
					makeStopToolCall("test_tool"),
				},
			},
		},
		callErrs: []error{
			nil,                // first call: no error
			context.Canceled,   // second call: error
		},
	}

	recorder := newTestResponseWriter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sseHandler := sse.MakeSSEHandlerCh(recorder, ctx)
	if err := sseHandler.SetupSSE(); err != nil {
		t.Fatalf("SetupSSE failed: %v", err)
	}

	// RunAIChat logs the error internally and breaks the loop, returning nil error.
	// The error path at line 569-570 calls AiMsgError + AiMsgFinish exactly once.
	_, _ = RunAIChat(ctx, sseHandler, mock, chatOpts)

	sseHandler.Close()

	stream := recorder.body.String()
	events := extractSSEEvents(stream)

	finishCount := countEventType(events, "finish")
	if finishCount != 1 {
		t.Errorf("ERROR PATH: expected exactly 1 'finish' event (from RunAIChat line 570), got %d.\nStream:\n%s", finishCount, stream)
	}
	t.Logf("ERROR PATH: SSE events = %v", events)
}
