# Adding a harness

Add one entry to `harnesses` in `internal/ax/adapters.go`, plus a preparation function for the native harness. The broker does not need a new implementation or a host-specific message format.

An adapter supplies the native tool-name prefix, session-ID validator, and a preparation function. That function receives the original native arguments and environment and returns the launch arguments, environment, cleanup function, or an actionable error. Add the command to CLI dispatch and doctor output.

Use a native API, lifecycle hook, or trusted plugin to identify the conversation the user selected. `bindAdapter` rejects rebinding an AX name to a different conversation. Do not discover identity by searching transcripts or observing terminal text.

For a Go integration, `startAdapterHost` accepts a native wake callback. Return success only when the host confirms acceptance. Grok demonstrates this with ACP prompt IDs and native queue events; a socket write alone is insufficient.

For a native plugin, the same private per-launch Unix socket exposes:

| Operation | Contract |
| --- | --- |
| `POST /bind` | Report selected `native`, native `permission`, and `state` from the harness API. |
| `GET /next` | Wait for a fixed wake containing `id`, `native`, and `text`. |
| `POST /receipt` | Confirm the wake by `id`, or report an `error`. |

OpenCode's embedded JavaScript plugin demonstrates that path. Keep its long-lived connection alive without depending on a model turn. Verify the recipient's native ID before invoking the host API. Keep peer bodies in the shared MCP tools.

The model-facing tools are `list_agents`, `send_message`, `reply`, `get_message`, `ack_message`, and `delivery_status`. Reuse `ax bridge`; it filters model-supplied arguments and prevents forged identity, permissions, receipts, and policy changes.

Prove these behaviors before enabling an adapter: fresh launch and native resume, automatic readiness, unchanged prompts and settings, two-way delivery with another harness, native permission preservation, busy-session delivery, immutable identity, disconnect cleanup, and ambiguous handoff handling. Include a protocol-level test that rejects a false acceptance signal. Record the exact native version and live evidence in the verification document.
