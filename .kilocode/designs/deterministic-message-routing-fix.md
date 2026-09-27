# Deterministic Message Routing - Fix Documentation

## Problem

Wave AI was dual-outputting messages - sending the same content to BOTH:
1. The chat panel (for Sean to see)
2. The bridge outbox (for terminal agents to read)

This caused confusion because:
- Terminal agents would receive messages meant for Sean
- Sean would see replies meant for terminal agents
- Both Wave AI and CLI agents would go idle waiting for each other
- Message routing was non-deterministic and ambiguous

## Root Cause

The system prompt in `pkg/aiusechat/usechat-prompts.go` explicitly instructed Wave AI to:
```
When you use bridge_write_reply for a terminal-agent response, 
also send the same concise reply as a normal Wave AI assistant 
message after the tool result so Sean can see the response in the panel in real time.
```

This made dual-output MANDATORY instead of preventing it.

## Solution

### 1. System Prompt Changes (pkg/aiusechat/usechat-prompts.go)

**BEFORE:**
```
When replying to a terminal agent or Kilo CLI session, prefer term_send_input 
to type the reply into the target terminal; if term_send_input is unavailable, 
use bridge_write_reply to append the reply to S:\sean-machine-janitor\bridge\wave-outbox.jsonl. 
When you use bridge_write_reply for a terminal-agent response, also send the same 
concise reply as a normal Wave AI assistant message after the tool result so Sean 
can see the response in the panel in real time.
```

**AFTER:**
```
### MESSAGE ROUTING RULES (MANDATORY - NOT OPTIONAL):
1. IF you are responding to a USER (Sean) in the Wave AI chat panel → Use ONLY the normal chat response. DO NOT write to bridge files.
2. IF you are responding to a TERMINAL AGENT (Kilo CLI, OpenCode CLI, or any spawned agent) → Use ONLY bridge_write_reply to S:\sean-machine-janitor\bridge\wave-outbox.jsonl. DO NOT output to the chat panel.
3. IF you are delegating work to a terminal agent → Use term_spawn_agent or bridge_write_inbox. DO NOT output to the chat panel.
4. IF a terminal agent completed work and you are routing the result back → Use WaveAIAddContext to submit to the originating chat. DO NOT output to the chat panel yourself.
NEVER dual-output. NEVER reply to both the bridge AND the chat panel. Choose exactly one routing based on who the recipient is.
```

### 2. New Tools Added (pkg/aiusechat/tools_messagerouter.go)

**message_router** - Enforces deterministic routing:
- Input: `recipient_type` ("user" or "terminal_agent"), `message`, `target_block_id`
- Output: Routing decision with channel used
- Prevents dual-output by design

**validate_message_routing** - Pre-flight validation:
- Input: `intent`, `planned_channel`
- Validates routing BEFORE sending
- Returns error if routing is ambiguous

### 3. Backend Code (Already Correct)

The delegation monitor in `pkg/aiusechat/delegation_monitor.go` was already correctly routing completions:
- Uses ONLY `WaveAIAddContextCommand` to route agent completions back to originating chat
- No dual-output in backend code
- Properly stores completions when chat is not active
- Retries delivery when chat becomes active

## Routing Matrix

| Intent | Recipient | Channel | Tool | NEVER Use |
|--------|-----------|---------|------|-----------|
| Reply to Sean | User | Chat panel | Normal response | bridge_write_reply |
| Reply to agent | Terminal agent | Bridge outbox | bridge_write_reply | Chat panel output |
| Delegate work | Terminal agent | Bridge inbox | bridge_write_inbox | Chat panel output |
| Route completion | Originating chat | WaveAIAddContext | (backend auto) | Direct chat output |
| Wake up agent | Terminal agent | Bridge inbox + wsh poke | bridge_write_inbox + wsh ai -s -m | Chat panel output |

## Files Changed

1. `pkg/aiusechat/usechat-prompts.go` - System prompt routing rules
2. `pkg/aiusechat/tools_messagerouter.go` - NEW: Message routing tools
3. `pkg/aiusechat/tools.go` - Wire up new routing tools

## Testing

### Test Case 1: User Reply
```
User: "What's the status of the build?"
Wave AI: [Normal chat response - NO bridge output]
✓ Pass: Only Sean sees the response
```

### Test Case 2: Agent Reply
```
Kilo CLI: [Writes to bridge inbox]
Wave AI: [Reads inbox, uses bridge_write_reply]
✓ Pass: Only bridge outbox receives response, Sean does NOT see duplicate
```

### Test Case 3: Delegation
```
Sean: "Have Kilo refactor the auth module"
Wave AI: [Uses term_spawn_agent - NO chat panel output about the delegation]
Kilo CLI: [Works, completes]
Delegation Monitor: [Routes completion via WaveAIAddContext]
✓ Pass: Sean sees ONLY the completion in his chat, not the delegation command
```

### Test Case 4: Agent-to-Agent Message
```
Kilo CLI: [Needs to message OpenCode CLI]
Wave AI: [Reads Kilo's message from inbox, uses bridge_write_inbox to send to OpenCode]
✓ Pass: Message routed via bridge only, no chat panel output
```

## Expected Behavior After Fix

1. **Sean asks Wave AI a question** → Response appears ONLY in chat panel
2. **Kilo CLI sends a message** → Written to bridge outbox, Wave AI reads and routes appropriately
3. **Wave AI delegates to Kilo** → Uses term_spawn_agent, NO chat panel output about delegation
4. **Kilo completes work** → Delegation monitor routes completion via WaveAIAddContext to Sean's chat
5. **Wave AI needs to wake up Kilo** → Uses bridge_write_inbox + wsh poke, NO chat panel output

## Enforcement

The system is now **deterministic by design**:
- System prompt explicitly forbids dual-output
- Message router tool enforces single-channel routing
- Validation tool catches routing errors before sending
- Backend delegation monitor already uses correct single-channel routing

## Migration

No migration needed. The fix is forward-looking:
- Existing bridge files continue to work
- Existing delegations continue to work
- New messages follow deterministic routing immediately

## Future Enhancements

1. Add telemetry to detect dual-output attempts
2. Add bridge message validation (schema checking)
3. Add message priority levels (urgent vs normal)
4. Add message threading/conversation tracking
5. Add delivery confirmation receipts