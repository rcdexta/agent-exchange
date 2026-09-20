# Agent Exchange

Give your coding agents names. Let them talk to each other.

Agent Exchange (`ax`) connects Claude Code, Codex CLI, Grok Build, and OpenCode on your machine. Each agent keeps its harness's terminal UI, model, permissions, and existing conversations.

## Install

Ask your coding agent:

> Read https://raw.githubusercontent.com/rcdexta/agent-exchange/main/AGENTS.md and install Agent Exchange.

You can also follow [AGENTS.md](AGENTS.md) yourself. It covers installation, verification, updates, and removal. AX uses prebuilt binaries, so you do not need Go, Make, or a GitHub account.

- **macOS 13 or later:** native binary for Apple Silicon or Intel.
- **Linux:** static binary for ARM64 or x86_64.
- **Windows:** use WSL 2, with AX and the coding harnesses in the same Linux distribution.

Install and sign in to your coding harnesses separately.

## Try it

Open two terminals, in any repositories:

```sh
ax claude -name api
ax codex -name web
```

Ask Codex: **“Ask api whether the schema is ready.”** Claude receives the request and can reply into the same Codex conversation. Both agents connect automatically. Names work across repositories on the same machine for the same OS user.

Grok and OpenCode join the same exchange:

```sh
ax grok -name worker
ax opencode -name editor
```

## Resume a conversation

AX reads the name option and passes other arguments to the native harness. Use the harness's resume syntax to continue an existing conversation:

```sh
ax claude -name api -r "session-name"
ax codex -name web resume "session-name"
```

Launching the same AX name with no additional arguments resumes its saved conversation. See [usage and resume](claude.rc/README.md) for details.

## Supported coding harnesses

- **Claude Code:** MCP tools and a native Channel. Requires development Channel confirmation at launch.
- **Codex CLI:** MCP tools and a private native app server. Requires native queue support.
- **Grok Build:** MCP tools and a private native leader. Native leader-mode limitations apply.
- **OpenCode:** MCP tools and a TUI plugin loaded for each launch. Supports the full TUI; pure and mini modes are unsupported.

All four have completed live message exchanges on macOS, including resumed conversations. Automated checks cover release binaries and the broker across platforms. Live harness exchanges on Linux and WSL remain unverified. See [tested versions and evidence](claude.rc/verification.md).

## Add another harness

A new adapter connects a harness's launch, session selection, and message-wake APIs to AX. It reuses the broker, durable mailbox, MCP tools, and delegation policy.

Start with the [adapter guide](claude.rc/adapters.md). Add a registry entry and a native preparation function, connect the command to CLI dispatch and `ax doctor`, then test launch, resume, delivery, and permissions. OpenCode is the example for a native plugin; Grok is the example for a native server.

Adapters currently require a source change and rebuild. Installable adapter packages, scaffolding, and a shared compatibility runner are planned but not yet available.

## How it works

One Go broker serves the local agents. SQLite keeps queued messages across restarts. Agents send through AX tools, finish their turn, and wake for replies. A queued message, a message accepted by the harness, and an acknowledged message are distinct delivery states.

Delegated tasks retain their scope and the receiving agent's native permission controls. AX configures each launched process without rewriting global harness configuration or conversation transcripts.

See the [architecture](claude.rc/architecture.md) for the broker and delivery model.

Licensed under [MIT](LICENSE). Commercial and closed-source use is allowed; retain the copyright and license notice.
