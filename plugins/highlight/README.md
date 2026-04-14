# highlight

**Category:** Utility

Per-user keyword subscriptions. Drop a keyword in once and you'll get a DM
whenever it appears in any channel of the guild you're in. Great for
"ping me if anyone mentions my project", "tell me when someone asks about
deploys", or just staying on top of topics you care about without opening
every channel.

## Commands

| Command | Description |
|---------|-------------|
| `!highlight add <word>` | Subscribe to a keyword (2-64 chars, up to 25 per user) |
| `!highlight remove <word>` | Unsubscribe |
| `!highlight list` | Show your keywords, mutes, and cooldown |
| `!highlight mute @user` | Don't highlight messages from this author |
| `!highlight unmute @user` | Remove a user mute |
| `!highlight mute #channel` | Don't highlight messages in this channel |
| `!highlight unmute #channel` | Remove a channel mute |
| `!highlight cooldown <secs>` | Only DM at most once per N seconds per keyword |
| `!highlight clear` | Remove everything you have configured |

`!hl` is aliased to `!highlight`.

## Matching rules

- Case-insensitive.
- Single-word keywords match on word boundaries, so `deploy` won't match
  `deploying`, but `deploy!` will match `deploy`.
- Multi-word keywords (phrases) match as substrings.
- You never get highlighted on your own messages.
- You never get highlighted on messages that @mention you directly (Discord
  will notify you anyway).

## Notes

- Subscriptions are per-guild - enable highlights in each guild you want
  them in.
- The bot must be able to DM you, so make sure server DMs aren't blocked
  (or you'll silently miss them; errors are logged to the bot's operator).
- Storage is under `u:<userID>` per guild.
