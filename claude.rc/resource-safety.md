# Resource protection

AX treats messaging as optional. A resource pause must preserve the native coding process and any Codex or Grok backend serving its terminal.

## CPU budget

New broker, bridge, hook, and inbox processes share CPU accounting inside their private `AX_HOME`. Long-lived helpers sample their own user and system CPU every two seconds; hooks report when they finish. Accounting uses ten one-second buckets. After an initial ten-second observation period, five CPU seconds in the rolling window starts a sixty-second messaging cooldown.

The budget file has a fixed-size record and is replaced atomically. A nonblocking file lock coordinates writers. Long-lived helpers carry an unsent sample forward when another writer holds the lock; a short-lived hook's final report is best effort. There is no process-wide scan or registry of PIDs to signal.

During cooldown:

- The broker shuts down, preserving SQLite mail and receipts.
- Bridges close the broker connection, keep their MCP interface available, and return a clear paused status for tool calls. They wait before attempting to reconnect or start a broker.
- Native lifecycle hooks return without interfering with a permission prompt or tool operation.
- The inbox exits through its normal terminal cleanup.
- A standalone broker or bridge that continues consuming CPU after being paused can terminate itself. It never signals its parent, a process group, or a native backend. A harness may require its MCP connection to be reconnected after this fallback.

The persisted cooldown prevents newly started helpers from immediately restarting the same work. `ax doctor` reports the cooldown and configured protection. Existing processes from older binaries need to be replaced before their CPU is included; replacing the installed executable alone does not retrofit running helpers.

This is a sampled circuit breaker, not an OS-enforced percentage cap. It permits bursts, depends on the monitor being scheduled and completing its accounting, and cannot guarantee stopping every runtime or kernel failure. One Go execution slot per helper further limits parallel Go work without changing the native harness's environment. Native harness CPU and AX launchers that own native backends are outside the helper budget. AX does not install a cgroup or change native process priorities; doctor explicitly reports that no OS CPU quota is configured.

## Bounded work and storage

- Heartbeat, ping, list, status, and inbox reads do not trigger mailbox dispatch.
- Retention processes at most 256 old terminal messages per minute. Pending or uncertain handoffs are preserved. A large backlog can take longer than seven days to clear.
- Indexes cover pending FIFO heads, receipt history, and terminal retention. Unchanged lifecycle state does not rewrite the agent row; permission and blocked-state transitions still persist.
- Startup is serialized across callers by a separate startup lock. A held broker lifetime lock prevents another broker being spawned for an unavailable socket. Lock files keep their inodes.
- Failed bridge retries back off from one to fifteen seconds. A successful heartbeat resets the delay.
- Broker, Codex, and Grok diagnostic logs are trimmed above 8 MiB on startup and every five seconds. Trimming keeps the original inode and ordinary file descriptors, so a launcher exiting does not break a backend's output pipe. This is a periodic threshold, not a hard disk quota: logs can exceed it between checks or if the watcher is unavailable. Native conversation transcripts are untouched.
- Adapter HTTP connections are limited to sixteen, with eight admitted wake requests. Header and idle timeouts are separate from the fifteen-second wake context. Normal callers also cancel failed requests. Native adapter functions must honor that context; synchronous native I/O is not forcibly interrupted by it.
- Idle OpenCode long polls return after thirty seconds and reconnect without creating a model turn.
- Grok and the Codex permission proxy accept at most eight simultaneous connections. Grok wake tracking is capped at eight and pending session-selection tracking at sixty-four. Capacity does not close an already active TUI connection.

The existing open-mail count and byte bounds do not impose a hard total size limit on retained SQLite history, WAL files, or process RSS. Native transport ownership and complete crash isolation remain separate architectural work.

## Verification

The regression suite uses injected CPU samples, a subprocess reading persisted cooldown state, SQLite fixtures, in-memory connections, cancelled HTTP handlers, and a mock native process whose output survives closure of the launcher's log descriptor. It does not generate a real CPU storm or target running coding sessions.

Before installing a build, run the full race-enabled Go suite, vet, and the normal build in an environment permitting local Unix sockets. Verify idle CPU and message delivery in a separate `AX_HOME`; do not stress the user's live broker. Report socket integration tests as blocked when the execution environment forbids listeners, rather than treating a partial suite as full verification.
