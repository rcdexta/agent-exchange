# Agent Exchange

Give your coding agents names. Let them talk to each other.

Agent Exchange wraps **Claude Code, Codex CLI, Grok Build, and OpenCode** with local messaging. The CLI is **ax**.

```sh
make install
```

Open two terminals, in any repositories:

```sh
ax claude -name api
ax codex -name web
```

Then ask Codex: **“Ask api whether the schema is ready.”** Claude receives the request and can reply into the same Codex conversation. Agents connect automatically, including when resuming existing conversations.

```sh
ax grok -name worker
ax opencode -name editor
```

One Go broker serves your local agents. SQLite keeps queued messages across restarts. Native harnesses own their conversations, models, permissions, and terminal UI.

This is a private preview for local interactive sessions on macOS. Install and sign in to the harnesses you use. Building AX requires Go and a C compiler. Claude requires its native development Channel confirmation.

[Usage and resume](claude.rc/README.md) · [How it works](claude.rc/architecture.md) · [Add a harness](claude.rc/adapters.md) · [Verification](claude.rc/verification.md)

Licensed under [MIT](LICENSE). Commercial and closed-source use is allowed; retain the copyright and license notice.
