# verify

**Category:** Moderation

Gates new members behind a manual verification step. New joiners get an
"unverified" role that restricts them to a single channel; they run
`!verify` to swap that for the "verified" role and get access.

## Setup

1. Create two roles: `Unverified` and `Verified` (or whatever you want to
   call them).
2. Configure your channels so `Unverified` can only see one channel
   (e.g. `#verify`), and `Verified` has normal access.
3. Make sure the bot's role is above both so it can assign/remove them.
4. Run `!verify setup @Unverified @Verified`.
5. Optional: `!verify phrase I agree to the rules` to require a phrase.
6. Optional: `!verify channel #verify` to restrict the command to one channel.

## Commands

| Command | Description |
|---------|-------------|
| `!verify` | Complete verification (for users) |
| `!verify <phrase>` | Complete verification when a phrase is required |
| `!verify setup @unverified @verified` | Configure roles (Manage Roles) |
| `!verify phrase <text>` | Require a phrase (omit text to clear) |
| `!verify channel #chan` | Restrict the command to one channel |
| `!verify status` | Show current settings |

## Notes

- On every join, if an unverified role is configured, it's assigned
  automatically.
- Users already holding the verified role can run `!verify` safely - it
  just tells them they're already verified.
- The phrase check is case-insensitive.
