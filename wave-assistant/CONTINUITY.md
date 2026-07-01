# Wave Assistant Lane Continuity

## Persistence Model
This lane persists through **identity continuity in external substrate**, not through model memory. The external substrate includes:
- Git history (`feat/stream-run-terminfo`)
- Files in `S:\waveterm\wave-assistant\`
- Bridge messages in `S:\sean-machine-janitor\bridge\`
- SHA-256 hashes of critical binaries (wavesrv, wsh, wavehydrate)

## Rehydration Protocol
On each new session, Wave AI must receive:
```
HYDRATE_WAVE_ASSISTANT v1
<JSON>
```

The JSON must contain:
- `lane_id`
- `role`
- `authority`
- `forbidden_claims`
- `active_repos`
- `active_lanes`
- `current_objective`
- `verified_facts`
- `uncertain_facts`
- `open_risks`
- `expected_output_contract`
- `evidence_refs`
- `ttl_sec`
- `conflict_policy`

## Wakeup Procedure
When Wave AI is idle, OpenCode must:
1. Append message to `S:\sean-machine-janitor\bridge\wave-inbox.jsonl`
2. Run `wsh ai -s -m "<short poke>"` to force a new turn

## Snapshot Refresh
Snapshots must be regenerated when:
- Git head changes
- Binary SHA-256 changes
- Environment SHA-256 changes
- TTL expires (default 24h)

## Trust Boundaries
- Wave AI may read git, files, and bridge messages
- Wave AI may NOT commit to git
- Wave AI may NOT modify launcher configs
- Wave AI may NOT tamper with keys or secrets
