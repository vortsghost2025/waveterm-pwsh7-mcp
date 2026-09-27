# Delegation Correctness — Implementation Plan

## Step 1: Add ChatId to RPC type
- `pkg/wshrpc/wshrpctypes.go`: Add `ChatId string` to `CommandWaveAIAddContextData`
- Run `task generate`

## Step 2: Frontend chat-Id verification
- `frontend/app/store/tabrpcclient.ts`: In `handle_waveaiaddcontext`, compare `data.chatId` with `model.getChatId()`. On mismatch, reject with typed error.

## Step 3: Delivery with ChatId + durable store
- `pkg/aiusechat/delegation.go`:
  - Add `AttemptCount`, `NextAttemptAt`, `MaxAttempts`, `CompletionStoreBody` fields
  - Add `DeliveryStatePendingChat` = "pending_chat" state
  - Back DelegationStore with MemoryStore: sync Create/Update, restore on Start
  - Widen `IsAgentTerminal` to check cmd args, agent metadata, launcher scripts
- `pkg/aiusechat/delegation_monitor.go`:
  - Pass `ChatId` in delivery RPC
  - On chat mismatch → store completion in MemoryStore, set `pending_chat`
  - On other transient error → backoff with `min(2^attempt, 60)s`, up to 10 attempts
  - On each tick, check RTInfo `waveai:chatid` for `pending_chat` records → retry on match
  - `GetAllPending` includes retryable `delivery_failed`

## Step 4: Tests
10 tests covering detection, persistence, retry, chat routing, deduplication.

## Step 5: Verify
`gofmt -w`, `git diff --check`, `go test ./pkg/aiusechat -count=1`, `go build ./pkg/aiusechat`
