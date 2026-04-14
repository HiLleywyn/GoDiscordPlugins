// Package tags provides reusable text snippets callable by name.
//
// Anyone in the guild can invoke a saved tag with `!tag <name>`, which posts
// its stored content. Only the author of a tag (or someone with manage perms)
// can edit or delete it via `!tag edit` / `!tag delete`.
//
// Tag contents support a few variables:
//
//	{user}        - invoker's mention
//	{server}      - guild name
//	{membercount} - current member count
package tags

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

const maxContentLen = 1800

// tagData is the persisted record for one tag.
type tagData struct {
	Name      string    `json:"name"`
	Content   string    `json:"content"`
	AuthorID  string    `json:"author_id"`
	CreatedAt time.Time `json:"created_at"`
	Uses      int       `json:"uses"`
}

// Plugin is the tags plugin instance.
type Plugin struct {
	api pluginapi.API
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "tags",
		Version:     "1.0.0",
		Description: "Reusable text snippets callable by name.",
		Author:      "HiLleywyn",
		Commands:    []string{"tag"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "tag",
		Aliases:     []string{"t"},
		Description: "Create and recall text snippets.",
		Usage:       "<name> | add <name> <content> | edit <name> <content> | delete <name> | info <name> | list | search <query> | top",
		Handler:     p.handleCmd,
	})
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("tag")
	return nil
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !tag <name> | add <name> <content> | edit <name> <content> | delete <name> | info <name> | list | search <query> | top")
		return
	}

	head := strings.ToLower(ctx.Args[0])
	switch head {
	case "add", "create":
		p.cmdAdd(ctx)
	case "edit":
		p.cmdEdit(ctx)
	case "delete", "remove":
		p.cmdDelete(ctx)
	case "info":
		p.cmdInfo(ctx)
	case "list":
		p.cmdList(ctx)
	case "search":
		p.cmdSearch(ctx)
	case "top":
		p.cmdTop(ctx)
	default:
		// First arg is treated as a tag name to invoke.
		p.invokeTag(ctx, head)
	}
}

func (p *Plugin) cmdAdd(ctx *discord.CommandContext) {
	if len(ctx.Args) < 3 {
		ctx.Reply("Usage: !tag add <name> <content>")
		return
	}
	name := normaliseName(ctx.Args[1])
	if !validName(name) {
		ctx.Reply("Name must be 1-32 characters, letters/digits/_/- only.")
		return
	}
	if p.loadTag(ctx.GuildID, name) != nil {
		ctx.Reply("A tag with that name already exists. Use `!tag edit`.")
		return
	}
	content := strings.Join(ctx.Args[2:], " ")
	if len(content) > maxContentLen {
		ctx.Reply(fmt.Sprintf("Content too long (max %d characters).", maxContentLen))
		return
	}
	t := &tagData{
		Name:      name,
		Content:   content,
		AuthorID:  ctx.AuthorID,
		CreatedAt: time.Now(),
	}
	p.saveTag(ctx.GuildID, t)
	ctx.Reply("Tag `" + name + "` created.")
}

func (p *Plugin) cmdEdit(ctx *discord.CommandContext) {
	if len(ctx.Args) < 3 {
		ctx.Reply("Usage: !tag edit <name> <content>")
		return
	}
	name := normaliseName(ctx.Args[1])
	t := p.loadTag(ctx.GuildID, name)
	if t == nil {
		ctx.Reply("No tag named `" + name + "`.")
		return
	}
	if t.AuthorID != ctx.AuthorID && !hasManage(ctx) {
		ctx.Reply("Only the tag author (or a mod) can edit this tag.")
		return
	}
	content := strings.Join(ctx.Args[2:], " ")
	if len(content) > maxContentLen {
		ctx.Reply(fmt.Sprintf("Content too long (max %d characters).", maxContentLen))
		return
	}
	t.Content = content
	p.saveTag(ctx.GuildID, t)
	ctx.Reply("Tag `" + name + "` updated.")
}

func (p *Plugin) cmdDelete(ctx *discord.CommandContext) {
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !tag delete <name>")
		return
	}
	name := normaliseName(ctx.Args[1])
	t := p.loadTag(ctx.GuildID, name)
	if t == nil {
		ctx.Reply("No tag named `" + name + "`.")
		return
	}
	if t.AuthorID != ctx.AuthorID && !hasManage(ctx) {
		ctx.Reply("Only the tag author (or a mod) can delete this tag.")
		return
	}
	p.api.DeleteConfig(ctx.GuildID, "tag:"+name)
	ctx.Reply("Tag `" + name + "` deleted.")
}

