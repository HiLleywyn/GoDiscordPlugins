package welcome

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

// Plugin sends configurable messages when members join or leave a guild.
// Supports {user}, {server}, and {membercount} variable substitution.
type Plugin struct {
	api pluginapi.API
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "welcome",
		Version:     "1.0.0",
		Description: "Send custom messages when members join or leave.",
		Author:      "HiLleywyn",
		Commands:    []string{"welcome"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "welcome",
		Description: "Configure join and leave messages.",
		Usage:       "channel #chan | join <message> | leave <message> | test join|leave | show",
		Handler:     p.handleCmd,
	})
	api.OnMemberAdd(p.onMemberAdd)
	api.OnMemberRemove(p.onMemberRemove)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("welcome")
	return nil
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !welcome channel #chan | join <message> | leave <message> | test join|leave | show")
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "channel":
		if len(ctx.Args) < 2 {
			ctx.Reply("Provide a channel mention or ID.")
			return
		}
		chID := stripChannelMention(ctx.Args[1])
		p.api.SetConfig(ctx.GuildID, "channel", chID)
		ctx.Reply("Welcome channel set to <#" + chID + ">.")

	case "join":
		if len(ctx.Args) < 2 {
			ctx.Reply("Provide a message. Variables: {user}, {server}, {membercount}")
			return
		}
		msg := strings.Join(ctx.Args[1:], " ")
		p.api.SetConfig(ctx.GuildID, "join", msg)
		ctx.Reply("Join message set.")

	case "leave":
		if len(ctx.Args) < 2 {
			ctx.Reply("Provide a message. Variables: {user}, {server}, {membercount}")
			return
		}
		msg := strings.Join(ctx.Args[1:], " ")
		p.api.SetConfig(ctx.GuildID, "leave", msg)
		ctx.Reply("Leave message set.")

	case "test":
		if len(ctx.Args) < 2 {
			ctx.Reply("Specify 'join' or 'leave'.")
			return
		}
		chID := p.api.GetConfig(ctx.GuildID, "channel")
		if chID == "" {
			ctx.Reply("No channel set. Run !welcome channel #chan first.")
			return
		}
		switch strings.ToLower(ctx.Args[1]) {
		case "join":
			tmpl := p.api.GetConfig(ctx.GuildID, "join")
			if tmpl == "" {
				ctx.Reply("No join message set.")
				return
			}
			text := p.substitute(tmpl, ctx.Message.Author, ctx.GuildID)
			_, err := ctx.Bot.Rest.SendMessage(chID, text)
			if err != nil {
				ctx.Reply(fmt.Sprintf("Failed to send test message: %v", err))
			}
		case "leave":
			tmpl := p.api.GetConfig(ctx.GuildID, "leave")
			if tmpl == "" {
				ctx.Reply("No leave message set.")
				return
			}
			text := p.substitute(tmpl, ctx.Message.Author, ctx.GuildID)
			_, err := ctx.Bot.Rest.SendMessage(chID, text)
			if err != nil {
				ctx.Reply(fmt.Sprintf("Failed to send test message: %v", err))
			}
		default:
			ctx.Reply("Specify 'join' or 'leave'.")
		}

	case "show":
		ch := p.api.GetConfig(ctx.GuildID, "channel")
		if ch == "" {
			ch = "not set"
		} else {
			ch = "<#" + ch + ">"
		}
		join := p.api.GetConfig(ctx.GuildID, "join")
		if join == "" {
			join = "not set"
		}
		leave := p.api.GetConfig(ctx.GuildID, "leave")
		if leave == "" {
			leave = "not set"
		}
		ctx.Reply(fmt.Sprintf("channel: %s\njoin: %s\nleave: %s", ch, join, leave))

	default:
		ctx.Reply("Unknown subcommand. Try: channel, join, leave, test, show")
	}
}

// ---------------------------------------------------------------------------
// Event handlers
// ---------------------------------------------------------------------------

func (p *Plugin) onMemberAdd(bot *discord.Bot, ev *discord.GuildMemberAddEvent) {
	chID := p.api.GetConfig(ev.GuildID, "channel")
	if chID == "" {
		return
	}
	tmpl := p.api.GetConfig(ev.GuildID, "join")
	if tmpl == "" {
		return
	}
	text := p.substitute(tmpl, ev.User, ev.GuildID)
	_, err := bot.Rest.SendMessage(chID, text)
	if err != nil {
		p.api.Log("welcome: failed to send join message: %v", err)
	}
}

func (p *Plugin) onMemberRemove(bot *discord.Bot, ev *discord.GuildMemberRemoveEvent) {
	chID := p.api.GetConfig(ev.GuildID, "channel")
	if chID == "" {
		return
	}
	tmpl := p.api.GetConfig(ev.GuildID, "leave")
	if tmpl == "" {
		return
	}
	text := p.substitute(tmpl, ev.User, ev.GuildID)
	_, err := bot.Rest.SendMessage(chID, text)
	if err != nil {
		p.api.Log("welcome: failed to send leave message: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Variable substitution
// ---------------------------------------------------------------------------

// substitute replaces {user}, {server}, and {membercount} in a template.
// Member count is fetched via REST; on error it falls back to "?".
func (p *Plugin) substitute(tmpl string, user *discord.User, guildID string) string {
	if user == nil {
		tmpl = strings.ReplaceAll(tmpl, "{user}", "unknown")
	} else {
		tmpl = strings.ReplaceAll(tmpl, "{user}", user.Mention())
	}

	// {server} - guild name from REST
	guild, err := p.api.Rest().GetGuild(guildID)
	if err == nil && guild != nil {
		tmpl = strings.ReplaceAll(tmpl, "{server}", guild.Name)
		tmpl = strings.ReplaceAll(tmpl, "{membercount}", strconv.Itoa(guild.MemberCount))
	} else {
		tmpl = strings.ReplaceAll(tmpl, "{server}", guildID)
		tmpl = strings.ReplaceAll(tmpl, "{membercount}", "?")
	}

	return tmpl
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func stripChannelMention(s string) string {
	s = strings.TrimPrefix(s, "<#")
	s = strings.TrimSuffix(s, ">")
	return s
}
