# altcheck

**Category:** Moderation

Flags or auto-actions accounts that are newer than a configured minimum
age. Account creation time is derived directly from the Discord user ID
(snowflake), so no API calls or external data needed.

## Commands (Manage Messages)

| Command | Description |
|---------|-------------|
| `!altcheck threshold <days>` | Minimum account age in days (default 7) |
| `!altcheck action log\|kick\|ban` | What to do with young accounts (default `log`) |
| `!altcheck alert #channel` | Channel for alerts |
| `!altcheck check @user` | Manually check any user's account age |
| `!altcheck status` | Show current settings |

## How age is computed

Discord snowflake IDs encode a millisecond timestamp in their upper bits
(epoch 2015-01-01). So `altcheck` can tell an account's age from the ID
alone, without needing a REST call.

## Notes

- Threshold of `0` disables the check.
- `ban` falls back to kick if the framework doesn't expose a ban method -
  check the bot log if you expected a ban and got a kick.
- Bots are ignored.
- Consider pairing with `verify` so flagged users can still redeem
  themselves by manually verifying.
