// Package reactionroles lets users self-assign roles by reacting to a
// configured message. An admin creates a panel with !rr create, binds emoji
// to roles with !rr bind, and posts the panel embed which users can react to.
package reactionroles

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

// panelData holds the persisted state of a single reaction-role panel.
type panelData struct {
	GuildID   string            `json:"guild_id"`
	ChannelID string            `json:"channel_id"`
	MessageID string            `json:"message_id"`
	Title     string            `json:"title"`
	Bindings  map[string]string `json:"bindings"` // emoji -> roleID
}

// Plugin is the reactionroles plugin instance.
type Plugin struct {
	api pluginapi.API
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "reactionroles",
		Version:     "1.0.0",
		Description: "Self-assignable roles via reactions.",
		Author:      "HiLleywyn",
		Commands:    []string{"rr"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "rr",
		Description: "Reaction-role panel management.",
		Usage:       "create <title> | bind <panel> <emoji> @role | unbind <panel> <emoji> | post <panel> #channel | list",
		Handler:     p.handleCmd,
	})
	api.OnReactionAdd(p.onReactionAdd)
	api.OnReactionRemove(p.onReactionRemove)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("rr")
	return nil
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !rr create <title> | bind <panel> <emoji> @role | unbind <panel> <emoji> | post <panel> #channel | list")
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "create":
		p.cmdCreate(ctx)
	case "bind":
		p.cmdBind(ctx)
	case "unbind":
		p.cmdUnbind(ctx)
	case "post":
		p.cmdPost(ctx)
	case "list":
		p.cmdList(ctx)
	case "delete":
		p.cmdDelete(ctx)
	default:
		ctx.Reply("Unknown subcommand. Try: create, bind, unbind, post, list, delete")
	}
}

func (p *Plugin) cmdCreate(ctx *discord.CommandContext) {
	if len(ctx.Args) < 3 {
		ctx.Reply("Usage: !rr create <panel-name> <title...>")
		return
	}
	name := strings.ToLower(ctx.Args[1])
	title := strings.Join(ctx.Args[2:], " ")

	if p.api.GetConfig(ctx.GuildID, "panel:"+name) != "" {
		ctx.Reply("A panel named '" + name + "' already exists.")
		return
	}

	pnl := &panelData{
		GuildID:  ctx.GuildID,
		Title:    title,
		Bindings: map[string]string{},
	}
	p.savePanel(ctx.GuildID, name, pnl)
	ctx.Reply("Panel '" + name + "' created. Add bindings with `!rr bind " + name + " <emoji> @role`.")
}

func (p *Plugin) cmdBind(ctx *discord.CommandContext) {
	if len(ctx.Args) < 4 {
		ctx.Reply("Usage: !rr bind <panel> <emoji> @role")
		return
	}
	name := strings.ToLower(ctx.Args[1])
	emoji := ctx.Args[2]
	roleID := discord.ParseRoleMention(ctx.Args[3])

	pnl, ok := p.loadPanel(ctx.GuildID, name)
	if !ok {
		ctx.Reply("No panel named '" + name + "'.")
		return
	}
	pnl.Bindings[emoji] = roleID
	p.savePanel(ctx.GuildID, name, pnl)

	// If the panel is already posted, add the reaction to the live message.
	if pnl.MessageID != "" {
		_ = ctx.Bot.Rest.AddReaction(pnl.ChannelID, pnl.MessageID, emoji)
		_ = p.rerenderPanel(ctx.Bot, name, pnl)
	}
	ctx.Reply(fmt.Sprintf("Bound %s -> <@&%s> on panel '%s'.", emoji, roleID, name))
}

func (p *Plugin) cmdUnbind(ctx *discord.CommandContext) {
	if len(ctx.Args) < 3 {
		ctx.Reply("Usage: !rr unbind <panel> <emoji>")
		return
	}
	name := strings.ToLower(ctx.Args[1])
	emoji := ctx.Args[2]

	pnl, ok := p.loadPanel(ctx.GuildID, name)
	if !ok {
		ctx.Reply("No panel named '" + name + "'.")
		return
	}
	delete(pnl.Bindings, emoji)
	p.savePanel(ctx.GuildID, name, pnl)

	if pnl.MessageID != "" {
		_ = p.rerenderPanel(ctx.Bot, name, pnl)
	}
	ctx.Reply("Binding removed.")
}

func (p *Plugin) cmdPost(ctx *discord.CommandContext) {
	if len(ctx.Args) < 3 {
		ctx.Reply("Usage: !rr post <panel> #channel")
		return
	}
	name := strings.ToLower(ctx.Args[1])
	chID := discord.ParseChannelMention(ctx.Args[2])

	pnl, ok := p.loadPanel(ctx.GuildID, name)
	if !ok {
		ctx.Reply("No panel named '" + name + "'.")
		return
	}
	if len(pnl.Bindings) == 0 {
		ctx.Reply("Panel has no bindings yet. Use `!rr bind " + name + " <emoji> @role` first.")
		return
	}

	embed := buildEmbed(pnl)
	msg, err := ctx.Bot.Rest.SendEmbed(chID, embed)
	if err != nil || msg == nil {
		ctx.Reply("Failed to post panel: " + errStr(err))
		return
	}
	for emoji := range pnl.Bindings {
		_ = ctx.Bot.Rest.AddReaction(chID, msg.ID, emoji)
	}

	pnl.ChannelID = chID
	pnl.MessageID = msg.ID
	p.savePanel(ctx.GuildID, name, pnl)

	// Index the message ID -> panel name for reaction lookup.
	p.api.SetConfig(ctx.GuildID, "msg:"+msg.ID, name)
	ctx.Reply("Panel posted.")
}

