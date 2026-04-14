// Package verify gates new members behind a manual verification step.
//
// Flow: on join, the user is given an "unverified" role (configured per
// guild) which should be the only role they have, restricting their
// access to a single #verify channel. Running `!verify` there swaps the
// role to the configured "verified" role.
//
// An optional phrase can be required to pass - users must send the exact
// phrase as their verification. Simple, but stops button-click bots.
//
// Commands:
//
//	!verify                            Complete verification
//	!verify setup @unverified @verified  Admin: configure roles
//	!verify phrase <text>              Admin: set required phrase (or "" to clear)
//	!verify channel #chan              Admin: limit !verify to one channel
//	!verify status                     Show current settings
package verify

import (
	"strings"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

// Plugin is the verify plugin instance.
type Plugin struct {
	api pluginapi.API
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "verify",
		Version:     "1.0.0",
		Description: "Gate new members behind a manual verification step.",
		Author:      "HiLleywyn",
		Commands:    []string{"verify"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "verify",
		Description: "Complete verification and get access to the server.",
		Usage:       "[setup @unverified @verified | phrase <text> | channel #chan | status]",
		Handler:     p.handleCmd,
	})
	api.OnMemberAdd(p.onMemberAdd)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("verify")
	return nil
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if len(ctx.Args) > 0 {
		switch strings.ToLower(ctx.Args[0]) {
		case "setup":
			p.cmdSetup(ctx)
			return
		case "phrase":
			p.cmdPhrase(ctx)
			return
		case "channel":
			p.cmdChannel(ctx)
			return
		case "status":
			p.cmdStatus(ctx)
			return
		}
	}
	p.cmdVerify(ctx)
}

func (p *Plugin) cmdVerify(ctx *discord.CommandContext) {
	unverified := p.api.GetConfig(ctx.GuildID, "unverified")
	verified := p.api.GetConfig(ctx.GuildID, "verified")
	if unverified == "" || verified == "" {
		ctx.Reply("Verification is not configured in this server.")
		return
	}

	// Optional channel restriction.
	if ch := p.api.GetConfig(ctx.GuildID, "channel"); ch != "" && ch != ctx.ChannelID {
		ctx.Reply("Please run `!verify` in <#" + ch + ">.")
		return
	}

	// Optional phrase check.
	if phrase := p.api.GetConfig(ctx.GuildID, "phrase"); phrase != "" {
		provided := strings.TrimSpace(strings.Join(ctx.Args, " "))
		if !strings.EqualFold(provided, phrase) {
			ctx.Reply("Wrong phrase. Read the rules and try again.")
			return
		}
	}

	// Already verified?
	if ctx.Member != nil {
		for _, r := range ctx.Member.Roles {
			if r == verified {
				ctx.Reply("You're already verified.")
				return
			}
		}
	}

	if err := ctx.Bot.Rest.AddMemberRole(ctx.GuildID, ctx.AuthorID, verified); err != nil {
		p.api.Log("verify: add %s: %v", verified, err)
		ctx.Reply("Couldn't grant the verified role - contact a mod.")
		return
	}
	if err := ctx.Bot.Rest.RemoveMemberRole(ctx.GuildID, ctx.AuthorID, unverified); err != nil {
		p.api.Log("verify: remove %s: %v", unverified, err)
	}
	ctx.Reply("Verified. Welcome in.")
}

func (p *Plugin) cmdSetup(ctx *discord.CommandContext) {
	if !isAdmin(ctx) {
		ctx.Reply("You need Manage Roles to set this up.")
		return
	}
	if len(ctx.Args) < 3 {
		ctx.Reply("Usage: !verify setup @unverified @verified")
		return
	}
	unv := discord.ParseRoleMention(ctx.Args[1])
	ver := discord.ParseRoleMention(ctx.Args[2])
	if unv == "" || ver == "" {
		ctx.Reply("Provide two valid role mentions.")
		return
	}
	p.api.SetConfig(ctx.GuildID, "unverified", unv)
	p.api.SetConfig(ctx.GuildID, "verified", ver)
	ctx.Reply("Verification configured. New joiners will get <@&" + unv + ">.")
}

func (p *Plugin) cmdPhrase(ctx *discord.CommandContext) {
	if !isAdmin(ctx) {
		ctx.Reply("You need Manage Roles for this.")
		return
	}
	if len(ctx.Args) < 2 {
		p.api.DeleteConfig(ctx.GuildID, "phrase")
		ctx.Reply("Phrase cleared; `!verify` with no argument will pass.")
		return
	}
	phrase := strings.Join(ctx.Args[1:], " ")
	p.api.SetConfig(ctx.GuildID, "phrase", phrase)
	ctx.Reply("Phrase set. Users must run `!verify " + phrase + "` to pass.")
}

func (p *Plugin) cmdChannel(ctx *discord.CommandContext) {
	if !isAdmin(ctx) {
		ctx.Reply("You need Manage Roles for this.")
		return
	}
	if len(ctx.Args) < 2 {
		p.api.DeleteConfig(ctx.GuildID, "channel")
		ctx.Reply("Channel restriction cleared.")
		return
	}
	chID := discord.ParseChannelMention(ctx.Args[1])
	if chID == "" {
		ctx.Reply("Provide a valid channel.")
		return
	}
	p.api.SetConfig(ctx.GuildID, "channel", chID)
	ctx.Reply("`!verify` will only work in <#" + chID + ">.")
}

func (p *Plugin) cmdStatus(ctx *discord.CommandContext) {
	unv := p.api.GetConfig(ctx.GuildID, "unverified")
	ver := p.api.GetConfig(ctx.GuildID, "verified")
	if unv == "" || ver == "" {
		ctx.Reply("Not configured. Run `!verify setup @unverified @verified`.")
		return
	}
	msg := "Unverified role: <@&" + unv + ">\nVerified role: <@&" + ver + ">"
	if ch := p.api.GetConfig(ctx.GuildID, "channel"); ch != "" {
		msg += "\nChannel: <#" + ch + ">"
	}
	if phrase := p.api.GetConfig(ctx.GuildID, "phrase"); phrase != "" {
		msg += "\nPhrase: `" + phrase + "`"
	}
	ctx.Reply(msg)
}

// ---------------------------------------------------------------------------
// Event handler
// ---------------------------------------------------------------------------

func (p *Plugin) onMemberAdd(bot *discord.Bot, ev *discord.GuildMemberAddEvent) {
	if ev.User == nil || ev.User.Bot {
		return
	}
	unverified := p.api.GetConfig(ev.GuildID, "unverified")
	if unverified == "" {
		return
	}
	if err := bot.Rest.AddMemberRole(ev.GuildID, ev.User.ID, unverified); err != nil {
		p.api.Log("verify: onJoin add %s: %v", unverified, err)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func isAdmin(ctx *discord.CommandContext) bool {
	if ctx.Member == nil {
		return false
	}
	return ctx.Member.HasPermission(discord.PermissionManageRoles)
}
