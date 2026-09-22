# Delivery and recovery

Start by checking the agents and installed harnesses from a terminal:

```sh
ax agents
ax doctor
```

## An agent is missing

Confirm that the session was launched through AX with the name you expect. Names work across repositories, but the sessions must share the same OS user and AX state directory. `AX_HOME` can select a separate directory for testing; agents using different state directories do not share a broker. The `list_agents` result names the runtime it read, so an agent can compare `ax_home` without leaving its session.

On Windows, AX and both harnesses must run in the same WSL 2 distribution. If the harness is missing from doctor, install and sign in to that harness separately.

`ax agents` names the condition. `unstarted` means the session enrolled but its harness never reported it running; `inactive` means the harness is running but has not called an AX tool yet.

If tools are connected but the agent has not appeared yet, ask it to call AX’s `list_agents` tool once. Do not leave it polling for another agent to join.

## A message is queued

Queued means the broker stored the message. It does not mean the recipient has read it or completed the task. An offline recipient can receive stored mail when its named session returns, subject to expiration.

Inspect a specific message using the ID returned by AX:

```sh
ax status MESSAGE_ID
```

Delivery evidence progresses from queued to native handoff, content fetch, and acknowledgment. An acknowledgment records receipt. Ask for an explicit result when you need proof that a review, test, or other task finished.

AX 0.6.1 notifies the sender when queued mail expires or is refused. These notifications report delivery failure; they do not resend the task.

## The message is held by policy

An unknown permission mode or an unapproved bypass mode can hold mail. The receiving harness’s permissions still apply to delegated work. Inspect the launch settings before changing policy.

The terminal owner can deliberately hold or accept incoming mail:

```sh
ax policy api hold
ax policy api accept
```

These commands change AX’s incoming-mail policy; they do not grant native tool permissions or disable a sandbox. See [permissions and delegation](README.md#permissions-and-delegation).

## A handoff is uncertain

AX does not automatically repeat a handoff whose native acceptance is uncertain. Repeating it could execute the task twice. Later mail can wait behind that handoff.

Inspect its status and the recipient’s conversation first. If you deliberately choose to abandon that handoff:

```sh
ax resolve MESSAGE_ID abandon
```

Abandonment releases the queue. It does not cancel work the harness already accepted and does not claim the message was delivered.

## After an update

Replacing the binary does not replace already-running processes. The broker and relaunched sessions must both use the new binary. Follow the [running-session update instructions](../AGENTS.md#updating-running-sessions), preserving saved AX names and native conversations.

The [resource-protection guide](resource-safety.md) describes the helper cooldown and CPU accounting included in AX 0.6.1. Run `ax doctor` to see whether messaging is paused.

## Report a problem

Include `ax version`, your OS, the harness and its version, and the relevant agent names and message IDs. Share the observed delivery state and what you expected. Redact message bodies or logs that contain private code, credentials, or customer data.

[Open an issue on GitHub](https://github.com/summationai/agent-exchange/issues).
