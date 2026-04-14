# welcome

Sends custom messages when members join or leave a guild.

## Commands

| Command | Description |
|---------|-------------|
| `!welcome channel #channel` | Set the channel for welcome/leave messages |
| `!welcome join <message>` | Set the join message |
| `!welcome leave <message>` | Set the leave message |
| `!welcome test join` | Send a test join message |
| `!welcome test leave` | Send a test leave message |
| `!welcome show` | Show current config |

## Variables

Use these in join and leave messages:

| Variable | Replaced with |
|----------|--------------|
| `{user}` | Mention of the user who joined/left |
| `{server}` | Guild name |
| `{membercount}` | Current member count |

### Example

```
!welcome join Welcome to {server}, {user}! You are member #{membercount}.
!welcome leave {user} has left. We are now {membercount} members.
```

## Config keys

| Key | Value |
|-----|-------|
| `channel` | Channel ID for messages |
| `join` | Join message template |
| `leave` | Leave message template |

## Required intents

- `GuildMembers` - to receive join and leave events
- `GuildMessages` - to send messages
