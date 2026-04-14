# automod

**Category:** Moderation

Lightweight content and rate filters. Each filter is opt-in; when any filter trips, the offending message is deleted and a single configured action is applied to the author.

## Commands

| Command | Description |
|---------|-------------|
| `!automod invites on\|off` | Block discord.gg invite links |
| `!automod caps <percent>` | Max uppercase percent in messages >= 8 letters (0 = off) |
| `!automod mentions <n>` | Max mentions per message (0 = off) |
| `!automod spam <count>/<seconds>` | Max messages per window per user (e.g. `5/10`) |
| `!automod action delete\|timeout:<min>\|kick\|warn` | Punishment on violation |
| `!automod exempt add\|remove @role` | Roles exempted from all filters |
| `!automod show` | Show current settings |

## Actions

| Value | Effect |
|-------|--------|
| `delete` | Just delete the message (default) |
| `timeout:<min>` | Delete + timeout the user for N minutes |
| `kick` | Delete + kick the user |
| `warn` | Delete + post a public warning ping |

## Required intents / permissions

- `MessageContent` and `GuildMessages` - to inspect messages
- `Manage Messages` - to delete violations
- `Moderate Members` - for `timeout:<min>`
- `Kick Members` - for `kick`

## Notes

- Spam tracking is in-memory; restarting the bot clears spam counters (but not config).
- The caps filter ignores messages shorter than 8 letters.
- Exempted roles are never filtered, regardless of content.
