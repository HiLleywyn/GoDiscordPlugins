# slowmode

**Category:** Moderation

Automatically adjusts a channel's per-user rate limit based on observed message rate. When a channel starts firehosing, the plugin raises slowmode to cool it down; once activity drops, slowmode decays back to zero.

## Commands

| Command | Description |
|---------|-------------|
| `!slowmode enable #chan` | Start tracking the channel |
| `!slowmode disable #chan` | Stop tracking and clear slowmode |
| `!slowmode threshold <msgs/min>` | Rate at which slowmode begins (default 20) |
| `!slowmode max <seconds>` | Cap on applied slowmode (default 30, max 21600) |
| `!slowmode show` | Show current settings and tracked channels |

## Algorithm

Every 30 seconds the plugin looks at how many messages each tracked channel received in the last minute and picks a new slowmode value:

| Observed rate | New slowmode |
|---------------|--------------|
| `>= 2 * threshold` | `max` (cap) |
| `>= threshold` | Linear between 1s and `max` |
| `< threshold / 2` | Current value decays by 5s (down to 0) |
| Otherwise | Unchanged |

This keeps slowmode reactive without flapping.

## Required permissions

- `Manage Channels` (to set slowmode)
- `MessageContent` and `GuildMessages` - to count messages
