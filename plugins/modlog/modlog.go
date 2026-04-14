// Package modlog provides a per-guild moderation log channel.
//
// Once a log channel is configured, the plugin posts embeds for each tracked
// event. Individual events can be toggled on or off.
//
// Tracked events:
//
//	delete - a message was deleted (content + author if the message was cached)
//	edit   - a message was edited (before/after if cached)
//	join   - a member joined the guild
//	leave  - a member left the guild
//	ban    - a user was banned
//	unban  - a user was unbanned
//
// Commands:
//
//	!modlog channel #chan      Set the log channel
//	!modlog enable <event>     Enable tracking for an event
//	!modlog disable <event>    Disable tracking for an event
//	!modlog show               Show current settings
package modlog

import (
	"fmt"
	"strings"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

// Plugin is the modlog plugin instance.
type Plugin struct {
	api pluginapi.API
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "modlog",
		Version:     "1.0.0",
		Description: "Log moderator-relevant events to a channel.",
		Author:      "HiLleywyn",
		Commands:    []string{"modlog"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "modlog",
		Description: "Configure the moderation log.",
		Usage:       "channel #chan | enable <event> | disable <event> | show",
		Handler:     p.handleCmd,
	})
	api.OnMessageDelete(p.onMessageDelete)
	api.OnMessageUpdate(p.onMessageUpdate)
	api.OnMemberAdd(p.onMemberAdd)
	api.OnMemberRemove(p.onMemberRemove)
	api.OnGuildBanAdd(p.onBanAdd)
	api.OnGuildBanRemove(p.onBanRemove)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("modlog")
	return nil
}

// allEvents lists every event name that can be toggled.
var allEvents = []string{"delete", "edit", "join", "leave", "ban", "unban"}

func (p *Plugin) channel(guildID string) string {
	return p.api.GetConfig(guildID, "channel")
}

// enabled reports whether a given event is being logged. Events default to
// enabled once a channel has been configured.
func (p *Plugin) enabled(guildID, event string) bool {
	return p.api.GetConfig(guildID, "event:"+event) != "false"
}

func (p *Plugin) post(guildID string, embed discord.Embed) {
	ch := p.channel(guildID)
	if ch == "" {
		return
	}
	if _, err := p.api.Rest().SendEmbed(ch, embed); err != nil {
		p.api.Log("modlog: send: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !modlog channel #chan | enable <event> | disable <event> | show")
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "channel":
		if len(ctx.Args) < 2 {
			ctx.Reply("Provide a channel mention or ID.")
			return
		}
		chID := discord.ParseChannelMention(ctx.Args[1])
		p.api.SetConfig(ctx.GuildID, "channel", chID)
		ctx.Reply("Mod log channel set to <#" + chID + ">.")

	case "enable", "disable":
		if len(ctx.Args) < 2 {
			ctx.Reply("Event names: " + strings.Join(allEvents, ", "))
			return
		}
		ev := strings.ToLower(ctx.Args[1])
		if !knownEvent(ev) {
			ctx.Reply("Unknown event '" + ev + "'. Valid: " + strings.Join(allEvents, ", "))
			return
		}
		if ctx.Args[0] == "enable" {
			p.api.SetConfig(ctx.GuildID, "event:"+ev, "true")
			ctx.Reply("Enabled event: " + ev)
		} else {
			p.api.SetConfig(ctx.GuildID, "event:"+ev, "false")
			ctx.Reply("Disabled event: " + ev)
		}

	case "show":
		ch := p.channel(ctx.GuildID)
		if ch == "" {
			ch = "not set"
		} else {
			ch = "<#" + ch + ">"
		}
		var flags []string
		for _, ev := range allEvents {
			mark := "on"
			if !p.enabled(ctx.GuildID, ev) {
				mark = "off"
			}
			flags = append(flags, fmt.Sprintf("%s=%s", ev, mark))
		}
		ctx.Reply(fmt.Sprintf("channel: %s\n%s", ch, strings.Join(flags, "  ")))

	default:
		ctx.Reply("Unknown subcommand. Try: channel, enable, disable, show")
	}
}

