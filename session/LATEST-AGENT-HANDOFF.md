# Agent Handoff

## Read This First

This repo already has an in-progress local change set on branch `prefer-pwsh7-windows`.
Do not clean the worktree or revert unrelated changes.

Recovery update on 2026-06-05:
- the running app is `Wave (Dev: alt)` from the repo checkout and is using the `%LOCALAPPDATA%\\waveterm-alt` profile
- `%LOCALAPPDATA%\\waveterm-alt\\config\\settings.json` still has `waveai:showcloudmodes=false`
- source filtering logic in `frontend/app/aipanel/ai-utils.ts` confirms that this hides built-in Wave cloud modes when custom models exist and the current mode is not itself a `waveai@...` mode
- `waveapp.log` shows live chats using `z-ai/glm-5.1`, consistent with the trimmed custom list being active
- important mismatch: repo-local `pkg/wconfig/defaultconfig/waveai.json` is now modified and contains embedded API tokens; this was not part of the prior handoff summary and should not be changed without explicit user direction

The current work splits into two threads:

1. Repo source changes that were already recovered and re-verified.
2. A local Wave Alt configuration fix outside the repo source tree.

## Repo State

- Repo: `S:\waveterm`
- Branch: `prefer-pwsh7-windows`
- Main saved session: `session/20260604_161837`

## What Was Done

### 1. Recovered and re-verified the interrupted repo patch

The existing uncommitted repo changes appear intentional and were not reverted.

Patch areas:

- `cmd/wave-mcp/allowlist.go`
  - broadened safe read-only Docker allowlisting
  - added separate validation for remote commands wrapped in `ssh "..."`
- `cmd/wave-mcp/allowlist_test.go`
  - added coverage for generic Docker reads and remote SSH validation
- `pkg/aiusechat/tools_command.go`
  - mirrored the MCP-side command allowlist / remote SSH validation
- `pkg/aiusechat/tools_command_test.go`
  - added matching test coverage
- `cmd/wsh/cmd/wshcmd-docker.go`
  - `docker exec -i` / `-t` now opens an interactive terminal block instead of forcing passthrough
- `frontend/app/aipanel/aimessage.tsx`
  - improved keying for tool groups
- `frontend/app/aipanel/aitooluse.tsx`
  - resets stale approval state when tool-call identity changes
- `frontend/app/aipanel/aitooluse-utils.ts`
  - helper extraction for tool-use signatures / approval handling
- `frontend/app/aipanel/aitooluse-utils.test.ts`
  - focused frontend unit coverage

### 2. Fixed the active Wave Alt model list locally

The user reported that Wave Alt still showed many non-working models for `codex continue`.
This turned out to be a local config issue, not a new repo source bug.

Changes made outside the repo:

- Backed up full active AI mode config:
  - `C:\Users\seand\AppData\Local\waveterm-alt\config\waveai.backup-20260604-171416.json`
- Rewrote active AI mode config to only keep models that successfully returned structured tool calls:
  - `C:\Users\seand\AppData\Local\waveterm-alt\config\waveai.json`
- Disabled built-in Wave cloud modes so only the reduced custom list should remain visible:
  - `C:\Users\seand\AppData\Local\waveterm-alt\config\settings.json`
  - added `"waveai:showcloudmodes": false`

Important: `waveai.json` contains live API tokens. Do not paste or diff token values into chat.

## Validated Wave Alt Models

The active Wave Alt config was reduced to these 10 tool-capable custom modes:

- `custom@nvidia-glm51`
- `custom@nvidia-qwen-coder`
- `custom@nvidia-step37flash`
- `custom@nvidia-nemotron-super`
- `custom@nvidia-nemotron-120b`
- `custom@nvidia-deepseek-v4`
- `custom@nvidia-mistral-large`
- `custom@nvidia-minimax`
- `custom@nvidia-mistral-small`
- `custom@nvidia-gemma4`

Observed failures from removed modes:

- `custom@openai`: account quota exceeded
- `custom@openrouter`: invalid model ID and insufficient credits on tested alternatives
- `custom@nvidia-qwen35-122b`: HTTP 500 during tool-call request
- `custom@nvidia-llama4`: timed out
- `custom@nvidia-kimi`: text response instead of structured tool call
- `custom@nvidia-jamba`: HTTP 404
- `custom@nvidia-step35flash`: no structured tool call
- `custom@nvidia-seed`: no structured tool call
- `custom@ollama`: tool call emitted as plain text, not structured `tool_calls`

## Verification Already Run

Repo patch verification completed successfully:

- `go test ./cmd/wave-mcp`
- `go test ./pkg/aiusechat`
- `go test ./cmd/wsh/cmd`
- `npm test -- --run frontend/app/aipanel/aitooluse-utils.test.ts`
- `git diff --check`

Known non-blocking failure:

- `npm exec tsc -- --noEmit`
  - fails only in pre-existing `frontend/preview/*` files unrelated to this patch

## What The Next Agent Needs To Do

Primary next step:

- Confirm with the user that Wave Alt now shows only the reduced validated model list in the visible picker and that `codex continue` works from one of those modes.
- Ask whether the repo-local `pkg/wconfig/defaultconfig/waveai.json` tokenized change is intentional before touching it.

If the user says the model picker is still wrong:

- Ask for the exact visible mode names that should not be there.
- Check whether they are:
  - built-in cloud modes still cached in the UI
  - modes from another config home / instance
  - stale UI state that did not refresh after the `settings.json` change

If more repo work is requested:

- Start from the saved session notes in `session/20260604_161837`
- Preserve the existing dirty worktree
- Treat `frontend/preview/*` TypeScript errors as pre-existing unless new evidence shows otherwise

## Useful Files

- `session/20260604_161837/session_state.md`
- `session/20260604_161837/handoff.md`
- `session/20260604_161837/timeline.md`
- `session/20260604_161837/files.md`
- `C:\Users\seand\AppData\Local\waveterm-alt\config\waveai.json`
- `C:\Users\seand\AppData\Local\waveterm-alt\config\settings.json`

## Constraints / Warnings

- Do not expose API tokens from the local Wave Alt config.
- Do not expose or commit API tokens currently present in repo-local `pkg/wconfig/defaultconfig/waveai.json`.
- Do not revert unrelated changes in the repo.
- Do not assume the branch name perfectly describes the patch; trust the actual diffs and tests.
- The current source patch was already verified; avoid gratuitous rework unless the user asks for it.