func (p *Plugin) cmdInfo(ctx *discord.CommandContext) {
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !tag info <name>")
		return
	}
	name := normaliseName(ctx.Args[1])
	t := p.loadTag(ctx.GuildID, name)
	if t == nil {
		ctx.Reply("No tag named `" + name + "`.")
		return
	}
	ctx.ReplyEmbed(discord.Embed{
		Title: "Tag: " + t.Name,
		Fields: []discord.EmbedField{
			{Name: "Author", Value: "<@" + t.AuthorID + ">", Inline: true},
			{Name: "Created", Value: fmt.Sprintf("<t:%d:R>", t.CreatedAt.Unix()), Inline: true},
			{Name: "Uses", Value: strconv.Itoa(t.Uses), Inline: true},
		},
		Color: 0x5865F2,
	})
}

func (p *Plugin) cmdList(ctx *discord.CommandContext) {
	names := p.allTagNames(ctx.GuildID)
	if len(names) == 0 {
		ctx.Reply("No tags defined.")
		return
	}
	sort.Strings(names)
	ctx.ReplyEmbed(discord.Embed{
		Title:       fmt.Sprintf("Tags (%d)", len(names)),
		Description: "`" + strings.Join(names, "`, `") + "`",
		Color:       0x5865F2,
	})
}

func (p *Plugin) cmdSearch(ctx *discord.CommandContext) {
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !tag search <query>")
		return
	}
	q := strings.ToLower(strings.Join(ctx.Args[1:], " "))
	var matches []string
	for _, name := range p.allTagNames(ctx.GuildID) {
		if strings.Contains(name, q) {
			matches = append(matches, name)
		}
	}
	if len(matches) == 0 {
		ctx.Reply("No matches.")
		return
	}
	sort.Strings(matches)
	ctx.ReplyEmbed(discord.Embed{
		Title:       fmt.Sprintf("Tag search: %q", q),
		Description: "`" + strings.Join(matches, "`, `") + "`",
		Color:       0x5865F2,
	})
}

func (p *Plugin) cmdTop(ctx *discord.CommandContext) {
	type row struct {
		name string
		uses int
	}
	var rows []row
	for _, name := range p.allTagNames(ctx.GuildID) {
		if t := p.loadTag(ctx.GuildID, name); t != nil {
			rows = append(rows, row{t.Name, t.Uses})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].uses > rows[j].uses })
	if len(rows) > 10 {
		rows = rows[:10]
	}
	if len(rows) == 0 {
		ctx.Reply("No tags defined.")
		return
	}
	var lines []string
	for i, r := range rows {
		lines = append(lines, fmt.Sprintf("**%d.** `%s` - %d uses", i+1, r.name, r.uses))
	}
	ctx.ReplyEmbed(discord.Embed{
		Title:       "Top tags",
		Description: strings.Join(lines, "\n"),
		Color:       0x5865F2,
	})
}

// invokeTag posts the content of the named tag, or an error if it doesn't exist.
func (p *Plugin) invokeTag(ctx *discord.CommandContext, name string) {
	name = normaliseName(name)
	t := p.loadTag(ctx.GuildID, name)
	if t == nil {
		ctx.Reply("No tag named `" + name + "`. Try `!tag list`.")
		return
	}
	t.Uses++
	p.saveTag(ctx.GuildID, t)

	rendered := p.substitute(t.Content, ctx)
	ctx.Reply(rendered)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (p *Plugin) loadTag(guildID, name string) *tagData {
	raw := p.api.GetConfig(guildID, "tag:"+name)
	if raw == "" {
		return nil
	}
	var t tagData
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		p.api.Log("tags: unmarshal %s: %v", name, err)
		return nil
	}
	return &t
}

func (p *Plugin) saveTag(guildID string, t *tagData) {
	b, err := json.Marshal(t)
	if err != nil {
		p.api.Log("tags: marshal %s: %v", t.Name, err)
		return
	}
	p.api.SetConfig(guildID, "tag:"+t.Name, string(b))
}

func (p *Plugin) allTagNames(guildID string) []string {
	var names []string
	for k := range p.api.AllConfig(guildID) {
		if strings.HasPrefix(k, "tag:") {
			names = append(names, strings.TrimPrefix(k, "tag:"))
		}
	}
	return names
}

func (p *Plugin) substitute(tmpl string, ctx *discord.CommandContext) string {
	out := strings.ReplaceAll(tmpl, "{user}", "<@"+ctx.AuthorID+">")
	if strings.Contains(out, "{server}") || strings.Contains(out, "{membercount}") {
		if g, err := p.api.Rest().GetGuild(ctx.GuildID); err == nil && g != nil {
			out = strings.ReplaceAll(out, "{server}", g.Name)
			out = strings.ReplaceAll(out, "{membercount}", strconv.Itoa(g.MemberCount))
		}
	}
	return out
}

// hasManage is a best-effort check for whether the command author is a mod.
// It defers to the framework's member permission helper when available.
func hasManage(ctx *discord.CommandContext) bool {
	if ctx.Member == nil {
		return false
	}
	return ctx.Member.HasPermission(discord.PermissionManageMessages)
}

func normaliseName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func validName(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') ||
			r == '_' || r == '-'
		if !ok {
			return false
		}
	}
	return true
}
