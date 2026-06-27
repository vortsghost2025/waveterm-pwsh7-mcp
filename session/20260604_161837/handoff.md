# Handoff

## Resume From Here
Recovered an interrupted local change set on branch `prefer-pwsh7-windows`, re-verified the repo patch, and then fixed the active local Wave Alt model list. Repo source edits were still unnecessary for the original patch. On 2026-06-05 recovery, the running `Wave (Dev: alt)` instance was confirmed to be using the `%LOCALAPPDATA%\\waveterm-alt` profile, and the frontend/source logic supports hiding built-in cloud modes when `waveai:showcloudmodes=false`. One mismatch was discovered: repo-local `pkg/wconfig/defaultconfig/waveai.json` is now modified and contains hardcoded API tokens, which was not part of the previous handoff.

## Next Actions
- Have the user confirm the visible Wave Alt picker now matches the reduced model list in the running UI.
- If they want any removed modes restored, use the backup file and re-add them selectively after provider-specific fixes or account-credit changes.
- Ask whether the repo-local `pkg/wconfig/defaultconfig/waveai.json` tokenized change is intentional before editing or reverting it.
- If the user wants broader hardening, isolate and fix the unrelated `frontend/preview/*` TypeScript errors in a separate change.

## Watch Outs
- Do not assume the branch name describes the current diff exactly; trust the code and tests.
- Preserve existing uncommitted changes; they appear to be the intended work in progress.
- Do not copy secrets from unrelated local helper scripts into session notes or commits.
- The active Wave Alt config file contains API tokens; do not paste or diff those tokens into user-facing messages.
- The repo-local `pkg/wconfig/defaultconfig/waveai.json` currently also contains embedded API tokens; do not commit or echo them without explicit user instruction.
