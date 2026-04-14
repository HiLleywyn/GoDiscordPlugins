# customrole

**Category:** Utility

Lets eligible users claim one personal role from a curated pool. Common
use case: "server boosters and Patreon subscribers get to pick a custom
color role". Admins pre-create the roles (with whatever names and colors
they want) and add them to the pool; users pick one.

## Why a pool, not truly custom roles?

Creating and deleting Discord roles on the fly needs elevated REST
permissions and adds drift risk. The pool model lets you curate the
exact set of options (aesthetically consistent colors, no offensive
names) while still letting users self-serve.

## Setup

1. Create the custom roles you want to offer (e.g. `Rose`, `Sky`,
   `Mint`, `Sun`). Give them whatever color you want; no permissions.
2. Put them **below** the role that controls their eligibility (if
   any), but above the default member role.
3. Make sure the bot's role is above all of them.
4. As admin: `!customrole pool add @Rose`, `@Sky`, etc.
5. Optional: `!customrole requires @Booster` to limit picks to boosters.

## Commands

### User

| Command | Description |
|---------|-------------|
| `!customrole pick @role` | Claim a role from the pool (replaces previous pick) |
| `!customrole drop` | Release your current custom role |
| `!customrole list` | Show available roles in the pool |

`!crole` is aliased to `!customrole`.

### Admin (Manage Roles)

| Command | Description |
|---------|-------------|
| `!customrole pool add @role` | Add a role to the pool |
| `!customrole pool remove @role` | Remove one |
| `!customrole requires @role` | Only members with this role can pick |
| `!customrole requires clear` | No eligibility requirement |

## Notes

- Each user can hold exactly one pool role at a time; picking a new one
  drops the old one automatically.
- Storage: pool entries under `pool:<roleID>`, claims under `u:<userID>`.
