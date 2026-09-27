# Design: Delegation Correctness Gaps

## Gap 1 — Exact Chat Routing

### Problem
`CommandWaveAIAddContextData` has no `ChatId` field. The frontend handler (`handle_waveaiaddcontext`) submits into whatever chat is currently active in the WaveAI panel. The delegation monitor routes by `OriginatingTabId` only — delivery goes to the tab, not to the specific chat.

### Solution

**Backend (RPC)**:
- Add `ChatId string` field to `CommandWaveAIAddContextData` in `wshrpctypes.go`.
- Populate `ChatId` from `DelegationRecord.OriginatingChatId` when the delegation monitor calls `WaveAIAddContextCommand`.
- Run `task generate` to update `wshclientapi.ts` and `gotypes.d.ts`.

**Backend (monitor)**: When delivery is attempted and the active chatId (from wstore RTInfo `waveai:chatid` key for the tab) does not match `OriginatingChatId`:
- Store the completion in `aistore.MemoryStore` under scope `delegation-completion`, keyed by `originatingChatId:deliveryId`.
- The completion record stores: `deliveryId`, `originatingChatId`, `originatingTabId`, `targetBlockId`, `completedAt`, `deliveryState`, `attemptCount`, `nextAttemptAt`, `lastError`.
- On each monitor tick, check pending completions against the tab's current `waveai:chatid` RTInfo value. On match, retry delivery.
- Mark delivered only after the exact originating chat accepts the queued completion.
- Deduplicate by `deliveryId`.
- **Do not** switch the user's active chat or inject into a different chat.

**Frontend**: In `handle_waveaiaddcontext`:
- Compare `data.chatId` against `WaveAIModel.getChatId()`.
- On match → submit directly via `model.appendText` + `model.handleSubmit`.
- On mismatch → return a typed mismatch/not-active error to the backend (via RPC error). The backend retains the durable completion and retries when the chat becomes active.
- The backend is authoritative for durable storage. Frontend never writes directly to `aistore.MemoryStore`.

**Activation signal**: No new RPC. The delegation monitor uses the existing `waveai:chatid` RTInfo value (set by `SetRTInfoCommand`, stored in wstore) to detect when the correct chat becomes active. On each monitor tick, for each pending completion, query `GetRTInfoCommand` (or directly read RTInfo from wstore) and compare the stored `waveai:chatid` with the completion's `OriginatingChatId`. On match, deliver.

## Gap 2 — Durable Delegation Storage

### Problem
`DelegationStore` is a plain `map[string]*DelegationRecord` — no persistence. Delegations are lost on process restart.

### Solution
Back the `DelegationStore` with `aistore.MemoryStore` using scope `delegation`, keyed by `deliveryId`. On monitor `Start()`, restore unfinished delegations (`pending`, `delivering`, `delivery_failed` with pending retry). Keep the in-memory map for fast O(1) access, but sync every mutation (insert, update, delete) to MemoryStore. Use `Put` with the `Key` field set to `deliveryId` for upsert semantics, and `Delete` on terminal completion.

## Gap 3 — Kilo Terminal Detection

### Problem
`IsAgentTerminal` checks only `block.Meta["cmd"]` for `"kilo"`, `"agent"`, or `"aienv"`. Misses launcher scripts and spawned agents.

### Solution
Widen detection using `waveobj` metadata constants. Return `true` if ANY of these match:

1. `MetaKey_Cmd` in `{"kilo", "opencode", "agent", "aienv", "claude"}` — direct agent binary.
2. `MetaKey_CmdArgs` contains any token matching a known agent binary (e.g., `kilo`, `opencode`, `claude`), or contains a reference to a `START-KILO-*.cmd` / `START-OPENCODE-*.cmd` launcher script.
3. `MetaKey_CmdRunOnStart` is set with a value containing a known agent binary.
4. Agent metadata keys (`"agent:model"`, `"agent:mode"`) are present — terminal was spawned via `term_spawn_agent`.

## Gap 4 — Retryable Delivery Failure

### Problem
`GetAllPending` explicitly excludes `delivery_failed`. `processDelegation` also skips it. No attempt tracking, no backoff.

### Solution
Add fields to `DelegationRecord`:
- `AttemptCount int` — number of delivery attempts.
- `NextAttemptAt int64` — unix millis earliest time to retry (0 = immediate).
- `MaxAttempts int` — cap (10).
- `LastError string` — last delivery error for diagnostics.

**Retry logic**:
- `GetAllPending` includes `delivery_failed` when `NextAttemptAt == 0 || NextAttemptAt <= now.UnixMilli()`.
- On delivery failure: increment `AttemptCount`, if `AttemptCount >= MaxAttempts`, mark terminal `delivery_failed` with no retry. Otherwise, compute `delay = min(2^(AttemptCount-1), 60) seconds`, set `NextAttemptAt = now + delay`.
- After `MaxAttempts` failed attempts, stay in `delivery_failed` and stop retrying.
