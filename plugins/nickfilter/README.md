# nickfilter

**Category:** Moderation

Detects and (optionally) actions disallowed nicknames. Supports a
custom word/substring list plus built-in checks for:

- **Hoisting** - leading non-letter characters used to push a user to
  the top of the member list
- **Zalgo** - excessive combining marks (Z̸̧̖͚̄͊ą̷̯̈̓l̴̨̊g̴̙͒o̴̜̔)
- **Mass-mention injection** - `@everyone` / `@here` in a nickname

## Commands (Manage Nicknames)

| Command | Description |
|---------|-------------|
| `!nickfilter add <word>` | Add a banned substring (case-insensitive) |
| `!nickfilter remove <word>` | Remove one |
| `!nickfilter list` | Show the banned list |
| `!nickfilter action log\|kick\|rename` | What to do on a hit (default `log`) |
| `!nickfilter replacement <name>` | Fallback nickname when action is `rename` |
| `!nickfilter alert #channel` | Alert channel |

## Actions

| Action | What it does |
|--------|--------------|
| `log` | Post an alert in the configured alert channel |
| `kick` | Kick the user from the guild |
| `rename` | Reset the user's nickname to the replacement name |

## Notes

- New joiners are checked automatically via `OnMemberAdd`.
- Existing members are re-checked whenever they send a message, so
  users who change their nick after joining will still get caught.
- The `rename` action calls `bot.Rest.SetNickname`. If your framework
  doesn't expose that method you'll see a log error - fall back to
  `log` or `kick`.
- The bot's role must be above the target member's roles to rename
  or kick them.
