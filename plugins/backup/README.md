# backup

**Category:** Utility

Snapshots and restores all plugin configuration for a guild. Think of
it as "save file" for every other plugin in this collection.

## What it backs up

Every plugin in this collection stores its state via the per-guild
config store:

- `automod` word lists, thresholds, exemptions
- `reactionroles` panels
- `starboard` channel and thresholds
- `tags` entries
- `welcome`/`goodbye` messages
- `autoresponder` rules
- `autorole` role IDs
- `customrole` pool
- ... everything else with persistent config

A snapshot captures that entire keyspace. A restore writes it back.

## What it does NOT back up

This is **not** a full Discord server backup. It does not snapshot:

- Channels or categories
- Roles (permissions, positions)
- Emojis or stickers
- Messages
- Member roles / nicknames
- Server settings (verification level, icons, banners)

Discord-wide backup needs server-wide REST access and is better
handled by dedicated tools. This plugin solves the part that's
*usually* actually painful - the plugin config itself.

## Commands

All commands require **Administrator**.

| Command | Description |
|---------|-------------|
| `!backup create [name]` | Snapshot current plugin config |
| `!backup list` | List snapshots (newest first) |
| `!backup show <id>` | Show snapshot summary and per-plugin entry counts |
| `!backup delete <id>` | Delete a snapshot |
| `!backup export <id>` | Upload snapshot as a JSON file |
| `!backup import` | Import from a JSON file attached to the same message |
| `!backup restore <id> confirm` | Overwrite current config with a snapshot |

## Typical workflows

### Scheduled off-box backups

```
!backup create nightly
!backup export <id>
```

Download the file, store it somewhere safe. Repeat on a schedule.

### Cloning a template server

On the template server:
```
!backup create template
!backup export <id>
```

Download the JSON. On the target server, attach the file and run:
```
!backup import
!backup restore <new-id> confirm
```

### Rolling back a config mistake

```
!backup create before-edit
... make risky config changes ...
!backup restore <id> confirm
```

## Safety rails

- `restore` is destructive and always requires the literal word
  `confirm` as the second argument.
- Before applying a restore, the plugin automatically creates a
  rollback snapshot named `auto-before-restore-<id>`. If the restored
  state is wrong, restore the rollback.
- Snapshots are stored in the same config store as other plugins,
  under keys prefixed with `snap:`. The restore command explicitly
  skips `snap:*` so your snapshots survive restores of themselves.

## Notes

- Reload the bot's plugins after a restore if you want long-lived
  plugin state (background tickers, in-memory caches) to pick up the
  new config.
- Imported snapshots get a new ID and are re-keyed to the current
  guild, so they're safe to move between servers.
