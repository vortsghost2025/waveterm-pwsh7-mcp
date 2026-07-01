# Agent Heartbeat Coordinator — Design Spec

**Date:** 2026-07-01  
**Status:** Approved  
**Author:** OpenCode + Wave AI collaboration  
**Repo:** `S:\waveterm`  

---

## Problem

OpenCode and Wave AI exchange work through bridge JSONL files at `S:\sean-machine-janitor\bridge\`. After one exchange, both agents go idle. There is no mechanism to:

- Detect that a reply is needed and wake the idle agent
- Distinguish "Wave AI replied in chat only" from "Wave AI replied via bridge"
- Serialize turns so both agents don't blast messages simultaneously
- Stop the loop and surface to the user when a decision, block, or shipment is detected

The current workaround is a manual `wsh ai -s -m` poke after every handoff. This is unreliable and doesn't scale to continuous collaboration.

---

## Solution

A **Heartbeat Coordinator** — a persistent background process that watches both bridge files, manages turns, detects escalation triggers, and dispatches wakeups automatically.

### Architecture

```
OpenCode ──write──▶ wave-outbox.jsonl ──read──▶ Coordinator ──wakeup──▶ Wave AI
Wave AI  ──write──▶ wave-inbox.jsonl  ──read──▶ Coordinator ──wakeup──▶ OpenCode
```

The coordinator is **additive**. It does not modify existing agents, tools, or protocols. It watches the bridge files and uses `wsh ai -s -m` to wake agents when their turn arrives.

---

## Components

### 1. Bridge Watcher

Watches `wave-inbox.jsonl` and `wave-outbox.jsonl` for new content using byte-offset tracking (not mtime or file existence).

**Behavior:**
- On startup, reads both files to establish baseline byte offsets
- Polls every 2 seconds for byte-offset changes
- When a write is detected, records the new message and triggers the turn manager
- If a write is detected but no follow-up appears within the reply timeout, flags "replied in chat only"

**Why byte-offset:** mtime can be unreliable on Windows under heavy I/O. File existence doesn't distinguish "new content" from "same file." Byte-offset is unambiguous — if the file grew, something was appended.

### 2. Turn Manager

Serializes agent turns so only one agent is active at a time.

**State tracked in `coordinator-state.json`:**
```json
{
  "active_turn": "opencode | wave-ai | idle",
  "last_wakeup": "ISO timestamp",
  "last_reply_offset_inbox": 0,
  "last_reply_offset_outbox": 0,
  "escalation_pending": false,
  "started_at": "ISO timestamp"
}
```

**Turn rules:**
- Coordinator assigns `active_turn` based on which agent wrote last
- Only the active turn owner receives wakeups
- When an agent writes a reply, it calls `turn_complete` (implicit via file write) → coordinator switches turn
- If both agents try to go active simultaneously, the state file acts as the serialization point
- Turn ownership is determined by which file had the most recent write

**Serialization mechanism:** Directory-creation mutex (`New-Item -ItemType Directory -Name "coordinator-lock" -ErrorAction SilentlyContinue`). If the directory already exists, another instance holds the lock. This is more reliable than PowerShell's `Lock-File` cmdlet, which fails silently across processes on Windows.

### 3. Wakeup Dispatcher

Sends wakeup messages to agents via `wsh ai -s -m`.

**Wakeup message format:**
```
TURN_POKE v1\nYour turn. Read bridge inbox/outbox and reply via bridge_write_reply.\n
```

**Behavior:**
- When turn switches to OpenCode: no wakeup needed (OpenCode is in a continuous turn)
- When turn switches to Wave AI: runs `wsh ai -s -m "TURN_POKE v1\nYour turn. Read bridge outbox and reply via bridge_write_reply.\n"`
- If Wave AI doesn't write to `wave-inbox.jsonl` within 90s: sends escalation nudge with explicit "use bridge_write_reply" reminder
- If escalation nudge also fails within 90s: flags escalation to user

### 4. Escalation Gate

Detects patterns that require human oversight and pauses the loop.

**Escalation triggers (regex patterns scanned in every bridge message):**
- `DECISION:` — agent needs a decision from the user
- `BLOCKED:` — agent is blocked and cannot proceed
- `SHIPMENT:` — agent has something ready for the user (PR, binary, etc.)
- `ASK:` or `\?` at end of message — soft trigger: "ask user when next turn opens" (not a full stop, but flagged)

**Escalation behavior:**
1. Coordinator writes an `escalation` record to `wave-outbox.jsonl`:
   ```json
   {
     "timestamp": "ISO",
     "type": "escalation",
     "trigger": "DECISION:",
     "message": "Agent needs decision: ...",
     "source": "coordinator"
   }
   ```
2. Sets `active_turn: "idle"` and `escalation_pending: true`
3. Stops dispatching wakeups
4. User sees escalation in bridge outbox → resolves → user manually restarts coordinator or runs `wsh ai -s -m` to resume

**Ping-pong limit:** If the same task ping-pongs more than 4 rounds without converging, coordinator auto-escalates with `PING_PONG_LIMIT` trigger.

**API error threshold:** If API error rate exceeds 50% in any 5-minute window (tracked via coordinator heartbeat), coordinator escalates with `ERROR_RATE` trigger.

### 5. Heartbeat

Writes a heartbeat line to `coordinator-heartbeat.jsonl` every 30 seconds.

**Heartbeat record:**
```json
{
  "timestamp": "ISO",
  "active_turn": "opencode | wave-ai | idle",
  "last_inbox_offset": 0,
  "last_outbox_offset": 0,
  "escalation_pending": false
}
```

**Stale detection:** If no heartbeat appears for 90 seconds, the coordinator is considered dead. A watchdog (external to this spec) can restart it. The 90s threshold prevents false positives under load (heartbeat is 30s, so 3 consecutive misses = 90s).

**Restart recovery:** On startup, coordinator reads the last heartbeat to determine `active_turn` and last-seen offsets, then resumes watching from those points. No messages are lost.

---

## Coordinator Process

**Language:** PowerShell 7+  
**Location:** `S:\waveterm\scripts\wave-coordinator.ps1`  
**Launcher:** `S:\waveterm\scripts\Start-Coordinator.ps1`  
**Stopper:** `S:\waveterm\scripts\Stop-Coordinator.ps1`  
**State:** `S:\waveterm\agent-coordination\coordinator-state.json`  
**Heartbeat:** `S:\waveterm\agent-coordination\coordinator-heartbeat.jsonl`  
**Log:** `S:\waveterm\agent-coordination\coordinator-log.txt`  

**Process model:**
- Runs as detached background process (`Start-Process -WindowStyle Hidden`)
- No TUI, no dashboard, no interactive input
- All state persisted to disk
- Stops cleanly when sent a stop signal (file-based: creates `coordinator-stop` sentinel file)

**Startup sequence:**
1. Acquire directory-creation mutex (`coordinator-lock`)
2. If mutex already held, exit (another instance running)
3. Read `coordinator-state.json` (or create defaults)
4. Read last heartbeat from `coordinator-heartbeat.jsonl`
5. Establish baseline byte offsets for both bridge files
6. Start watcher loop (2s poll interval)
7. Write first heartbeat

**Shutdown sequence:**
1. Check for `coordinator-stop` sentinel file every 5s
2. On detection: release mutex, write final heartbeat, exit
3. `Stop-Coordinator.ps1` creates sentinel, waits for process to exit, cleans up

---

## Integration with Existing Systems

### No changes to:
- `wsh` CLI (uses existing `wsh ai -s -m`)
- `wavehydrate` (uses existing HYDRATE_WAVE_ASSISTANT v1 protocol)
- `Send-WaveNudge.ps1` (existing nudge helper, coordinator uses same mechanism)
- Bridge file format (existing JSONL format unchanged)
- AGENTS_PROTOCOL.md (coordinator replaces the manual wakeup crutch in the Wakeup section)

### AGENTS_PROTOCOL.md update:
Remove the manual wakeup step (section "Wakeup"). Replace with:

> "The coordinator watches both bridge files and dispatches wakeups automatically. Agents no longer need to manually call `wsh ai -s -m` after writing to the bridge. The coordinator handles turn transitions."

---

## Error Handling

| Failure Mode | Detection | Recovery |
|---|---|---|
| Coordinator crashes | Heartbeat stale > 90s | External watchdog restarts coordinator (defer to v2) |
| `wsh ai` unavailable | Wakeup returns non-zero exit | Log error, escalate to user after 3 consecutive failures |
| Bridge file locked by another process | Read returns error | Retry once after 5s, escalate if still locked |
| State file corrupted | JSON parse fails | Reset to defaults (lose turn state, not messages) |
| Both agents silent > 30min | No bridge writes | Coordinator goes idle, user notified via log |
| Wave AI rate limited (429) | API error in log | Coordinator backs off 30s before next wakeup |

---

## Out of Scope (v2)

- Auto-restart watchdog (user restarts coordinator manually if it dies)
- Metrics collection or observability dashboard
- Lane state machine (from approach C — defer until we have 3+ agents)
- Graceful shutdown signal (sentinel file is v1's stop mechanism)
- Cross-repo coordination (stays local to `S:\waveterm\` + `S:\sean-machine-janitor\bridge\`)

---

## Success Criteria

1. Multi-round agent loop runs for 10+ exchanges without manual intervention
2. "Replied in chat only" is detected and corrected within 90s
3. Escalation triggers fire within 5 seconds of detection
4. Coordinator survives a 30-minute idle period and resumes correctly
5. User can start, stop, and inspect coordinator state without reading source code
