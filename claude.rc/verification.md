# Verification

Private preview, September 19, 2026. Live host tests run on macOS arm64 in an isolated AX state directory.

| Harness | Version | Native integration |
| --- | --- | --- |
| Claude Code | 2.1.278 | MCP and native development Channel |
| Codex CLI | 0.154.0 | MCP and private native app server |
| Grok Build | 1.0.34 | MCP and private native leader |
| OpenCode | 1.18.31 | MCP and per-launch TUI plugin |

## Live evidence

Grok and OpenCode automatically connected, discovered each other, and completed a round trip containing `AX_GROK_OPENCODE_OK`. Both records reached `acknowledged`. The same named conversations were closed and resumed with unchanged native IDs; queued mail then reached the resumed recipient without replaying accepted messages.

Codex and Grok completed another round trip containing `AX_CODEX_GROK_OK`, with both messages acknowledged. Codex used its native read-only mode and Grok used its default permission mode. Grok's AX tools worked without approving unrelated tools or changing global permissions.

OpenCode and Claude completed a round trip containing `AX_CLAUDE_OPENCODE_OK`, with both messages acknowledged. Grok then sent two separate requests to OpenCode without waiting; both replies returned and all four records were acknowledged. Ten messages across these tests reached acknowledgment. Recent native handoffs completed in 0–24 milliseconds; model responses took additional seconds.

OpenCode's idle listener remained connected beyond the native fetch client's default five-minute timeout after the adapter disabled that timeout. Its later cross-harness reply still arrived automatically.

All four hosts reconnected after restarting only the isolated broker. OpenCode and Grok then completed another round trip containing `AX_FINAL_RESTART_OK`, bringing the total to twelve acknowledged messages. A resumed OpenCode session also loaded an existing relative plugin from nested TUI configuration alongside AX.

The preceding Claude/Codex implementation has live evidence for two-way delivery, cross-directory discovery, adoption of existing conversations, immutable bindings, broker restart, scoped delegation, and explicit Codex bypass resume. The new preview keeps those paths and their regression coverage.

## Broker repair

An overnight log inspection found later messages blocked behind a request Codex had already fetched but never acknowledged. FIFO now advances after confirmed native handoff or content fetch, independently of task completion. The live broker released the waiting messages and preserved the fetched request without replay. Recovery also recognizes stored confirmation receipts that older brokers downgraded during disconnect.

## Automated checks

The race-enabled Go suite and vet pass locally. Focused tests cover immutable adapter binding, native configuration passthrough, Grok queue acceptance versus socket write, the six-tool permission boundary, confirmed handoff recovery, and genuinely uncertain delivery barriers. CI runs the Go suite, vet, and build on macOS and Linux.

The initial repository scope passed autoreview after fixing preservation of nested OpenCode TUI plugins and their relative paths.

Live Linux harnesses, Windows, remote transport, every native subcommand, and all host policy configurations are outside the verified scope. Native acceptance is distinct from a model acknowledgment and from successful task completion.
