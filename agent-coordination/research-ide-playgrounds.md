# Research: Adopt IDE Playground Patterns for Agent Playgrounds

Source: `S:\waveterm\docs\june29ideas.txt` (June 29 2026)

## Premise

Wave Terminal's value-add for Wave AI is unusually rich in *terminal* and
*bridge* tools (file I/O, run_command, scan_terminals, term_send_input) but
*thin* in *IDE-style* affordances:
- multi-pane diff view
- symbol search across workspace
- hover-tooltip definitions
- live type-aware completion exposed as tools
- per-symbol jump-history
- workspace-wide reference graph queries

The bridge experiment (browser agent reading/writing ChatGPT via Wave's
`web_navigate` + `term_send_input`, klick-see-klick loop) is one prompt to
adopt a pattern. The IDE adoptions below are the next ones.

## Hypotheses worth testing

1. **Symbol query as a tool.** Add `go_symbols` (returns file,line,kind for a
   matches across the workspace). AI agents use it the same way they currently
   rely on `grep` + manual reads; faster convergence, fewer tool calls.
2. **Call-graph as a tool.** Add `go_references "Symbol"` (all callers/callees).
   Wave AI gets a "type-aware grep" for free. Or, where licensing allows,
   reuse `gopls`'s internal indexes.
3. **Workspace snapshot as a tool.** `workspace_snapshot` returns a single
   compressed file suitable for fast full-context re-hydration after AI session
   drops. Cuts the "the user reloaded I lost my goal" loop.
4. **Diff preview as a tool.** `git_diff_pending` returns compact unified diff
   of uncommitted/branch changes — replaces the shell `git diff | cat` crutch.
5. **Test result summarizer as a tool.** `go_test_summary` parses `go test`
   output into {pass/fail counts, broken files, top error lines}; AI test
   debugging is a poor use of model attention without it.

## What we're already seeing

The schema validator work shows Wave Terminal can ship AI tools fast and they
ship stable when guarded by harness tests (`tools_validate_test.go`, ~290
lines, 7 fixture cases). So velocity is not the bottleneck. Bottleneck is
*what* to ship — which tool exists in IDEs that we could re-expose as
agent ground-truth.

## Status

- [ ] Pick a sample IDE to model on (Cursor, VS Code+Cody, Replit Agent,
      Claude Code, Codex CLI, aider — see pros/cons below)
- [ ] Map each agent-touchable IDE affordance against Wave Terminal's
      existing tool surface
- [ ] Pick 3 to prototype, gated by `tools_validate_test.go`
- [ ] If the prototypes move the needle, write up a "playgrounds" RFC

## Comparison rough

| Surface | Cursor | VS Code+Cody | Claude Code | Aider | Wave Terminal |
| --- | --- | --- | --- | --- | --- |
| Symbol search | yes | yes | partial | no | grep |
| Call graph | yes | yes | partial | no | none |
| Diff preview UI | yes | yes | yes | yes | git status only |
| Workspace context handoff | yes | yes | yes | yes | none |
| Test summarizer | partial | partial | partial | partial | none |
| Multi-tab workspace | yes | yes | yes | yes | yes — naturally |
| Free-form tool registry | no | no | partial | partial | yes (this repo) |

## Open questions

- Does a free-form tool registry beat a fixed IDE affordance set when the
  user-supplied LLM is the consumer? Vote: probably yes for power users, no
  for junior devs.
- Persistence: should playground state survive user absence? (Workspace
  snapshot + a planned-task queue both argue yes.)

## Review

- [ ] wave-ai: surface any tools you've wished existed.
- [ ] user: confirm direction when you have a beat.

Last touched: 2026-06-29 17:00 EDT.
