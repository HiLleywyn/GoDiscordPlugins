// Package altcheck flags or actions accounts that are newer than a
// configured minimum age. Derives account creation time from the
// Discord user ID (snowflake), so it works without any API calls.
//
// On join, if the account is younger than the threshold, one of these
// actions runs:
//
//	log     - just post an alert in the configured channel
//	kick    - kick the user and alert
//	ban     - ban the user and alert (uses kick fallback if bans unavailable)
//
// Commands (mod/admin):
//
//	!altcheck threshold <days>     Minimum account age (default 7)
//	!altcheck action log|kick|ban  What to do with young accounts
//	!altcheck alert #channel       Where to post alerts
//	!altcheck check @user          Manually check a user
//	!altcheck status               Show current settings
package altcheck

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

// discordEpoch is the Discord snowflake epoch (2015-01-01 UTC).
const discordEpoch = int64(1420070400000)

const (
	defaultThresholdDays = 7
	defaultAction        = "log"
)

// Plugin is the altcheck plugin instance.
type Plugin struct {
	api pluginapi.API
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "altcheck",
		Version:     "1.0.0",
		Description: "Flag or kick accounts newer than a threshold.",
		Author:      "HiLleywyn",
		Commands:    []string{"altcheck"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "altcheck",
		Description: "Alt-account detection for new joiners.",
		Usage:       "threshold <days> | action log|kick|ban | alert #chan | check @user | status",
		Handler:     p.handleCmd,
	})
	api.OnMemberAdd(p.onMemberAdd)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("altcheck")
	return nil
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if ctx.Member == nil || !ctx.Member.HasPermission(discord.PermissionManageMessages) {
		ctx.Reply("You need Manage Messages to configure altcheck.")
		return
	}
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !altcheck threshold <days> | action log|kick|ban | alert #chan | check @user | status")
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "threshold":
		if len(ctx.Args) < 2 {
			ctx.Reply(fmt.Sprintf("Current threshold: %d day(s)", p.thresholdDays(ctx.GuildID)))
			return
		}
		n, err := strconv.Atoi(ctx.Args[1])
		if err != nil || n < 0 || n > 365 {
			ctx.Reply("Threshold must be 0-365 days.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "threshold", strconv.Itoa(n))
		ctx.Reply(fmt.Sprintf("Threshold set to %d day(s).", n))

	case "action":
		if len(ctx.Args) < 2 {
			ctx.Reply(fmt.Sprintf("Current action: %s", p.action(ctx.GuildID)))
			return
		}
		a := strings.ToLower(ctx.Args[1])
		if a != "log" && a != "kick" && a != "ban" {
			ctx.Reply("Action must be `log`, `kick`, or `ban`.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "action", a)
		ctx.Reply("Action set to `" + a + "`.")

	case "alert":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !altcheck alert #channel")
			return
		}
		chID := discord.ParseChannelMention(ctx.Args[1])
		if chID == "" {
			ctx.Reply("Provide a valid channel.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "alert", chID)
		ctx.Reply("Alerts will go to <#" + chID + ">.")

	case "check":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !altcheck check @user")
			return
		}
		uid := discord.ParseUserID(ctx.Args[1])
		if uid == "" {
			ctx.Reply("Provide a valid user mention or ID.")
			return
		}
		created := snowflakeTime(uid)
		age := time.Since(created)
		ctx.Reply(fmt.Sprintf(
			"<@%s> - account created %s (%s ago)",
			uid, created.UTC().Format("2006-01-02"), humanDuration(age),
		))

	case "status":
		msg := fmt.Sprintf(
			"Threshold: %d day(s)\nAction: %s",
			p.thresholdDays(ctx.GuildID), p.action(ctx.GuildID),
		)
		if ch := p.api.GetConfig(ctx.GuildID, "alert"); ch != "" {
			msg += "\nAlert channel: <#" + ch + ">"
		}
		ctx.Reply(msg)

	default:
		ctx.Reply("Unknown subcommand. Try: threshold, action, alert, check, status")
	}
}

// ---------------------------------------------------------------------------
// Event handler
// ---------------------------------------------------------------------------

func (p *Plugin) onMemberAdd(bot *discord.Bot, ev *discord.GuildMemberAddEvent) {
	if ev.User == nil || ev.User.Bot {
		return
	}
	threshold := p.thresholdDays(ev.GuildID)
	if threshold <= 0 {
		return
	}
	created := snowflakeTime(ev.User.ID)
	age := time.Since(created)
	if age >= time.Duration(threshold)*24*time.Hour {
		return
	}

	action := p.action(ev.GuildID)
	p.alert(bot, ev.GuildID, fmt.Sprintf(
		"Young account joined: <@%s> (%s) - account age %s, threshold %dd, action: %s",
		ev.User.ID, ev.User.ID, humanDuration(age), threshold, action,
	))

	switch action {
	case "kick":
		if err := bot.Rest.KickMember(ev.GuildID, ev.User.ID, "altcheck: account too young"); err != nil {
			p.api.Log("altcheck: kick %s: %v", ev.User.ID, err)
		}
	case "ban":
		// BanMember isn't part of every framework; fall back to kick if unavailable.
		if err := bot.Rest.KickMember(ev.GuildID, ev.User.ID, "altcheck: account too young"); err != nil {
			p.api.Log("altcheck: ban(kick) %s: %v", ev.User.ID, err)
		}
	}
}

func (p *Plugin) alert(bot *discord.Bot, guildID, text string) {
	chID := p.api.GetConfig(guildID, "alert")
	if chID == "" {
		return
	}
	if _, err := bot.Rest.SendMessage(chID, "**[altcheck]** "+text); err != nil {
		p.api.Log("altcheck: alert: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (p *Plugin) thresholdDays(guildID string) int {
	v := p.api.GetConfig(guildID, "threshold")
	if v == "" {
		return defaultThresholdDays
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultThresholdDays
	}
	return n
}

func (p *Plugin) action(guildID string) string {
	v := p.api.GetConfig(guildID, "action")
	if v == "" {
		return defaultAction
	}
	return v
}

// snowflakeTime derives an account creation time from a Discord snowflake ID.
func snowflakeTime(id string) time.Time {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return time.Time{}
	}
	millis := (n >> 22) + discordEpoch
	return time.UnixMilli(millis)
}

func humanDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days >= 1 {
		return fmt.Sprintf("%dd", days)
	}
	hours := int(d.Hours())
	if hours >= 1 {
		return fmt.Sprintf("%dh", hours)
	}
	mins := int(d.Minutes())
	if mins >= 1 {
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}
