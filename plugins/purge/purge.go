// Package purge provides a bulk message delete command with filters.
//
// Usage:
//
//	!purge <count> [filter...]
//
// Filters (all optional, combined with AND):
//
//	user @user       only messages from this user
//	bots             only messages from bots
//	humans           only messages from non-bots
//	contains <text>  only messages whose content contains <text>
//	links            only messages containing a URL
//	embeds           only messages with attachments or embeds
//
// Count may be 1..500. The plugin scans up to 500 recent messages and
// deletes the first <count> that match. Messages older than 14 days fall
// back to individual deletes (Discord API limitation).
package purge

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

const (
	maxPurge    = 500
	scanWindow  = 500
	bulkMaxAge  = 14 * 24 * time.Hour
)

// Plugin is the purge plugin instance.
type Plugin struct {
	api pluginapi.API
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "purge",
		Version:     "1.0.0",
		Description: "Bulk-delete recent messages with filters.",
		Author:      "HiLleywyn",
		Commands:    []string{"purge"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "purge",
		Aliases:     []string{"clean", "prune"},
		Description: "Bulk-delete recent messages with optional filters.",
		Usage:       "<count> [user @user] [bots|humans] [contains <text>] [links] [embeds]",
		Handler:     p.handleCmd,
	})
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("purge")
	return nil
}

// filter holds the parsed filter criteria for one invocation.
type filter struct {
	userID   string
	botsOnly bool
	humans   bool
	contains string
	links    bool
	embeds   bool
}

func (f filter) match(m *discord.Message) bool {
	if f.userID != "" && (m.Author == nil || m.Author.ID != f.userID) {
		return false
	}
	if f.botsOnly && (m.Author == nil || !m.Author.Bot) {
		return false
	}
	if f.humans && (m.Author != nil && m.Author.Bot) {
		return false
	}
	if f.contains != "" && !strings.Contains(strings.ToLower(m.Content), f.contains) {
		return false
	}
	if f.links && !containsLink(m.Content) {
		return false
	}
	if f.embeds && len(m.Attachments) == 0 && len(m.Embeds) == 0 {
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !purge <count> [user @user] [bots|humans] [contains <text>] [links] [embeds]")
		return
	}

	count, err := strconv.Atoi(ctx.Args[0])
	if err != nil || count < 1 {
		ctx.Reply("Count must be a positive integer.")
		return
	}
	if count > maxPurge {
		ctx.Reply(fmt.Sprintf("Maximum purge is %d messages per call.", maxPurge))
		return
	}

	f, err := parseFilter(ctx.Args[1:])
	if err != nil {
		ctx.Reply("Filter error: " + err.Error())
		return
	}

	// Delete the invoking command itself so it doesn't count against the limit.
	_ = ctx.Bot.Rest.DeleteMessage(ctx.ChannelID, ctx.Message.ID)

	msgs, err := ctx.Bot.Rest.GetChannelMessages(ctx.ChannelID, scanWindow, "")
	if err != nil {
		ctx.Reply("Failed to fetch messages: " + err.Error())
		return
	}

	var matched []*discord.Message
	for _, m := range msgs {
		if len(matched) >= count {
			break
		}
		if f.match(m) {
			matched = append(matched, m)
		}
	}

	if len(matched) == 0 {
		sendTemp(ctx, "No messages matched.")
		return
	}

	bulk, old := splitByAge(matched, bulkMaxAge)
	deleted := 0

	if len(bulk) > 0 {
		ids := make([]string, len(bulk))
		for i, m := range bulk {
			ids[i] = m.ID
		}
		if err := ctx.Bot.Rest.BulkDeleteMessages(ctx.ChannelID, ids); err != nil {
			p.api.Log("purge: bulk delete: %v", err)
		} else {
			deleted += len(bulk)
		}
	}

	for _, m := range old {
		if err := ctx.Bot.Rest.DeleteMessage(ctx.ChannelID, m.ID); err != nil {
			p.api.Log("purge: individual delete %s: %v", m.ID, err)
			continue
		}
		deleted++
	}

	sendTemp(ctx, fmt.Sprintf("Purged %d message(s).", deleted))
}

// ---------------------------------------------------------------------------
// Filter parsing
// ---------------------------------------------------------------------------

func parseFilter(args []string) (filter, error) {
	var f filter
	for i := 0; i < len(args); i++ {
		tok := strings.ToLower(args[i])
		switch tok {
		case "user":
			if i+1 >= len(args) {
				return f, fmt.Errorf("user filter needs a user")
			}
			i++
			uid := discord.ParseUserID(args[i])
			if uid == "" {
				return f, fmt.Errorf("invalid user mention")
			}
			f.userID = uid
		case "bots":
			f.botsOnly = true
		case "humans":
			f.humans = true
		case "contains":
			if i+1 >= len(args) {
				return f, fmt.Errorf("contains filter needs text")
			}
			i++
			// Join the rest of args into one search term to allow spaces.
			f.contains = strings.ToLower(strings.Join(args[i:], " "))
			return f, nil
		case "links":
			f.links = true
		case "embeds":
			f.embeds = true
		default:
			return f, fmt.Errorf("unknown filter '%s'", tok)
		}
	}
	if f.botsOnly && f.humans {
		return f, fmt.Errorf("cannot combine 'bots' and 'humans'")
	}
	return f, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// splitByAge partitions messages into those young enough for bulk delete and
// those that must be deleted individually.
func splitByAge(msgs []*discord.Message, maxAge time.Duration) (bulk, old []*discord.Message) {
	cutoff := time.Now().Add(-maxAge)
	for _, m := range msgs {
		if m.Timestamp.After(cutoff) {
			bulk = append(bulk, m)
		} else {
			old = append(old, m)
		}
	}
	return
}

func containsLink(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "http://") || strings.Contains(lower, "https://")
}

// sendTemp posts a confirmation that auto-deletes after a few seconds, so
// purge feedback doesn't pile up in chat.
func sendTemp(ctx *discord.CommandContext, text string) {
	msg, err := ctx.Bot.Rest.SendMessage(ctx.ChannelID, text)
	if err != nil || msg == nil {
		return
	}
	go func(id string) {
		time.Sleep(5 * time.Second)
		_ = ctx.Bot.Rest.DeleteMessage(ctx.ChannelID, id)
	}(msg.ID)
}
