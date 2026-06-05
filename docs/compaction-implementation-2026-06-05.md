# Context Window Compaction — Implementation Notes

**Date:** 2026-06-05 08:10 UTC  
**Author:** Wave AI (assistant)  
**Issue:** `Unterminated string` / 400 error when chat grows too large  

---

## Problem

The `ChatStore` in `pkg/aiusechat/chatstore/chatstore.go` grows unbounded — every user message, assistant response, tool call, and tool result is appended to `NativeMessages` and sent in **full** to the LLM API on every step. There was zero compaction, summarization, or context window management.

In `RunAIChat` (`usechat.go`), each step:
1. Calls `chatstore.DefaultChatStore.Get()` which copied **all** `NativeMessages`
2. Sent them all to the API
3. Appended more messages back (tool results, assistant responses)

As the conversation grew (especially with tool use — each `run_command` adds ~3 native messages), the JSON body swelled until it exceeded the model's context window, producing the `Unterminated string` JSON parse error (the request body was so large it got truncated at the transport layer, cutting a JSON string mid-way).

---

## What Was Changed

### 1. `pkg/aiusechat/uctypes/uctypes.go`
- **Added `GetContentSummary() string`** to the `GenAIMessage` interface — returns a short human-readable summary of a message (e.g. `"user: list files in ~/src"`, `"assistant: tool_calls[read_dir,run_command]"`).
- **Added `CompactionSummaryMessage`** type — a synthetic `GenAIMessage` (role: `"user"`) that carries the text summary of compacted messages. This is what gets sent to the API in place of the dropped messages.
- **Added `GetContentSummary()` implementation on `AIMessage`** — extracts the first text part.
- Added `crypto/rand` and `fmt` imports (needed for `CompactionSummaryMessage.GetMessageId()`).

### 2. `pkg/aiusechat/chatstore/chatstore.go`
- **Added `MaxNativeMessages = 50`** constant — when a conversation exceeds 50 native messages, compaction triggers. 50 ≈ 16 conversation turns (each turn produces ~3 native messages with tool use).
- **Added `compactionSummaryLen = 150`** — max chars per message in the summary.
- **Added `compactNativeMessages()` function** — the core compaction logic:
  - If `len(msgs) <= MaxNativeMessages`, returns as-is
  - Keeps `msgs[0]` (first message, often system-ish context)
  - Drops messages from index 1 to `len(msgs)-keepTail`
  - Builds a text summary of the dropped messages using `GetContentSummary()`
  - Inserts a `CompactionSummaryMessage` with that summary
  - Keeps the most recent `MaxNativeMessages-2` messages intact
  - Logs the compaction event
- **Modified `ChatStore.Get()`** — now calls `compactNativeMessages()` on the copy instead of a raw `copy()`. The **original** `chat.NativeMessages` in the store is **never modified** — compaction only affects the API-bound copy.

### 3. Backend implementations of `GetContentSummary()`
Replaced stub implementations with real ones in all 4 backends:

- **`pkg/aiusechat/openaichat/openaichat-types.go`** — `StoredChatMessage.GetContentSummary()`:
  - Handles tool_calls, content_parts, tool results, and plain text
  - Added `fmt` and `strings` imports

- **`pkg/aiusechat/openai/openai-backend.go`** — `OpenAIChatMessage.GetContentSummary()`:
  - Handles function_call, function_call_output, and message content blocks

- **`pkg/aiusechat/anthropic/anthropic-backend.go`** — `anthropicChatMessage.GetContentSummary()`:
  - Handles text, tool_use, and tool_result content blocks

- **`pkg/aiusechat/gemini/gemini-types.go`** — `GeminiChatMessage.GetContentSummary()`:
  - Handles text, function_call, and function_response parts
  - Added `fmt` import

### 4. Backend message conversion — handle `CompactionSummaryMessage`
All 4 backends now handle `*uctypes.CompactionSummaryMessage` in their message conversion loops, inserting it as a `"user"` role message with the summary text:

