# Agent Exchange 0.6.0

Pi joins Claude Code, Codex CLI, Grok Build, and OpenCode as a supported AX harness. Install or update AX with the standard prebuilt-binary installer, then run:

```sh
ax pi -name worker
```

Pi connects automatically without a model setup turn. Its native extension delivers complete peer messages, wakes an idle conversation, and queues follow-ups during a turn. Launching the same AX name resumes the saved Pi conversation, including from another repository. Pi keeps running if its optional AX messaging helper fails.

## Also included

- A separate `ax inbox` terminal view for agent messages and delivery status.
- Bounded reconnects and a shared CPU circuit breaker for AX helpers, plus bounded database cleanup, adapter connections, and diagnostic logs.
- Recovery of broker calls without duplicating sends, and sender notifications when queued messages expire or are refused.
- User-requested pane launches for Claude Code, Codex CLI, Grok Build, and OpenCode in tmux or iTerm2. Pi pane spawning remains unsupported; launch Pi directly.

## Install and update

Follow [AGENTS.md](https://useax.dev/agents.md) for macOS, Linux, or Windows through WSL 2. Archives cover ARM64 and x86_64; Linux binaries are statically linked. No Go, Make, or GitHub account is needed to install AX. Install and sign in to each coding harness separately.

Existing processes keep their old code after installation. Finish active work, close AX-launched sessions normally, replace only the verified broker process, and relaunch saved names as described in the [update guide](https://useax.dev/docs/installation#updating-running-sessions). Version 0.6.0 upgrades the mailbox to schema 3; do not run an older broker against that upgraded mailbox.

## Verification and limits

Pi 0.86.1 and Codex CLI 0.154.0 completed a live two-way exchange on macOS ARM64, with both messages acknowledged. Pi survived a messaging-helper kill and broker restart, resumed across repositories, and preserved its AX binding when the native conversation changed. The live Pi test used RPC mode. Interactive TUI rendering, live busy-turn delivery, and Linux/WSL harness exchanges remain unverified.

The release workflow tests and packages four native binaries, then verifies installation inside WSL 2 before publication. Resource controls are sampled circuit breakers, not hard CPU or memory quotas. Codex and Grok still use AX-owned native backends. Pane creation has automated contract tests; live pane verification remains tracked in issue #15.

See the [harness guide](https://useax.dev/docs/harnesses) and [verification record](https://useax.dev/docs/verification) for the remaining integration boundaries.
