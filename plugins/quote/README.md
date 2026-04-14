# quote

**Category:** Engagement

A per-guild quote book. Save memorable messages, then pull them back up
later at random, by id, or by author.

## Commands

| Command | Description |
|---------|-------------|
| `!quote` | Random quote |
| `!quote <id>` | Show quote by id |
| `!quote by @user` | Random quote by that author |
| `!quote add <text> -- <author>` | Save a new quote (author can be a mention or free text) |
| `!quote list` | Show the 15 newest quotes |
| `!quote search <text>` | Find quotes containing `<text>` (in body or author) |
| `!quote remove <id>` | Delete a quote (author of the quote or Manage Messages) |

## Format

The separator between quote body and author is ` -- ` (space-dash-dash-space).
If the author is a user mention, the quote is linked back to that user and
`!quote by @user` will find it.

Example:

```
!quote add Technically, it compiles. -- @alice
```

## Notes

- Quotes are stored per-guild under `q:<id>`.
- A deleted quote's id is not reused; the counter keeps climbing.
- Max body length 1500 chars.