- **`pkg/aiusechat/openaichat/openaichat-backend.go`** — `RunChatStep()`
- **`pkg/aiusechat/openai/openai-backend.go`** — `RunOpenAIChatStep()`
- **`pkg/aiusechat/anthropic/anthropic-backend.go`** — `RunAnthropicChatStep()`
- **`pkg/aiusechat/gemini/gemini-backend.go`** — `RunGeminiChatStep()`

---

## Design Decisions

1. **Compaction only affects the API-bound copy, not the stored chat.** The full conversation remains in `ChatStore.chats[chatId].NativeMessages` for UI rendering and future summarization. Only the copy returned by `Get()` is compacted.

2. **MaxNativeMessages = 50** was chosen because:
   - A typical tool-using conversation produces ~3 native messages per turn
   - 50 messages ≈ 16 conversation turns, which is well within most models' context windows
   - This can be tuned later or made model-dependent

3. **The compaction summary is a simple text digest**, not an LLM-generated summary. This avoids an extra API call and latency. The summary format is:
   ```
   [Earlier conversation compacted — 42 messages summarized:]
   user: What files are in ~/src?
   assistant: tool_calls[read_dir]
   tool[read_dir]: {"path": "/home/user/src"...
   user: Show me the Go code
   assistant: Here are the Go files...
   ```

4. **`CompactionSummaryMessage` uses role `"user"`** so the model treats it as conversational context. This is a common pattern (Claude's own API uses `"user"` for inserted context).

---

## Future Improvements (Phase 2+)

- **Token-based compaction** instead of message-count-based (estimate tokens using tiktoken or similar)
- **LLM-powered summarization** of the dropped messages (call a cheaper model to generate a concise summary)
- **Model-aware limits** (different models have different context windows)
- **Compaction metrics** in `AIMetrics` (count of compaction events, messages compacted)
- **Preserve important tool results** (e.g., file writes, edits) even when they're old
- **Configurable MaxNativeMessages** via `waveai:maxnativemessages` RTInfo setting

---

## Files Modified

| File | Change |
|------|--------|
| `pkg/aiusechat/uctypes/uctypes.go` | Added `GetContentSummary()` to interface, `CompactionSummaryMessage` type, `AIMessage.GetContentSummary()` |
| `pkg/aiusechat/chatstore/chatstore.go` | Added `MaxNativeMessages`, `compactNativeMessages()`, modified `Get()` |
| `pkg/aiusechat/openaichat/openaichat-types.go` | Real `GetContentSummary()` + imports |
| `pkg/aiusechat/openaichat/openaichat-backend.go` | Handle `CompactionSummaryMessage` in conversion loop |
| `pkg/aiusechat/openai/openai-backend.go` | Real `GetContentSummary()` + handle `CompactionSummaryMessage` |
| `pkg/aiusechat/anthropic/anthropic-backend.go` | Real `GetContentSummary()` + handle `CompactionSummaryMessage` |
| `pkg/aiusechat/gemini/gemini-types.go` | Real `GetContentSummary()` + import |
| `pkg/aiusechat/gemini/gemini-backend.go` | Handle `CompactionSummaryMessage` in conversion loop |

---

## Build & Test

Run from the waveterm root:

```bash
go build ./pkg/aiusechat/...
go test ./pkg/aiusechat/...
```

The `cmd/wave-mcp/` binary is separate and unaffected (it has its own tool system).

---

## Also in this batch: Discovery Tools (committed by other agent)

Commit `48ad775c` added 3 new discovery tools to `pkg/aiusechat/tools_discovery.go`,
registered in `tools.go`:

| Tool | Description |
|------|-------------|
| `list_workspaces` | Enumerate all open workspaces (ID, name, icon, color, active tab, tab count) |
| `list_tabs` | Enumerate all tabs across workspaces (ID, name, workspace, block count, is_active). Optional `workspace_id` filter. |
| `get_widget` | Get full metadata for a single widget by 8-char ID or full block OID (view type, name, meta, tab ID, shell runtime info) |

All three are gated behind `widgetAccess=true` and use `wstore` in-process (no shelling out).
Test file: `pkg/aiusechat/tools_discovery_test.go` (input-parse + tool-def tests).
