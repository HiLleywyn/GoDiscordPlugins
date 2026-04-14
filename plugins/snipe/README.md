# snipe

**Category:** Utility

Retrieves the last few deleted or edited messages in the current channel. Buffers are in-memory only and entries auto-expire after 10 minutes - snipe is meant for "wait, what did that say?" moments, not permanent logging.

## Commands

| Command | Description |
|---------|-------------|
| `!snipe [n]` | Show the nth most recent deleted message (default 1) |
| `!editsnipe [n]` | Show the previous content of the nth most recent edit |

`n` ranges from 1 to 10. The plugin keeps up to 10 entries per channel per category.

## Behaviour

- Bot messages and messages whose author isn't cached are ignored.
- Deleted messages are gone from Discord's side - the plugin only works for messages it saw while running.
- Edits that don't change the content (embed fetches, etc.) are skipped.
- Entries older than 10 minutes are discarded on a 1-minute sweep.

## Required intents

- `GuildMessages` and `MessageContent` - to observe deletes and edits
