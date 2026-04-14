# tags

**Category:** Utility

Reusable text snippets ("tags") that anyone in the guild can recall by name. Great for FAQ entries, server rules, common links, recurring copy-pastes.

## Commands

| Command | Description |
|---------|-------------|
| `!tag <name>` | Post the content of a tag |
| `!tag add <name> <content>` | Create a new tag |
| `!tag edit <name> <content>` | Replace a tag's content (author/mod only) |
| `!tag delete <name>` | Delete a tag (author/mod only) |
| `!tag info <name>` | Show author, creation time, use count |
| `!tag list` | List every tag in the guild |
| `!tag search <query>` | Find tags whose name contains `<query>` |
| `!tag top` | Top 10 tags by use count |

## Variables

Tag content can reference a few variables. They're substituted at invocation time:

| Variable | Replaced with |
|----------|---------------|
| `{user}` | Mention of the user invoking the tag |
| `{server}` | Guild name |
| `{membercount}` | Current member count |

## Constraints

- Tag names: 1-32 characters, lowercase letters/digits/`_`/`-` only.
- Content: max 1800 characters.
- Users can only edit or delete tags they created, unless they have `Manage Messages`.
