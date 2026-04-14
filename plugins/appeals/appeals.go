// Package appeals implements a ban-appeal inbox.
//
// Users can submit an appeal with `!appeal <text>`, which posts the
// appeal as an embed in a configured mod channel. Mods review it,
// approve/deny with a note, and the user is DMed with the decision.
// Each appeal gets an incrementing numeric id.
//
// Users don't need to be in the server to run this - if the bot shares
// any guild with them, they can `!appeal` in that guild's context, or
// DM the command to the bot if DM commands are supported by the
// framework. This plugin assumes channel invocation.
//
// Commands:
//
//	!appeal <text>                 File a new appeal (users)
//	!appeal channel #chan          Admin: set mod review channel
//	!appeal approve <id> [note]    Mod: approve
//	!appeal deny <id> [note]       Mod: deny
//	!appeal show <id>              Mod: show one appeal
//	!appeal list [open|closed]     Mod: list appeals
package appeals

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
	statusOpen     = "open"
	statusApproved = "approved"
	statusDenied   = "denied"
)

// appealData is the persisted record.
type appealData struct {
	ID        int       `json:"id"`
	UserID    string    `json:"user_id"`
	UserTag   string    `json:"user_tag"`
	Content   string    `json:"content"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	ModID     string    `json:"mod_id,omitempty"`
	ModNote   string    `json:"mod_note,omitempty"`
	MessageID string    `json:"message_id,omitempty"`
	ChannelID string    `json:"channel_id,omitempty"`
}

// Plugin is the appeals plugin instance.
type Plugin struct {
	api pluginapi.API
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "appeals",
		Version:     "1.0.0",
		Description: "Ban-appeal inbox with mod review workflow.",
		Author:      "HiLleywyn",
		Commands:    []string{"appeal"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "appeal",
		Description: "File or manage ban appeals.",
		Usage:       "<text> | channel #chan | approve <id> [note] | deny <id> [note] | show <id> | list [open|closed]",
		Handler:     p.handleCmd,
	})
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("appeal")
	return nil
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !appeal <text> | channel #chan | approve <id> [note] | deny <id> [note] | show <id> | list [open|closed]")
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "channel":
		p.cmdChannel(ctx)
	case "approve":
		p.cmdDecision(ctx, statusApproved)
	case "deny":
		p.cmdDecision(ctx, statusDenied)
	case "show":
		p.cmdShow(ctx)
	case "list":
		p.cmdList(ctx)
	default:
		p.cmdFile(ctx)
	}
}

func (p *Plugin) cmdFile(ctx *discord.CommandContext) {
	chID := p.api.GetConfig(ctx.GuildID, "channel")
	if chID == "" {
		ctx.Reply("Appeals are not set up in this server.")
		return
	}
	text := strings.TrimSpace(strings.Join(ctx.Args, " "))
	if text == "" {
		ctx.Reply("Provide an appeal reason.")
		return
	}
	if len(text) > 1500 {
		ctx.Reply("Appeal too long (max 1500 characters).")
		return
	}

	// Rate-limit: one open appeal per user.
	for _, a := range p.all(ctx.GuildID) {
		if a.UserID == ctx.AuthorID && a.Status == statusOpen {
			ctx.Reply(fmt.Sprintf("You already have an open appeal (**#%d**). Wait for a mod to review it.", a.ID))
			return
		}
	}

	id := p.nextID(ctx.GuildID)
	ad := &appealData{
		ID:        id,
		UserID:    ctx.AuthorID,
		UserTag:   tagOf(ctx),
		Content:   text,
		Status:    statusOpen,
		CreatedAt: time.Now(),
		ChannelID: chID,
	}

	msg, err := ctx.Bot.Rest.SendEmbed(chID, renderAppeal(ad))
	if err != nil || msg == nil {
		ctx.Reply("Failed to post appeal to mod channel.")
		return
	}
	ad.MessageID = msg.ID
	p.save(ctx.GuildID, ad)
	ctx.Reply(fmt.Sprintf("Appeal **#%d** submitted. A mod will review it.", id))
}

func (p *Plugin) cmdChannel(ctx *discord.CommandContext) {
	if !isAdmin(ctx) {
		ctx.Reply("You need Manage Messages for this.")
		return
	}
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !appeal channel #chan")
		return
	}
	chID := discord.ParseChannelMention(ctx.Args[1])
	if chID == "" {
		ctx.Reply("Provide a valid channel.")
		return
	}
	p.api.SetConfig(ctx.GuildID, "channel", chID)
	ctx.Reply("Appeals will be posted in <#" + chID + ">.")
}

func (p *Plugin) cmdDecision(ctx *discord.CommandContext, status string) {
	if !isAdmin(ctx) {
		ctx.Reply("You need Manage Messages for this.")
		return
	}
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !appeal " + status + " <id> [note]")
		return
	}
	id, err := strconv.Atoi(ctx.Args[1])
	if err != nil {
		ctx.Reply("Id must be a number.")
		return
	}
	ad := p.load(ctx.GuildID, id)
	if ad == nil {
		ctx.Reply("No appeal with that id.")
		return
	}
	if ad.Status != statusOpen {
		ctx.Reply("That appeal is already closed.")
		return
	}

	ad.Status = status
	ad.ModID = ctx.AuthorID
	if len(ctx.Args) > 2 {
		ad.ModNote = strings.Join(ctx.Args[2:], " ")
	}
	p.save(ctx.GuildID, ad)

	// Update the embed in the mod channel.
	if ad.ChannelID != "" && ad.MessageID != "" {
		if _, err := ctx.Bot.Rest.EditEmbed(ad.ChannelID, ad.MessageID, renderAppeal(ad)); err != nil {
			p.api.Log("appeals: edit embed: %v", err)
		}
	}

	// DM the user with the decision.
	dm := fmt.Sprintf("Your appeal **#%d** was **%s**.", ad.ID, status)
	if ad.ModNote != "" {
		dm += "\nMod note: " + ad.ModNote
	}
	if _, err := ctx.Bot.Rest.SendDM(ad.UserID, dm); err != nil {
		p.api.Log("appeals: dm %s: %v", ad.UserID, err)
	}
	ctx.Reply(fmt.Sprintf("Appeal **#%d** %s.", ad.ID, status))
}

func (p *Plugin) cmdShow(ctx *discord.CommandContext) {
	if !isAdmin(ctx) {
		ctx.Reply("You need Manage Messages for this.")
		return
	}
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !appeal show <id>")
		return
	}
	id, err := strconv.Atoi(ctx.Args[1])
	if err != nil {
		ctx.Reply("Id must be a number.")
		return
	}
	ad := p.load(ctx.GuildID, id)
	if ad == nil {
		ctx.Reply("No appeal with that id.")
		return
	}
	ctx.ReplyEmbed(renderAppeal(ad))
}

func (p *Plugin) cmdList(ctx *discord.CommandContext) {
	if !isAdmin(ctx) {
		ctx.Reply("You need Manage Messages for this.")
		return
	}
	filter := "open"
	if len(ctx.Args) >= 2 {
		filter = strings.ToLower(ctx.Args[1])
	}
	var rows []*appealData
	for _, a := range p.all(ctx.GuildID) {
		switch filter {
		case "open":
			if a.Status == statusOpen {
				rows = append(rows, a)
			}
		case "closed":
			if a.Status != statusOpen {
				rows = append(rows, a)
			}
		case "all":
			rows = append(rows, a)
		}
	}
	if len(rows) == 0 {
		ctx.Reply("No appeals match.")
		return
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID > rows[j].ID })
	if len(rows) > 15 {
		rows = rows[:15]
	}
	var lines []string
	for _, a := range rows {
		lines = append(lines, fmt.Sprintf("**#%d** [%s] %s - %s",
			a.ID, a.Status, a.UserTag, truncate(a.Content, 70)))
	}
	ctx.ReplyEmbed(discord.Embed{
		Title:       "Appeals - " + filter,
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

func (p *Plugin) load(guildID string, id int) *appealData {
	raw := p.api.GetConfig(guildID, fmt.Sprintf("a:%d", id))
	if raw == "" {
		return nil
	}
	var ad appealData
	if err := json.Unmarshal([]byte(raw), &ad); err != nil {
		return nil
	}
	return &ad
}

func (p *Plugin) save(guildID string, ad *appealData) {
	b, err := json.Marshal(ad)
	if err != nil {
		p.api.Log("appeals: marshal: %v", err)
		return
	}
	p.api.SetConfig(guildID, fmt.Sprintf("a:%d", ad.ID), string(b))
}

func (p *Plugin) all(guildID string) []*appealData {
	var out []*appealData
	for k, v := range p.api.AllConfig(guildID) {
		if !strings.HasPrefix(k, "a:") {
			continue
		}
		var ad appealData
		if err := json.Unmarshal([]byte(v), &ad); err != nil {
			continue
		}
		adc := ad
		out = append(out, &adc)
	}
	return out
}

// ---------------------------------------------------------------------------
// Rendering / helpers
// ---------------------------------------------------------------------------

func renderAppeal(ad *appealData) discord.Embed {
	color := 0x5865F2
	switch ad.Status {
	case statusApproved:
		color = 0x57F287
	case statusDenied:
		color = 0xED4245
	}
	fields := []discord.EmbedField{
		{Name: "User", Value: fmt.Sprintf("<@%s> (%s)", ad.UserID, ad.UserID), Inline: true},
		{Name: "Status", Value: ad.Status, Inline: true},
	}
	if ad.ModID != "" {
		fields = append(fields, discord.EmbedField{
			Name: "Reviewer", Value: "<@" + ad.ModID + ">", Inline: true,
		})
	}
	if ad.ModNote != "" {
		fields = append(fields, discord.EmbedField{Name: "Mod note", Value: ad.ModNote})
	}
	return discord.Embed{
		Title:       fmt.Sprintf("Appeal #%d", ad.ID),
		Description: ad.Content,
		Color:       color,
		Fields:      fields,
		Footer: &discord.EmbedFooter{
			Text: ad.CreatedAt.UTC().Format("2006-01-02 15:04 UTC"),
		},
	}
}

func tagOf(ctx *discord.CommandContext) string {
	if ctx.Message != nil && ctx.Message.Author != nil {
		return ctx.Message.Author.Tag()
	}
	return ctx.AuthorID
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func isAdmin(ctx *discord.CommandContext) bool {
	if ctx.Member == nil {
		return false
	}
	return ctx.Member.HasPermission(discord.PermissionManageMessages)
}
