// Package backup snapshots and restores all plugin configuration for
// a guild. This is not a full Discord server backup (channels, roles,
// emojis, permissions) - that requires server-wide REST access and is
// better handled by dedicated tools. Instead, this plugin solves the
// thing that's usually *actually* painful: the plugin settings
// themselves.
//
// Every plugin in this collection stores its state via the
// per-guild config store (automod word lists, reactionroles panels,
// tags, welcome messages, starboard thresholds, ...). A snapshot
// captures that entire keyspace. A restore writes it back.
//
// Use cases:
//
//   - Moving config from a staging server to a production server.
//   - Rolling back a batch of config edits you regret.
//   - Cloning a "template" server's settings to a fresh server.
//   - Nightly off-box snapshots via `!backup export`.
//
// Commands (all require Administrator):
//
//	!backup create [name]        Snapshot current plugin config
//	!backup list                 List snapshots
//	!backup show <id>            Show snapshot summary
//	!backup delete <id>          Delete a snapshot
//	!backup export <id>          Upload snapshot as JSON
//	!backup import               Import from attached JSON (via reply)
//	!backup restore <id> confirm Restore a snapshot. Requires `confirm`.
package backup

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

// snapshot is the full backup record.
type snapshot struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	CreatedAt time.Time         `json:"created_at"`
	GuildID   string            `json:"guild_id"`
	GuildName string            `json:"guild_name"`
	Config    map[string]string `json:"config"`
}

// Plugin is the backup plugin instance.
type Plugin struct {
	api pluginapi.API
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "backup",
		Version:     "1.0.0",
		Description: "Snapshot and restore plugin configuration for a guild.",
		Author:      "HiLleywyn",
		Commands:    []string{"backup"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "backup",
		Description: "Snapshot and restore plugin configuration.",
		Usage:       "create [name] | list | show <id> | delete <id> | export <id> | import | restore <id> confirm",
		Handler:     p.handleCmd,
	})
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("backup")
	return nil
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if ctx.Member == nil || !ctx.Member.HasPermission(discord.PermissionAdministrator) {
		ctx.Reply("You need Administrator for this.")
		return
	}
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !backup create [name] | list | show <id> | delete <id> | export <id> | import | restore <id> confirm")
		return
	}
	switch strings.ToLower(ctx.Args[0]) {
	case "create", "snap", "snapshot":
		p.cmdCreate(ctx)
	case "list", "ls":
		p.cmdList(ctx)
	case "show", "info":
		p.cmdShow(ctx)
	case "delete", "rm", "del":
		p.cmdDelete(ctx)
	case "export", "dump":
		p.cmdExport(ctx)
	case "import", "load":
		p.cmdImport(ctx)
	case "restore":
		p.cmdRestore(ctx)
	default:
		ctx.Reply("Unknown subcommand. See `!backup` for usage.")
	}
}

// ---------------------------------------------------------------------------
// create / list / show / delete
// ---------------------------------------------------------------------------

func (p *Plugin) cmdCreate(ctx *discord.CommandContext) {
	name := "snapshot"
	if len(ctx.Args) >= 2 {
		name = strings.Join(ctx.Args[1:], " ")
	}
	if len(name) > 80 {
		name = name[:80]
	}

	guildName := ""
	if g, err := p.api.Rest().GetGuild(ctx.GuildID); err == nil && g != nil {
		guildName = g.Name
	}

	cfg := p.api.AllConfig(ctx.GuildID)
	clean := make(map[string]string, len(cfg))
	for k, v := range cfg {
		if strings.HasPrefix(k, "snap:") {
			continue // don't snapshot the snapshots themselves
		}
		clean[k] = v
	}

	id := fmt.Sprintf("%d", time.Now().UnixNano()/int64(time.Millisecond))
	snap := snapshot{
		ID:        id,
		Name:      name,
		CreatedAt: time.Now(),
		GuildID:   ctx.GuildID,
		GuildName: guildName,
		Config:    clean,
	}
	if err := p.save(ctx.GuildID, &snap); err != nil {
		ctx.Reply("Failed to save snapshot: " + err.Error())
		return
	}
	ctx.Reply(fmt.Sprintf("Snapshot created. id=`%s` name=`%s` entries=%d", id, name, len(clean)))
}

