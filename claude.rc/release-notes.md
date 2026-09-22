# Agent Exchange 0.7.0

AX 0.7.0 adds pending-message recovery, clearer delivery receipts, connection diagnostics, and recovery for a Claude session that exits before saving its first transcript. Claude Code, Codex CLI, Grok Build, OpenCode, and Pi remain supported.

## Update saved launch commands

Replace the old `-name` option with `-n` in scripts and saved commands:

```sh
ax claude -n api
ax codex -n web
```

Keep the same agent names to preserve their identities and saved conversations. AX also passes the name to a harness that supports displaying it. Other native arguments retain their existing behavior.

## Delivery and recovery

- A complete broker response is preserved when the connection closes immediately afterward, avoiding false transport failures.
- The `list_pending` tool lists a recipient's unacknowledged incoming mail, with bounded pages and delivery state. Listing does not acknowledge or replay a task.
- Send and reply receipts include recipient readiness and delivery evidence. Queued, handed off, acknowledged, and completed remain separate states; AX does not infer task completion.
- Send and reply accept `ttl_seconds` from 1 to 604800. The default remains twelve hours. Expiry affects queued delivery and does not cancel work already handed off.

See the [messaging recovery guide](https://github.com/summationai/agent-exchange/blob/main/claude.rc/recovery.md).

## Startup and diagnostics

Claude can emit SessionStart before creating a transcript. AX now records that known-new startup and reuses the same conversation ID while the native-provided path remains absent. Once the path exists, AX uses normal resume. It does not read or modify transcript contents, replace the AX identity, or migrate legacy bindings automatically.

`ax doctor` reports runtime paths, session identity, and fresh broker discovery. Bounded probes check relevant Claude privacy and authentication settings without printing credentials, changing privacy preferences, or restarting sessions. See the [diagnostic guide](https://github.com/summationai/agent-exchange/blob/main/claude.rc/doctor.md).

## Install and update

Follow [AGENTS.md](https://useax.dev/agents.md) for macOS, Linux, or Windows through WSL 2. The repository and downloads now use `summationai/agent-exchange`.

Existing processes keep their old code after installation. Follow the [running-session update procedure](https://useax.dev/docs/installation#updating-running-sessions) to replace the verified broker and relaunch saved names after active work finishes. This release retains mailbox schema 3, introduced in 0.6.1. Saved identities and mail are preserved.

## Known limits and open reports

- [Issue #29](https://github.com/summationai/agent-exchange/issues/29): the reported discovery mismatch with two Claude and two Codex sessions remains unexplained. Four-peer broker and MCP fixture checks pass, but do not reproduce the native TUI report.
- [Issue #31](https://github.com/summationai/agent-exchange/issues/31): the interrupted-startup case is fixed and reproduced with Claude Code 2.1.278 initialization-only mode. The beta reporter's exact interactive sequence remains unconfirmed. Existing files and older saved AX names retain normal resume behavior.
- Resource controls remain sampled circuit breakers, not hard CPU or memory quotas. Codex and Grok still use AX-owned native backends.
- Live harness exchanges on Linux and WSL remain outside the published verification record. Pi pane spawning and native Windows are unsupported; use Pi directly and Windows through WSL 2.

Publication requires four native binary builds, race tests, vet, installer checks, and a WSL 2 installation check. Those checks do not establish native TUI compatibility. See the [verification record](https://useax.dev/docs/verification) for the recorded live-test boundaries.
