# raidguard

**Category:** Moderation

Anti-raid protection. Watches for join floods and automatically kicks (or
times out) new members when a raid is detected. Also exposes a manual
raid-mode toggle for when you can see it coming.

## Commands (Manage Messages)

| Command | Description |
|---------|-------------|
| `!raidguard on` | Force raid mode on |
| `!raidguard off` | Force raid mode off |
| `!raidguard status` | Show current mode + recent join count |
| `!raidguard threshold <n>` | Joins required to trigger (default 8) |
| `!raidguard window <secs>` | Time window for joins (default 10) |
| `!raidguard cooldown <secs>` | Quiet period before raid mode auto-clears (default 120) |
| `!raidguard action kick\|timeout` | What to do with raiders (default `kick`) |
| `!raidguard alert #channel` | Channel to post raid alerts in |

## Defaults

- 8 joins in 10 seconds → raid mode
- 120 seconds of quiet → raid mode clears
- Action: kick

## Notes

- In raid mode, every new member is actioned before any other plugin
  (like `welcome` or `autorole`) sees them.
- `timeout` uses a 24-hour timeout so mods can verify and untimeout real
  members without losing them.
- Bots are always ignored.
- Per-guild state is in-memory; the rolling join window resets on
  bot restart but config (thresholds, action, alert channel) is persisted.
