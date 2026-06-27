# Timeline

## 2026-06-04 16:18:37 America/Toronto
- User asked: continue
- Context gathered: checked skill requirements, `git status --short --branch`, `git diff --stat`, and diffs for modified backend/frontend files
- Decision: reconstruct state from local changes because there was no prior `session/` directory to resume from
- Result: identified likely work areas in command allowlisting, remote `ssh` validation, docker exec behavior, and tool-approval UI state handling

## 2026-06-04 16:20:12 America/Toronto
- Context gathered: ran focused Go tests, focused Vitest, repo diff sanity check, and repo-wide TypeScript check
- Decision: avoid new code edits because targeted verification is green and the only failing broad check is unrelated to the touched patch
- Result: current resumed patch appears internally consistent; only `frontend/preview/*` type issues remain outside this work

## 2026-06-04 16:56:05 America/Toronto
- User asked: continue
- Context gathered: resumed from `session/20260604_161837`, re-ran focused Go tests, focused Vitest, `git diff --check`, and repo-wide `tsc --noEmit`
- Decision: keep the source patch unchanged because all targeted checks still pass and the repo-wide TypeScript failure remains isolated to existing `frontend/preview/*` files
- Result: the current patch is verified again in the live session and is ready for review, commit, or separate follow-up cleanup

## 2026-06-04 17:16:45 America/Toronto
- User asked: Wave Alt reload still showed many non-working models in the model list
- Context gathered: inspected Wave Alt launcher and active `%LOCALAPPDATA%\\waveterm-alt\\config\\waveai.json`; ran live plain-chat and tool-call smoke tests against each configured provider/model; checked OpenRouter public models list; tested local Ollama endpoint separately
- Decision: treat this as a bad active model list rather than a repo runtime bug, back up the full local config, and reduce the active list to models that returned structured tool calls successfully
- Result: active Wave Alt config now contains 10 validated tool-capable models; original full list preserved in timestamped backup file

## 2026-06-04 17:20:45 America/Toronto
- User asked: continue after restarting the app
- Context gathered: inspected active alt `settings.json`; found no `waveai:showcloudmodes` override, which meant built-in Wave cloud modes could still appear alongside the reduced custom list
- Decision: disable cloud modes in the active alt settings rather than changing repo source
- Result: `%LOCALAPPDATA%\\waveterm-alt\\config\\settings.json` now sets `waveai:showcloudmodes=false`

## 2026-06-05 07:57:35 America/Toronto
- User asked: continue
- Context gathered: resumed from `session/LATEST-AGENT-HANDOFF.md`; verified the running Electron process is `Wave (Dev: alt)` from the repo checkout; confirmed `%LOCALAPPDATA%\\waveterm-alt\\config\\waveai.json` still contains the reduced 10-model list and `settings.json` still sets `waveai:showcloudmodes=false`; checked frontend filtering logic and `waveapp.log`
- Decision: treat the Wave Alt restart as likely successful because the live app is using a custom model from the reduced list and the source code path should hide built-in cloud modes under the current settings
- Result: recovered state is consistent for the live Alt instance, but found an unmentioned repo-local modification to `pkg/wconfig/defaultconfig/waveai.json` that contains embedded API tokens and requires explicit user direction
