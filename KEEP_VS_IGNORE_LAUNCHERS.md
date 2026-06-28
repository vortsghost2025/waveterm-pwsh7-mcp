# KEEP_VS_IGNORE_LAUNCHERS.md
**Generated**: 2026-06-28 | **NO EDITS MADE — REPORT ONLY**

---

## KEEP — Daily Use Launchers

| Launcher | Path | Role | Action |
|---|---|---|---|
| Wave Alt | `S:\waveterm\launch-wave-alt.ps1` | Primary Wave instance | **KEEP** — daily driver |
| Wave Bridge | `S:\waveterm\launch-wave-bridge.ps1` | Dev/test secondary Wave | **KEEP** — seeded from alt |
| Wave Main | `S:\waveterm\launch-wave.ps1` | Default Wave (no suffix) | **KEEP** — but no data dir exists (waveterm\config missing) |
| OpenCode Desktop | Start Menu → OpenCode.lnk | Main agent | **KEEP** — daily driver |
| CP Launcher | `S:\WE4FREE-Control-Plane\tools\cp-launch-opencode-control-plane.ps1` | Control plane | **KEEP** — orchestrator |
| Kilo B | `~\AppData\Local\waveterm-alt\data\bin\kilo-b.ps1` | Wave-embedded agent | **KEEP** — daily agent |

## KEEP — Test/Infra Launchers (low priority but useful)

| Launcher | Path | Role | Action |
|---|---|---|---|
| Kilo A | `~\AppData\Local\AgentProfiles\kilo-a\` | Isolated test | **KEEP** — minimal key surface |
| OpenCode A/B | `~\AppData\Local\AgentProfiles\opencode-{a,b}\` | Test profiles | **KEEP** — no active keys |

## IGNORE — Dormant/Stale Instances

| Instance | Path | Reason |
|---|---|---|
| waveterm (no suffix) | `~\AppData\Local\waveterm\` | No config dir exists — never initialized |
| waveterm-dev | `~\AppData\Local\waveterm-dev\` | Dev instance — likely stale |
| waveterm-dev-alt | `~\AppData\Local\waveterm-dev-alt\` | Dev alt — likely stale |
| waveterm-updater | `~\AppData\Local\waveterm-updater\` | Auto-updater artifact — not a runtime |
| WaveTermAltTest | `~\AppData\Local\WaveTermAltTest\` | Test instance — likely stale |

## TODO — New Launchers Needed

| Launcher | Path | Purpose | Status |
|---|---|---|---|
| Wave-OpenCode Bridge | `S:\waveterm\launch-wave-opencode-bridge.ps1` | Launch opencode inside Wave Alt tab, inheriting WAVETERM_JWT | **TO CREATE** |
