# sticky

**Category:** Utility

Keeps a configured message pinned to the visual bottom of a channel by
reposting it whenever enough new messages have scrolled it away. Useful for
rules reminders, posting guidelines, or "please use threads" notices.

## Commands (Manage Messages)

| Command | Description |
|---------|-------------|
| `!sticky set <text>` | Set or replace the sticky in the current channel (max 1500 chars) |
| `!sticky clear` | Remove the sticky from the current channel |
| `!sticky show` | Preview the current sticky |
| `!sticky list` | List every channel that has a sticky in this guild |

## How it reposts

To avoid spamming, the sticky is only reposted when **both**:

- at least 3 new messages have been posted since the last repost, **and**
- at least 5 seconds have elapsed since the last repost

This means in a quiet channel the sticky stays put; in a fast-moving
channel it's kept in view without drowning out conversation.

## Notes

- Only one sticky per channel.
- The bot needs Manage Messages in the channel to clean up its old sticky
  copy before reposting.
- State is stored per-guild under `ch:<channelID>`.
