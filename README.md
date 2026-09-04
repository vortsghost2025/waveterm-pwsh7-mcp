<p align="center">
  <a href="https://www.waveterm.dev">
	<picture>
		<source media="(prefers-color-scheme: dark)" srcset="./assets/wave-dark.png">
		<source media="(prefers-color-scheme: light)" srcset="./assets/wave-light.png">
		<img alt="Wave Terminal Logo" src="./assets/wave-light.png" width="240">
	</picture>
  </a>
  <br/>
</p>

# Wave Terminal

> **This is a heavily modified Wave Terminal fork focused on making Wave agent-operable, not just AI-assisted.** The upstream project is maintained by the Wave team. This fork preserves the upstream application while adding Windows/PowerShell 7 execution, AI-accessible terminal control, agent/MCP tooling, model-provider routing, and verification/reliability work.

## What this fork is building

The goal is to turn Wave into an execution and coordination surface where AI agents can inspect the workspace, find terminals and widgets, run controlled shell work, interact with terminal sessions, and hand work to external CLI agents instead of requiring a human to manually relay every step.

This is not a cosmetic reskin. The modifications are aimed at giving Wave AI and connected agents practical control-plane capabilities while keeping the work inspectable and testable.

### Agent-operable terminal control

Published work in this fork includes:

- **AI-accessible command execution** — `run_interactive_command` provides structured, time-bounded shell execution with approval, command allowlisting, output limits, timeout handling, exit status, stdout, and stderr.
- **Terminal/widget discovery** — `term_list_widgets` lets agents enumerate terminal and widget targets directly from Wave's in-process store rather than relying on brittle shell parsing.
- **Workspace discovery** — `list_workspaces`, `list_tabs`, and `get_widget` let agents discover the Wave UI hierarchy and inspect target widgets before acting.
- **WSH terminal input** — agents/tools can send input to terminal widgets through the WSH command surface.
- **Streamed command execution** — WSH RPC work streams stdout/stderr/exit events while commands are running instead of waiting for a single final result.
- **Terminal information RPC** — terminal/session information is exposed through WSH for programmatic inspection.
- **MCP-oriented command and filesystem tooling** — the fork adds guarded file operations, command execution, terminal discovery, and related integration paths for external tooling.

Upstream Wave's README currently describes command execution as **"Coming Soon"**. This fork already contains implemented command-execution and terminal-control paths intended for agent use.

### Agent orchestration and communication

The broader direction of the fork is autonomous and human-supervised agent coordination inside the terminal environment:

- Wave AI can use terminal/workspace discovery instead of operating blindly.
- Terminal execution paths are designed so work can be launched and monitored programmatically.
- External CLI-agent orchestration is being developed so Wave can spawn or route work to CLI agents rather than requiring manual copy/paste handoffs.
- WSH-based messaging plus inbox/outbox-style communication is being developed for asynchronous agent-to-agent coordination, so workers can exchange work and results without the user acting as the message bus.

**Status: in active integration, not yet on the clean review branch.** The mesh pieces in flight are: multi-terminal CLI-agent spawn from the shell, `wsh` idle nudges between agents, the outbox/inbox handoff protocol, and an OpenCode ↔ Wave bridge (external supervisor that can monitor sessions, run commands outside Electron, and relaunch a session). They exist as working branches, not merged features — see [`docs/AGENT-MESH.md`](docs/AGENT-MESH.md) for the current state of each piece. Until they land, the shipped surface is exactly what's listed under [Agent-operable terminal control](#agent-operable-terminal-control) above.

### Windows and PowerShell 7

Windows is a first-class target in this fork rather than an afterthought:

- PowerShell 7-aware shell selection and execution.
- Cross-platform shell execution paths using PowerShell on Windows.
- Windows-specific runtime verification rather than assuming Unix shell behavior.
- PowerShell-oriented terminal behavior and WSH integration.

### AI provider and model routing

The fork also extends Wave's AI-provider surface:

- NVIDIA NIM / BYOK model configuration through `NVIDIA_API_KEY` environment-based secret lookup.
- Additional NVIDIA/Nemotron model modes, including large-context agentic/coding models.
- OpenAI-compatible provider routing without embedding API-key values in the repository.

