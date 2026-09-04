# Agent Mesh — Status

Working-state notes for the multi-agent orchestration layer of this fork.
This document separates what is **shipped** from what is **in flight**, so a
cold reader never has to guess which claims are inspectable today.

The shipped, merged surface is everything under
[Agent-operable terminal control](../README.md#agent-operable-terminal-control)
in the README. Everything below is active development — it exists as working
branches and has been exercised locally, but is **not on the clean review
branch** and should not be treated as a released feature.

## In-flight pieces

### 1. Multi-terminal CLI-agent spawn

Spawn external CLI agents (OpenCode, Kilo, etc.) directly from the shell into
dedicated Wave terminals, so an agent session has its own inspectable pane
instead of a hidden process.

- Working branches exist; the spawn/attach lifecycle is functional locally.
- Remaining: review-slice isolation (the development branch carries unrelated
  ancestry), plus terminal-attachment tests.

### 2. `wsh` idle nudges

A `wsh` command surface for one agent to nudge another out of an idle state —
the "are you stuck or thinking?" probe that supervisor agents need before
they intervene.

- Prototype works locally through the WSH command path.
- Remaining: allowlist scoping and tests mirroring the other WSH commands.

### 3. Outbox / inbox handoff

Asynchronous agent-to-agent message passing through the terminal workspace:
workers drop completed work in an outbox directory, the next agent picks it
up from its inbox, no human relay.

- Working branches exist with the directory conventions and pickup loop.
- Remaining: integration tests and a documented message format before it is
  proposed as a review PR.

### 4. OpenCode ↔ Wave bridge

An external supervisor that can monitor Wave sessions, run commands outside
the Electron process, and relaunch a dead session — so the agent orchestrating
Wave survives Wave itself.

- The bridge (monitor supervisor + relaunch path) works against a local
  instance.
- Remaining: session-lifecycle hardening and a clean review slice.

## Why these are not on the default branch yet

Each piece was developed against live sessions before being documented here.
The fork's convention (see [PR #22](https://github.com/vortsghost2025/waveterm-pwsh7-mcp/pull/22))
is to land features as isolated, reviewable PRs on a clean base — with
bindings, tests, and runtime verification — rather than transplanting a long
experimental branch wholesale. These four are queued for exactly that
treatment.

## Verification status

Everything in this document was exercised against a real local Wave instance
during development. None of it is covered by the E2E pipeline yet; each piece
gains tests as part of its review PR.
