# Agent Exchange

Give your coding agents names. Let them talk to each other.

**Agent Exchange** connects Claude Code, Codex CLI, Grok Build, and OpenCode on your machine. The CLI is **ax**. Keep each harness's terminal UI, model, permissions, and existing conversations.

## Install

Prebuilt binaries are available for macOS and Linux on x86_64 and ARM64. On Windows, use the Linux installer inside WSL 2. Go and Make are not required.

Ask your coding agent:

> Read https://raw.githubusercontent.com/rcdexta/agent-exchange/main/agents.md and install Agent Exchange.

The [installation guide](agents.md) contains the install command, verification steps, updates, and uninstall instructions. You can also follow it yourself. No GitHub account is required. Install and sign in to the coding harnesses you want to use separately.

| Platform | Installation |
| --- | --- |
| macOS 13 or later | Native binary, Apple Silicon or Intel |
| Linux | Static binary, ARM64 or x86_64 |
| Windows | Inside WSL 2, with AX and the harnesses in the same distribution |

[Installation, updates, and uninstalling](agents.md)

The inbox and resource protection described below are unreleased. The installer downloads the latest published release.

## Try it

Open two terminals, in any repositories:

```sh
ax claude -name api
ax codex -name web
```

Ask Codex: **“Ask api whether the schema is ready.”** Claude receives the request and can reply into the same Codex conversation. Agents connect automatically. Names work across repositories on your machine, for your OS user.

Open `ax inbox` in a third terminal or a terminal split to watch messages separately. Use `ax inbox api` to filter messages to and from one agent. The read-only view shows delivery status and message bodies; it does not acknowledge messages or control agent sessions. [Inbox controls and behavior](claude.rc/inbox.md).

Grok and OpenCode join the same exchange:

```sh
ax grok -name worker
ax opencode -name editor
```

AX consumes the name option and forwards other arguments to the native harness. Resume an existing conversation with the native syntax, or launch the same AX name with no additional arguments to resume its saved conversation.

```sh
ax claude -name api -r "session-name"
ax codex -name web resume "session-name"
```

## Supported coding harnesses

| Harness | Integration | Current boundary |
| --- | --- | --- |
| Claude Code | MCP tools and native Channel | Development Channel confirmation at launch |
| Codex CLI | MCP tools and private native app server | Requires native queue support |
| Grok Build | MCP tools and private native leader | Native leader-mode limitations apply |
| OpenCode | MCP tools and per-launch TUI plugin | Full TUI; pure and mini modes are unsupported |

All four have completed live message exchanges on macOS, including resumed conversations. Release binaries and the broker have automated platform checks; live harness verification on Linux and WSL is still pending. [Tested versions and evidence](claude.rc/verification.md).

## Add another harness

The broker, durable mailbox, MCP tools, and delegation policy are shared. A new adapter connects the harness's native launch, session selection, and message-wake APIs to that core.

Today, contributors add a registry entry and native preparation function, wire the command into CLI dispatch and doctor, and test launch, resume, delivery, and permission behavior. OpenCode demonstrates the native-plugin route; Grok demonstrates a native-server route. See the [adapter guide](claude.rc/adapters.md) for the contract and implementation checklist.

Adding an adapter currently requires an AX source change and rebuild. Independently installable adapters, scaffolding, and a shared compatibility runner are planned; they are not available as CLI commands yet.

## How it works

One Go broker serves your local agents. SQLite keeps queued messages across restarts. Agents send through AX tools, finish their turn, and wake for replies. Queued, accepted by the harness, and acknowledged are distinct delivery states.

Tasks delegated through your AX agents retain their scope and the recipient's native permission controls. AX configures each launched process without rewriting global harness configuration or conversation transcripts.

[Usage and resume](claude.rc/README.md) · [Architecture](claude.rc/architecture.md) · [Verification](claude.rc/verification.md)

## Resource protection

The broker, bridges, lifecycle hooks, and inbox share a CPU budget. After an initial ten-second observation period, five CPU seconds across these helpers in a ten-second window pauses messaging for sixty seconds. Native coding sessions keep running during this pause; queued messages remain in SQLite.

Run `ax doctor` to see whether messaging is paused and when the cooldown ends. Bridges normally keep their MCP connection open and reconnect to the broker after cooldown. A standalone helper that continues consuming sustained CPU can terminate itself; its harness may then require the MCP connection to be reconnected.

This is a sampled circuit breaker, not a hard CPU quota. Native harnesses and AX launchers that own native backends are outside the budget. Complete crash isolation is still pending for the Codex and Grok native backend integrations. Database cleanup, diagnostic logs, and adapter connections also have bounded work, but AX does not enforce a hard memory or total database size limit. [Protection behavior and limits](claude.rc/resource-safety.md).

Updating the binary preserves saved conversations and mail. Already-running processes keep their old code, so new protections apply only after those helpers are replaced. Follow the [update guide](agents.md); replacing the executable alone does not upgrade a running broker or bridge.

Licensed under [MIT](LICENSE). Commercial and closed-source use is allowed; retain the copyright and license notice.
