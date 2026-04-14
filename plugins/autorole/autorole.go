// Package autorole assigns one or more roles to every new guild member
// automatically on join.
//
// Commands (mod/admin):
//
//	!autorole add @role       Add a role to the auto-assign list
//	!autorole remove @role    Remove a role from the list
//	!autorole list            Show configured roles
//	!autorole delay <seconds> Delay before applying (e.g. to wait for screening)
//	!autorole clear           Remove every configured role
package autorole

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

// Plugin is the autorole plugin instance.
type Plugin struct {
	api pluginapi.API
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "autorole",
		Version:     "1.0.0",
		Description: "Assign roles automatically when a member joins.",
		Author:      "HiLleywyn",
		Commands:    []string{"autorole"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "autorole",
		Description: "Configure roles that get assigned automatically on join.",
		Usage:       "add @role | remove @role | list | delay <seconds> | clear",
		Handler:     p.handleCmd,
	})
	api.OnMemberAdd(p.onMemberAdd)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("autorole")
	return nil
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !autorole add @role | remove @role | list | delay <seconds> | clear")
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "add":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !autorole add @role")
			return
		}
		roleID := discord.ParseRoleMention(ctx.Args[1])
		if roleID == "" {
			ctx.Reply("Provide a valid role mention or ID.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "role:"+roleID, "true")
		ctx.Reply(fmt.Sprintf("Role <@&%s> will be assigned to new members.", roleID))

	case "remove":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !autorole remove @role")
			return
		}
		roleID := discord.ParseRoleMention(ctx.Args[1])
		p.api.DeleteConfig(ctx.GuildID, "role:"+roleID)
		ctx.Reply(fmt.Sprintf("Role <@&%s> removed from auto-assign.", roleID))

	case "list":
		roles := p.configuredRoles(ctx.GuildID)
		if len(roles) == 0 {
			ctx.Reply("No auto-roles configured.")
			return
		}
		var mentions []string
		for _, r := range roles {
			mentions = append(mentions, fmt.Sprintf("<@&%s>", r))
		}
		delay := p.intConfig(ctx.GuildID, "delay", 0)
		msg := "Auto-roles: " + strings.Join(mentions, " ")
		if delay > 0 {
			msg += fmt.Sprintf("\nDelay before applying: %ds", delay)
		}
		ctx.Reply(msg)

	case "delay":
		if len(ctx.Args) < 2 {
			ctx.Reply(fmt.Sprintf("Current delay: %ds", p.intConfig(ctx.GuildID, "delay", 0)))
			return
		}
		n, err := strconv.Atoi(ctx.Args[1])
		if err != nil || n < 0 || n > 3600 {
			ctx.Reply("Delay must be 0-3600 seconds.")
			return
		}
		if n == 0 {
			p.api.DeleteConfig(ctx.GuildID, "delay")
		} else {
			p.api.SetConfig(ctx.GuildID, "delay", strconv.Itoa(n))
		}
		ctx.Reply(fmt.Sprintf("Delay set to %ds.", n))

	case "clear":
		for _, r := range p.configuredRoles(ctx.GuildID) {
			p.api.DeleteConfig(ctx.GuildID, "role:"+r)
		}
		ctx.Reply("All auto-roles cleared.")

	default:
		ctx.Reply("Unknown subcommand. Try: add, remove, list, delay, clear")
	}
}

// ---------------------------------------------------------------------------
// Event handler
// ---------------------------------------------------------------------------

func (p *Plugin) onMemberAdd(bot *discord.Bot, ev *discord.GuildMemberAddEvent) {
	if ev.User == nil || ev.User.Bot {
		return
	}
	roles := p.configuredRoles(ev.GuildID)
	if len(roles) == 0 {
		return
	}

	delay := p.intConfig(ev.GuildID, "delay", 0)
	apply := func() {
		for _, roleID := range roles {
			if err := bot.Rest.AddMemberRole(ev.GuildID, ev.User.ID, roleID); err != nil {
				p.api.Log("autorole: add %s to %s: %v", roleID, ev.User.ID, err)
			}
		}
	}
	if delay > 0 {
		go func() {
			time.Sleep(time.Duration(delay) * time.Second)
			apply()
		}()
	} else {
		apply()
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (p *Plugin) configuredRoles(guildID string) []string {
	var out []string
	for k := range p.api.AllConfig(guildID) {
		if strings.HasPrefix(k, "role:") {
			out = append(out, strings.TrimPrefix(k, "role:"))
		}
	}
	return out
}

func (p *Plugin) intConfig(guildID, key string, def int) int {
	v := p.api.GetConfig(guildID, key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
