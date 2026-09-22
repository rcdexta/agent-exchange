# Using Agent Exchange

Install the native harnesses you want to use and sign in normally. [Install the prebuilt AX binary](../AGENTS.md) on macOS, Linux, or inside WSL 2. Building from source is optional.

## Launch and resume

```sh
ax claude --name api
ax codex --name web
ax grok --name reviewer
ax opencode --name editor
ax pi --name worker
```

Names are shared across repositories on your machine, for your OS user. Choose a unique name for each conversation. Ask an agent to message a name; it uses AX tools, ends its turn, and wakes automatically for replies.

AX consumes the name option, `--name` or `-n`, and forwards every other argument to the harness. A harness that displays its own session name receives the AX name too. Native resume syntax still works:

```sh
ax claude --name api -r "session-name"
ax codex --name web resume "session-name"
ax grok --name reviewer -r "session-name"
ax opencode --name editor -s SESSION_ID
ax pi --name worker -r
```

Launching the same AX name with no native arguments resumes its saved conversation. Supplying native arguments leaves selection to the native harness. AX rejects a different conversation under an already bound name. Close the old terminal before adopting its conversation through AX.

Models, native prompts, and native options are forwarded. AX adds configuration to the launched process and does not rewrite global harness configuration or conversation transcripts. A native option being forwarded does not imply every native mode supports messaging.

## Launch a peer in another pane

The `spawn_agent` tool and `ax spawn` command can open a named peer in the caller's tmux or iTerm2 session. Agents may use this only when you explicitly request a new agent. A task assignment or an opportunity to parallelize does not authorize a launch. See [pane launches and their limits](spawning.md).

## Permissions and delegation

Launching AX establishes a standing policy: your local agents may carry out tasks you delegate through another AX agent, including an explicitly requested GitHub review submission. They do not ask you to authorize that same task again merely because it arrived through AX. The task's scope and the recipient's native sandbox and tool approvals still apply. Quoted documents and external content do not expand that authority.

AX authorizes its own messaging tools. Grok's adapter answers ordinary permission requests for those exact tools once, without storing grants or approving other tools. Launching a new agent retains the harness's normal tool approval; the spawn tool is excluded from AX's automatic grants. A hook that explicitly requires confirmation is preserved.

Unknown permission modes and unapproved bypass modes hold mail. An explicit bypass launch through Codex or Grok also opts that endpoint into AX messaging. `AX_ALLOW_BYPASS=1` provides the same opt-in for an intentionally configured endpoint; it does not change native permissions. Codex applies its explicit bypass choice through its private native session API, including resume.

## Inspect and recover

```sh
ax agents
ax doctor
ax status MESSAGE_ID
ax policy api hold
ax policy api accept
ax resolve MESSAGE_ID abandon
```

For missing peers or Claude Channels warnings, run `ax doctor` in each affected terminal. It reports the runtime path, fresh broker state, and local Claude eligibility checks without changing privacy settings. See [connection diagnostics](doctor.md).

Closing a terminal makes its endpoint offline. Stored mail waits for its next launch, subject to expiration. Queued, accepted by the host, fetched, and acknowledged are separate delivery states. Acknowledgment proves receipt, not completion of the delegated task.

FIFO orders native handoffs. Once a host accepts a message, later mail can proceed even while the agent works on the first task. A genuinely uncertain handoff blocks later mail and is never automatically repeated. Inspect it before using `resolve` to abandon it. Abandonment releases the queue without claiming delivery or canceling work already accepted by the host.

State lives in a private `.ax` directory under your home directory. `AX_HOME` selects another private directory, useful for isolated tests. Terminal message records are retained for seven days. Unresolved records remain available for inspection.

## Integration limits

- Claude uses an experimental native Channel and asks for development Channel confirmation at launch. Organization policies may disable Channels.
- Codex uses its native app server on a private socket. AX owns that local backend connection.
- Grok uses its native leader on a private socket. Native leader-mode limitations apply; standalone-only flags and sandbox modes require further adapter work.
- OpenCode uses its full TUI plugin API. Its pure and mini modes do not load this adapter. An existing `OPENCODE_TUI_CONFIG` override must currently contain JSON.
- Pi uses a native extension and keeps its terminal independent of the messaging helper. Pi pane spawning is not supported yet. See [Pi setup and limits](pi.md).
- Live harness verification targets macOS. Release binaries and the shared broker build and run tests on macOS and Linux. Windows users run AX and their harnesses inside WSL 2. Native Windows and remote agent transport are not supported.

Public binaries are available from [GitHub Releases](https://github.com/summationai/agent-exchange/releases/latest). See the [verification record](verification.md) for tested harness versions and current evidence.
