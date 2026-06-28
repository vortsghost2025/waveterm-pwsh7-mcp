# KEY_ROTATION_REQUIRED.md
**Generated**: 2026-06-28

---

## ROTATE_REQUIRED — Confirmed Exposed

| Provider | Location | How Exposed | Priority |
|---|---|---|---|
| **Morph API** | `~\.config\opencode\opencode.json` → morph-mcp command args | Visible in MCP config (plaintext `sk-` prefix) + pasted in GPT chat | **HIGH** |
| **Figma Access Token** | `~\.config\opencode\opencode.json` → figma MCP env | Pasted in GPT chat (currently disabled, but token still in config) | **HIGH** |
| **Brave Search API** | `~\.config\opencode\opencode.json` → brave-search MCP env | Visible in MCP config (plaintext `BSA` prefix) | **MEDIUM** |
| **GitHub Personal Access Token** | `~\.config\opencode\opencode.json` → github MCP env | Visible in MCP config (ROTATED in redacted output but key still present) | **MEDIUM** |

## ROTATE_REQUIRED — In Secrets Files (Not Yet Chat-Exposed)

| Provider | Location | Notes | Priority |
|---|---|---|---|
| NVIDIA_API_KEY | `~\AppData\Local\AgentProfiles\kilo-b\secrets.ps1` | Active, 1 key | **LOW** (not exposed but in dot-sourced PS1) |
| OPENROUTER_API_KEY | `~\AppData\Local\AgentProfiles\kilo-b\secrets.ps1` | Active, 1 key | **LOW** |
| GOOGLE_API_KEY | `~\AppData\Local\AgentProfiles\kilo-b\secrets.ps1` | Active, 1 key | **LOW** |
| GEMINI_API_KEY | `~\AppData\Local\AgentProfiles\kilo-b\secrets.ps1` | Active, 1 key | **LOW** |
| NVIDIA_API_KEY | `~\AppData\Local\AgentProfiles\kilo-a\secrets.ps1` | Active, 1 key | **LOW** |

## Action Items

1. **Morph API key** — rotate at https://morphllm.com dashboard, then update `opencode.json` MCP config
2. **Figma token** — rotate at https://www.figma.com/developers/api#access-tokens, then update `opencode.json` figma MCP
3. **Brave API key** — rotate at https://api.search.brave.com, then update `opencode.json` brave-search MCP
4. **GitHub PAT** — rotate at https://github.com/settings/tokens, then update `opencode.json` github MCP
5. After rotation, consider moving keys OUT of `opencode.json` and into env vars referenced with `${VAR_NAME}` syntax
