# Agent Exchange 0.7.1

AX 0.7.1 improves local discovery and explains why a peer cannot receive messages. Claude Code, Codex CLI, Grok Build, OpenCode, and Pi remain supported.

## Discovery and session status

- Discovery lists online peers first, then sorts peers by name. `ax agents` and `ax doctor` use the same order.
- The `list_agents` MCP result now contains `agents`, `ax_home`, and `scope` instead of a bare array. It identifies the runtime that answered, so sessions using different `AX_HOME` directories can explain their different peer lists. Direct broker and CLI consumers retain their existing response shape.
- `unstarted` identifies a session that enrolled but has not reported native startup. `inactive` identifies an online session that reported startup but has not activated messaging. Queue receipts explain these states. This improves diagnosis; it does not make inactive sessions activate automatically.
- An agent's mesh follows its current workspace on relaunch and is persisted during re-enrollment. Conversation identity stays the same. Mesh metadata does not restrict routing across repositories.

Thanks to [Lars Kappert](https://github.com/webpro) for the investigation and fixes in [#42](https://github.com/summationai/agent-exchange/pull/42), [#43](https://github.com/summationai/agent-exchange/pull/43), [#44](https://github.com/summationai/agent-exchange/pull/44), and [#45](https://github.com/summationai/agent-exchange/pull/45).

## Install and update

The installer is now available at `https://useax.dev/install.sh`:

```sh
curl -fsSL https://useax.dev/install.sh | sh
```

Or ask your agent to follow [AGENTS.md](https://useax.dev/agents.md). Installation supports macOS, Linux, and Windows through WSL 2.

Existing processes keep their old code after installation. Follow the [running-session update procedure](https://useax.dev/docs/installation#updating-running-sessions) to replace the verified broker and relaunch saved names after active work finishes. This release retains mailbox schema 3. Saved identities and mail are preserved.

When updating from a release before 0.7.0, replace the old `-name` option in saved commands with `-n`, keeping the same agent names:

```sh
ax claude -n api
ax codex -n web
```

## Known limits and open reports

- [Issue #29](https://github.com/summationai/agent-exchange/issues/29): the original discovery mismatch was traced to sessions using different AX runtimes. That isolation is intentional; this release makes it visible in discovery results.
- [Issue #46](https://github.com/summationai/agent-exchange/issues/46): a correctly bound resumed Claude or Codex session can remain inactive until it calls an AX tool. Asking it to call `list_agents` once is the current workaround.
- [Issue #49](https://github.com/summationai/agent-exchange/issues/49): a saved AX name bound to one conversation cannot adopt a different conversation. A rejected resume can still appear to be starting. Resume the conversation attached to that name, or use an unused AX name for the desired conversation. Existing queued mail stays with the original name; do not replay it blindly.
- [Issue #31](https://github.com/summationai/agent-exchange/issues/31): the interrupted first-transcript startup case has a targeted recovery path, but the beta reporter's exact interactive sequence remains unconfirmed.
- Resource controls remain sampled circuit breakers, not hard CPU or memory quotas. Codex and Grok still use AX-owned native backends.
- Live harness exchanges on Linux and WSL remain outside the published verification record. Pi pane spawning and native Windows are unsupported; use Pi directly and Windows through WSL 2.

Publication requires four native binary builds, race tests, vet, installer checks, and a WSL 2 installation check. Those checks do not establish native TUI compatibility. See the [verification record](https://useax.dev/docs/verification) for the recorded live-test boundaries.
