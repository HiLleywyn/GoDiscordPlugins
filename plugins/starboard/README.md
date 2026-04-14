# starboard

Reposts highly-reacted messages to a designated starboard channel.

## Commands

| Command | Description |
|---------|-------------|
| `!starboard channel #channel` | Set the starboard channel |
| `!starboard threshold <n>` | Set the star count needed (default: 3) |
| `!starboard emoji <emoji>` | Set the trigger emoji (default: star) |
| `!starboard show` | Show current settings |

## How it works

When a message receives at least `threshold` reactions matching the configured emoji, it is reposted to the starboard channel as an embed. Each message is only posted once regardless of how many more reactions it gets.

Starred message IDs are stored in plugin config under `starred:<messageID>`. To allow a message to be starred again, delete that key.

## Permissions

Any user can run the configuration commands. Restrict the commands in Discord using channel permission overwrites if needed.

## Required intents

- `GuildMessageReactions` - to receive reaction events
- `GuildMessages` - to fetch message content
- `MessageContent` - to read message text
