// Package afk lets users mark themselves as AFK with an optional reason.
//
// While marked AFK:
//   - if anyone mentions the user, the bot replies with their reason and
//     how long ago they went AFK
//   - when the user sends their next message, AFK is cleared and the bot
//     welcomes them back
//
// State is persisted per-guild via plugin config under the key
// `afk:<userID>` so AFK survives bot restarts.
package afk

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

// afkData is the per-user AFK state.
type afkData struct {
	Reason  string    `json:"reason"`
	Since   time.Time `json:"since"`
	Pings   int       `json:"pings"`
}

// Plugin is the afk plugin instance.
type Plugin struct {
	api pluginapi.API
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "afk",
		Version:     "1.0.0",
		Description: "Mark yourself AFK; the bot notifies people who ping you.",
		Author:      "HiLleywyn",
		Commands:    []string{"afk"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "afk",
		Description: "Mark yourself as away. Use without arguments to clear.",
		Usage:       "[reason]",
		Handler:     p.handleCmd,
	})
	api.OnMessage(p.onMessage)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("afk")
	return nil
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		// Toggle: if already AFK, clear; otherwise set with no reason.
		if p.load(ctx.GuildID, ctx.AuthorID) != nil {
			p.clear(ctx.GuildID, ctx.AuthorID)
			ctx.Reply("AFK cleared.")
			return
		}
		p.set(ctx.GuildID, ctx.AuthorID, "AFK")
		ctx.Reply("You are now AFK.")
		return
	}

	reason := strings.Join(ctx.Args, " ")
	if len(reason) > 300 {
		reason = reason[:300] + "..."
	}
	p.set(ctx.GuildID, ctx.AuthorID, reason)
	ctx.Reply("You are now AFK: " + reason)
}

// ---------------------------------------------------------------------------
// Message handler
// ---------------------------------------------------------------------------

func (p *Plugin) onMessage(bot *discord.Bot, msg *discord.Message) {
	if msg.GuildID == "" || msg.Author == nil || msg.Author.Bot {
		return
	}

	// 1. Clear AFK for the author if they're marked AFK and are typing
	//    something that isn't just `!afk <new reason>`.
	if a := p.load(msg.GuildID, msg.Author.ID); a != nil {
		if !strings.HasPrefix(msg.Content, "!afk") {
			p.clear(msg.GuildID, msg.Author.ID)
			welcome := fmt.Sprintf("Welcome back <@%s>. You were AFK for %s.",
				msg.Author.ID, shortDuration(time.Since(a.Since)))
			if a.Pings > 0 {
				welcome += fmt.Sprintf(" You were pinged %d time(s).", a.Pings)
			}
			_, _ = bot.Rest.SendMessage(msg.ChannelID, welcome)
		}
	}

	// 2. For every mentioned user who is AFK, post a single short notice.
	notified := map[string]bool{}
	for _, u := range msg.Mentions {
		if u == nil || notified[u.ID] {
			continue
		}
		notified[u.ID] = true
		a := p.load(msg.GuildID, u.ID)
		if a == nil {
			continue
		}
		// Bump the ping counter on the stored record.
		a.Pings++
		p.save(msg.GuildID, u.ID, a)

		text := fmt.Sprintf("<@%s> is AFK: %s (%s ago)",
			u.ID, a.Reason, shortDuration(time.Since(a.Since)))
		_, _ = bot.Rest.SendMessage(msg.ChannelID, text)
	}
}

// ---------------------------------------------------------------------------
// Persistence helpers
// ---------------------------------------------------------------------------

func (p *Plugin) load(guildID, userID string) *afkData {
	raw := p.api.GetConfig(guildID, "afk:"+userID)
	if raw == "" {
		return nil
	}
	var a afkData
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		return nil
	}
	return &a
}

func (p *Plugin) save(guildID, userID string, a *afkData) {
	b, err := json.Marshal(a)
	if err != nil {
		p.api.Log("afk: marshal: %v", err)
		return
	}
	p.api.SetConfig(guildID, "afk:"+userID, string(b))
}

func (p *Plugin) set(guildID, userID, reason string) {
	p.save(guildID, userID, &afkData{
		Reason: reason,
		Since:  time.Now(),
	})
}

func (p *Plugin) clear(guildID, userID string) {
	p.api.DeleteConfig(guildID, "afk:"+userID)
}

func shortDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