func knownEvent(ev string) bool {
	for _, e := range allEvents {
		if e == ev {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Event handlers
// ---------------------------------------------------------------------------

func (p *Plugin) onMessageDelete(bot *discord.Bot, ev *discord.MessageDeleteEvent) {
	if ev.GuildID == "" || !p.enabled(ev.GuildID, "delete") || p.channel(ev.GuildID) == "" {
		return
	}

	fields := []discord.EmbedField{
		{Name: "Channel", Value: "<#" + ev.ChannelID + ">", Inline: true},
		{Name: "Message ID", Value: ev.MessageID, Inline: true},
	}
	if ev.CachedMessage != nil {
		if ev.CachedMessage.Author != nil {
			if ev.CachedMessage.Author.Bot {
				return
			}
			fields = append(fields, discord.EmbedField{
				Name:   "Author",
				Value:  ev.CachedMessage.Author.Mention(),
				Inline: true,
			})
		}
		if ev.CachedMessage.Content != "" {
			fields = append(fields, discord.EmbedField{
				Name:  "Content",
				Value: truncate(ev.CachedMessage.Content, 1024),
			})
		}
	}

	p.post(ev.GuildID, discord.Embed{
		Title:  "Message Deleted",
		Color:  0xED4245,
		Fields: fields,
	})
}

func (p *Plugin) onMessageUpdate(bot *discord.Bot, ev *discord.MessageUpdateEvent) {
	if ev.GuildID == "" || !p.enabled(ev.GuildID, "edit") || p.channel(ev.GuildID) == "" {
		return
	}
	if ev.NewMessage == nil || ev.NewMessage.Author == nil || ev.NewMessage.Author.Bot {
		return
	}

	before := ""
	if ev.OldMessage != nil {
		before = ev.OldMessage.Content
	}
	after := ev.NewMessage.Content
	if before == after {
		return
	}

	fields := []discord.EmbedField{
		{Name: "Author", Value: ev.NewMessage.Author.Mention(), Inline: true},
		{Name: "Channel", Value: "<#" + ev.ChannelID + ">", Inline: true},
	}
	if before != "" {
		fields = append(fields, discord.EmbedField{
			Name:  "Before",
			Value: truncate(before, 1024),
		})
	}
	fields = append(fields, discord.EmbedField{
		Name:  "After",
		Value: truncate(after, 1024),
	})

	p.post(ev.GuildID, discord.Embed{
		Title:  "Message Edited",
		Color:  0xFEE75C,
		Fields: fields,
	})
}

func (p *Plugin) onMemberAdd(bot *discord.Bot, ev *discord.GuildMemberAddEvent) {
	if !p.enabled(ev.GuildID, "join") || p.channel(ev.GuildID) == "" || ev.User == nil {
		return
	}
	p.post(ev.GuildID, discord.Embed{
		Title: "Member Joined",
		Color: 0x57F287,
		Fields: []discord.EmbedField{
			{Name: "User", Value: ev.User.Mention(), Inline: true},
			{Name: "Tag", Value: ev.User.Tag(), Inline: true},
			{Name: "ID", Value: ev.User.ID, Inline: true},
		},
	})
}

func (p *Plugin) onMemberRemove(bot *discord.Bot, ev *discord.GuildMemberRemoveEvent) {
	if !p.enabled(ev.GuildID, "leave") || p.channel(ev.GuildID) == "" || ev.User == nil {
		return
	}
	p.post(ev.GuildID, discord.Embed{
		Title: "Member Left",
		Color: 0x747F8D,
		Fields: []discord.EmbedField{
			{Name: "User", Value: ev.User.Tag(), Inline: true},
			{Name: "ID", Value: ev.User.ID, Inline: true},
		},
	})
}

func (p *Plugin) onBanAdd(bot *discord.Bot, ev *discord.GuildBanAddEvent) {
	if !p.enabled(ev.GuildID, "ban") || p.channel(ev.GuildID) == "" || ev.User == nil {
		return
	}
	p.post(ev.GuildID, discord.Embed{
		Title: "User Banned",
		Color: 0x992D22,
		Fields: []discord.EmbedField{
			{Name: "User", Value: ev.User.Tag(), Inline: true},
			{Name: "ID", Value: ev.User.ID, Inline: true},
		},
	})
}

func (p *Plugin) onBanRemove(bot *discord.Bot, ev *discord.GuildBanRemoveEvent) {
	if !p.enabled(ev.GuildID, "unban") || p.channel(ev.GuildID) == "" || ev.User == nil {
		return
	}
	p.post(ev.GuildID, discord.Embed{
		Title: "User Unbanned",
		Color: 0x2ECC71,
		Fields: []discord.EmbedField{
			{Name: "User", Value: ev.User.Tag(), Inline: true},
			{Name: "ID", Value: ev.User.ID, Inline: true},
		},
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
