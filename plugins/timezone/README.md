# timezone

**Category:** Utility

Per-user timezones and locale-aware time formatting. Users set their
IANA timezone once, then anyone can ask "what time is it for Alice
right now?" or broadcast an event time that each viewer sees in their
own locale thanks to Discord's `<t:unix:F>` timestamp format.

## Commands

| Command | Description |
|---------|-------------|
| `!tz set <zone>` | Save your timezone (IANA, e.g. `America/New_York`) |
| `!tz clear` | Remove your saved timezone |
| `!tz` | Show your current local time |
| `!tz @user` | Show another user's current local time |
| `!when <time>` | Broadcast a time that renders locally for every viewer |

`!timezone` is aliased to `!tz`.

## `!when` examples

| You type | Everyone sees (in their own locale) |
|----------|-------------------------------------|
| `!when 15:30` | Today at 3:30 PM (in viewer's timezone) |
| `!when 3pm` | Today at 3:00 PM |
| `!when 9:00am` | Today at 9:00 AM |
| `!when tomorrow 8am` | Tomorrow at 8:00 AM |

If the target time is earlier in the day than right now, `!when` rolls
it forward to tomorrow automatically. Override with `tomorrow ...`.

## Notes

- Timezones are parsed with `time.LoadLocation`, so you need full
  IANA names (`Europe/London`, not `GMT+0`).
- `!when` uses your stored timezone to interpret ambiguous times; if
  you haven't set one it falls back to UTC.
- Storage is per-guild under `u:<userID>`.
