# threadkeeper

**Category:** Utility

Keeps long-lived threads from auto-archiving. Discord archives inactive
threads on a schedule (24h / 3d / 7d depending on boost level). If you
have a thread that should always be findable - an ongoing project
thread, an onboarding thread, a support queue - this plugin posts a
tiny heartbeat message on a schedule to keep it alive.

## Commands

Requires **Manage Messages**.

| Command | Description |
|---------|-------------|
| `!threadkeeper add #thread [hours]` | Keep a thread alive. Default interval: 20h |
| `!threadkeeper remove #thread` | Stop keeping it alive |
| `!threadkeeper list` | Show all tracked threads |
| `!threadkeeper message #thread <text>` | Customize the heartbeat text |

Interval must be between 1 and 168 hours.

## Example

```
!threadkeeper add #project-kraken 20
  OK. Keeping #project-kraken alive every 20 hours.

!threadkeeper message #project-kraken keepalive tick
  Heartbeat message updated.
```

## Notes

- The default heartbeat is `_(keeping this thread alive)_` in italics so
  it stays visually quiet.
- The plugin runs a background check every 15 minutes, so worst-case
  drift from your configured interval is ~15 minutes.
- Set the interval a few hours below the archive window. For a 24h
  archive, 20h is a safe default.
- Works on regular channels too if you want a scheduled nudge, but
  that's secondary - the real use case is threads.
