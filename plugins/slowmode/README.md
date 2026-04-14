# slowmode

Automatically adjusts a channel's slowmode based on messages per minute.

## Commands

| Command | Description |
|---------|-------------|
| `!slowmode auto <threshold>` | Enable auto slowmode; activates when rate exceeds threshold msgs/min |
| `!slowmode off` | Disable auto slowmode and clear any active slowmode for this channel |
| `!slowmode status` | Show threshold, current rate, and whether slowmode is active |

### Example

```
!slowmode auto 20
```

Enables auto slowmode in the current channel. When the message rate exceeds 20 messages per minute, slowmode is set to 5 seconds per user. Once the rate drops below 10 messages per minute for 2 continuous minutes, slowmode is removed.

## How it works

- Each message in a monitored channel is recorded in memory.
- A sliding 60-second window is maintained per channel.
- When messages/minute >= threshold, slowmode is applied (5s delay).
- When messages/minute < threshold/2 for 2 consecutive minutes, slowmode is removed.
- The `!slowmode off` command also makes an immediate REST call to clear any active slowmode.

## Config keys

| Key | Value |
|-----|-------|
| `auto:<channelID>` | Threshold (messages per minute) as a string |

## Notes

- Slowmode state is held in memory. If the bot restarts while slowmode is active, it will remain on the channel until the rate drops again (or `!slowmode off` is used).
- The slowmode value applied when activating is hardcoded to 5 seconds. Adjust it in the source if needed.
- Uses `ModifyChannel` from GoDiscord's REST client (`rate_limit_per_user` field).

## Required intents

- `GuildMessages` + `MessageContent` - to count messages
