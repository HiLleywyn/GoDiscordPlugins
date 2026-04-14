// Package suggestions implements a structured suggestion-box workflow.
//
// Anyone can file a suggestion with `!suggest <text>`. The bot posts it to
// the configured suggestions channel as an embed, pre-reacts with up/down
// vote emoji, and assigns a numeric ID. Mods can later mark a suggestion
// as approved, denied, or implemented, which recolours the embed and
// appends a mod note.
package suggestions

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

const (
	upvoteEmoji   = "\U0001F44D" // thumbs up
	downvoteEmoji = "\U0001F44E" // thumbs down
)

// status values for a suggestion.
const (
	statusPending     = "pending"
	statusApproved    = "approved"
	statusDenied      = "denied"
	statusImplemented = "implemented"
)

// suggestionData is the persisted record for one suggestion.
type suggestionData struct {
	ID          int       `json:"id"`
	AuthorID    string    `json:"author_id"`
	AuthorName  string    `json:"author_name"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"created_at"`
	ChannelID   string    `json:"channel_id"`
	MessageID   string    `json:"message_id"`
	Status      string    `json:"status"`
	ModeratorID string    `json:"moderator_id,omitempty"`
	ModNote     string    `json:"mod_note,omitempty"`
}

// Plugin is the suggestions plugin instance.
type Plugin struct {
	api pluginapi.API
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "suggestions",
		Version:     "1.0.0",
		Description: "Suggestion box with upvote/downvote and status tracking.",
		Author:      "HiLleywyn",
		Commands:    []string{"suggest", "suggestion"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "suggest",
		Description: "File a new suggestion.",
		Usage:       "<text>",
		Handler:     p.cmdSuggest,
	})
	api.AddCommand(&discord.Command{
		Name:        "suggestion",
		Aliases:     []string{"sug"},
		Description: "Manage suggestions (mod/admin).",
		Usage:       "channel #chan | approve <id> [note] | deny <id> [note] | implement <id> [note] | show <id> | list [status]",
		Handler:     p.cmdManage,
	})
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("suggest")
	p.api.RemoveCommand("suggestion")
	return nil
}

// ---------------------------------------------------------------------------
// !suggest <text>
// ---------------------------------------------------------------------------

func (p *Plugin) cmdSuggest(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !suggest <text>")
		return
	}
	chID := p.api.GetConfig(ctx.GuildID, "channel")
	if chID == "" {
		ctx.Reply("No suggestions channel set. Ask an admin to run `!suggestion channel #chan`.")
		return
	}

	content := strings.Join(ctx.Args, " ")
	if len(content) > 1500 {
		ctx.Reply("Suggestion too long (max 1500 characters).")
		return
	}

	id := p.nextID(ctx.GuildID)
	sug := &suggestionData{
		ID:         id,
		AuthorID:   ctx.AuthorID,
		AuthorName: tagOf(ctx),
		Content:    content,
		CreatedAt:  time.Now(),
		Status:     statusPending,
	}

	msg, err := ctx.Bot.Rest.SendEmbed(chID, renderSuggestion(sug))
	if err != nil || msg == nil {
		ctx.Reply("Failed to post suggestion: " + errStr(err))
		return
	}
	sug.ChannelID = chID
	sug.MessageID = msg.ID

	_ = ctx.Bot.Rest.AddReaction(chID, msg.ID, upvoteEmoji)
	_ = ctx.Bot.Rest.AddReaction(chID, msg.ID, downvoteEmoji)

	p.save(ctx.GuildID, sug)
	ctx.Reply(fmt.Sprintf("Suggestion **#%d** filed in <#%s>.", id, chID))
}

// ---------------------------------------------------------------------------
// !suggestion ...
// ---------------------------------------------------------------------------

func (p *Plugin) cmdManage(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !suggestion channel #chan | approve <id> [note] | deny <id> [note] | implement <id> [note] | show <id> | list [status]")
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "channel":
		if len(ctx.Args) < 2 {
			ctx.Reply("Provide a channel mention or ID.")
			return
		}
		chID := discord.ParseChannelMention(ctx.Args[1])
		p.api.SetConfig(ctx.GuildID, "channel", chID)
		ctx.Reply("Suggestions channel set to <#" + chID + ">.")

	case "approve":
		p.setStatus(ctx, statusApproved)
	case "deny":
		p.setStatus(ctx, statusDenied)
	case "implement", "implemented":
		p.setStatus(ctx, statusImplemented)

	case "show":
		if len(ctx.Args) < 2 {
			ctx.Reply("Provide a suggestion id.")
			return
		}
		id, err := strconv.Atoi(ctx.Args[1])
		if err != nil {
			ctx.Reply("Id must be a number.")
			return
		}
		sug := p.load(ctx.GuildID, id)
		if sug == nil {
			ctx.Reply("No suggestion with that id.")
			return
		}
		ctx.ReplyEmbed(renderSuggestion(sug))

	case "list":
		filter := ""
		if len(ctx.Args) >= 2 {
			filter = strings.ToLower(ctx.Args[1])
		}
		p.list(ctx, filter)

	default:
		ctx.Reply("Unknown subcommand. Try: channel, approve, deny, implement, show, list")
	}
}

