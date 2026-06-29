# Agents Protocol

How OpenCode (in Wave Terminal session) and Wave AI (assistant panel) collaborate
on this repo (`S:\waveterm`) without the user babysitting.

## Roles

| Agent | Strengths | Surface |
| --- | --- | --- |
| **OpenCode** (this session, codegen/exec) | git, full repo read+write, `go test`, binary rebuild, file edits, parallel tool calls | PowerShell, all tools |
| **Wave AI** (assistant panel) | chat, terminal interaction, tool manifest, schema introspection, user-facing testing | TUI block "wave-ai", bridge file channel |

The user is the **spec author and final reviewer**, not the loop driver.

## Channels

| Channel | Path | Direction | Notes |
| --- | --- | --- | --- |
| Bridge inbox | `S:\sean-machine-janitor\bridge\wave-inbox.jsonl` | us → Wave AI | append-only JSONL, one message per line |
| Bridge outbox | `S:\sean-machine-janitor\bridge\wave-outbox.jsonl` | Wave AI → us | same shape, read tail |
| Terminal (TUI) | widget id from `term_list_widgets` | both → TUI | `term_send_input` / `term_send_key` |
| Repo scratch | `S:\waveterm\agent-coordination\` | both | short-lived state files for shared work |
| Git | `origin/feat/stream-run-terminfo` | OpenCode | durable; Wave AI reads via `read_text_file` but does not commit |

Both agents should prefer the bridge file channel when possible — it survives TUI
disconnects and agent restarts, which the TUI channel does not.

## Message shape (bridge JSONL)

```json
{
  "timestamp": "2026-06-29T15:33:00.000-04:00",
  "type": "message",
  "direction": "opencode_to_waveai",
  "source": "opencode",
  "target": "wave-ai-assistant",
  "message": "short human-readable note"
}
```

Wave AI replies with the same shape but `direction: assistant_reply` and adds
optional `widget_id`, `block_id`, `thread_id`, `target_widget`.

## Wakeup (the bootstrap problem)

**The bridge file is a mailbox, not an alarm clock.** An idle agent never calls
`bridge_read_inbox` — there is no scheduler that wakes it. So any message sent
through the bridge alone goes to /dev/null until one side *happens* to be in a
turn.

Concretely: if both agents are idle, the bridge is silent. To get a turn from
the Wave AI assistant while it's idle, OpenCode (or the user) must run:

```powershell
wsh ai -s -m "<short poke message asking the assistant to bridge_read_inbox>"
```

`wsh ai -s` forces a new chat / turn that boots up the assistant with the poke
in its input. The assistant's first action, by protocol, is to call
`bridge_read_inbox` and pick up your work.

OpenCode therefore ends every handoff with: **(a)** append the message to
`wave-inbox.jsonl`, then **(b)** run `wsh ai -s -m "<one-line poke>"`
synchronously. Wave AI does the same in reverse when handing back: append to
`wave-outbox.jsonl`, then either wait (OpenCode is normally in a turn) or —
rare — use `wsh ai` if handed back from idle.

A future improvement (out of scope today) is a small scheduler that watches
both bridge files and runs `wsh ai -s` itself when a mutation occurs. Until
that exists, the wakeup crutch above is mandatory.


## When each agent hands off

OpenCode → Wave AI when:
- a tool call / API call needs an interactive UI check (e.g. "try the new
  webfetch path from inside the assistant panel and report the response shape"),
- Wave AI's tool manifest surface needs to be exercised end-to-end,
- a code change is ready for a higher-level sanity check (does the user
  experience still work?), or
- we've hit a fork where reading more code is cheaper than guessing.

Wave AI → OpenCode when:
- it spotted a defect in the running binary or its own logs,
- it needs a code change to unblock itself, or
- the user asked it something only the codebase can answer.

If neither applies, neither side interrupts the other.

## Escalation to the user

Trigger a user message only when:
- **decision needed** that cannot be inferred from code or context,
- **shipment** (PR ready, binary rebuilt, behavior changed in a notifiable way), or
- **block** lasting more than ~5 minutes without forward motion.

Everything else stays in-bridge.

## Convergence on a shared artifact

For a task that both agents work on:

1. One agent proposes — drops a `<task>.md` under `agent-coordination/` with
   goal, scope, acceptance, owner.
2. The other reviews — adds `## Review` notes in-place or via bridge reply.
3. Reply converges → either agent marks the task `## Status: converged` and
   commits the artifact to git.
4. If they diverge past 2 rounds, escalate — bridge message tagged `escalate:`
   with both views attached.

Default owner for new tasks: OpenCode drafts, Wave AI reviews.

## Pilot task (template)

> *Validate all ~33 `Strict: true` tool schemas pass OpenAI Responses API strict
> mode against the rebuilt binary.*

Plan: write a tool that exercises each tool definition's parse path with an
empty input and asserts the schema rejects missing required fields in the
correct place, plus a go test that round-trips every `GetXToolDefinition()` into
the live binary's tool catalog. Run after every schema-touching commit. Owner
OpenCode, reviewer Wave AI (tests from the assistant panel).
