# purge

**Category:** Moderation

Bulk-delete recent messages in the current channel, optionally filtered by author, content, or attachment type.

## Usage

```
!purge <count> [filter...]
```

`count` is 1-500. The plugin scans up to 500 recent messages and deletes the first `count` that match all filters.

## Filters

| Filter | Meaning |
|--------|---------|
| `user @user` | Only from the given user |
| `bots` | Only messages from bots |
| `humans` | Only non-bot messages |
| `contains <text>` | Message content contains text (case-insensitive, must be last) |
| `links` | Only messages containing `http://` or `https://` |
| `embeds` | Only messages with attachments or embeds |

Filters combine with AND. `bots` and `humans` cannot be combined.

## Examples

```
!purge 20                    # last 20 messages
!purge 50 user @spammer      # up to 50 recent messages from @spammer
!purge 100 bots              # clean up bot noise
!purge 30 contains lfg       # last 30 messages containing "lfg"
!purge 50 links              # last 50 messages with URLs
```

## Notes

- Discord's bulk delete endpoint refuses messages older than 14 days. The plugin detects these and falls back to individual deletes (slower, but still works).
- A short confirmation message is posted and auto-deleted after 5 seconds.
- The invoking `!purge` command itself is also removed.

## Required permissions

- `Manage Messages`
- `Read Message History`
