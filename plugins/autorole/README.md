# autorole

**Category:** Moderation

Automatically assigns one or more roles to every new member who joins the
guild. Supports an optional delay, useful when you want to wait for Discord's
membership screening to complete first.

## Commands

| Command | Description |
|---------|-------------|
| `!autorole add @role` | Add a role to the auto-assign list |
| `!autorole remove @role` | Remove a role from the list |
| `!autorole list` | Show configured roles and current delay |
| `!autorole delay <seconds>` | Delay before applying (0-3600, 0 disables) |
| `!autorole clear` | Remove every configured role |

## Notes

- Multiple roles are supported. Each is applied sequentially on join.
- Bots are ignored and never receive auto-roles.
- The delay is handy if you use membership screening - set it to a few seconds
  so the role is applied after the user accepts the rules screen.
- Make sure the bot's role is *higher* than every auto-role in the guild's
  role list, otherwise Discord will refuse the assignment.
