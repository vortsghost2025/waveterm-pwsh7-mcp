# Agent-Bridge: Enabling Terminal Agents to Communicate with Wave AI

**Status:** DRAFT (updated 2026-06-10)
**Author:** Wave AI + Sean
**Date:** 2026-06-06

---

> **UPDATE 2026-06-10:** The `wsh ai` CLI subcommand already exists (`cmd/wsh/cmd/wshcmd-ai.go`) and covers the "inject prompt into Wave AI" use case. Agents can run `wsh ai -m "message" -s` to send a message and submit it for processing, or `wsh ai -n -m "message" -s` to start a new chat. This covers Phase 2's `send-message` / `send-prompt` CLI requirement. The remaining gap is **receiving responses** — `wsh ai` is fire-and-forget (it injects context + optionally submits, but returns no AI response to the caller).

---

## Problem

Terminal agents (OpenCode, Kilo, etc.) running in Wave terminal widgets cannot **receive responses** from the Wave AI assistant panel. Agents can now **send** messages via `wsh ai`, but they have no way to:

1. Receive structured responses from Wave AI
2. Request scans of other terminals and get results back
3. Ask Wave AI to relay information between agents and get an answer
4. Poll or await a response to a previously sent message

Currently, the human operator must manually relay responses from Wave AI back to agents. This breaks the autonomous agent loop and creates a bottleneck.

## Goal

Enable terminal agents to send structured requests to the Wave AI assistant and receive responses — without human intervention.

## Architecture Overview

```
┌─────────────────┐     ┌──────────────┐     ┌─────────────────┐
│  Terminal Agent  │────▶│  AgentBridge │────▶│    Wave AI      │
│  (OpenCode/Kilo) │◀────│  (wsh RPC)   │◀────│  Assistant      │
└─────────────────┘     └──────────────┘     └─────────────────┘
        │                      │                       │
        │                      │                       │
        ▼                      ▼                       ▼
  ┌──────────┐          ┌──────────┐           ┌──────────────┐
  │ Filesystem│          │  WPS     │           │ All Terminal  │
  │ (S: drive)│          │  Events  │           │  Widgets      │
  └──────────┘          └──────────┘           └──────────────┘
```

## Design Options

### Option A: `wsh` RPC Command (Recommended)

Add a new `wsh` RPC command that terminal agents can invoke from their shell.

**How agents call it:**
```bash
# Simple request
wsh agentbridge --request "Scan terminal f77ffb4f and report status"

# Structured JSON request
wsh agentbridge --request '{"action":"scan_terminals","widget_ids":["f77ffb4f","082ef054"]}'

# Fire-and-forget (no response needed)
wsh agentbridge --notify "P0 fixes deployed to headless, all tests pass"
```

**How it works internally:**

1. Agent runs `wsh agentbridge` command
2. `wsh` sends RPC to Wave backend (`pkg/wshrpc/wshrpctypes.go`)
3. Backend routes request to the active Wave AI chat session
4. Wave AI processes the request (can read terminals, filesystem, etc.)
5. Response written to a response file or returned via RPC

**Key files to modify:**
- `pkg/wshrpc/wshrpctypes.go` — Add `AgentBridgeCommand` to `WshRpcInterface`
- `pkg/wshrpc/wshserver/wshserver.go` — Implement the command handler
- `cmd/wsh/main.go` — Add CLI subcommand `agentbridge`
- `pkg/aiusechat/usechat.go` — Add handler to inject prompt into active chat

