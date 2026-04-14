# birthday

**Category:** Engagement

Tracks user birthdays and announces them with an optional birthday role
for the day. Users self-register; admins configure the announcement
channel, role, and message template.

## Commands

### User

| Command | Description |
|---------|-------------|
| `!birthday set <MM-DD>` | Set birthday (month+day only) |
| `!birthday set <YYYY-MM-DD>` | Set birthday including year (enables `{age}` in announcements) |
| `!birthday clear` | Remove your birthday |
| `!birthday @user` | Show a user's birthday |
| `!birthday upcoming` | Show the next 7 days of birthdays |

`!bday` is aliased to `!birthday`.

### Admin (Manage Messages)

| Command | Description |
|---------|-------------|
| `!birthday channel #chan` | Channel for announcements |
| `!birthday role @role` | Role granted to the birthday person for the day (omit to clear) |
| `!birthday message <text>` | Template (supports `{user}`, `{age}`) |

## Defaults

- Template: `Happy birthday <@{user}>!`
- No role (just an announcement)
- No channel (no announcements until configured)

## How it works

A background ticker runs every hour. Once per day per guild it:

1. Removes the birthday role from yesterday's recipients (if a role
   was configured).
2. Finds everyone whose birthday is today, grants them the role, and
   posts the announcement.

Day transitions use UTC to avoid off-by-one fights with DST.

## Notes

- Setting a full `YYYY-MM-DD` unlocks `{age}` in the template.
  `MM-DD` leaves the age blank.
- Storage is per-guild under `u:<userID>`.
- The bot's role must be above the birthday role to assign/remove it.