func (p *Plugin) cmdList(ctx *discord.CommandContext) {
	all := p.api.AllConfig(ctx.GuildID)
	var names []string
	for k := range all {
		if strings.HasPrefix(k, "panel:") {
			names = append(names, strings.TrimPrefix(k, "panel:"))
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		ctx.Reply("No panels configured.")
		return
	}

	var lines []string
	for _, n := range names {
		pnl, _ := p.loadPanel(ctx.GuildID, n)
		state := "unposted"
		if pnl != nil && pnl.MessageID != "" {
			state = "posted in <#" + pnl.ChannelID + ">"
		}
		lines = append(lines, fmt.Sprintf("**%s** - %d bindings (%s)", n, len(pnl.Bindings), state))
	}
	ctx.ReplyEmbed(discord.Embed{
		Title:       "Reaction Role Panels",
		Description: strings.Join(lines, "\n"),
		Color:       0x5865F2,
	})
}

func (p *Plugin) cmdDelete(ctx *discord.CommandContext) {
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !rr delete <panel>")
		return
	}
	name := strings.ToLower(ctx.Args[1])
	pnl, ok := p.loadPanel(ctx.GuildID, name)
	if !ok {
		ctx.Reply("No panel named '" + name + "'.")
		return
	}
	if pnl.MessageID != "" {
		p.api.DeleteConfig(ctx.GuildID, "msg:"+pnl.MessageID)
	}
	p.api.DeleteConfig(ctx.GuildID, "panel:"+name)
	ctx.Reply("Panel '" + name + "' deleted.")
}

// ---------------------------------------------------------------------------
// Reaction handlers
// ---------------------------------------------------------------------------

func (p *Plugin) onReactionAdd(bot *discord.Bot, ev *discord.MessageReactionAddEvent) {
	if ev.GuildID == "" || ev.UserID == p.api.BotID() {
		return
	}
	name := p.api.GetConfig(ev.GuildID, "msg:"+ev.MessageID)
	if name == "" {
		return
	}
	pnl, ok := p.loadPanel(ev.GuildID, name)
	if !ok {
		return
	}
	roleID, ok := pnl.Bindings[emojiKey(ev.Emoji)]
	if !ok {
		// Stray reaction - remove it to keep the panel clean.
		_ = bot.Rest.RemoveUserReaction(ev.ChannelID, ev.MessageID, emojiKey(ev.Emoji), ev.UserID)
		return
	}
	if err := bot.Rest.AddMemberRole(ev.GuildID, ev.UserID, roleID); err != nil {
		p.api.Log("reactionroles: add role: %v", err)
	}
}

func (p *Plugin) onReactionRemove(bot *discord.Bot, ev *discord.MessageReactionRemoveEvent) {
	if ev.GuildID == "" || ev.UserID == p.api.BotID() {
		return
	}
	name := p.api.GetConfig(ev.GuildID, "msg:"+ev.MessageID)
	if name == "" {
		return
	}
	pnl, ok := p.loadPanel(ev.GuildID, name)
	if !ok {
		return
	}
	roleID, ok := pnl.Bindings[emojiKey(ev.Emoji)]
	if !ok {
		return
	}
	if err := bot.Rest.RemoveMemberRole(ev.GuildID, ev.UserID, roleID); err != nil {
		p.api.Log("reactionroles: remove role: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Panel storage and rendering
// ---------------------------------------------------------------------------

func (p *Plugin) loadPanel(guildID, name string) (*panelData, bool) {
	raw := p.api.GetConfig(guildID, "panel:"+name)
	if raw == "" {
		return nil, false
	}
	var pnl panelData
	if err := json.Unmarshal([]byte(raw), &pnl); err != nil {
		p.api.Log("reactionroles: unmarshal %s: %v", name, err)
		return nil, false
	}
	if pnl.Bindings == nil {
		pnl.Bindings = map[string]string{}
	}
	return &pnl, true
}

func (p *Plugin) savePanel(guildID, name string, pnl *panelData) {
	b, err := json.Marshal(pnl)
	if err != nil {
		p.api.Log("reactionroles: marshal %s: %v", name, err)
		return
	}
	p.api.SetConfig(guildID, "panel:"+name, string(b))
}

func (p *Plugin) rerenderPanel(bot *discord.Bot, name string, pnl *panelData) error {
	if pnl.ChannelID == "" || pnl.MessageID == "" {
		return nil
	}
	return bot.Rest.EditEmbed(pnl.ChannelID, pnl.MessageID, buildEmbed(pnl))
}

func buildEmbed(pnl *panelData) discord.Embed {
	// Deterministic ordering so the embed doesn't jitter when re-rendered.
	keys := make([]string, 0, len(pnl.Bindings))
	for k := range pnl.Bindings {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var lines []string
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("%s  <@&%s>", k, pnl.Bindings[k]))
	}
	desc := "React to assign a role."
	if len(lines) > 0 {
		desc += "\n\n" + strings.Join(lines, "\n")
	}

	return discord.Embed{
		Title:       pnl.Title,
		Description: desc,
		Color:       0x5865F2,
	}
}

// emojiKey normalises the reaction emoji to the same key format used in bindings.
// Users enter unicode emoji directly, so we store them as-is. Custom emoji are
// represented as ":name:id".
func emojiKey(e discord.Emoji) string {
	if e.ID != "" {
		return ":" + e.Name + ":" + e.ID
	}
	return e.Name
}

func errStr(err error) string {
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}
