# reactionroles

**Category:** Utility

Lets users self-assign roles by reacting to a configured panel message.

## Flow

1. Create a panel with a title:
   ```
   !rr create colors Pick your color
   ```
2. Bind reactions to roles:
   ```
   !rr bind colors red @Red
   !rr bind colors blue @Blue
   ```
3. Post the panel into a channel:
   ```
   !rr post colors #roles
   ```

Users click the reactions to pick up (or drop) roles. When a user adds a reaction the corresponding role is granted; removing the reaction removes the role.

## Commands

| Command | Description |
|---------|-------------|
| `!rr create <panel> <title...>` | Create an empty panel |
| `!rr bind <panel> <emoji> @role` | Add an emoji -> role binding |
| `!rr unbind <panel> <emoji>` | Remove a binding |
| `!rr post <panel> #channel` | Post the panel message |
| `!rr list` | List configured panels |
| `!rr delete <panel>` | Delete a panel |

Binding a new reaction after `!rr post` updates the live embed and adds the new reaction automatically.

## Notes

- Both unicode emoji (e.g. `🔵`) and custom emoji (e.g. `<:blobwave:12345>`) are supported.
- Stray reactions that don't match any binding are automatically removed.
- Panel state is persisted per-guild, so panels survive restarts.

## Required intents / permissions

- `GuildMessageReactions`
- `Manage Roles`
- The bot's top role must be above any role it hands out.
