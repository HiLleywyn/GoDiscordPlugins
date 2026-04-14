# modlog

**Category:** Moderation

Posts an audit log of moderator-relevant events to a designated channel. Events can be toggled individually.

## Commands

| Command | Description |
|---------|-------------|
| `!modlog channel #channel` | Set the log channel |
| `!modlog enable <event>` | Enable logging for an event |
| `!modlog disable <event>` | Disable logging for an event |
| `!modlog show` | Show current settings |

## Events

| Name | Fired when |
|------|------------|
| `delete` | A message is deleted |
| `edit` | A message is edited |
| `join` | A member joins the guild |
| `leave` | A member leaves or is kicked |
| `ban` | A user is banned |
| `unban` | A user is unbanned |

All events default to **enabled** once the channel is configured.

## Required intents

- `GuildMessages` and `MessageContent` - for delete/edit content
- `GuildMembers` - for join/leave
- `GuildBans` - for ban/unban