func (p *Plugin) cmdList(ctx *discord.CommandContext) {
	snaps := p.loadAll(ctx.GuildID)
	if len(snaps) == 0 {
		ctx.Reply("No snapshots. Create one with `!backup create`.")
		return
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].CreatedAt.After(snaps[j].CreatedAt) })
	var lines []string
	for _, s := range snaps {
		lines = append(lines, fmt.Sprintf("`%s` - **%s** - %d entries - %s",
			s.ID, s.Name, len(s.Config), s.CreatedAt.Format("2006-01-02 15:04")))
	}
	ctx.ReplyEmbed(discord.Embed{
		Title:       "Backups",
		Description: strings.Join(lines, "\n"),
		Color:       0x5865F2,
	})
}

func (p *Plugin) cmdShow(ctx *discord.CommandContext) {
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !backup show <id>")
		return
	}
	s := p.load(ctx.GuildID, ctx.Args[1])
	if s == nil {
		ctx.Reply("No snapshot with that id.")
		return
	}
	// Count config keys by plugin prefix (best-effort: the token before
	// the first `:` or `_`).
	byPlugin := map[string]int{}
	for k := range s.Config {
		prefix := "other"
		if i := strings.IndexAny(k, ":_"); i > 0 {
			prefix = k[:i]
		}
		byPlugin[prefix]++
	}
	type row struct {
		name  string
		count int
	}
	var rows []row
	for k, v := range byPlugin {
		rows = append(rows, row{k, v})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].count > rows[j].count })
	var bd strings.Builder
	fmt.Fprintf(&bd, "**Name:** %s\n**Created:** %s\n**Guild:** %s (%s)\n**Entries:** %d\n\n**By prefix:**\n",
		s.Name, s.CreatedAt.Format(time.RFC3339), s.GuildName, s.GuildID, len(s.Config))
	for _, r := range rows {
		fmt.Fprintf(&bd, "- `%s` - %d\n", r.name, r.count)
	}
	ctx.ReplyEmbed(discord.Embed{
		Title:       "Backup " + s.ID,
		Description: bd.String(),
		Color:       0x5865F2,
	})
}

func (p *Plugin) cmdDelete(ctx *discord.CommandContext) {
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !backup delete <id>")
		return
	}
	id := ctx.Args[1]
	if p.load(ctx.GuildID, id) == nil {
		ctx.Reply("No snapshot with that id.")
		return
	}
	p.api.DeleteConfig(ctx.GuildID, "snap:"+id)
	ctx.Reply("Snapshot `" + id + "` deleted.")
}

// ---------------------------------------------------------------------------
// export / import
// ---------------------------------------------------------------------------

func (p *Plugin) cmdExport(ctx *discord.CommandContext) {
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !backup export <id>")
		return
	}
	s := p.load(ctx.GuildID, ctx.Args[1])
	if s == nil {
		ctx.Reply("No snapshot with that id.")
		return
	}
	body, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		ctx.Reply("Marshal failed: " + err.Error())
		return
	}
	name := fmt.Sprintf("backup-%s-%s.json", s.GuildID, s.ID)
	caption := fmt.Sprintf("Backup `%s` (%d entries)", s.ID, len(s.Config))
	if _, err := p.api.Rest().SendFile(ctx.ChannelID, name, body, caption); err != nil {
		p.api.Log("backup: SendFile: %v", err)
		ctx.Reply("Failed to upload file: " + err.Error())
	}
}

