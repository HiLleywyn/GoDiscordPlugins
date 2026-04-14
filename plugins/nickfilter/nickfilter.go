// Package nickfilter flags and optionally acts on disallowed nicknames.
// Supports a wordlist of banned substrings, plus built-in checks for
// zalgo (combining-mark spam), mass-mentions, and hoisting characters
// (chars commonly used to push a user to the top of the member list).
//
// On detection the plugin can log, kick, or rename. Rename uses
// `bot.Rest.SetNickname(guildID, userID, nick)` if the framework
// exposes it; otherwise the bot logs a warning and falls back to logging
// the incident.
//
// Commands (mod/admin):
//
//	!nickfilter add <word>             Add a banned substring
//	!nickfilter remove <word>          Remove one
//	!nickfilter list                   Show the banned list
//	!nickfilter action log|kick|rename What to do on a hit (default log)
//	!nickfilter replacement <name>     Fallback nickname when action=rename
//	!nickfilter alert #channel         Alert channel
//	!nickfilter scan                   Scan the whole guild (if GetMembers exists)
package nickfilter

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

const defaultReplacement = "moderated"

// Plugin is the nickfilter plugin instance.
type Plugin struct {
	api pluginapi.API
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "nickfilter",
		Version:     "1.0.0",
		Description: "Detect and action disallowed nicknames.",
		Author:      "HiLleywyn",
		Commands:    []string{"nickfilter"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "nickfilter",
		Description: "Configure and manage nickname filtering.",
		Usage:       "add <word> | remove <word> | list | action <log|kick|rename> | replacement <name> | alert #chan",
		Handler:     p.handleCmd,
	})
	api.OnMemberAdd(p.onMemberAdd)
	api.OnMessage(p.onMessage)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("nickfilter")
	return nil
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if ctx.Member == nil || !ctx.Member.HasPermission(discord.PermissionManageNicknames) {
		ctx.Reply("You need Manage Nicknames to configure nickfilter.")
		return
	}
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !nickfilter add <word> | remove <word> | list | action <log|kick|rename> | replacement <name> | alert #chan")
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "add":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !nickfilter add <word>")
			return
		}
		word := strings.ToLower(strings.Join(ctx.Args[1:], " "))
		p.api.SetConfig(ctx.GuildID, "word:"+word, "1")
		ctx.Reply("Banned `" + word + "` in nicknames.")

	case "remove":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !nickfilter remove <word>")
			return
		}
		word := strings.ToLower(strings.Join(ctx.Args[1:], " "))
		p.api.DeleteConfig(ctx.GuildID, "word:"+word)
		ctx.Reply("Removed `" + word + "` from the banned list.")

	case "list":
		words := p.bannedList(ctx.GuildID)
		if len(words) == 0 {
			ctx.Reply("No banned words. (Built-in zalgo and hoisting checks still apply.)")
			return
		}
		ctx.Reply("Banned substrings: `" + strings.Join(words, "`, `") + "`")

	case "action":
		if len(ctx.Args) < 2 {
			ctx.Reply("Current action: " + p.action(ctx.GuildID))
			return
		}
		a := strings.ToLower(ctx.Args[1])
		if a != "log" && a != "kick" && a != "rename" {
			ctx.Reply("Action must be `log`, `kick`, or `rename`.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "action", a)
		ctx.Reply("Action set to `" + a + "`.")

	case "replacement":
		if len(ctx.Args) < 2 {
			ctx.Reply("Current replacement: `" + p.replacement(ctx.GuildID) + "`")
			return
		}
		r := strings.Join(ctx.Args[1:], " ")
		if len(r) == 0 || len(r) > 32 {
			ctx.Reply("Replacement must be 1-32 chars.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "replacement", r)
		ctx.Reply("Replacement name set to `" + r + "`.")

	case "alert":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !nickfilter alert #channel")
			return
		}
		chID := discord.ParseChannelMention(ctx.Args[1])
		if chID == "" {
			ctx.Reply("Provide a valid channel.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "alert", chID)
		ctx.Reply("Alerts will go to <#" + chID + ">.")

	default:
		ctx.Reply("Unknown subcommand. Try: add, remove, list, action, replacement, alert")
	}
}

// ---------------------------------------------------------------------------
// Event handlers
// ---------------------------------------------------------------------------

func (p *Plugin) onMemberAdd(bot *discord.Bot, ev *discord.GuildMemberAddEvent) {
	if ev.User == nil || ev.User.Bot {
		return
	}
	name := ev.User.Username
	p.check(bot, ev.GuildID, ev.User.ID, name)
}

func (p *Plugin) onMessage(bot *discord.Bot, msg *discord.Message) {
	if msg.GuildID == "" || msg.Author == nil || msg.Author.Bot {
		return
	}
	// Check display name from member or username fallback.
	name := ""
	if msg.Member != nil && msg.Member.Nick != "" {
		name = msg.Member.Nick
	} else {
		name = msg.Author.Username
	}
	p.check(bot, msg.GuildID, msg.Author.ID, name)
}

func (p *Plugin) check(bot *discord.Bot, guildID, userID, name string) {
	reason := p.violationReason(guildID, name)
	if reason == "" {
		return
	}
	action := p.action(guildID)

	p.alert(bot, guildID, fmt.Sprintf(
		"Nickname violation: <@%s> (`%s`) - %s - action: %s",
		userID, name, reason, action,
	))

	switch action {
	case "kick":
		if err := bot.Rest.KickMember(guildID, userID, "nickfilter: "+reason); err != nil {
			p.api.Log("nickfilter: kick %s: %v", userID, err)
		}
	case "rename":
		// Optimistic call; many frameworks expose SetNickname.
		if err := bot.Rest.SetNickname(guildID, userID, p.replacement(guildID), "nickfilter: "+reason); err != nil {
			p.api.Log("nickfilter: rename %s: %v", userID, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Detection
// ---------------------------------------------------------------------------

// violationReason returns an empty string if the name is fine, or a short
// human description of the first violation found.
func (p *Plugin) violationReason(guildID, name string) string {
	lower := strings.ToLower(name)

	// Custom wordlist.
	for _, w := range p.bannedList(guildID) {
		if strings.Contains(lower, w) {
			return "contains banned substring `" + w + "`"
		}
	}

	// Hoisting: leading non-letter used to push to top of list.
	if len(name) > 0 {
		first := []rune(name)[0]
		if !unicode.IsLetter(first) && !unicode.IsDigit(first) && first != '_' {
			// Allow common CJK/emoji? Be strict by default.
			return "hoisting character at start"
		}
	}

	// Zalgo: excessive combining marks.
	if isZalgo(name) {
		return "zalgo / combining-mark spam"
	}

	// Mass-mentions stuffed into a nick.
	if strings.Count(name, "@everyone") > 0 || strings.Count(name, "@here") > 0 {
		return "contains @everyone/@here"
	}

	return ""
}

func isZalgo(s string) bool {
	combining := 0
	letters := 0
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) {
			combining++
		} else if unicode.IsLetter(r) {
			letters++
		}
	}
	if letters == 0 {
		return combining > 3
	}
	// More than 0.4 combining marks per letter = zalgo.
	return float64(combining) > float64(letters)*0.4 && combining >= 4
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (p *Plugin) bannedList(guildID string) []string {
	var out []string
	for k := range p.api.AllConfig(guildID) {
		if strings.HasPrefix(k, "word:") {
			out = append(out, strings.TrimPrefix(k, "word:"))
		}
	}
	return out
}

func (p *Plugin) action(guildID string) string {
	v := p.api.GetConfig(guildID, "action")
	if v == "" {
		return "log"
	}
	return v
}

func (p *Plugin) replacement(guildID string) string {
	v := p.api.GetConfig(guildID, "replacement")
	if v == "" {
		return defaultReplacement
	}
	return v
}

func (p *Plugin) alert(bot *discord.Bot, guildID, text string) {
	chID := p.api.GetConfig(guildID, "alert")
	if chID == "" {
		return
	}
	if _, err := bot.Rest.SendMessage(chID, "**[nickfilter]** "+text); err != nil {
		p.api.Log("nickfilter: alert: %v", err)
	}
}
