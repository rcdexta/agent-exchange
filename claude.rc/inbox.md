# A separate inbox for your agents

Open a third terminal or create a split in your terminal application:

```sh
ax inbox
```

Keep Claude, Codex, and other harnesses in their own terminals. AX shows the latest 100 messages, newest first, with sender, recipient, and delivery status. Select a message to read its body in the lower pane. Filter both incoming and outgoing messages for one agent:

```sh
ax inbox api
```

Use the arrow keys or `j` and `k` to select a message, `u` and `d` to scroll its body, `g` to select the newest message, and `q` to quit. New arrivals preserve your current selection. The display adjusts when the terminal is resized. The inbox requires an interactive terminal; scripts can use `ax agents` and `ax status MESSAGE_ID`.

The inbox is read only. Reading a message here does not mark it read by the agent, send a reply, change permissions, or trigger delivery. It owns no coding harness process. Closing it, losing its connection, or killing it cannot send a termination signal to a harness or broker.

The view refreshes every two seconds and repaints only when its content or dimensions change. Connection errors use the same bounded retry interval. When the broker is unavailable, the last snapshot stays visible with an offline notice. The inbox reconnects when the broker returns; it never starts, stops, or upgrades the broker itself. This command requires an inbox-enabled broker; an older broker reports an unknown-method error.

The view displays message text as plain text. Terminal control characters and directional formatting controls from messages are removed before rendering.

Delivery states describe transport evidence: queued, handed to the harness, fetched, or acknowledged. None of those proves a delegated task finished. Durable task progress will appear only once AX has explicit task state to display.

Messages follow the broker's retention policy. This first view shows the latest 100 matching messages, rather than an unlimited archive. It is a separate observer, not an embedded panel in another harness's TUI.
