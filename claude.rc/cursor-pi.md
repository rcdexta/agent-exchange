# Pi and Cursor adapter status

This work is in development. It is not included in a released AX binary.

## Pi

The development command is `ax pi -name worker`. AX consumes its name option and forwards the remaining arguments to Pi. Pi's native session picker, explicit session selection, and prompts retain their native syntax. Launching an existing AX name without other arguments selects the saved Pi session file, including when launched from another repository. An AX name remains bound to one conversation; use a new AX name for a different conversation.

AX writes a private, versioned extension and replaces its own process with Pi. Pi owns its terminal and starts an optional messaging child. The child runs the shared AX bridge and connects to the same broker as the other harnesses. Killing that child removes messaging temporarily; it does not own Pi's terminal or kill Pi. A bounded retry reconnects it.

The extension registers six tools with the `ax_` prefix. It connects automatically without a model setup turn. Peer bodies enter through Pi's custom-message API: idle sessions wake immediately, while busy sessions receive a follow-up after their current work finishes. AX adds no permission bypass or forced tool activation. Pi's extensions and execution environment keep control over native tools.

Contract tests cover argument forwarding, saved-session selection, native process replacement and name-lock lifetime, message routing, busy delivery, child-process failure and retry, and immutable identity. The Pi host API and bridge responses are simulated in these tests; schema construction is stubbed. They are not evidence of a live model exchange. Real Pi loading, authentication, two-way exchange with another harness, broker restart, and resume still require live verification.

This session's sandbox rejects Unix-socket listeners, so the full broker integration suite could not run. The native session-file persistence test is included for an environment that permits those listeners.

This draft also needs session-switch and fork lifecycle coverage across Pi versions, connection-generation checks after asynchronous bridge calls, and the MCP initialized notification. It is being preserved as unfinished work, not proposed for release.

Primary API references: [Pi extensions](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/extensions.md), [Pi extension loader](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/src/core/extensions/loader.ts).

## Cursor and hcom

Inspected hcom commit `fabb309b57cb33b39c69b773cd759723a10e94b5`.

hcom launches Cursor inside a pseudo-terminal it owns. A notification-driven delivery loop checks that Cursor is idle, its input is empty, the user is not typing, and no approval prompt is visible. It injects the fixed `<hcom>` trigger, verifies that it rendered, and sends Enter. Native hooks supply the full queued message through additional context or a completion follow-up. It verifies hook-side consumption separately from successful terminal writes.

This is terminal injection, not a native Cursor API for waking an existing idle TUI. The hcom proxy forwards termination signals to its child and owns the controlling terminal. Its source explicitly closes the child's copy of the terminal master so terminal teardown delivers a hangup. Copying this ownership model would preserve the crash coupling the AX redesign is intended to remove.

Cursor's installed CLI has native lifecycle hooks, but no verified API for waking the installed native TUI while idle. The chosen approach is to use native hooks and deliver pending messages during subsequent prompts or completion hooks, preserving crash isolation. No Cursor adapter is enabled by this change.

Sources: [Cursor launch](https://github.com/aannoo/hcom/blob/fabb309b57cb33b39c69b773cd759723a10e94b5/src/tools/cursor_preprocessing.rs), [delivery state machine](https://github.com/aannoo/hcom/blob/fabb309b57cb33b39c69b773cd759723a10e94b5/src/delivery.rs#L1880), [Cursor hooks](https://github.com/aannoo/hcom/blob/fabb309b57cb33b39c69b773cd759723a10e94b5/src/hooks/cursor.rs), [terminal ownership](https://github.com/aannoo/hcom/blob/fabb309b57cb33b39c69b773cd759723a10e94b5/src/pty/mod.rs#L639).
