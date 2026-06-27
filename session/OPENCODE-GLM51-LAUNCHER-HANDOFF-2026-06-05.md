# OpenCode GLM-5.1 Launcher Handoff

Date: 2026-06-05

## Goal

Make `C:\Users\seand\Desktop\OpenCode.lnk` default to a long-context `GLM-5.1` route without changing `Wave Alt`.

## Final State

The final working target is not the direct Z.AI billing path.

It is now:

- launcher: `C:\Users\seand\Desktop\OpenCode.lnk`
- provider ID: `zai-200k`
- model ID: `z-ai/glm-5.1`
- display name: `GLM-5.1 203K (OpenRouter)`
- backend base URL: `https://openrouter.ai/api/v1`
- auth env var: `OPENROUTER_API_KEY`
- configured context limit: `203000`
- configured output limit: `131072`

## Why It Changed

An initial direct Z.AI `200K` profile was added first, but the launcher reported insufficient funds because the funded account was OpenRouter, not Z.AI.

The long-context profile was then switched from direct Z.AI billing to OpenRouter so the launcher can use the credits that were added there.

## Files Changed

Primary config:

- `C:\Users\seand\.config\kilo\opencode.json`
- `C:\Users\seand\.config\kilo\kilo.jsonc`
- `C:\Users\seand\AppData\Local\ai.opencode.desktop\kilo\model.json`

Environment:

- Windows user env var `OPENROUTER_API_KEY` was set
- Windows user env var `ZAI_API_KEY` was also set earlier, but the final launcher route uses `OPENROUTER_API_KEY`

## Current Model Defaults

OpenCode desktop state file:

- `code` role defaults to `providerID: zai-200k`
- `code` role defaults to `modelID: z-ai/glm-5.1`

Other roles were intentionally left alone:

- `code-reviewer` stays on NVIDIA `openai/gpt-oss-120b`
- `plan` stays on NVIDIA `openai/gpt-oss-120b`
- `debug` stays on NVIDIA `openai/gpt-oss-120b`

## Not Changed

- `C:\Users\seand\Desktop\Wave Alt.lnk`
- `%LOCALAPPDATA%\waveterm-alt\config\waveai.json`

Wave Alt was explicitly left out of this launcher test.

## Important Restart Note

If OpenCode was already running, it can keep using the old in-memory model route even after config changes.

To test the new route:

1. Fully quit OpenCode, including any tray or background process.
2. Reopen from `C:\Users\seand\Desktop\OpenCode.lnk`.
3. Prefer a fresh session instead of resuming an old one.
4. Verify the active `code` model is `zai-200k / z-ai/glm-5.1`.

## Expected Result

After a clean restart, the launcher should default to the OpenRouter-backed long-context `GLM-5.1` route instead of the NVIDIA-hosted `128K` route.

## If It Still Fails

If OpenCode still reports insufficient funds after a full restart:

- the issue is no longer the unfunded direct Z.AI route
- the likely issue is OpenRouter credits, model-level limits, or account/rate-limit policy on `z-ai/glm-5.1`

## Secret Hygiene

Live keys were found in a Google Doc and then moved into Windows user environment variables. Do not keep live secrets in shared docs longer than necessary.
