// Package sticky keeps a configured message pinned visually to the bottom
// of a channel by reposting it whenever a new message arrives.
//
// Useful for channel rules, reminders, or "read this before posting" notices.
// Each guild can configure a sticky per channel.
//
// Commands (mod/admin):
//
//	!sticky set <text>      Set/replace the sticky for the current channel
//	!sticky clear           Remove the sticky for the current channel
//	!sticky show            Preview the current sticky
//	!sticky list            List every channel that has a sticky in this guild
//
// A short cooldown and a "messages since last repost" threshold avoid
// spamming the channel during active conversation.
package sticky

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

// stickyData is the persisted record for one sticky.
type stickyData struct {
	Content   string `json:"content"`
	MessageID string `json:"message_id,omitempty"`
}

// runtimeState is per-channel in-memory state to avoid reposting on every
// single message.
type runtimeState struct {
	lastRepost time.Time
	since      int
}

const (
	minCooldown  = 5 * time.Second
	msgThreshold = 3
)

// Plugin is the sticky plugin instance.
type Plugin struct {
	api pluginapi.API

	mu    sync.Mutex
	state map[string]*runtimeState // keyed by channelID
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "sticky",
		Version:     "1.0.0",
		Description: "Keep a message pinned to the bottom of a channel.",
		Author:      "HiLleywyn",
		Commands:    []string{"sticky"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	p.state = make(map[string]*runtimeState)
	api.AddCommand(&discord.Command{
		Name:        "sticky",
		Description: "Pin a message to the bottom of a channel.",
		Usage:       "set <text> | clear | show | list",
		Handler:     p.handleCmd,
	})
	api.OnMessage(p.onMessage)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("sticky")
	return nil
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if ctx.Member == nil || !ctx.Member.HasPermission(discord.PermissionManageMessages) {
		ctx.Reply("You need Manage Messages to configure stickies.")
		return
	}
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !sticky set <text> | clear | show | list")
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "set":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !sticky set <text>")
			return
		}
		text := strings.Join(ctx.Args[1:], " ")
		if len(text) > 1500 {
			ctx.Reply("Sticky too long (max 1500 chars).")
			return
		}
		// Clean up any existing sticky message we posted.
		if old := p.load(ctx.GuildID, ctx.ChannelID); old != nil && old.MessageID != "" {
			_ = ctx.Bot.Rest.DeleteMessage(ctx.ChannelID, old.MessageID)
		}
		sd := &stickyData{Content: text}
		// Post the new one immediately.
		msg, err := ctx.Bot.Rest.SendMessage(ctx.ChannelID, formatSticky(text))
		if err == nil && msg != nil {
			sd.MessageID = msg.ID
		}
		p.save(ctx.GuildID, ctx.ChannelID, sd)
		p.resetState(ctx.ChannelID)
		ctx.Reply("Sticky set for this channel.")

	case "clear":
		sd := p.load(ctx.GuildID, ctx.ChannelID)
		if sd == nil {
			ctx.Reply("No sticky set in this channel.")
			return
		}
		if sd.MessageID != "" {
			_ = ctx.Bot.Rest.DeleteMessage(ctx.ChannelID, sd.MessageID)
		}
		p.api.DeleteConfig(ctx.GuildID, "ch:"+ctx.ChannelID)
		ctx.Reply("Sticky cleared for this channel.")

	case "show":
		sd := p.load(ctx.GuildID, ctx.ChannelID)
		if sd == nil {
			ctx.Reply("No sticky set in this channel.")
			return
		}
		ctx.Reply("Sticky for this channel:\n" + formatSticky(sd.Content))

	case "list":
		channels := p.configuredChannels(ctx.GuildID)
		if len(channels) == 0 {
			ctx.Reply("No stickies configured in this guild.")
			return
		}
		var mentions []string
		for _, c := range channels {
			mentions = append(mentions, "<#"+c+">")
		}
		ctx.Reply("Stickies: " + strings.Join(mentions, " "))

	default:
		ctx.Reply("Unknown subcommand. Try: set, clear, show, list")
	}
}

// ---------------------------------------------------------------------------
// Message hook
// ---------------------------------------------------------------------------

func (p *Plugin) onMessage(bot *discord.Bot, msg *discord.Message) {
	if msg.GuildID == "" || msg.Author == nil || msg.Author.Bot {
		return
	}
	sd := p.load(msg.GuildID, msg.ChannelID)
	if sd == nil {
		return
	}

	p.mu.Lock()
	st, ok := p.state[msg.ChannelID]
	if !ok {
		st = &runtimeState{}
		p.state[msg.ChannelID] = st
	}
	st.since++
	cooldownOK := time.Since(st.lastRepost) >= minCooldown
	threshOK := st.since >= msgThreshold
	shouldPost := cooldownOK && threshOK
	if shouldPost {
		st.lastRepost = time.Now()
		st.since = 0
	}
	p.mu.Unlock()

	if !shouldPost {
		return
	}

	// Delete old sticky copy (if any) and post a new one.
	if sd.MessageID != "" {
		_ = bot.Rest.DeleteMessage(msg.ChannelID, sd.MessageID)
	}
	newMsg, err := bot.Rest.SendMessage(msg.ChannelID, formatSticky(sd.Content))
	if err != nil || newMsg == nil {
		p.api.Log("sticky: repost: %v", err)
		return
	}
	sd.MessageID = newMsg.ID
	p.save(msg.GuildID, msg.ChannelID, sd)
}

// ---------------------------------------------------------------------------
// Persistence helpers
// ---------------------------------------------------------------------------

func (p *Plugin) load(guildID, channelID string) *stickyData {
	raw := p.api.GetConfig(guildID, "ch:"+channelID)
	if raw == "" {
		return nil
	}
	var sd stickyData
	if err := json.Unmarshal([]byte(raw), &sd); err != nil {
		return nil
	}
	return &sd
}

func (p *Plugin) save(guildID, channelID string, sd *stickyData) {
	b, err := json.Marshal(sd)
	if err != nil {
		p.api.Log("sticky: marshal: %v", err)
		return
	}
	p.api.SetConfig(guildID, "ch:"+channelID, string(b))
}

func (p *Plugin) configuredChannels(guildID string) []string {
	var out []string
	for k := range p.api.AllConfig(guildID) {
		if strings.HasPrefix(k, "ch:") {
			out = append(out, strings.TrimPrefix(k, "ch:"))
		}
	}
	return out
}

func (p *Plugin) resetState(channelID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.state, channelID)
}

func formatSticky(text string) string {
	return fmt.Sprintf("**Sticky**\n%s", text)
}