**Pros:**
- Agents already have `wsh` available (it's how Wave terminal blocks communicate)
- Works over SSH — headless agents can call it too
- No filesystem polling needed
- Bidirectional — can return responses

**Cons:**
- Requires modifying Wave backend Go code
- Need to define how to "inject" a prompt into an active Wave AI session

---

### Option B: File-Based Bridge (Simplest, No Code Changes)

Agents write request files to a well-known directory. Wave AI (or the human) polls the directory.

**Bridge directory:**
```
S:/.wave-bridge/
├── requests/           # Agents write here
│   ├── req-20260606-001.json
│   └── req-20260606-002.json
├── responses/          # Wave AI writes here
│   ├── res-20260606-001.json
│   └── res-20260606-002.json
└── .lock               # Mutex for concurrent access
```

**Request format:**
```json
{
  "id": "req-20260606-001",
  "source": "kucoin-lane-build",
  "source_widget": "f77ffb4f",
  "timestamp": "2026-06-06T14:30:00Z",
  "action": "scan_terminals",
  "params": {
    "widget_ids": ["082ef054", "a224cd7d"],
    "question": "Is the headless running the broken elif version?"
  },
  "priority": "P0",
  "response_path": "S:/.wave-bridge/responses/res-20260606-001.json"
}
```

**Response format:**
```json
{
  "id": "res-20260606-001",
  "request_id": "req-20260606-001",
  "timestamp": "2026-06-06T14:30:15Z",
  "status": "completed",
  "result": {
    "headless_commit": "9cf98df",
    "elif_status": "FIXED on local main (ba5e88c)",
    "headless_running": "execution_engine (not paper_trade_runner)",
    "all_trades_long": true
  }
}
```

**Pros:**
- Zero code changes to Wave
- Works with any agent that can write files
- Works over SMB/SSH — headless agents can write to the bridge too
- Human can also read/write the bridge manually
- Fits the lane-relay protocol pattern (inbox/outbox)

**Cons:**
- Requires polling (not event-driven)
- No direct injection into Wave AI session
- Human still needs to tell Wave AI "check the bridge"
- File locking on Windows (NFM-014)

---

### Option C: WPS Event Bridge (Event-Driven)

Use Wave's existing WPS (Wave PubSub) system to publish agent bridge events. The Wave AI frontend subscribes to these events and auto-injects them into the chat.

**How it works:**

1. Agent runs `wsh agentbridge` (same CLI as Option A)
2. Backend publishes a `wps.Event_AgentBridgeRequest` event
3. Wave AI frontend subscribes to `agent:bridgerequest` events
4. Frontend auto-injects the request as a user message into the active chat
5. Wave AI processes and responds normally
6. Response is written to a response file or published as a `wps.Event_AgentBridgeResponse`

**Key files:**
- `pkg/wps/wpstypes.go` — Add `Event_AgentBridgeRequest`, `Event_AgentBridgeResponse`
- `pkg/tsgen/tsgenevent.go` — Register event data types
- `frontend/` — Subscribe to bridge events, inject into chat input
- `pkg/wshrpc/wshrpctypes.go` — Add `AgentBridgeCommand`
- `cmd/wsh/main.go` — Add CLI subcommand

**Pros:**
- Truly event-driven (no polling)
- Auto-injects into Wave AI chat (no human relay)
- Uses Wave's existing pubsub infrastructure
- Frontend can show bridge activity in real-time

**Cons:**
- Most complex implementation
- Requires both backend and frontend changes
- Need to handle race conditions (multiple agents sending simultaneously)
- Security: need to scope which agents can send bridge requests

---

## Recommended Implementation Path

**Phase 1 (Ship Today):** Option B — File-Based Bridge
- Zero code changes
- Agents start using it immediately
- Human relays manually at first
- Validates the request/response format

**Phase 2 (Next Sprint):** Option A — `wsh agentbridge` RPC
- Add the RPC command
- Backend can read the request and inject into chat
- Agents can call it from shell
- Works over SSH to headless

**Phase 3 (Full Automation):** Option C — WPS Event Bridge
- Event-driven, auto-inject into chat
- Full autonomous agent↔Wave AI communication
- Frontend shows bridge activity
- Security scoping

---

## Security Considerations

1. **Agent identity:** Requests must include source agent identity. Wave AI should verify the source before acting.
2. **Scope limiting:** An agent should only be able to request information within its lane's observability scope (NFM-020 / NFM-032). Cross-lane reads require explicit permission.
3. **Rate limiting:** Prevent agents from flooding the bridge. Max N requests per minute per agent.
4. **No credential exposure:** Bridge requests must never include API keys, passwords, or secrets.
5. **UBO audit:** All bridge requests should be logged to `ubo-audit.jsonl` per the governance protocol.
6. **Fail-closed:** If the bridge is unavailable, agents must not block. Requests should timeout and the agent continues without the response.

---

## Agent CLI Interface (Phase 2)

```bash
# Scan request — ask Wave AI to read terminal scrollback
wsh agentbridge scan --widget f77ffb4f --question "What's the current status?"

# Filesystem query — ask Wave AI to check a file
wsh agentbridge query --question "Does S:/kucoin-lane/src/data/dex_intelligence/ exist?"

# Cross-agent coordination — relay a message
wsh agentbridge notify --target kucoin-lane-build --message "P0 fixes deployed"

# Structured request with JSON
wsh agentbridge request --data '{
  "action": "verify_claims",
  "claims": [
    {"claim": "headless at 9cf98df", "type": "git_sha"},
    {"claim": "all trades are LONG", "type": "runtime_behavior"},
    {"claim": "SESSION_STATE cycle=14", "type": "file_content"}
  ]
}'

# Check for responses to previous requests
wsh agentbridge responses --since 2026-06-06T14:00:00Z

# Simple ping — is Wave AI available?
wsh agentbridge ping
```

---

## Response Delivery Options

### Option R1: Synchronous RPC Response
Agent calls `wsh agentbridge`, blocks until Wave AI responds. Simple but blocks the agent.

### Option R2: Response File (Async)
Agent calls `wsh agentbridge --async`, gets back a request ID. Later checks `S:/.wave-bridge/responses/res-{id}.json` for the response.

### Option R3: WPS Event Subscription
Agent subscribes to `agent:bridgeresponse:{request_id}` via WPS. Gets notified when response arrives.

**Recommended:** R2 for Phase 1-2, R3 for Phase 3.

---

## Integration with Existing Lane Protocol

The bridge should integrate with the lane-relay protocol defined in `BOOTSTRAP.md`:

- Bridge requests are a new message type: `type: agent-bridge-request`
- They follow the same inbox/outbox pattern
- They're schema-validated and logged
- They respect lane observability boundaries (NFM-020)
- UBO actions are audit-logged

This means the bridge isn't a separate system — it's an extension of the existing governance lattice. The Wave AI assistant becomes a **sixth lane** (or a meta-lane) that can be messaged like any other.

---

## Open Questions

1. **Should Wave AI be able to initiate requests to agents?** (e.g., "Hey kucoin agent, what's your cycle count?") This would require agents to have a listener, not just a CLI.
2. **Should bridge requests be signed?** Per the cryptographic identity layer, all inter-lane messages should be signed. Bridge requests should probably follow the same pattern.
3. **What's the max response size?** Wave AI responses can be long. Need a truncation strategy for RPC responses.
4. **Multi-session handling:** What happens if multiple Wave AI sessions are active? Which one receives the bridge request?
5. **Headless agent access:** Headless agents can SSH to the Windows machine, but can they call `wsh` remotely? Need to test if `wsh agentbridge` works over SSH.