### Verification-first development

The modification history intentionally includes evidence of how changes were validated, not just the final code:

- targeted regression tests,
- generated-binding checks,
- Go build and vet validation,
- Windows PowerShell runtime checks,
- scoped command/allowlist tests,
- and clean transplantation of features when an earlier development branch became unsuitable for review.

The point is to make the implementation trail inspectable rather than present agent behavior as a black box.

### Reliability and CI

The default branch carries a self-verifying CI chain on top of the feature work:

- **AI-driven E2E pipeline** — every `TestDriver.ai Build` completion triggers a `TestDriver.ai Run` that provisions a fresh Windows VM, installs the built artifact, and has an AI tester walk the real onboarding UI by sight (Continue → the 4-step feature wizard → Get Started → CPU-graph assertion). See [`testdriver/onboarding.test.mjs`](testdriver/onboarding.test.mjs).
- **Self-diagnosing failures** — any wizard-walk failure automatically dumps the sandbox's Wave/wavesrv process state and the last 40 lines of `waveapp.log` into the test output, so a flake names its own cause instead of leaving a red X.
- **Hardened workflow trust model** — the `workflow_run` gate checks out test definitions only from the repository's default branch and only after verifying the triggering run originated in this repository; fork-controlled test code never executes in the privileged (OIDC) context.
- **Lean production binaries** — `wavesrv` builds with stripped symbols (`-s -w`), cutting the binary ~30% and speeding first-launch AV scans of the unsigned exe.
- **Runner-environment workarounds documented in-place** — e.g. the npm 10 arborist crash workaround ([PR #37](https://github.com/vortsghost2025/waveterm-pwsh7-mcp/pull/37)), so the next image regression is a five-minute fix instead of a debugging session.

## Current clean feature review

[`PR #22 — feat(wshrpc): add stream command execution and terminal info`](https://github.com/vortsghost2025/waveterm-pwsh7-mcp/pull/22) is a deliberately isolated review of the streamed-command/terminal-info/input work.

- **1 commit**
- **9 changed files**
- **512 additions**
- generated Go/TypeScript bindings
- targeted WSH RPC test coverage
- Go build/vet verification
- Windows PowerShell 7 runtime verification

It was rebuilt on a clean `main` base after the original development branch accumulated unsuitable ancestry for review. A full-suite `pkg/tsgen` failure was separately reproduced on the pristine base and documented as pre-existing instead of being misattributed to the feature.

The recent reliability arc — PRs [#30](https://github.com/vortsghost2025/waveterm-pwsh7-mcp/pull/30)–[#37](https://github.com/vortsghost2025/waveterm-pwsh7-mcp/pull/37) — took the E2E chain from never-green to a working, self-diagnosing pipeline: launch-wait and SDK-native polling, a fixed trusted-checkout bug (tests were silently running from a stale branch), the real 4-step wizard walk, one-retry sandbox allowance, stripped `wavesrv` symbols, evidence dumps on failure, and the npm 10 runner workaround. See [Reliability and CI](#reliability-and-ci) above.

The broader experimental history remains visible in the repository so the progression from upstream Wave to this agent-operable fork can be inspected directly.

---

<div align="center">

[English](README.md) | [한국어](README.ko.md) | [繁體中文](README.zh-TW.md)

</div>

[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fwavetermdev%2Fwaveterm.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Fwavetermdev%2Fwaveterm?ref=badge_shield)

Wave is an open-source, AI-integrated terminal for macOS, Linux, and Windows. It works with any AI model. Bring your own API keys for OpenAI, Claude, or Gemini, or run local models via Ollama and LM Studio. No accounts required.

Wave also supports durable SSH sessions that survive network interruptions and restarts, with automatic reconnection. Edit remote files with a built-in graphical editor and preview files inline without leaving the terminal.

![WaveTerm Screenshot](./assets/wave-screenshot.webp)

## Key Features

- Wave AI - Context-aware terminal assistant that reads your terminal output, analyzes widgets, and performs file operations
- Durable SSH Sessions - Remote terminal sessions survive connection interruptions, network changes, and Wave restarts with automatic reconnection
- Flexible drag & drop interface to organize terminal blocks, editors, web browsers, and AI assistants
- Built-in editor for editing remote files with syntax highlighting and modern editor features
- Rich file preview system for remote files (markdown, images, video, PDFs, CSVs, directories)
- Quick full-screen toggle for any block - expand terminals, editors, and previews for better visibility, then instantly return to multi-block view
- AI chat widget with support for multiple models (OpenAI, Claude, Azure, Perplexity, Ollama)
- Command Blocks for isolating and monitoring individual commands
- One-click remote connections with full terminal and file system access
- Secure secret storage using native system backends - store API keys and credentials locally, access them across SSH sessions
- Rich customization including tab themes, terminal styles, and background images
- Powerful `wsh` command system for managing your workspace from the CLI and sharing data between terminal sessions
- Connected file management with `wsh file` - seamlessly copy and sync files between local and remote SSH hosts

## Wave AI

Wave AI is your context-aware terminal assistant with access to your workspace:

- **Terminal Context**: Reads terminal output and scrollback for debugging and analysis
- **File Operations**: Read, write, and edit files with automatic backups and user approval
- **CLI Integration**: Use `wsh ai` to pipe output or attach files directly from the command line
- **BYOK Support**: Bring your own API keys for OpenAI, Claude, Gemini, Azure, and other providers
- **Local Models**: Run local models with Ollama, LM Studio, and other OpenAI-compatible providers
- **Free Beta**: Included AI credits while we refine the experience
- **Coming Soon** (upstream): Command execution (with approval)
- **In this fork**: Command execution paths are already implemented — see [Agent-operable terminal control](#agent-operable-terminal-control) above. Upstream's status line does not reflect this fork.

Learn more in our [Wave AI documentation](https://docs.waveterm.dev/waveai) and [Wave AI Modes documentation](https://docs.waveterm.dev/waveai-modes).

## Installation

Wave Terminal works on macOS, Linux, and Windows.

Platform-specific installation instructions can be found [here](https://docs.waveterm.dev/gettingstarted).

You can also install Wave Terminal directly from: [www.waveterm.dev/download](https://www.waveterm.dev/download).

### Minimum requirements

Wave Terminal runs on the following platforms:

- macOS 11 or later (arm64, x64)
- Windows 10 1809 or later (x64)
- Linux based on glibc-2.28 or later (Debian 10, RHEL 8, Ubuntu 20.04, etc.) (arm64, x64)

The WSH helper runs on the following platforms:

- macOS 11 or later (arm64, x64)
- Windows 10 or later (x64)
- Linux Kernel 2.6.32 or later (x64), Linux Kernel 3.1 or later (arm64)

## Roadmap

Wave is constantly improving! Our roadmap will be continuously updated with our goals for each release. You can find it [here](./ROADMAP.md).

Want to provide input to our future releases? Connect with us on [Discord](https://discord.gg/XfvZ334gwU) or open a [Feature Request](https://github.com/wavetermdev/waveterm/issues/new/choose)!

## Links

- Homepage &mdash; https://www.waveterm.dev
- Download Page &mdash; https://www.waveterm.dev/download
- Documentation &mdash; https://docs.waveterm.dev
- X &mdash; https://x.com/wavetermdev
- Discord Community &mdash; https://discord.gg/XfvZ334gwU

## Building from Source

See [Building Wave Terminal](BUILD.md).

## Contributing

Wave uses GitHub Issues for issue tracking.

Find more information in our [Contributions Guide](CONTRIBUTING.md), which includes:

- [Ways to contribute](CONTRIBUTING.md#contributing-to-wave-terminal)
- [Contribution guidelines](CONTRIBUTING.md#before-you-start)

### Sponsoring Wave ❤️

If Wave Terminal is useful to you or your company, consider sponsoring development.

Sponsorship helps support the time spent building and maintaining the project.

- https://github.com/sponsors/wavetermdev

## License

Wave Terminal is licensed under the Apache-2.0 License. For more information on our dependencies, see [here](./ACKNOWLEDGEMENTS.md).
