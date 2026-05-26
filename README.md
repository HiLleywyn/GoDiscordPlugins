# GoDiscordPlugins

Plugin collection, mainly for the [Carlos](https://github.com/hilleywyn/Carlos) Discord bot, but usable across any bot using the [GoDiscord](https://github.com/HiLleywyn/GoDiscord) framework.

## What is this

Each directory under `plugins/` is a standalone Go module that implements the `pluginapi.PluginEntry` interface. Drop a plugin into `Carlos/plugins/<name>/` and Carlos loads it automatically on startup.

## Installing a plugin

```
!plugin install starboard
```

Or manually: copy the plugin directory into `Carlos/plugins/`, then restart the bot.

## Writing a plugin

```go
package myplugin

import (
    "github.com/hilleywyn/carlos/pluginapi"
    discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

type Plugin struct{ api pluginapi.API }

func (p *Plugin) Manifest() pluginapi.PluginManifest {
    return pluginapi.PluginManifest{
        Name:        "myplugin",
        Version:     "1.0.0",
        Description: "Does something useful.",
        Author:      "YourName",
        Commands:    []string{"mycommand"},
    }
}

func (p *Plugin) Load(api pluginapi.API) error {
    p.api = api
    api.AddCommand(&discord.Command{
        Name:    "mycommand",
        Handler: p.handle,
    })
    return nil
}

func (p *Plugin) Unload() error { return nil }

func (p *Plugin) handle(ctx *discord.CommandContext) {
    ctx.Reply("hello")
}
```

Config is per-guild and per-plugin. Use `api.GetConfig`, `api.SetConfig`, `api.DeleteConfig`.

## Categories

Plugins live in one of three categories. Browse by what you need:

- **Moderation** - keep the server healthy
- **Utility** - general-purpose tools for users and admins
- **Engagement** - community interaction, rewards, onboarding

## Available plugins

### Moderation

| Plugin | Command | Description |
|--------|---------|-------------|
| [automod](plugins/automod) | `!automod` | Filter spam, invites, excessive caps, and mass mentions |
| [slowmode](plugins/slowmode) | `!slowmode` | Auto-adjust channel slowmode based on messages per minute |
| [autorole](plugins/autorole) | `!autorole` | Auto-assign roles on member join, with optional delay |
| [raidguard](plugins/raidguard) | `!raidguard` | Detect join floods and auto-kick or timeout suspected raiders |
| [verify](plugins/verify) | `!verify` | Gate new members behind a verification step before granting access |
| [altcheck](plugins/altcheck) | `!altcheck` | Flag or auto-kick accounts younger than a configured age |
| [appeals](plugins/appeals) | `!appeals`, `!appeal` | Ban appeal inbox with approve / deny mod review workflow |
| [nickfilter](plugins/nickfilter) | `!nickfilter` | Block hoisting, zalgo and mention-injection in nicknames |

### Utility

| Plugin | Command | Description |
|--------|---------|-------------|
| [reactionroles](plugins/reactionroles) | `!rr` | Self-assignable roles via reaction panels |
| [reminder](plugins/reminder) | `!remindme`, `!reminders` | Personal reminders delivered in channel or by DM |
| [tags](plugins/tags) | `!tag` | Reusable text snippets (FAQ entries, copy-pastes) |
| [snipe](plugins/snipe) | `!snipe`, `!editsnipe` | Recover recently deleted or edited messages |
| [afk](plugins/afk) | `!afk` | Mark yourself AFK; bot notifies people who ping you |
| [sticky](plugins/sticky) | `!sticky` | Keep a message pinned to the bottom of a channel |
| [highlight](plugins/highlight) | `!highlight`, `!hl` | DM yourself when subscribed keywords appear |
| [timezone](plugins/timezone) | `!tz`, `!when` | Per-user timezones and cross-TZ time conversion |
| [embed](plugins/embed) | `!embed` | Interactive embed builder for announcements |
| [customrole](plugins/customrole) | `!customrole`, `!crole` | Let eligible members claim one personal role from a curated pool |
| [todo](plugins/todo) | `!todo` | Personal task lists per user, with optional due dates |
| [threadkeeper](plugins/threadkeeper) | `!threadkeeper` | Keep long-running threads from auto-archiving |
| [channelexport](plugins/channelexport) | `!channelexport`, `!export` | Export recent channel messages as a text dump |
| [backup](plugins/backup) | `!backup` | Snapshot and restore plugin configuration for a guild |

### Engagement

| Plugin | Command | Description |
|--------|---------|-------------|
| [starboard](plugins/starboard) | `!starboard` | Repost highly-reacted messages to a dedicated channel |
| [autoresponder](plugins/autoresponder) | `!ar` | Respond to trigger keywords with preset replies |
| [giveaway](plugins/giveaway) | `!giveaway` | Timed giveaways with reaction-based entry |
| [welcome](plugins/welcome) | `!welcome` | Custom join and leave messages with variable substitution |
| [leveling](plugins/leveling) | `!rank`, `!leaderboard`, `!xp` | Message XP, ranks, leaderboard, and role rewards |
| [poll](plugins/poll) | `!poll` | Reaction-based polls with optional timed closing |
| [suggestions](plugins/suggestions) | `!suggest`, `!suggestion` | Suggestion box with voting and status tracking |
| [quote](plugins/quote) | `!quote` | Save memorable messages, random recall, by-author lookup |
| [birthday](plugins/birthday) | `!birthday`, `!bday` | Track birthdays, wish members happy birthday, apply a temp role |
| [stats](plugins/stats) | `!stats` | Server message activity stats with weekly digests |

## Manifest

`manifest.json` at the repo root enumerates every plugin in a form the Carlos `!plugin install` command can read. It now includes a top-level `categories` list and a `category` field on every plugin entry.

```json
{
  "version": "2",
  "categories": [
    {"id": "moderation", "name": "Moderation", "description": "..."},
    {"id": "utility",    "name": "Utility",    "description": "..."},
    {"id": "engagement", "name": "Engagement", "description": "..."}
  ],
  "plugins": [
    {
      "name": "automod",
      "category": "moderation",
      "version": "1.0.0",
      ...
    }
  ]
}
```

New plugins should set `category` to one of the IDs above. Proposals for new categories are welcome in a PR.

## Validating locally

`tools/validate.sh` walks `plugins/` and runs `go build ./...` in each module. It expects sibling checkouts of `Carlos` and `GoDiscord` two directories up (matching the `replace` directives in each plugin's `go.mod`).
