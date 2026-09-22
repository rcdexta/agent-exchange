# Diagnose an AX connection

Run `ax doctor` in the terminal and repository where you launch the affected agent. Compare its output with the same command in a working terminal.

Doctor shows the installed AX version, resolved AX home and broker socket, resource protection status, installed harness versions, and a fresh agent list from the running broker. Inside an AX session, it also shows the calling session's name and identity when available. It does not start or restart the broker or coding sessions.

Different `AX_HOME` directories intentionally use different brokers. Agents using the same home can discover each other across repositories and harnesses. If the broker is unreachable, launch a named session or run `ax agents` to start it. If doctor sees peers that an agent does not, ask the agent to call its AX discovery tool again and compare the fresh result. Include the runtime paths and agent states in a bug report; do not share session JSON files, which contain credentials.

## Claude reports that Channels are unavailable

Doctor checks shell settings that disable Claude's feature-flag fetching:

- `DO_NOT_TRACK` and `DISABLE_GROWTHBOOK` are enabled by `1` or `true`, including uppercase variants. `0` and `false` leave them off.
- `DISABLE_TELEMETRY` and `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` are enabled by any nonempty value, including `0` and `false`.

These settings can prevent Claude Channels from becoming available. Review your privacy preference before changing one. AX does not unset them or enable telemetry for you. This check concerns Claude; a working Codex connection does not establish Claude's Channels availability.

Doctor also reads selected fields from `claude auth status`. An effective `analyticsDisabled=true` can reveal privacy or provider settings even when the shell variables are absent. Claude settings can supply environment values, so check those too. Unsupported status output or a failed probe is reported as unknown. Doctor does not print account identifiers, credentials, or the raw authentication response.

Local checks cannot prove that Channels are available. Team and Enterprise owners must enable Channels under Claude.ai → Admin settings → Claude Code → Channels. First-party Anthropic authentication is required; third-party providers such as Bedrock, Vertex, and Foundry do not support Channels. Launch Claude through AX for the session opt-in. MCP tools can connect while Channels delivery remains unavailable.

See Claude's [feature-flag settings](https://code.claude.com/docs/en/env-vars#features-that-need-feature-flag-fetching) and [Channels requirements](https://code.claude.com/docs/en/channels).

Each harness probe has a three-second deadline and a 16 KiB limit on each output stream. A failed probe leaves the remaining checks available. Paths and agent names may identify your local projects; review the output before sharing it publicly.
