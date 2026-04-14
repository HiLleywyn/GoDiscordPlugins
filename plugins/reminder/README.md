# reminder

**Category:** Utility

Personal reminders. Users pick a duration and a note; when it fires, the bot pings them in the channel they set it in (or by DM if they opt in).

## Commands

| Command | Description |
|---------|-------------|
| `!remindme <duration> <note>` | Schedule a reminder |
| `!reminders [list]` | List your pending reminders |
| `!reminders cancel <id>` | Cancel one of your reminders |
| `!reminders dm on\|off` | Toggle DM delivery |

### Duration format

`<number><suffix>` where suffix is:

- `s` - seconds
- `m` - minutes
- `h` - hours
- `d` - days

Examples:

```
!remindme 10m check the oven
!remindme 2h stand up and stretch
!remindme 7d weekly retro
```

Minimum duration is 1 minute; maximum is 365 days.

## Behaviour

- Reminders are checked every 30 seconds, so firing may lag by up to that.
- DM mode falls back to channel delivery if the user has DMs closed.
- Notes are truncated to 500 characters.
- Reminder IDs are short strings; `!reminders list` shows them next to each entry.
