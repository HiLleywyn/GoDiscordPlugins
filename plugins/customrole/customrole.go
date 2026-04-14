// Package customrole lets eligible users claim one personal role from a
// pre-seeded pool. Typical use case: boosters or patreons get to pick a
// personal colored role from a curated palette.
//
// Admins pre-create roles (with any name/color they want) and add them
// to the pool with `!customrole pool add @role`. Eligible users then
// `!customrole pick @role` to claim one. Picking a new role releases
// the old one.
//
// Eligibility can be gated by a required role (e.g. "Server Booster").
//
// Commands:
//
//	!customrole pick @role         Claim a role from the pool
//	!customrole drop               Release your current custom role
//	!customrole list               Show available roles in the pool
//	!customrole pool add @role     Admin: add a role to the pool
//	!customrole pool remove @role  Admin: remove one
//	!customrole requires @role     Admin: require holding this role to pick
//	!customrole requires clear     Admin: no eligibility requirement
package customrole

import (
	"strings"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

// Plugin is the customrole plugin instance.
type Plugin struct {
	api pluginapi.API
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "customrole",
		Version:     "1.0.0",
		Description: "Claim a personal role from a curated pool.",
		Author:      "HiLleywyn",
		Commands:    []string{"customrole", "crole"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "customrole",
		Aliases:     []string{"crole"},
		Description: "Claim a personal role from the configured pool.",
		Usage:       "pick @role | drop | list | pool add @role | pool remove @role | requires @role | requires clear",
		Handler:     p.handleCmd,
	})
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("customrole")
	return nil
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !customrole pick @role | drop | list | pool add/remove @role | requires @role")
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "pick":
		p.cmdPick(ctx)
	case "drop":
		p.cmdDrop(ctx)
	case "list":
		p.cmdList(ctx)
	case "pool":
		p.cmdPool(ctx)
	case "requires":
		p.cmdRequires(ctx)
	default:
		ctx.Reply("Unknown subcommand.")
	}
}

func (p *Plugin) cmdPick(ctx *discord.CommandContext) {
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !customrole pick @role")
		return
	}
	roleID := discord.ParseRoleMention(ctx.Args[1])
	if roleID == "" {
		ctx.Reply("Provide a valid role.")
		return
	}
	if !p.inPool(ctx.GuildID, roleID) {
		ctx.Reply("That role isn't in the pool. Try `!customrole list`.")
		return
	}

	// Eligibility check.
	if req := p.api.GetConfig(ctx.GuildID, "requires"); req != "" {
		if ctx.Member == nil || !hasRole(ctx.Member, req) {
			ctx.Reply("You need <@&" + req + "> to pick a custom role.")
			return
		}
	}

	// Drop the previous pick if any.
	prev := p.api.GetConfig(ctx.GuildID, "u:"+ctx.AuthorID)
	if prev != "" && prev != roleID {
		if err := ctx.Bot.Rest.RemoveMemberRole(ctx.GuildID, ctx.AuthorID, prev); err != nil {
			p.api.Log("customrole: remove prev %s: %v", prev, err)
		}
	}

	if err := ctx.Bot.Rest.AddMemberRole(ctx.GuildID, ctx.AuthorID, roleID); err != nil {
		ctx.Reply("Failed to assign the role. Is the bot role high enough?")
		return
	}
	p.api.SetConfig(ctx.GuildID, "u:"+ctx.AuthorID, roleID)
	ctx.Reply("You now have <@&" + roleID + ">.")
}

func (p *Plugin) cmdDrop(ctx *discord.CommandContext) {
	prev := p.api.GetConfig(ctx.GuildID, "u:"+ctx.AuthorID)
	if prev == "" {
		ctx.Reply("You don't have a custom role.")
		return
	}
	if err := ctx.Bot.Rest.RemoveMemberRole(ctx.GuildID, ctx.AuthorID, prev); err != nil {
		p.api.Log("customrole: drop %s: %v", prev, err)
	}
	p.api.DeleteConfig(ctx.GuildID, "u:"+ctx.AuthorID)
	ctx.Reply("Custom role dropped.")
}

func (p *Plugin) cmdList(ctx *discord.CommandContext) {
	pool := p.poolList(ctx.GuildID)
	if len(pool) == 0 {
		ctx.Reply("The pool is empty. An admin needs to add roles with `!customrole pool add @role`.")
		return
	}
	var mentions []string
	for _, r := range pool {
		mentions = append(mentions, "<@&"+r+">")
	}
	msg := "Available custom roles: " + strings.Join(mentions, " ")
	if req := p.api.GetConfig(ctx.GuildID, "requires"); req != "" {
		msg += "\nRequires: <@&" + req + ">"
	}
	ctx.Reply(msg)
}

func (p *Plugin) cmdPool(ctx *discord.CommandContext) {
	if !isAdmin(ctx) {
		ctx.Reply("You need Manage Roles for this.")
		return
	}
	if len(ctx.Args) < 3 {
		ctx.Reply("Usage: !customrole pool add @role | pool remove @role")
		return
	}
	roleID := discord.ParseRoleMention(ctx.Args[2])
	if roleID == "" {
		ctx.Reply("Provide a valid role.")
		return
	}
	switch strings.ToLower(ctx.Args[1]) {
	case "add":
		p.api.SetConfig(ctx.GuildID, "pool:"+roleID, "1")
		ctx.Reply("Added <@&" + roleID + "> to the pool.")
	case "remove", "rm", "del":
		p.api.DeleteConfig(ctx.GuildID, "pool:"+roleID)
		ctx.Reply("Removed <@&" + roleID + "> from the pool.")
	default:
		ctx.Reply("Usage: !customrole pool add|remove @role")
	}
}

func (p *Plugin) cmdRequires(ctx *discord.CommandContext) {
	if !isAdmin(ctx) {
		ctx.Reply("You need Manage Roles for this.")
		return
	}
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !customrole requires @role | requires clear")
		return
	}
	if strings.EqualFold(ctx.Args[1], "clear") {
		p.api.DeleteConfig(ctx.GuildID, "requires")
		ctx.Reply("Eligibility requirement cleared.")
		return
	}
	roleID := discord.ParseRoleMention(ctx.Args[1])
	if roleID == "" {
		ctx.Reply("Provide a valid role.")
		return
	}
	p.api.SetConfig(ctx.GuildID, "requires", roleID)
	ctx.Reply("Only members with <@&" + roleID + "> can pick a custom role.")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (p *Plugin) inPool(guildID, roleID string) bool {
	return p.api.GetConfig(guildID, "pool:"+roleID) != ""
}

func (p *Plugin) poolList(guildID string) []string {
	var out []string
	for k := range p.api.AllConfig(guildID) {
		if strings.HasPrefix(k, "pool:") {
			out = append(out, strings.TrimPrefix(k, "pool:"))
		}
	}
	return out
}

func hasRole(m *discord.Member, roleID string) bool {
	for _, r := range m.Roles {
		if r == roleID {
			return true
		}
	}
	return false
}

func isAdmin(ctx *discord.CommandContext) bool {
	if ctx.Member == nil {
		return false
	}
	return ctx.Member.HasPermission(discord.PermissionManageRoles)
}
