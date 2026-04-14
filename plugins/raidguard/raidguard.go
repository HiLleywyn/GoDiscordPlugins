// Package raidguard detects join floods and reacts before a raid can do
// damage. When more than `threshold` members join within `window` seconds,
// the plugin enters "raid mode": new joiners are automatically kicked
// (or timed out) and a mod channel gets an alert.
//
// Raid mode auto-clears after `cooldown` seconds of quiet, or manually
// with `!raidguard off`.
//
// Commands (mod/admin):
//
//	!raidguard on                 Force raid mode on
//	!raidguard off                Force raid mode off
//	!raidguard status             Show current mode + recent joins
//	!raidguard threshold <n>      Joins required to trigger (default 8)
//	!raidguard window <secs>      Time window for joins (default 10)
//	!raidguard cooldown <secs>    Seconds of quiet before auto-clearing (default 120)
//	!raidguard action kick|timeout Action taken on raiders (default kick)
//	!raidguard alert #channel     Where to post raid alerts
package raidguard

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

const (
	defaultThreshold = 8
	defaultWindowSec = 10
	defaultCooldown  = 120
	defaultAction    = "kick"
)

// guildState holds per-guild raid detection state.
type guildState struct {
	joins    []time.Time
	raidMode bool
	modeSet  time.Time
}

