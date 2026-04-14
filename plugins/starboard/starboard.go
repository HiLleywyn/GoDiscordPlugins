package starboard

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

// Plugin implements the starboard feature.
// When N users react with a configured emoji on a message, it gets reposted
// to a designated channel.
type Plugin struct {
	api pluginapi.API
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "starboard",
		Version:     "1.0.0",
		Description: "Pin highly-reacted messages to a starboard channel.",
		Author:      "HiLleywyn",
		Commands:    []string{"starboard"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "starboard",
		Description: "Configure the starboard.",
		Usage:       "channel #chan | threshold <n> | emoji <emoji> | show",
		Handler:     p.handleCmd,
	})
	api.OnReactionAdd(p.onReactionAdd)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("starboard")
	return nil
}

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

const defaultEmoji = "\u2b50"
const defaultThreshold = 3

func (p *Plugin) getChannel(guildID string) string {
	return p.api.GetConfig(guildID, "channel")
}

func (p *Plugin) getEmoji(guildID string) string {
	v := p.api.GetConfig(guildID, "emoji")
	if v == "" {
		return defaultEmoji
	}
	return v
}

func (p *Plugin) getThreshold(guildID string) int {
	v := p.api.GetConfig(guildID, "threshold")
	if v == "" {
		return defaultThreshold
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return defaultThreshold
	}
	return n
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !starboard channel #chan | threshold <n> | emoji <emoji> | show")
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "channel":
		if len(ctx.Args) < 2 {
			ctx.Reply("Provide a channel mention or ID.")
			return
		}
		chID := stripChannelMention(ctx.Args[1])
		p.api.SetConfig(ctx.GuildID, "channel", chID)
		ctx.Reply("Starboard channel set to <#" + chID + ">.")

	case "threshold":
		if len(ctx.Args) < 2 {
			ctx.Reply("Provide a number.")
			return
		}
		n, err := strconv.Atoi(ctx.Args[1])
		if err != nil || n < 1 {
			ctx.Reply("Threshold must be a positive integer.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "threshold", strconv.Itoa(n))
		ctx.Reply(fmt.Sprintf("Star threshold set to %d.", n))

	case "emoji":
		if len(ctx.Args) < 2 {
			ctx.Reply("Provide an emoji.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "emoji", ctx.Args[1])
		ctx.Reply("Star emoji set to " + ctx.Args[1] + ".")

	case "show":
		ch := p.getChannel(ctx.GuildID)
		if ch == "" {
			ch = "not set"
		} else {
			ch = "<#" + ch + ">"
		}
		ctx.Reply(fmt.Sprintf(
			"channel: %s | threshold: %d | emoji: %s",
			ch, p.getThreshold(ctx.GuildID), p.getEmoji(ctx.GuildID),
		))

	default:
		ctx.Reply("Unknown subcommand. Try: channel, threshold, emoji, show")
	}
}

// ---------------------------------------------------------------------------
// Reaction handler
// ---------------------------------------------------------------------------

func (p *Plugin) onReactionAdd(bot *discord.Bot, ev *discord.MessageReactionAddEvent) {
	if ev.GuildID == "" {
		return
	}

	// Check emoji matches config.
	emojiStr := emojiName(ev.Emoji)
	if emojiStr != p.getEmoji(ev.GuildID) {
		return
	}

	// Check we have a starboard channel configured.
	starCh := p.getChannel(ev.GuildID)
	if starCh == "" {
		return
	}

	// Check message not already starred.
	starredKey := "starred:" + ev.MessageID
	if p.api.GetConfig(ev.GuildID, starredKey) == "true" {
		return
	}

	// Fetch message to count reactions.
	msg, err := bot.Rest.GetMessage(ev.ChannelID, ev.MessageID)
	if err != nil {
		p.api.Log("starboard: failed to fetch message %s: %v", ev.MessageID, err)
		return
	}

	// Don't star bot messages to the starboard channel (avoid loops).
	if msg.ChannelID == starCh {
		return
	}

	count := 0
	for _, r := range msg.Reactions {
		if emojiName(r.Emoji) == p.getEmoji(ev.GuildID) {
			count = r.Count
			break
		}
	}

	if count < p.getThreshold(ev.GuildID) {
		return
	}

	// Mark as starred before posting to avoid races.
	p.api.SetConfig(ev.GuildID, starredKey, "true")

	// Build the embed.
	authorName := ""
	if msg.Author != nil {
		authorName = msg.Author.Tag()
	}
	msgLink := fmt.Sprintf("https://discord.com/channels/%s/%s/%s",
		ev.GuildID, ev.ChannelID, ev.MessageID)

	embed := discord.Embed{
		Color:       0xFFD700,
		Description: msg.Content,
		Author: &discord.EmbedAuthor{
			Name: authorName,
		},
		Fields: []discord.EmbedField{
			{
				Name:  "Source",
				Value: fmt.Sprintf("[Jump to message](%s) in <#%s>", msgLink, ev.ChannelID),
			},
		},
		Footer: &discord.EmbedFooter{
			Text: fmt.Sprintf("%s %d", p.getEmoji(ev.GuildID), count),
		},
	}

	// Attach image if the original has one.
	if len(msg.Attachments) > 0 && isImage(msg.Attachments[0].Filename) {
		embed.Image = &discord.EmbedImage{URL: msg.Attachments[0].URL}
	}

	_, err = bot.Rest.SendEmbed(starCh, embed)
	if err != nil {
		p.api.Log("starboard: failed to post to starboard: %v", err)
		// Unmark so it can be retried next reaction.
		p.api.DeleteConfig(ev.GuildID, starredKey)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// emojiName returns a comparable string for both unicode and custom emoji.
func emojiName(e discord.Emoji) string {
	if e.ID != "" {
		return e.Name + ":" + e.ID
	}
	return e.Name
}

// stripChannelMention converts <#123> to 123, or returns the string unchanged.
func stripChannelMention(s string) string {
	s = strings.TrimPrefix(s, "<#")
	s = strings.TrimSuffix(s, ">")
	return s
}

func isImage(filename string) bool {
	lower := strings.ToLower(filename)
	return strings.HasSuffix(lower, ".png") ||
		strings.HasSuffix(lower, ".jpg") ||
		strings.HasSuffix(lower, ".jpeg") ||
		strings.HasSuffix(lower, ".gif") ||
		strings.HasSuffix(lower, ".webp")
}