func (p *Plugin) setStatus(ctx *discord.CommandContext, status string) {
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !suggestion " + status + " <id> [note]")
		return
	}
	id, err := strconv.Atoi(ctx.Args[1])
	if err != nil {
		ctx.Reply("Id must be a number.")
		return
	}
	sug := p.load(ctx.GuildID, id)
	if sug == nil {
		ctx.Reply("No suggestion with that id.")
		return
	}

	sug.Status = status
	sug.ModeratorID = ctx.AuthorID
	if len(ctx.Args) > 2 {
		sug.ModNote = strings.Join(ctx.Args[2:], " ")
	}
	p.save(ctx.GuildID, sug)

	if _, err := ctx.Bot.Rest.EditEmbed(sug.ChannelID, sug.MessageID, renderSuggestion(sug)); err != nil {
		p.api.Log("suggestions: edit embed: %v", err)
	}
	ctx.Reply(fmt.Sprintf("Suggestion **#%d** marked as %s.", id, status))
}

func (p *Plugin) list(ctx *discord.CommandContext, filter string) {
	var rows []*suggestionData
	for k, v := range p.api.AllConfig(ctx.GuildID) {
		if !strings.HasPrefix(k, "sug:") {
			continue
		}
		var sug suggestionData
		if err := json.Unmarshal([]byte(v), &sug); err != nil {
			continue
		}
		if filter != "" && sug.Status != filter {
			continue
		}
		rows = append(rows, &sug)
	}
	if len(rows) == 0 {
		ctx.Reply("No suggestions match.")
		return
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID > rows[j].ID })

	max := 15
	if len(rows) > max {
		rows = rows[:max]
	}
	var lines []string
	for _, r := range rows {
		lines = append(lines, fmt.Sprintf("**#%d** [%s] %s - %s",
			r.ID, r.Status, shortAuthor(r.AuthorName), truncate(r.Content, 80)))
	}
	title := "Suggestions"
	if filter != "" {
		title += " - " + filter
	}
	ctx.ReplyEmbed(discord.Embed{
		Title:       title,
		Description: strings.Join(lines, "\n"),
		Color:       0x5865F2,
	})
}

// ---------------------------------------------------------------------------
// Persistence and rendering
// ---------------------------------------------------------------------------

func (p *Plugin) nextID(guildID string) int {
	cur, _ := strconv.Atoi(p.api.GetConfig(guildID, "counter"))
	cur++
	p.api.SetConfig(guildID, "counter", strconv.Itoa(cur))
	return cur
}

func (p *Plugin) load(guildID string, id int) *suggestionData {
	raw := p.api.GetConfig(guildID, fmt.Sprintf("sug:%d", id))
	if raw == "" {
		return nil
	}
	var sug suggestionData
	if err := json.Unmarshal([]byte(raw), &sug); err != nil {
		return nil
	}
	return &sug
}

func (p *Plugin) save(guildID string, sug *suggestionData) {
	b, err := json.Marshal(sug)
	if err != nil {
		p.api.Log("suggestions: marshal: %v", err)
		return
	}
	p.api.SetConfig(guildID, fmt.Sprintf("sug:%d", sug.ID), string(b))
}

func renderSuggestion(sug *suggestionData) discord.Embed {
	color := 0x5865F2 // pending: blurple
	switch sug.Status {
	case statusApproved:
		color = 0x57F287 // green
	case statusDenied:
		color = 0xED4245 // red
	case statusImplemented:
		color = 0xFEE75C // yellow
	}

	fields := []discord.EmbedField{
		{Name: "Status", Value: sug.Status, Inline: true},
		{Name: "Author", Value: "<@" + sug.AuthorID + ">", Inline: true},
	}
	if sug.ModeratorID != "" {
		fields = append(fields, discord.EmbedField{
			Name:   "Moderator",
			Value:  "<@" + sug.ModeratorID + ">",
			Inline: true,
		})
	}
	if sug.ModNote != "" {
		fields = append(fields, discord.EmbedField{
			Name:  "Mod note",
			Value: sug.ModNote,
		})
	}

	return discord.Embed{
		Title:       fmt.Sprintf("Suggestion #%d", sug.ID),
		Description: sug.Content,
		Color:       color,
		Fields:      fields,
		Footer: &discord.EmbedFooter{
			Text: sug.CreatedAt.UTC().Format("2006-01-02 15:04 UTC"),
		},
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func tagOf(ctx *discord.CommandContext) string {
	if ctx.Message != nil && ctx.Message.Author != nil {
		return ctx.Message.Author.Tag()
	}
	return ctx.AuthorID
}

func shortAuthor(name string) string {
	if len(name) > 20 {
		return name[:17] + "..."
	}
	return name
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func errStr(err error) string {
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}