// Plugin is the raidguard plugin instance.
type Plugin struct {
	api pluginapi.API

	mu     sync.Mutex
	states map[string]*guildState
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "raidguard",
		Version:     "1.0.0",
		Description: "Detect join floods and auto-kick/timeout raiders.",
		Author:      "HiLleywyn",
		Commands:    []string{"raidguard"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	p.states = make(map[string]*guildState)
	api.AddCommand(&discord.Command{
		Name:        "raidguard",
		Description: "Anti-raid protection.",
		Usage:       "on | off | status | threshold <n> | window <s> | cooldown <s> | action kick|timeout | alert #chan",
		Handler:     p.handleCmd,
	})
	api.OnMemberAdd(p.onMemberAdd)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("raidguard")
	return nil
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if ctx.Member == nil || !ctx.Member.HasPermission(discord.PermissionManageMessages) {
		ctx.Reply("You need Manage Messages to configure raidguard.")
		return
	}
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !raidguard on | off | status | threshold <n> | window <s> | cooldown <s> | action kick|timeout | alert #chan")
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "on":
		p.setMode(ctx.GuildID, true)
		ctx.Reply("Raid mode **enabled**. New joiners will be actioned automatically.")
	case "off":
		p.setMode(ctx.GuildID, false)
		ctx.Reply("Raid mode **disabled**.")
	case "status":
		st := p.state(ctx.GuildID)
		p.mu.Lock()
		defer p.mu.Unlock()
		recent := countWithin(st.joins, time.Duration(p.intCfg(ctx.GuildID, "window", defaultWindowSec))*time.Second)
		mode := "off"
		if st.raidMode {
			mode = "ON (since " + st.modeSet.Format("15:04:05") + ")"
		}
		ctx.Reply(fmt.Sprintf(
			"Raid mode: **%s**\nRecent joins in window: %d / %d\nAction: %s",
			mode, recent, p.intCfg(ctx.GuildID, "threshold", defaultThreshold),
			p.strCfg(ctx.GuildID, "action", defaultAction),
		))
	case "threshold":
		if len(ctx.Args) < 2 {
			ctx.Reply(fmt.Sprintf("Current threshold: %d", p.intCfg(ctx.GuildID, "threshold", defaultThreshold)))
			return
		}
		n, err := strconv.Atoi(ctx.Args[1])
		if err != nil || n < 2 || n > 100 {
			ctx.Reply("Threshold must be 2-100.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "threshold", strconv.Itoa(n))
		ctx.Reply(fmt.Sprintf("Threshold set to %d.", n))
	case "window":
		if len(ctx.Args) < 2 {
			ctx.Reply(fmt.Sprintf("Current window: %ds", p.intCfg(ctx.GuildID, "window", defaultWindowSec)))
			return
		}
		n, err := strconv.Atoi(ctx.Args[1])
		if err != nil || n < 1 || n > 600 {
			ctx.Reply("Window must be 1-600 seconds.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "window", strconv.Itoa(n))
		ctx.Reply(fmt.Sprintf("Window set to %ds.", n))
	case "cooldown":
		if len(ctx.Args) < 2 {
			ctx.Reply(fmt.Sprintf("Current cooldown: %ds", p.intCfg(ctx.GuildID, "cooldown", defaultCooldown)))
			return
		}
		n, err := strconv.Atoi(ctx.Args[1])
		if err != nil || n < 10 || n > 3600 {
			ctx.Reply("Cooldown must be 10-3600 seconds.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "cooldown", strconv.Itoa(n))
		ctx.Reply(fmt.Sprintf("Cooldown set to %ds.", n))
	case "action":
		if len(ctx.Args) < 2 {
			ctx.Reply(fmt.Sprintf("Current action: %s", p.strCfg(ctx.GuildID, "action", defaultAction)))
			return
		}
		a := strings.ToLower(ctx.Args[1])
		if a != "kick" && a != "timeout" {
			ctx.Reply("Action must be `kick` or `timeout`.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "action", a)
		ctx.Reply("Action set to `" + a + "`.")
	case "alert":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !raidguard alert #channel")
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
		ctx.Reply("Unknown subcommand. Try: on, off, status, threshold, window, cooldown, action, alert")
	}
}

// ---------------------------------------------------------------------------
// Event handler
// ---------------------------------------------------------------------------

func (p *Plugin) onMemberAdd(bot *discord.Bot, ev *discord.GuildMemberAddEvent) {
	if ev.User == nil || ev.User.Bot {
		return
	}

	threshold := p.intCfg(ev.GuildID, "threshold", defaultThreshold)
	window := time.Duration(p.intCfg(ev.GuildID, "window", defaultWindowSec)) * time.Second
	cooldown := time.Duration(p.intCfg(ev.GuildID, "cooldown", defaultCooldown)) * time.Second
	action := p.strCfg(ev.GuildID, "action", defaultAction)

	st := p.state(ev.GuildID)

	p.mu.Lock()
	// Prune outside the window.
	now := time.Now()
	cutoff := now.Add(-window)
	kept := st.joins[:0]
	for _, t := range st.joins {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	st.joins = append(kept, now)

	// Auto-clear raid mode after cooldown of quiet.
	if st.raidMode && now.Sub(st.modeSet) > cooldown && len(st.joins) == 1 {
		st.raidMode = false
	}

	trigger := !st.raidMode && len(st.joins) >= threshold
	if trigger {
		st.raidMode = true
		st.modeSet = now
	}
	inRaid := st.raidMode
	p.mu.Unlock()

	if trigger {
		p.alert(bot, ev.GuildID, fmt.Sprintf(
			"RAID DETECTED: %d joins in %s. Entering raid mode (action: %s).",
			threshold, window, action))
	}
	if inRaid {
		p.action(bot, ev.GuildID, ev.User.ID, action)
	}
}

func (p *Plugin) action(bot *discord.Bot, guildID, userID, action string) {
	switch action {
	case "timeout":
		until := time.Now().Add(24 * time.Hour)
		if err := bot.Rest.TimeoutMember(guildID, userID, until, "raidguard auto-timeout"); err != nil {
			p.api.Log("raidguard: timeout %s: %v", userID, err)
		}
	default: // kick
		if err := bot.Rest.KickMember(guildID, userID, "raidguard auto-kick"); err != nil {
			p.api.Log("raidguard: kick %s: %v", userID, err)
		}
	}
}

func (p *Plugin) alert(bot *discord.Bot, guildID, text string) {
	chID := p.api.GetConfig(guildID, "alert")
	if chID == "" {
		return
	}
	if _, err := bot.Rest.SendMessage(chID, "**[raidguard]** "+text); err != nil {
		p.api.Log("raidguard: alert: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (p *Plugin) state(guildID string) *guildState {
	p.mu.Lock()
	defer p.mu.Unlock()
	st, ok := p.states[guildID]
	if !ok {
		st = &guildState{}
		p.states[guildID] = st
	}
	return st
}

func (p *Plugin) setMode(guildID string, on bool) {
	st := p.state(guildID)
	p.mu.Lock()
	defer p.mu.Unlock()
	st.raidMode = on
	st.modeSet = time.Now()
}

func (p *Plugin) intCfg(guildID, key string, def int) int {
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

func (p *Plugin) strCfg(guildID, key, def string) string {
	v := p.api.GetConfig(guildID, key)
	if v == "" {
		return def
	}
	return v
}

func countWithin(ts []time.Time, window time.Duration) int {
	cutoff := time.Now().Add(-window)
	n := 0
	for _, t := range ts {
		if t.After(cutoff) {
			n++
		}
	}
	return n
}
