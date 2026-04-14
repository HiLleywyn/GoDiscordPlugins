# stats

**Category:** Engagement

Tracks message volume by channel and by user, rolls it up per ISO week,
and posts a weekly digest. Think of it as a lightweight "server activity
report" that doesn't need any external analytics pipeline.

## What it tracks

For each guild:

- total messages this week
- messages per channel this week
- messages per user this week

Bot messages are ignored. Data is bucketed by ISO week (`YYYY-WW`), so
weeks roll over cleanly on Monday.

## Commands

### User

| Command | Description |
|---------|-------------|
| `!stats` | Summary for the current week |
| `!stats week` | Previous completed week |
| `!stats channel #chan` | This week's count for one channel |
| `!stats user @user` | This week's count for one user |
| `!stats top channels` | Top 10 channels this week |
| `!stats top users` | Top 10 posters this week |

### Admin (Manage Messages)

| Command | Description |
|---------|-------------|
| `!stats digest #chan` | Post a weekly digest to this channel |
| `!stats digest off` | Stop posting weekly digests |
| `!stats digest` | Show current digest channel |

## Weekly digest

A background ticker runs every hour. Once per week it posts a digest of
the *previous* completed week (total messages, top 5 channels, top 5
posters) to the configured digest channel. Nothing is posted if the
previous week had zero messages.

## Notes

- Counts are stored in per-guild config under `total:YYYY-WW`,
  `ch:<id>:YYYY-WW`, `usr:<id>:YYYY-WW`. Old weeks are not auto-pruned;
  they stay in config indefinitely so you can query history with custom
  tooling if needed.
- This plugin only counts message *frequency*. It does not store message
  content. If you need content moderation, see `automod`.
- Reactions, voice activity, and edits are not tracked.
