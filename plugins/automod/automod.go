// Package automod implements simple automatic moderation filters:
// invite links, excessive caps, mass mentions, and message-rate spam.
//
// All filters are opt-in per guild and can be configured via the !automod
// command. A single action is applied when any filter triggers.
package automod

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

// inviteRE matches common forms of Discord invite links.
var inviteRE = regexp.MustCompile(`(?i)(discord\.gg/|discord(app)?\.com/invite/)[A-Za-z0-9-]+`)

// userActivity stores recent message timestamps per user for spam detection.
type userActivity struct {
	timestamps []time.Time
}

func (a *userActivity) prune(cutoff time.Time) {
	n := 0
	for _, t := range a.timestamps {
		if t.After(cutoff) {
			a.timestamps[n] = t
			n++
		}
	}
	a.timestamps = a.timestamps[:n]
}

type activityKey struct {
	guildID string
	userID  string
}

// Plugin is the automod plugin instance.
type Plugin struct {
	api    pluginapi.API
	cancel context.CancelFunc

	mu       sync.Mutex
	activity map[activityKey]*userActivity
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "automod",
		Version:     "1.0.0",
		Description: "Filter spam, invites, caps, and mass mentions.",
		Author:      "HiLleywyn",
		Commands:    []string{"automod"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	p.activity = make(map[activityKey]*userActivity)

	api.AddCommand(&discord.Command{
		Name:        "automod",
		Description: "Configure automod filters.",
		Usage:       "invites on|off | caps <%> | mentions <n> | spam <count>/<seconds> | action delete|timeout:<min>|kick|warn | exempt add|remove @role | show",
		Handler:     p.handleCmd,
	})
	api.OnMessage(p.onMessage)

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go p.sweep(ctx)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("automod")
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

// sweep periodically discards stale per-user activity state.
func (p *Plugin) sweep(ctx context.Context) {
	t := time.NewTicker(2 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			cutoff := time.Now().Add(-5 * time.Minute)
			p.mu.Lock()
			for k, a := range p.activity {
				a.prune(cutoff)
				if len(a.timestamps) == 0 {
					delete(p.activity, k)
				}
			}
			p.mu.Unlock()
		}
	}
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		p.showCmd(ctx)
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "invites":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !automod invites on|off")
			return
		}
		on := strings.EqualFold(ctx.Args[1], "on")
		p.api.SetConfig(ctx.GuildID, "invites", boolStr(on))
		ctx.Reply(fmt.Sprintf("Invite filter: %v", on))

	case "caps":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !automod caps <percent 0-100, 0 to disable>")
			return
		}
		n, err := strconv.Atoi(strings.TrimSuffix(ctx.Args[1], "%"))
		if err != nil || n < 0 || n > 100 {
			ctx.Reply("Percent must be between 0 and 100.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "caps", strconv.Itoa(n))
		ctx.Reply(fmt.Sprintf("Caps threshold: %d%%", n))

	case "mentions":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !automod mentions <n, 0 to disable>")
			return
		}
		n, err := strconv.Atoi(ctx.Args[1])
		if err != nil || n < 0 {
			ctx.Reply("Must be a non-negative integer.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "mentions", strconv.Itoa(n))
		ctx.Reply(fmt.Sprintf("Max mentions per message: %d", n))

	case "spam":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !automod spam <count>/<seconds>  (e.g. 5/10)")
			return
		}
		parts := strings.SplitN(ctx.Args[1], "/", 2)
		if len(parts) != 2 {
			ctx.Reply("Format: count/seconds, e.g. 5/10")
			return
		}
		count, err := strconv.Atoi(parts[0])
		if err != nil || count < 0 {
			ctx.Reply("count must be a non-negative integer.")
			return
		}
		secs, err := strconv.Atoi(parts[1])
		if err != nil || secs <= 0 {
			ctx.Reply("seconds must be a positive integer.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "spam_count", strconv.Itoa(count))
		p.api.SetConfig(ctx.GuildID, "spam_window", strconv.Itoa(secs))
		ctx.Reply(fmt.Sprintf("Spam: max %d messages per %d seconds.", count, secs))

	case "action":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !automod action delete|timeout:<min>|kick|warn")
			return
		}
		a := strings.ToLower(ctx.Args[1])
		if !validAction(a) {
			ctx.Reply("Action must be one of: delete, timeout:<min>, kick, warn")
			return
		}
		p.api.SetConfig(ctx.GuildID, "action", a)
		ctx.Reply("Action set to: " + a)

	case "exempt":
		if len(ctx.Args) < 3 {
			ctx.Reply("Usage: !automod exempt add|remove @role")
			return
		}
		roleID := discord.ParseRoleMention(ctx.Args[2])
		key := "exempt:" + roleID
		switch strings.ToLower(ctx.Args[1]) {
		case "add":
			p.api.SetConfig(ctx.GuildID, key, "true")
			ctx.Reply(fmt.Sprintf("Role <@&%s> is now exempt from automod.", roleID))
		case "remove":
			p.api.DeleteConfig(ctx.GuildID, key)
			ctx.Reply(fmt.Sprintf("Role <@&%s> exemption removed.", roleID))
		default:
			ctx.Reply("Usage: !automod exempt add|remove @role")
		}

	case "show":
		p.showCmd(ctx)

	default:
		ctx.Reply("Unknown subcommand. Try: invites, caps, mentions, spam, action, exempt, show")
	}
}

func (p *Plugin) showCmd(ctx *discord.CommandContext) {
	invites := p.api.GetConfig(ctx.GuildID, "invites") == "true"
	caps := p.intConfig(ctx.GuildID, "caps", 0)
	mentions := p.intConfig(ctx.GuildID, "mentions", 0)
	spamCount := p.intConfig(ctx.GuildID, "spam_count", 0)
	spamWindow := p.intConfig(ctx.GuildID, "spam_window", 0)
	action := p.api.GetConfig(ctx.GuildID, "action")
	if action == "" {
		action = "delete"
	}
	ctx.Reply(fmt.Sprintf(
		"invites=%v | caps=%d%% | mentions=%d | spam=%d/%ds | action=%s",
		invites, caps, mentions, spamCount, spamWindow, action,
	))
}

// ---------------------------------------------------------------------------
// Message handler
// ---------------------------------------------------------------------------

func (p *Plugin) onMessage(bot *discord.Bot, msg *discord.Message) {
	if msg.GuildID == "" || msg.Author == nil || msg.Author.Bot {
		return
	}
	if p.isExempt(msg) {
		return
	}

	reason := p.checkViolations(msg)
	if reason == "" {
		return
	}

	// Every violation starts by deleting the offending message.
	if err := bot.Rest.DeleteMessage(msg.ChannelID, msg.ID); err != nil {
		p.api.Log("automod: delete: %v", err)
	}

	action := p.api.GetConfig(msg.GuildID, "action")
	if action == "" {
		action = "delete"
	}
	p.applyAction(bot, msg, reason, action)
}

// checkViolations returns a short reason string for the first filter the
// message trips, or "" if it is clean.
func (p *Plugin) checkViolations(msg *discord.Message) string {
	if p.api.GetConfig(msg.GuildID, "invites") == "true" && inviteRE.MatchString(msg.Content) {
		return "Discord invites are not allowed here"
	}

	if threshold := p.intConfig(msg.GuildID, "caps", 0); threshold > 0 && len(msg.Content) >= 8 {
		upper, letters := 0, 0
		for _, r := range msg.Content {
			switch {
			case r >= 'A' && r <= 'Z':
				upper++
				letters++
			case r >= 'a' && r <= 'z':
				letters++
			}
		}
		if letters >= 8 && (upper*100/letters) > threshold {
			return fmt.Sprintf("Too many caps (>%d%%)", threshold)
		}
	}

	if max := p.intConfig(msg.GuildID, "mentions", 0); max > 0 {
		if len(msg.Mentions) > max {
			return fmt.Sprintf("Too many mentions (>%d)", max)
		}
	}

	count := p.intConfig(msg.GuildID, "spam_count", 0)
	window := p.intConfig(msg.GuildID, "spam_window", 0)
	if count > 0 && window > 0 {
		if p.recordSpam(msg.GuildID, msg.Author.ID, count, window) {
			return fmt.Sprintf("Spam (>%d messages per %ds)", count, window)
		}
	}

	return ""
}

func (p *Plugin) applyAction(bot *discord.Bot, msg *discord.Message, reason, action string) {
	switch {
	case action == "delete":
		// Message was already deleted above.
	case action == "kick":
		if err := bot.Rest.KickMember(msg.GuildID, msg.Author.ID, reason); err != nil {
			p.api.Log("automod: kick: %v", err)
		}
	case action == "warn":
		_, _ = bot.Rest.SendMessage(msg.ChannelID,
			fmt.Sprintf("<@%s> %s", msg.Author.ID, reason))
	case strings.HasPrefix(action, "timeout:"):
		mins, err := strconv.Atoi(strings.TrimPrefix(action, "timeout:"))
		if err == nil && mins > 0 {
			until := time.Now().Add(time.Duration(mins) * time.Minute)
			if err := bot.Rest.TimeoutMember(msg.GuildID, msg.Author.ID, until, reason); err != nil {
				p.api.Log("automod: timeout: %v", err)
			}
		}
	}
}

// recordSpam adds the current timestamp to the user's activity and returns
// true if the count-within-window threshold is now exceeded.
func (p *Plugin) recordSpam(guildID, userID string, count, windowSecs int) bool {
	key := activityKey{guildID, userID}
	now := time.Now()
	cutoff := now.Add(-time.Duration(windowSecs) * time.Second)

	p.mu.Lock()
	defer p.mu.Unlock()

	a := p.activity[key]
	if a == nil {
		a = &userActivity{}
		p.activity[key] = a
	}
	a.prune(cutoff)
	a.timestamps = append(a.timestamps, now)
	return len(a.timestamps) > count
}

func (p *Plugin) isExempt(msg *discord.Message) bool {
	if msg.Member == nil {
		return false
	}
	for _, r := range msg.Member.Roles {
		if p.api.GetConfig(msg.GuildID, "exempt:"+r) == "true" {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func validAction(a string) bool {
	if a == "delete" || a == "kick" || a == "warn" {
		return true
	}
	if strings.HasPrefix(a, "timeout:") {
		n, err := strconv.Atoi(strings.TrimPrefix(a, "timeout:"))
		return err == nil && n > 0
	}
	return false
}
