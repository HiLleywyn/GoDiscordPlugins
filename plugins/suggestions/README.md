# suggestions

**Category:** Engagement

A structured suggestion-box workflow. Anyone can file a suggestion; mods can
approve, deny, or mark it as implemented. Each suggestion is posted as an
embed in the configured channel, pre-reacted with thumbs up/down for voting,
and assigned an incrementing numeric ID.

## Commands

### User

| Command | Description |
|---------|-------------|
| `!suggest <text>` | File a new suggestion (max 1500 chars) |

### Mod/admin

| Command | Description |
|---------|-------------|
| `!suggestion channel #chan` | Set the suggestions channel |
| `!suggestion approve <id> [note]` | Mark a suggestion as approved |
| `!suggestion deny <id> [note]` | Mark a suggestion as denied |
| `!suggestion implement <id> [note]` | Mark a suggestion as implemented |
| `!suggestion show <id>` | Repost a suggestion embed |
| `!suggestion list [status]` | List the most recent 15 suggestions, optionally filtered |

`!sug` is aliased to `!suggestion`.

## Status colors

| Status | Color |
|--------|-------|
| `pending` | Blurple |
| `approved` | Green |
| `denied` | Red |
| `implemented` | Yellow |

## Notes

- Suggestions are stored in plugin config under `sug:<id>`, keyed by an
  incrementing counter (`counter` key).
- When a mod sets a status, the original embed is edited in-place; voters see
  the new color immediately.
- `list` without a filter shows everything, newest first. Filters are
  `pending`, `approved`, `denied`, `implemented`.
