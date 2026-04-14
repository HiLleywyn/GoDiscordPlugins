# embed

**Category:** Utility

Interactive embed builder for admins. Start a draft, fill it in
field-by-field through commands, preview it, then send it to a channel
or edit an existing embed message in place.

Drafts are persisted per-user per-guild so you can walk away and come
back without losing your work.

## Commands (Manage Messages)

### Build

| Command | Description |
|---------|-------------|
| `!embed new` | Start a fresh draft |
| `!embed title <text>` | Set the title |
| `!embed desc <text>` | Set the description |
| `!embed color <#hex\|int>` | Set the side color (e.g. `#5865F2`) |
| `!embed author <text>` | Set the author line |
| `!embed footer <text>` | Set the footer text |
| `!embed image <url>` | Set the main image URL |
| `!embed thumbnail <url>` | Set the thumbnail URL |
| `!embed field <name> \| <value>` | Add a field (pipe-separated) |
| `!embed field inline <name> \| <value>` | Add an inline field |
| `!embed fields clear` | Remove all fields |
| `!embed drop` | Discard the current draft |

### Preview / send

| Command | Description |
|---------|-------------|
| `!embed preview` | Post the draft as a reply to test it |
| `!embed send #chan` | Send the draft to a channel |
| `!embed edit #chan <msgID>` | Replace an existing embed message in-place |

## Example

```
!embed new
!embed title Server Rules
!embed desc Read these carefully before posting.
!embed color #5865F2
!embed field 1. Be kind | Don't be rude to other members.
!embed field 2. No spam | Keep it clean.
!embed preview
!embed send #rules
```

## Notes

- Drafts are stored under `d:<userID>`. Each user gets one draft per
  guild; `!embed new` overwrites the existing one.
- Field values support markdown, including mentions and links.
- `edit` only works on messages the bot originally sent; Discord
  doesn't allow bots to edit other authors' messages.
