# Session State

- Session: `20260604_161837`
- Repo: `S:\waveterm`
- Branch: `prefer-pwsh7-windows`
- Started: `2026-06-04 16:18:37 America/Toronto`
- Updated: `2026-06-05 07:57:35 America/Toronto`

## Goal
Resume the interrupted local work in this repo and carry the current feature or fix to a verified state without discarding existing uncommitted changes.

## Current Subtask
Recover the resumed session after app restart, verify the running Wave Alt instance is pointed at the intended local config, and identify any mismatches between the saved handoff and current git state.

## Loaded Skills
- `using-superpowers` - required process skill at conversation start; check relevant skills before acting.
- `session-memory` - recover or create durable checkpoint files under `session/`.

## Current Status
No prior `session/` directory existed. Recovered context from local git state instead.

Current uncommitted work appears to include:
- broader command allowlisting for local docker inspection
- remote `ssh "...remote command..."` validation using a separate allowlist
- docker exec flags that open an interactive terminal block for `-i` / `-t`
- frontend fixes for tool approval state reset keyed by tool call signatures

Verification results:
- `go test ./cmd/wave-mcp` passed
- `go test ./pkg/aiusechat` passed
- `go test ./cmd/wsh/cmd` passed
- `npm test -- --run frontend/app/aipanel/aitooluse-utils.test.ts` passed
- `git diff --check` passed
- `npm exec tsc -- --noEmit` failed, but only in pre-existing `frontend/preview/*` mock/preview files unrelated to this patch

Current conclusion:
- the resumed patch still verifies cleanly in the current session
- no additional source edits were needed after recovery
- the only remaining broad-check failure is still the unrelated `frontend/preview/*` TypeScript surface

Wave Alt model diagnosis:
- active Wave Alt config had 19 modes, mostly NVIDIA custom OpenAI-chat entries plus OpenAI/OpenRouter/Ollama
- direct live smoke tests were run against each configured provider for:
  - plain chat completion (`Reply with exactly OK`)
  - tool calling (`adder` function, expecting structured `tool_calls`)
- active config was backed up to:
  - `C:\Users\seand\AppData\Local\waveterm-alt\config\waveai.backup-20260604-171416.json`
- live config was rewritten to keep only the 10 models that returned structured tool calls successfully:
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

Observed failures in removed modes:
- `custom@openai`: OpenAI account quota exceeded
- `custom@openrouter`: configured model ID invalid; valid Claude IDs also fail due insufficient OpenRouter credits
- `custom@nvidia-qwen35-122b`: provider returned HTTP 500 on tool call request
- `custom@nvidia-llama4`: timed out even with 90s request timeout
- `custom@nvidia-kimi`: answered with text/no structured tool call
- `custom@nvidia-jamba`: provider returned 404 not found
- `custom@nvidia-step35flash`: no structured tool call
- `custom@nvidia-seed`: no structured tool call
- `custom@ollama`: local model returned JSON-looking tool call as plain text, not structured `tool_calls`

Wave Alt visibility follow-up:
- active alt `settings.json` originally did not set `waveai:showcloudmodes`
- this meant built-in Wave cloud modes could still appear in the picker even after `waveai.json` was reduced
- active `%LOCALAPPDATA%\\waveterm-alt\\config\\settings.json` now sets:
  - `waveai:showcloudmodes=false`

Resume verification on 2026-06-05:
- running Electron app title is `Wave (Dev: alt)` and launches from `S:\waveterm\...electron.exe .`
- active config files under `%LOCALAPPDATA%\\waveterm-alt\\config\\` still contain the reduced 10-model list and `waveai:showcloudmodes=false`
- `frontend/app/aipanel/ai-utils.ts` confirms cloud modes are hidden when `showCloudModes` is false and custom models exist, unless the current mode is itself a `waveai@...` mode
- `waveapp.log` shows current live chats using model `z-ai/glm-5.1` with tool calls, which is consistent with the reduced custom list being active after restart

Mismatch discovered during recovery:
- repo-local `pkg/wconfig/defaultconfig/waveai.json` is currently modified and now contains custom NVIDIA entries with hardcoded API tokens
- this was not listed in the previous handoff's repo patch summary and conflicts with the "local config only" fix description
- the file was not edited or reverted during this recovery pass; it needs explicit user direction before any cleanup

## Plan
- [x] Re-run targeted Go and frontend tests for the modified repo files.
- [x] Investigate the active Wave Alt model list with live provider calls.
- [x] Back up the live config and rewrite it to a validated tool-capable subset.
- [x] Disable Wave cloud modes in the active alt settings so only custom validated modes remain visible.
- [x] Verify the running dev-alt instance is using the intended `%LOCALAPPDATA%\\waveterm-alt` config and that source logic should hide built-in cloud modes.
- [ ] User confirms the visible picker matches the reduced list in the running UI.
- [ ] User decides whether the repo-local `pkg/wconfig/defaultconfig/waveai.json` change with embedded tokens is intentional or should be cleaned up.

## Assumptions
- The current diff is the work the user wants continued.
- The untracked frontend util/test files belong to the same interrupted change.
- The repo-wide `tsc --noEmit` failure is not introduced by the touched files because every reported error comes from `frontend/preview/*`.

## Blockers
Need explicit user instruction before touching the repo-local `pkg/wconfig/defaultconfig/waveai.json` because it contains embedded API tokens and may be a user-made change.
