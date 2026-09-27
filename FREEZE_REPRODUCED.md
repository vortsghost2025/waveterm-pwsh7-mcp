# FREEZE REPRODUCED — Wave AI Delegation SSE Freeze Boundary

## Result: **REPRODUCED** — The freeze boundary is at `usechat.go:595-597`.

## Exact Blocking Code

```go
// usecat.go:595-597
if GetDelegationStore().HasActiveDelegation(chatOpts.ChatId) {
    break
}
```

When `HasActiveDelegation` returns `true`, the `RunAIChat` loop **breaks** at line 596.
This break exits the `for` loop in `RunAIChat` (line 523-606). After the loop, the function
returns at line 607:

```go
return metrics, nil
```

There is **no** `sseHandler.AiMsgFinish(...)` call on this path. Not in `RunAIChat`,
not in `processAllToolCalls`, not anywhere in the delegation-break path.

## Contrast: Error Path Has AiMsgFinish

Compare with the **error path** at `usechat.go:569-571`:

```go
_ = sseHandler.AiMsgError(err.Error())
_ = sseHandler.AiMsgFinish("", nil)
break
```

The error path explicitly calls `AiMsgFinish`. The delegation-break path does not.

## Contrast: Backend Normal Completion Has AiMsgFinish

All four real backends (openai, openaichat, anthropic, gemini) call `AiMsgFinish` in a
defer when the stop kind is NOT `StopKindToolUse`:

- `openai-backend.go:684-690` — skips `AiMsgFinish` when `rtnStopReason.Kind == StopKindToolUse`
- `openaichat-backend.go:307` — calls `AiMsgFinish(finishReason, nil)`
- `anthropic-backend.go:602-605` — calls `AiMsgFinish` in defer
- `gemini-backend.go:493-496` — calls `AiMsgFinish` in defer

## Test Evidence

Three Go unit tests in `pkg/aiusechat/freeze_boundary_test.go` prove the boundary:

### Test 1: `TestFreezeBoundary_NormalCompletion_EmitsFinish` — PASSED
- Mock backend returns `StopKindDone` and calls `AiMsgFinish("stop", nil)`
- SSE events captured: `[start, text-start, text-delta, text-end, finish]`
- **Result: `finish` event IS present** — frontend receives `finish` and transitions to `idle`

### Test 2: `TestFreezeBoundary_DelegationBreak_NoFinish` — PASSED
- Pre-populated delegation store with `DelegationStateDelegated` for test chat ID
- Mock backend returns `StopKindToolUse` with tool call `test_tool` (does NOT call `AiMsgFinish`)
- `RunAIChat` is called directly (full integration path including `processAllToolCalls`)
- SSE events captured: `[start, text-start, text-delta, text-end, tool-input-start, tool-input-delta, tool-input-available, data-tooluse, data-tooluse]`
- **Result: `finish` event is NOT present** — no `finish` event, only `[DONE]` terminator
- Stream length: 785 bytes

### Test 3: `TestFreezeBoundary_BackendSkipFinishOnToolUse` — PASSED
- Mock backend returns `StopKindToolUse` (no delegation in store)
- SSE events captured: `[start, text-start, text-delta, text-end, tool-input-start, tool-input-delta, tool-input-available]`
- **Result: `finish` event is NOT present** — proves backends skip `AiMsgFinish` for tool use
- This matches the real backend pattern: `AiMsgFinish` is skipped when `StopKindToolUse`

## Root Cause Analysis

The freeze is caused by **two coupled issues**:

1. **Backend defer skips `AiMsgFinish` for `StopKindToolUse`** (`usechat.go:589-603`):
   When the backend returns `StopKindToolUse`, it does NOT call `AiMsgFinish` because
   the loop is expected to continue. This is intentional — the loop should send
   tool results and continue.

2. **Delegation break exits without `AiMsgFinish`** (`usechat.go:595-596`):
   When `HasActiveDelegation` returns true after processing tool calls, the loop
   breaks. But unlike the error path (`usechat.go:569-571`) and the normal
   completion path, the delegation-break path **never calls `AiMsgFinish`**.

The frontend (Vercel AI SDK `DefaultChatTransport` in `aipanel.tsx:16,2025`) expects
a `finish` event to transition from `streaming`/`running` to `idle`. Without it,
the UI stays in a `running` state indefinitely — the user sees a permanent
"thinking..." spinner. The SSE stream ends with `[DONE]` but no `finish` event.

## SSE Event Reference

The `AiMsgFinish` function at `ssehandler.go:448-453`:

```go
func (h *SSEHandlerCh) AiMsgFinish(finishReason string, usage interface{}) error {
    resp := map[string]interface{}{
        "type": AiMsgFinish,
    }
    return h.WriteJsonData(resp)
}
```

This serializes to `data: {"type":"finish"}\n\n` — a JSON SSE event with `type: "finish"`.
The frontend's `DefaultChatTransport` uses this event to set `status: "idle"`.

## Fix

The minimal fix is at `usechat.go:595-597`. After the `break`, the function should
emit `AiMsgFinish` before returning:

```go
if GetDelegationStore().HasActiveDelegation(chatOpts.ChatId) {
    _ = sseHandler.AiMsgFinish("tool_calls", nil)
    break
}
```

Alternatively, the `AiMsgFinish` call could be moved to a `defer` in `RunAIChat`
that checks whether it was already sent, but that would be a larger refactor.

## Files Modified

- `pkg/aiusechat/freeze_boundary_test.go` — new test file (3 tests)

## Files Analyzed (read-only)

- `pkg/aiusechat/usechat.go:463-608` — `RunAIChat` loop, delegation break at 595
- `pkg/web/sse/ssehandler.go:448-453` — `AiMsgFinish` (accepts finishReason + usage, drops them)
- `pkg/web/sse/ssehandler.go:357-371` — `Close()` sends `[DONE]` via `writeDirectly`
- `pkg/aiusechat/delegation.go:150-161` — `HasActiveDelegation`
- `pkg/aiusechat/delegation.go:82-98` — `DelegationStore` singleton + `GetDelegationStore()`
- `pkg/aiusechat/usechat-backend.go:20-56` — `UseChatBackend` interface
- `pkg/aiusechat/openai/openai-backend.go:684-690` — backend defer, skips `AiMsgFinish` on `StopKindToolUse`
- `pkg/aiusechat/delegation_test.go:12-74` — existing delegation store test patterns
- `frontend/app/aipanel/aipanel.tsx:16,2025` — frontend `useChat` with `DefaultChatTransport`