func (p *Plugin) cmdImport(ctx *discord.CommandContext) {
	if ctx.Message == nil || len(ctx.Message.Attachments) == 0 {
		ctx.Reply("Attach a backup JSON file and run `!backup import` in the same message (or reply).")
		return
	}
	att := ctx.Message.Attachments[0]
	raw, err := p.api.Rest().DownloadAttachment(att.URL)
	if err != nil {
		ctx.Reply("Could not download attachment: " + err.Error())
		return
	}
	var s snapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		ctx.Reply("Attached file is not a valid backup JSON: " + err.Error())
		return
	}
	if s.Config == nil {
		ctx.Reply("Backup has no config entries.")
		return
	}
	// Re-key to this guild and generate a fresh id.
	s.ID = fmt.Sprintf("%d", time.Now().UnixNano()/int64(time.Millisecond))
	s.GuildID = ctx.GuildID
	if s.Name == "" {
		s.Name = "imported"
	}
	if err := p.save(ctx.GuildID, &s); err != nil {
		ctx.Reply("Failed to save imported snapshot: " + err.Error())
		return
	}
	ctx.Reply(fmt.Sprintf("Imported. id=`%s` entries=%d. Use `!backup restore %s confirm` to apply it.", s.ID, len(s.Config), s.ID))
}

// ---------------------------------------------------------------------------
// restore
// ---------------------------------------------------------------------------

func (p *Plugin) cmdRestore(ctx *discord.CommandContext) {
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !backup restore <id> confirm")
		return
	}
	confirmed := len(ctx.Args) >= 3 && strings.EqualFold(ctx.Args[2], "confirm")
	s := p.load(ctx.GuildID, ctx.Args[1])
	if s == nil {
		ctx.Reply("No snapshot with that id.")
		return
	}
	if !confirmed {
		ctx.Reply(fmt.Sprintf(
			"This will overwrite **every** plugin's current config for this guild with the %d entries in snapshot `%s`.\nRun `!backup restore %s confirm` to proceed.",
			len(s.Config), s.ID, s.ID))
		return
	}

	// Before destructive restore, auto-snapshot the current state as a
	// rollback point.
	current := p.api.AllConfig(ctx.GuildID)
	guildName := ""
	if g, err := p.api.Rest().GetGuild(ctx.GuildID); err == nil && g != nil {
		guildName = g.Name
	}
	rollbackCfg := map[string]string{}
	for k, v := range current {
		if strings.HasPrefix(k, "snap:") {
			continue
		}
		rollbackCfg[k] = v
	}
	rollback := &snapshot{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()/int64(time.Millisecond)),
		Name:      "auto-before-restore-" + s.ID,
		CreatedAt: time.Now(),
		GuildID:   ctx.GuildID,
		GuildName: guildName,
		Config:    rollbackCfg,
	}
	if err := p.save(ctx.GuildID, rollback); err != nil {
		ctx.Reply("Could not create rollback snapshot, refusing to restore: " + err.Error())
		return
	}

	// Wipe current config (but keep snap:* entries so rollback survives),
	// then write snapshot values.
	for k := range current {
		if strings.HasPrefix(k, "snap:") {
			continue
		}
		p.api.DeleteConfig(ctx.GuildID, k)
	}
	applied := 0
	for k, v := range s.Config {
		if strings.HasPrefix(k, "snap:") {
			continue
		}
		p.api.SetConfig(ctx.GuildID, k, v)
		applied++
	}

	ctx.Reply(fmt.Sprintf(
		"Restored snapshot `%s` (%s). Applied %d entries.\nAuto-rollback saved as `%s`. Reload plugins for all changes to take effect.",
		s.ID, s.Name, applied, rollback.ID))
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

func (p *Plugin) save(guildID string, s *snapshot) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	p.api.SetConfig(guildID, "snap:"+s.ID, string(b))
	return nil
}

func (p *Plugin) load(guildID, id string) *snapshot {
	raw := p.api.GetConfig(guildID, "snap:"+id)
	if raw == "" {
		return nil
	}
	var s snapshot
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return nil
	}
	return &s
}

func (p *Plugin) loadAll(guildID string) []*snapshot {
	var out []*snapshot
	for k, v := range p.api.AllConfig(guildID) {
		if !strings.HasPrefix(k, "snap:") {
			continue
		}
		var s snapshot
		if err := json.Unmarshal([]byte(v), &s); err != nil {
			continue
		}
		out = append(out, &s)
	}
	return out
}
