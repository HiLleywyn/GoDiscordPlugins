# GoDiscordPlugins

Plugin collection for the [Carlos](https://github.com/hilleywyn/Carlos) Discord bot.

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

## Available plugins

| Plugin | Command | Description |
|--------|---------|-------------|
| starboard | `!starboard` | Reposts highly-starred messages to a dedicated channel |
| autoresponder | `!ar` | Responds to trigger keywords with preset replies |
| giveaway | `!giveaway` | Timed giveaways with reaction-based entry and winner selection |
| welcome | `!welcome` | Custom join and leave messages with variable substitution |
| slowmode | `!slowmode` | Auto-adjusts channel slowmode based on messages per minute |
