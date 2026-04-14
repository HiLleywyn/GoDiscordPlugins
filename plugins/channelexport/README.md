# channelexport

**Category:** Utility

Exports recent channel activity as a plain-text dump. Useful for
incident postmortems, context handoffs, quoting a discussion in a bug
report, or archiving a conversation you're about to wrap up.

## How it works

The plugin hooks into every message and keeps a rolling **in-memory
buffer** of up to 2000 messages per channel. When you run
`!channelexport`, it renders the buffer to a text file and uploads it
as an attachment.

### Important limitation

The buffer only contains messages the bot **saw after it started up**.
If you need a complete archive including messages from before the bot
came online, use Discord's official Data Export or a dedicated
archiver tool - this plugin is meant for day-to-day "give me the last
few hundred messages from this channel" use cases.

## Commands

Requires **Manage Messages**.

| Command | Description |
|---------|-------------|
| `!channelexport` | Export current channel, default 200 messages |
| `!channelexport #chan` | Export another channel |
| `!channelexport #chan 500` | Export last 500 messages |
| `!channelexport 50` | 50 messages from current channel |
| `!channelexport clear` | Clear the buffer for the current channel |

Alias: `!export`. Maximum export size is 2000 messages (matches the
buffer cap).

## Output format

```
Channel export for #general
Exported at 2025-10-15T08:12:43Z
212 messages
------------------------------------------------------------

[2025-10-15 07:42:01] alice#1234 (123...):
  Hey did we decide on the schema for the new endpoint?

[2025-10-15 07:42:30] bob#5678 (456...):
  Yeah I put the draft in the design doc
  line two of bob's reply
```

## Notes

- The buffer is in-memory only. Restarting the bot drops everything.
- Only text content is captured. Attachments, embeds, and reactions
  are not included.
- Per-channel buffer cap is 2000. Once full, the oldest messages get
  evicted first (ring buffer).
