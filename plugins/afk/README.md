# afk

**Category:** Utility

Lets users mark themselves as away with an optional reason. While a user is AFK:

- anyone who pings them gets a short "X is AFK: reason (Xm ago)" reply
- when the AFK user sends their next message, the bot welcomes them back and tells them how many times they were pinged

## Commands

| Command | Description |
|---------|-------------|
| `!afk` | Toggle AFK on (no reason) or off |
| `!afk <reason>` | Set AFK with a reason (max 300 chars) |

State persists across restarts (per-guild, per-user).

## Notes

- Sending `!afk <new reason>` while already AFK updates the reason without clearing the welcome-back message.
- Ping notices are deduplicated per message: a single message that mentions the same AFK user twice only produces one reply.
- Bot messages are ignored; bots can't be marked AFK.
