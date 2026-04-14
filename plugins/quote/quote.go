// Package quote lets users save memorable messages into a per-guild
// quote book and recall them later, either at random or by author.
//
// Commands:
//
//	!quote add <text> -- <author>   Save a new quote manually
//	!quote                          Show a random quote
//	!quote <id>                     Show quote by id
//	!quote by @user                 Show a random quote by that author
//	!quote list                     List recent quotes (embed, 15 newest)
//	!quote remove <id>              Delete a quote (mod/admin or author)
//	!quote search <text>            Search quote contents
//
// Quotes are stored per-guild under config keys `q:<id>` with an
// incrementing `counter`.
package quote

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

// quoteData is the persisted record for one quote.
type quoteData struct {
	ID        int       `json:"id"`
	Content   string    `json:"content"`
	AuthorID  string    `json:"author_id,omitempty"`
	Author    string    `json:"author"`
	AddedByID string    `json:"added_by_id"`
	AddedAt   time.Time `json:"added_at"`
}

// Plugin is the quote plugin instance.
type Plugin struct {
	api pluginapi.API
	rng *rand.Rand
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "quote",
		Version:     "1.0.0",
		Description: "Save memorable messages and recall them later.",
		Author:      "HiLleywyn",
		Commands:    []string{"quote"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	p.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	api.AddCommand(&discord.Command{
		Name:        "quote",
		Description: "Save, recall, and manage server quotes.",
		Usage:       "[add <text> -- <author> | <id> | by @user | list | remove <id> | search <text>]",
		Handler:     p.handleCmd,
	})
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("quote")
	return nil
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	// No args: random quote.
	if len(ctx.Args) == 0 {
		p.random(ctx, "")
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "add":
		p.add(ctx)
	case "by":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !quote by @user")
			return
		}
		uid := discord.ParseUserID(ctx.Args[1])
		if uid == "" {
			ctx.Reply("Provide a valid user mention or ID.")
			return
		}
		p.random(ctx, uid)
	case "list":
		p.list(ctx)
	case "remove", "delete", "del":
		p.remove(ctx)
	case "search", "find":
		p.search(ctx)
	default:
		// Numeric? Show by id.
		if id, err := strconv.Atoi(ctx.Args[0]); err == nil {
			p.showID(ctx, id)
			return
		}
		ctx.Reply("Usage: !quote add <text> -- <author> | <id> | by @user | list | remove <id> | search <text>")
	}
}

func (p *Plugin) add(ctx *discord.CommandContext) {
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !quote add <text> -- <author>")
		return
	}
	raw := strings.Join(ctx.Args[1:], " ")

	// Split on " -- " to separate body from author.
	content := raw
	author := ""
	authorID := ""
	if idx := strings.LastIndex(raw, " -- "); idx >= 0 {
		content = strings.TrimSpace(raw[:idx])
		author = strings.TrimSpace(raw[idx+4:])
	}
	if content == "" {
		ctx.Reply("Quote cannot be empty.")
		return
	}
	if len(content) > 1500 {
		ctx.Reply("Quote too long (max 1500 chars).")
		return
	}
	if author == "" {
		author = "unknown"
	} else if uid := discord.ParseUserID(author); uid != "" {
		authorID = uid
	}

	id := p.nextID(ctx.GuildID)
	q := &quoteData{
		ID:        id,
		Content:   content,
		AuthorID:  authorID,
		Author:    author,
		AddedByID: ctx.AuthorID,
		AddedAt:   time.Now(),
	}
	p.save(ctx.GuildID, q)
	ctx.Reply(fmt.Sprintf("Quote **#%d** saved.", id))
}

func (p *Plugin) random(ctx *discord.CommandContext, authorID string) {
	rows := p.all(ctx.GuildID)
	if authorID != "" {
		filtered := rows[:0]
		for _, q := range rows {
			if q.AuthorID == authorID {
				filtered = append(filtered, q)
			}
		}
		rows = filtered
	}
	if len(rows) == 0 {
		if authorID != "" {
			ctx.Reply("No quotes for that user.")
		} else {
			ctx.Reply("No quotes yet. Add one with `!quote add <text> -- <author>`.")
		}
		return
	}
	q := rows[p.rng.Intn(len(rows))]
	ctx.ReplyEmbed(renderQuote(q))
}

func (p *Plugin) showID(ctx *discord.CommandContext, id int) {
	q := p.load(ctx.GuildID, id)
	if q == nil {
		ctx.Reply("No quote with that id.")
		return
	}
	ctx.ReplyEmbed(renderQuote(q))
}

