# appeals

**Category:** Moderation

A ban-appeal inbox. Users file appeals with `!appeal <text>`; the
submission lands as an embed in a configured mod channel. Mods approve
or deny with an optional note, and the user is automatically DMed with
the decision.

## Setup

1. Create a private mod channel (e.g. `#appeal-review`).
2. Run `!appeal channel #appeal-review` once as an admin.

## Commands

### User

| Command | Description |
|---------|-------------|
| `!appeal <text>` | File a new appeal (max 1500 chars, one open at a time) |

### Mod (Manage Messages)

| Command | Description |
|---------|-------------|
| `!appeal channel #chan` | Set the review channel |
| `!appeal approve <id> [note]` | Approve an appeal (user is DMed) |
| `!appeal deny <id> [note]` | Deny an appeal (user is DMed) |
| `!appeal show <id>` | Show one appeal embed |
| `!appeal list [open\|closed\|all]` | List recent appeals (default: open) |

## Notes

- Each user may only have **one open appeal** at a time. Mods must
  approve or deny the existing one before the user can file again.
- On a decision, the embed in the mod channel is edited in-place
  (green for approved, red for denied).
- The user gets a DM with the status and the mod note.
- Storage is per-guild under `a:<id>`.
- Since appeal filers are often banned, they need to be able to
  DM/message the bot somewhere. The typical pattern: keep a single
  "appeals" server that anyone can join just to submit an appeal.
