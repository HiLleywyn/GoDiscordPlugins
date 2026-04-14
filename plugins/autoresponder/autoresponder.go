package autoresponder

import (
	"fmt"
	"strings"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

// Plugin implements keyword-based auto-responses.
// Trigger/response pairs are stored per guild. On every message the plugin
// checks for matching triggers (case-insensitive substring) and replies.
type Plugin struct {
	api pluginapi.API
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "autoresponder",
		Version:     "1.0.0",
		Description: "Respond to trigger keywords with preset replies.",
		Author:      "HiLleywyn",
		Commands:    []string{"ar"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "ar",
		Description: "Manage auto-responses.",
		Usage:       "add <trigger> <response> | remove <trigger> | list | toggle",
		Handler:     p.handleCmd,
	})
	api.OnMessage(p.onMessage)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("ar")
	return nil
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !ar add <trigger> <response> | remove <trigger> | list | toggle")
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "add":
		// !ar add <trigger> <response...>
		// Args[1] = trigger, rest = response
		if len(ctx.Args) < 3 {
			ctx.Reply("Usage: !ar add <trigger> <response>")
			return
		}
		trigger := strings.ToLower(ctx.Args[1])
		response := strings.Join(ctx.Args[2:], " ")
		p.api.SetConfig(ctx.GuildID, "trigger:"+trigger, response)
		ctx.Reply(fmt.Sprintf("Added trigger '%s'.", trigger))

	case "remove":
		if len(ctx.Args) < 2 {
			ctx.Reply("Provide the trigger to remove.")
			return
		}
		trigger := strings.ToLower(ctx.Args[1])
		p.api.DeleteConfig(ctx.GuildID, "trigger:"+trigger)
		ctx.Reply(fmt.Sprintf("Removed trigger '%s'.", trigger))

	case "list":
		all := p.api.AllConfig(ctx.GuildID)
		var pairs []string
		for k, v := range all {
			if strings.HasPrefix(k, "trigger:") {
				trig := strings.TrimPrefix(k, "trigger:")
				pairs = append(pairs, fmt.Sprintf("  %s -> %s", trig, v))
			}
		}
		if len(pairs) == 0 {
			ctx.Reply("No triggers set.")
			return
		}
		ctx.Reply("Triggers:\n" + strings.Join(pairs, "\n"))

	case "toggle":
		current := p.api.GetConfig(ctx.GuildID, "enabled")
		if current == "false" {
			p.api.SetConfig(ctx.GuildID, "enabled", "true")
			ctx.Reply("Auto-responder enabled.")
		} else {
			p.api.SetConfig(ctx.GuildID, "enabled", "false")
			ctx.Reply("Auto-responder disabled.")
		}

	default:
		ctx.Reply("Unknown subcommand. Try: add, remove, list, toggle")
	}
}

// ---------------------------------------------------------------------------
// Message handler
// ---------------------------------------------------------------------------

func (p *Plugin) onMessage(bot *discord.Bot, msg *discord.Message) {
	// Skip DMs, bot messages, and guilds with the responder disabled.
	if msg.GuildID == "" {
		return
	}
	if msg.Author != nil && msg.Author.Bot {
		return
	}
	if p.api.GetConfig(msg.GuildID, "enabled") == "false" {
		return
	}

	lower := strings.ToLower(msg.Content)
	all := p.api.AllConfig(msg.GuildID)

	sent := 0
	for k, v := range all {
		if sent >= 5 {
			break
		}
		if !strings.HasPrefix(k, "trigger:") {
			continue
		}
		trigger := strings.TrimPrefix(k, "trigger:")
		if strings.Contains(lower, trigger) {
			_, err := bot.Rest.SendMessage(msg.ChannelID, v)
			if err != nil {
				p.api.Log("autoresponder: send error: %v", err)
			}
			sent++
		}
	}
}