func (p *Plugin) list(ctx *discord.CommandContext) {
	rows := p.all(ctx.GuildID)
	if len(rows) == 0 {
		ctx.Reply("No quotes yet.")
		return
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID > rows[j].ID })
	if len(rows) > 15 {
		rows = rows[:15]
	}
	var lines []string
	for _, q := range rows {
		lines = append(lines, fmt.Sprintf("**#%d** %s - *%s*",
			q.ID, truncate(q.Content, 80), q.Author))
	}
	ctx.ReplyEmbed(discord.Embed{
		Title:       "Recent quotes",
		Description: strings.Join(lines, "\n"),
		Color:       0x5865F2,
	})
}

func (p *Plugin) remove(ctx *discord.CommandContext) {
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !quote remove <id>")
		return
	}
	id, err := strconv.Atoi(ctx.Args[1])
	if err != nil {
		ctx.Reply("Id must be a number.")
		return
	}
	q := p.load(ctx.GuildID, id)
	if q == nil {
		ctx.Reply("No quote with that id.")
		return
	}
	// Only the person who added it or a mod can delete.
	if q.AddedByID != ctx.AuthorID && !hasManage(ctx) {
		ctx.Reply("You can only remove quotes you added (or mods can).")
		return
	}
	p.api.DeleteConfig(ctx.GuildID, fmt.Sprintf("q:%d", id))
	ctx.Reply(fmt.Sprintf("Quote **#%d** deleted.", id))
}

func (p *Plugin) search(ctx *discord.CommandContext) {
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !quote search <text>")
		return
	}
	needle := strings.ToLower(strings.Join(ctx.Args[1:], " "))
	var hits []*quoteData
	for _, q := range p.all(ctx.GuildID) {
		if strings.Contains(strings.ToLower(q.Content), needle) ||
			strings.Contains(strings.ToLower(q.Author), needle) {
			hits = append(hits, q)
		}
	}
	if len(hits) == 0 {
		ctx.Reply("No quotes match.")
		return
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].ID > hits[j].ID })
	if len(hits) > 10 {
		hits = hits[:10]
	}
	var lines []string
	for _, q := range hits {
		lines = append(lines, fmt.Sprintf("**#%d** %s - *%s*",
			q.ID, truncate(q.Content, 80), q.Author))
	}
	ctx.ReplyEmbed(discord.Embed{
		Title:       "Quote search: " + needle,
		Description: strings.Join(lines, "\n"),
		Color:       0x5865F2,
	})
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

func (p *Plugin) nextID(guildID string) int {
	cur, _ := strconv.Atoi(p.api.GetConfig(guildID, "counter"))
	cur++
	p.api.SetConfig(guildID, "counter", strconv.Itoa(cur))
	return cur
}

func (p *Plugin) load(guildID string, id int) *quoteData {
	raw := p.api.GetConfig(guildID, fmt.Sprintf("q:%d", id))
	if raw == "" {
		return nil
	}
	var q quoteData
	if err := json.Unmarshal([]byte(raw), &q); err != nil {
		return nil
	}
	return &q
}

func (p *Plugin) save(guildID string, q *quoteData) {
	b, err := json.Marshal(q)
	if err != nil {
		p.api.Log("quote: marshal: %v", err)
		return
	}
	p.api.SetConfig(guildID, fmt.Sprintf("q:%d", q.ID), string(b))
}

func (p *Plugin) all(guildID string) []*quoteData {
	var out []*quoteData
	for k, v := range p.api.AllConfig(guildID) {
		if !strings.HasPrefix(k, "q:") {
			continue
		}
		var q quoteData
		if err := json.Unmarshal([]byte(v), &q); err != nil {
			continue
		}
		qc := q
		out = append(out, &qc)
	}
	return out
}

// ---------------------------------------------------------------------------
// Rendering / helpers
// ---------------------------------------------------------------------------

func renderQuote(q *quoteData) discord.Embed {
	footer := q.AddedAt.UTC().Format("2006-01-02")
	return discord.Embed{
		Title:       fmt.Sprintf("Quote #%d", q.ID),
		Description: "\"" + q.Content + "\"\n\n\u2014 " + q.Author,
		Color:       0xFEE75C,
		Footer: &discord.EmbedFooter{
			Text: "added " + footer,
		},
	}
}

func hasManage(ctx *discord.CommandContext) bool {
	if ctx.Member == nil {
		return false
	}
	return ctx.Member.HasPermission(discord.PermissionManageMessages)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
