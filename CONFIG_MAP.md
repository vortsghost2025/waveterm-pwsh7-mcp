# CONFIG_MAP.md — Runtime Family Audit
**Generated**: 2026-06-28 | **NO EDITS MADE — REPORT ONLY**

---

## 1. Runtime Families

### R1: OpenCode Desktop (Normal)
| Property | Value |
|---|---|
| **Binary** | `opencode` (scoop-installed) |
| **Launcher** | Windows Start Menu → OpenCode.lnk |
| **Working Dir** | Varies (usually `S:\waveterm` or `S:\WE4FREE-Control-Plane`) |
| **Config Root** | `~\.config\opencode\` (shared global `opencode.json`) |
| **Data Root** | `~\.local\share\opencode\` |
| **Env** | User-level env vars (NVIDIA_API_KEY, OPENROUTER_API_KEY, etc.) |
| **Keys by NAME** | MORPH_API_KEY (in MCP cmd), BRAVE_API_KEY (in MCP cmd), GITHUB_PERSONAL_ACCESS_TOKEN, FIGMA_ACCESS_TOKEN, SLACK_BOT_TOKEN |
| **MCP Enabled** | morph-mcp, github, filesystem, brave-search, sequential-thinking, genesis-memory, ssh, session |
| **MCP Disabled** | MCP_DOCKER, everything, memory, figma, puppeteer, slack, archivist-* |
| **Skill Dir** | `~\.agents\skills\` (500+ skills — **20K+ token bloat**) |
| **wave-mcp** | **NO** — WAVETERM_JWT not available |
| **Wildcard** | N/A (opencode has permission:*:allow) |
| **Usage** | Daily driver |

### R2: Wave-Launched OpenCode (Bridge)
| Property | Value |
|---|---|
| **Binary** | `opencode` (inside Wave terminal tab) |
| **Launcher** | **NOT YET CREATED** — needs new launcher script |
| **Working Dir** | `S:\waveterm` (inherited from Wave) |
| **Config Root** | `~\.config\opencode\` (SAME as R1 — LEAK RISK) |
| **Data Root** | `~\.local\share\opencode\` (SAME as R1) |
| **Env** | Inherits WAVETERM_JWT from Wave shell + user env vars |
| **Keys by NAME** | Same as R1 + WAVETERM_JWT (per-tab, ephemeral) |
| **MCP** | Same as R1 + **wave-mcp** (once configured) |
| **wave-mcp** | **YES** — WAVETERM_JWT inherited from Wave shell |
| **Wildcard** | N/A |
| **Usage** | Bridge/control runtime (when created) |

### R3: Wave Alt (Primary Wave Instance)
| Property | Value |
|---|---|
| **Binary** | `waveterm` (Electron, `npm run start`) |
| **Launcher** | `S:\waveterm\launch-wave-alt.ps1` |
| **Working Dir** | `S:\waveterm` |
| **Config Root** | `~\AppData\Local\waveterm-alt\config\` |
| **Data Root** | `~\AppData\Local\waveterm-alt\data\` |
| **Env** | WAVETERM_INSTANCE_SUFFIX=alt, WAVETERM_DATA_HOME, WAVETERM_CONFIG_HOME |
| **Keys by NAME** | NVIDIA_API_KEY, OPENROUTER_API_KEY (in waveai.json via env var refs) |
| **wave-mcp** | YES (runs inside Wave, WAVETERM_JWT per-tab) |
| **Auto-Approve** | User UI toggle (status unknown — likely ON per prior sessions) |
| **Wildcard** | YES (wildcard access enabled for AI) |
| **Usage** | Daily use + AI agent test bed |

### R4: Wave Bridge Dev
| Property | Value |
|---|---|
| **Binary** | `waveterm` (Electron, `npm run start`) |
| **Launcher** | `S:\waveterm\launch-wave-bridge.ps1` |
| **Working Dir** | `S:\waveterm` |
| **Config Root** | `~\AppData\Local\waveterm-bridge\config\` (seeded from alt) |
| **Data Root** | `~\AppData\Local\waveterm-bridge\data\` |
| **Env** | WAVETERM_INSTANCE_SUFFIX=bridge, seeded config from alt |
| **Keys by NAME** | Same as Wave Alt (one-time file copy, not live sync) |
| **wave-mcp** | YES (if running) |
| **Wildcard** | Unknown (inherited from alt seed, may differ) |
| **Usage** | Dev/test secondary instance |

### R5: Kilo B (Wave-Embedded Agent)
| Property | Value |
|---|---|
| **Binary** | `kilo` (fork of opencode, runs inside Wave Alt tabs) |
| **Launcher** | `~\AppData\Local\waveterm-alt\data\bin\kilo-b.ps1` |
| **Working Dir** | Varies (launched from Wave Alt terminal) |
| **Config Root** | `~\AppData\Local\AgentProfiles\kilo-b\config\kilo\` (`kilo.jsonc`) |
| **Data Root** | `~\AppData\Local\AgentProfiles\kilo-b\data\kilo\` |
| **State Root** | `~\AppData\Local\AgentProfiles\kilo-b\state\` |
| **Env** | Sources `~\AppData\Local\AgentProfiles\kilo-b\secrets.ps1` |
| **Keys by NAME** | NVIDIA_API_KEY, OPENROUTER_API_KEY, GOOGLE_API_KEY, GEMINI_API_KEY (from secrets.ps1) |
| **MCP** | kilo's own `kilo.jsonc` config (has node_modules, zod, etc.) |
| **Skill Dir** | `~\AppData\Local\AgentProfiles\kilo-b\config\kilo\skills\` (if present) |
| **wave-mcp** | YES (runs inside Wave Alt, inherits WAVETERM_JWT) |
| **Wildcard** | YES (via Wave Alt's wildcard access) |
| **Usage** | Daily AI agent (swarm test runtime) |

### R6: Kilo Control Plane
| Property | Value |
|---|---|
| **Binary** | `kilo` or `opencode` (resolved by CP launcher) |
| **Launcher** | `S:\WE4FREE-Control-Plane\tools\cp-launch-opencode-control-plane.ps1` |
| **Working Dir** | `S:\WE4FREE-Control-Plane` |
| **Config Root** | `~\AppData\Local\WE4FREE-Control-Plane\runtime-profiles\{kilo,opencode}\` |
| **Data Root** | Profiles include `.local\share\{kilo,opencode}\` |
| **Env** | Isolated per-profile via HOME/USERPROFILE override |
| **Keys by NAME** | Via profile secrets (none at we-control-kilo-runtime; kilo/opencode profiles have their own) |
| **Skill Dir** | `~\.agents\skills\` (if HOME points to user profile) |
| **wave-mcp** | NO (runs outside Wave) |
| **Wildcard** | Via opencode permission config |
| **Usage** | Orchestrated control plane (test/deploy) |

### R7: Kilo A (Isolated)
| Property | Value |
|---|---|
| **Binary** | `kilo` |
| **Config Root** | `~\AppData\Local\AgentProfiles\kilo-a\config\` |
| **Data Root** | `~\AppData\Local\AgentProfiles\kilo-a\data\` |
| **Env Sources** | `~\AppData\Local\AgentProfiles\kilo-a\secrets.ps1` |
| **Keys by NAME** | NVIDIA_API_KEY (only) |
| **wave-mcp** | NO (unless launched inside Wave) |
| **Usage** | Isolated test profile |

### R8: OpenCode A/B Profiles
| Property | Value |
|---|---|
| **Binary** | `opencode` |
| **Config Root** | `~\AppData\Local\AgentProfiles\opencode-{a,b}\config\opencode\` |
| **Data Root** | `~\AppData\Local\AgentProfiles\opencode-{a,b}\data\opencode\` |
| **Env Sources** | `secrets.ps1` (all keys commented out — inherits from user env) |
| **Keys by NAME** | None active (all commented in secrets.ps1) |
| **wave-mcp** | NO |
| **Usage** | Test profiles (not daily use) |

### R9: Federation VPS
| Property | Value |
|---|---|
| **Host** | `federation-vps` / `187.77.3.56` (root) |
| **SSH Alias** | federation-vps |
| **Identity** | `~\.ssh\id_ed25519` |
| **Services** | Docker-based backend + PostgreSQL |
| **Keys by NAME** | None on local machine (env on VPS) |
| **wave-mcp** | NO |
| **Usage** | Remote consciousness simulation backend |

### R10: VPS2 (Hermes)
| Property | Value |
|---|---|
| **Host** | `vps2` / `2.25.206.123` (root) |
| **SSH Alias** | vps2 |
| **Identity** | `~\.ssh\hermes_vps_kvm2` |
| **Usage** | Hermes workspace (remote assistant stack) |

### R11: Headless
| Property | Value |
|---|---|
| **Host** | `headless` / `100.95.40.99` (we4free, Tailscale) |
| **SSH Alias** | headless |
| **Identity** | `~\.ssh\id_ed25519` |
| **Usage** | Headless dev/test box |

---

## 2. Special Wave Rule

**wave-mcp.exe requires WAVETERM_JWT.** This is a per-terminal Ed25519-signed JWT set as an env var in every Wave shell tab. It authenticates against the Wave server's domain socket.

**Therefore:**
- Only Wave Alt (R3), Wave Bridge (R4), and Kilo B (R5) can use wave-mcp
- Normal OpenCode Desktop (R1) CANNOT use wave-mcp
- OpenCode launched INSIDE a Wave tab (R2) CAN use wave-mcp — it inherits WAVETERM_JWT

**Bridge architecture must be:**
```
Normal OpenCode Desktop → no wave-mcp, standard agent tools
OpenCode inside Wave tab → wave-mcp available, full terminal control
Kilo B inside Wave Alt  → wave-mcp available, full terminal control
```

---

## 3. Config Leakage Risks

| Risk | Source → Target | Severity |
|---|---|---|
| Shared `~\.config\opencode\opencode.json` | R1 + R2 use same config | HIGH — morph API key, brave key, github token all in MCP config |
| Wave Alt → Bridge seed copy | R3 → R4 one-time file copy | MEDIUM — keys may be stale, not live-updated |
| Kilo B secrets.ps1 | R5 has 4 active API keys | MEDIUM — isolated but 4 keys in one file |
| opencode permission:*:allow | R1 has blanket allow | HIGH — no guardrails on tool execution |
| Skill list bloat (500+) | R1 + any runtime using `~\.agents\skills\` | HIGH — ~20K token context waste |

---

## 4. Security Notes

- All secrets in this document referenced by NAME ONLY
- No token values included (see KEY_ROTATION_REQUIRED.md for flags)
- `opencode.json` contains live Morph API key and Brave API key in MCP command args — readable by any process
- Kilo B `secrets.ps1` has 4 live API keys in dot-sourced PowerShell
