# autoresponder

Responds to trigger keywords in messages with preset replies.

## Commands

| Command | Description |
|---------|-------------|
| `!ar add <trigger> <response>` | Add a trigger/response pair |
| `!ar remove <trigger>` | Remove a trigger |
| `!ar list` | List all triggers for this server |
| `!ar toggle` | Enable or disable the auto-responder |

## How it works

When a message is sent in a guild, the plugin checks the message text for any configured triggers (case-insensitive substring match). If a match is found the bot sends the configured response to the same channel.

At most 5 responses are sent per message to prevent spam when multiple triggers match.

Bot messages are ignored.

## Config keys

| Key | Value |
|-----|-------|
| `trigger:<trigger>` | The response text for that trigger |
| `enabled` | `false` to disable (anything else = enabled) |

## Permissions

Any user can run the configuration commands. Restrict them with Discord channel permissions if needed.

## Required intents

- `GuildMessages` + `MessageContent` - to read message text
