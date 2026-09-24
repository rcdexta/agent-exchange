# Pi

Pi joins the same local exchange as Claude Code, Codex CLI, Grok Build, and OpenCode. AX loads a native Pi extension for each launch, connects automatically, and adds tools with the `ax_` prefix. Connecting does not spend a model turn.

Pi support is included in AX 0.6.1. The integration is tested with [Pi 0.86.1](https://github.com/earendil-works/pi). Older Pi versions are unverified.

## Start a Pi session

Install and sign in to Pi separately, using the current `@earendil-works/pi-coding-agent` package. Check that `pi` is on your PATH.

[Install or update AX](../AGENTS.md), then launch a named Pi session:

```sh
ax pi --name worker
```

In another terminal, start a named peer, for example `ax codex --name web`. Ask Pi: **“Ask web what it is working on.”** Both agents use the same broker and durable mailbox. Names work across repositories for the same OS user.

When the installed Pi version supports a native session display name, AX passes its name to Pi too. The same name is applied when resuming the saved conversation. Versions without that option still launch; AX keeps the messaging name without passing an unsupported argument.

Opening a Pi pane through `ax spawn` or `spawn_agent` is not supported yet. Start Pi in a terminal yourself. A Pi session can still launch other supported harnesses when you explicitly request it.

## Resume and switch conversations

AX consumes the name option and passes other arguments through to Pi. Use Pi’s native session picker or select a session file:

```sh
ax pi --name worker -r
ax pi --name worker --session /path/to/session.jsonl
```

Launching `ax pi --name worker` with no additional arguments resumes the saved Pi session file, even from another repository. Each AX name stays attached to one conversation. If you start a new conversation or fork inside Pi, AX messaging stops for that name; the native Pi session continues. Launch with a new AX name to connect the new conversation.

## Delivery and recovery

Peer messages arrive through Pi’s custom-message API with the complete body. Idle sessions start a turn; busy sessions receive a follow-up after their current turn. Replies and acknowledgments use the shared AX tools. Pi’s tool selection, extensions, and execution environment retain control over native actions. AX adds no permission bypass or forced tool activation.

AX replaces its launcher process with Pi. The extension starts an optional `ax bridge` child and reconnects with bounded backoff when that child fails. Pi owns its terminal and remains alive during messaging failures. AX does not rewrite Pi’s global configuration or transcripts.

A `channel_written` receipt means AX wrote to the extension’s pipe. Pi’s message API does not return an acceptance confirmation, so this receipt does not prove a model read the message. A reply or acknowledgment is separate evidence of receipt.

## Verified behavior

Pi 0.86.1 and Codex CLI 0.154.0 completed a live two-way exchange on macOS ARM64. Both messages reached `acknowledged`. Pi connected without a setup turn, survived a killed bridge and an isolated broker restart, and resumed the same conversation from another repository. Switching to a new Pi conversation preserved the original AX binding.

Contract tests cover busy follow-ups, reloads during an outstanding connection attempt, new and fork lifecycle events, stale connection rejection, argument forwarding, and the native process’s name lock. The live test used Pi’s RPC mode; interactive TUI rendering, live busy-turn delivery, and Linux/WSL exchanges remain unverified. See the [verification record](verification.md#pi).

The integration uses Pi’s [native extension API](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md).
