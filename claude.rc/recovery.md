# Messaging recovery

Tracks [issue #4](https://github.com/rcdexta/agent-exchange/issues/4).

Each MCP tool call has a ten-second budget for obtaining a broker connection,
binding the session, confirming readiness and executing its operation. An attempt
gets at most four seconds within that shared budget. After a transport failure,
the bridge retires that connection and can try once more through the existing
reconnect loop. Waiting for a replacement uses a notification, not a polling loop.
Broker rejections are returned without automatic retry.

Send and reply keep the same `client_message_id` across both attempts. Success
and failure results expose the key so a later tool call can reuse it. Retrying
with the same key and unchanged arguments returns the original durable message.

Failures include structured fields in addition to their JSON text representation:

- `retryable`: whether recovering the transport or waiting for resource cooldown
  can make a later attempt useful.
- `submission`: `not_submitted`, `outcome_unknown`, or `not_applicable` for reads.
- `client_message_id`: the key to preserve when retrying a send or reply.
- `connection_state`: `reconnecting` for transport failures.
- `retry_after_ms`: the remaining resource cooldown, when applicable.

`outcome_unknown` means the request may have been stored despite the missing
response. It does not mean delivery failed. A broker error after submission is
also treated conservatively because it can report a commit failure.

These changes do not restart native harnesses or replace the running AX binary.
Pending-request discovery and sender expiry notifications are tracked separately
in [#5](https://github.com/rcdexta/agent-exchange/issues/5) and
[#3](https://github.com/rcdexta/agent-exchange/issues/3).
