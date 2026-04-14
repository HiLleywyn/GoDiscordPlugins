// Package threadkeeper keeps specified threads and channels alive by
// posting a low-noise heartbeat on a schedule.
//
// Discord archives inactive threads after 24h (or 3/7 days depending on
// server boost level). That's annoying for long-lived discussion
// threads, project threads, or onboarding threads you want users to
// always be able to find. This plugin periodically posts a tiny
// heartbeat message to keep them unarchived.
//
// It also works for regular channels where you want a scheduled pin-it
// nudge, though that's a secondary use case.
//
// Commands:
//
//	!threadkeeper add #thread [interval-hours]  Keep a thread alive
//	!threadkeeper remove #thread                Stop keeping it alive
//	!threadkeeper list                          Show tracked threads
//	!threadkeeper message #thread <text>        Custom heartbeat text
package threadkeeper

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

const defaultHeartbeat = "_(keeping this thread alive)_"

type keeperRecord struct {
	ChannelID    string    `json:"channel_id"`
	IntervalHrs  int       `json:"interval_hrs"`
	Message      string    `json:"message"`
	LastPostedAt time.Time `json:"last_posted_at"`
}

// Plugin is the threadkeeper plugin instance.
type Plugin struct {
	api    pluginapi.API
	cancel context.CancelFunc
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "threadkeeper",
		Version:     "1.0.0",
		Description: "Keep long-running threads from auto-archiving.",
		Author:      "HiLleywyn",
		Commands:    []string{"threadkeeper"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "threadkeeper",
		Description: "Keep threads alive on a schedule.",
		Usage:       "add #thread [hours] | remove #thread | list | message #thread <text>",
		Handler:     p.handleCmd,
	})

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go p.tick(ctx)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("threadkeeper")
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if ctx.Member == nil || !ctx.Member.HasPermission(discord.PermissionManageMessages) {
		ctx.Reply("You need Manage Messages for this.")
		return
	}
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !threadkeeper add #thread [hours] | remove #thread | list | message #thread <text>")
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "add":
		p.cmdAdd(ctx)
	case "remove", "rm", "del":
		p.cmdRemove(ctx)
	case "list":
		p.cmdList(ctx)
	case "message", "msg":
		p.cmdMessage(ctx)
	default:
		ctx.Reply("Usage: !threadkeeper add #thread [hours] | remove #thread | list | message #thread <text>")
	}
}

func (p *Plugin) cmdAdd(ctx *discord.CommandContext) {
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !threadkeeper add #thread [hours]")
		return
	}
	chID := discord.ParseChannelMention(ctx.Args[1])
	if chID == "" {
		ctx.Reply("Provide a valid thread or channel.")
		return
	}
	hrs := 20
	if len(ctx.Args) >= 3 {
		if v, err := strconv.Atoi(ctx.Args[2]); err == nil && v >= 1 && v <= 168 {
			hrs = v
		} else {
			ctx.Reply("Interval must be between 1 and 168 hours.")
			return
		}
	}
	rec := &keeperRecord{
		ChannelID:   chID,
		IntervalHrs: hrs,
		Message:     defaultHeartbeat,
	}
	p.save(ctx.GuildID, rec)
	ctx.Reply(fmt.Sprintf("OK. Keeping <#%s> alive every %d hours.", chID, hrs))
}

func (p *Plugin) cmdRemove(ctx *discord.CommandContext) {
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !threadkeeper remove #thread")
		return
	}
	chID := discord.ParseChannelMention(ctx.Args[1])
	if chID == "" {
		ctx.Reply("Provide a valid thread.")
		return
	}
	p.api.DeleteConfig(ctx.GuildID, "k:"+chID)
	ctx.Reply(fmt.Sprintf("No longer keeping <#%s> alive.", chID))
}

func (p *Plugin) cmdList(ctx *discord.CommandContext) {
	records := p.loadAll(ctx.GuildID)
	if len(records) == 0 {
		ctx.Reply("No threads are being kept alive.")
		return
	}
	var lines []string
	for _, r := range records {
		lines = append(lines, fmt.Sprintf("<#%s> - every %dh", r.ChannelID, r.IntervalHrs))
	}
	ctx.ReplyEmbed(discord.Embed{
		Title:       "Kept-alive threads",
		Description: strings.Join(lines, "\n"),
		Color:       0x5865F2,
	})
}

func (p *Plugin) cmdMessage(ctx *discord.CommandContext) {
	if len(ctx.Args) < 3 {
		ctx.Reply("Usage: !threadkeeper message #thread <text>")
		return
	}
	chID := discord.ParseChannelMention(ctx.Args[1])
	if chID == "" {
		ctx.Reply("Provide a valid thread.")
		return
	}
	rec := p.load(ctx.GuildID, chID)
	if rec == nil {
		ctx.Reply("That thread is not being kept alive. Add it first.")
		return
	}
	text := strings.Join(ctx.Args[2:], " ")
	if len(text) > 400 {
		ctx.Reply("Heartbeat text must be 400 chars or less.")
		return
	}
	rec.Message = text
	p.save(ctx.GuildID, rec)
	ctx.Reply("Heartbeat message updated.")
}

// ---------------------------------------------------------------------------
// Background ticker
// ---------------------------------------------------------------------------

func (p *Plugin) tick(ctx context.Context) {
	t := time.NewTicker(15 * time.Minute)
	defer t.Stop()
	// Run once shortly after start to warm things up.
	first := time.NewTimer(30 * time.Second)
	defer first.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-first.C:
			p.run()
		case <-t.C:
			p.run()
		}
	}
}

func (p *Plugin) run() {
	rest := p.api.Rest()
	now := time.Now()
	for _, guildID := range p.api.GuildIDs() {
		records := p.loadAll(guildID)
		for _, r := range records {
			next := r.LastPostedAt.Add(time.Duration(r.IntervalHrs) * time.Hour)
			if now.Before(next) {
				continue
			}
			text := r.Message
			if text == "" {
				text = defaultHeartbeat
			}
			if _, err := rest.SendMessage(r.ChannelID, text); err != nil {
				p.api.Log("threadkeeper: send to %s: %v", r.ChannelID, err)
				continue
			}
			r.LastPostedAt = now
			p.save(guildID, r)
		}
	}
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

func (p *Plugin) load(guildID, chID string) *keeperRecord {
	raw := p.api.GetConfig(guildID, "k:"+chID)
	if raw == "" {
		return nil
	}
	var r keeperRecord
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return nil
	}
	return &r
}

func (p *Plugin) loadAll(guildID string) []*keeperRecord {
	var out []*keeperRecord
	for k, v := range p.api.AllConfig(guildID) {
		if !strings.HasPrefix(k, "k:") {
			continue
		}
		var r keeperRecord
		if err := json.Unmarshal([]byte(v), &r); err != nil {
			continue
		}
		out = append(out, &r)
	}
	return out
}

func (p *Plugin) save(guildID string, r *keeperRecord) {
	b, err := json.Marshal(r)
	if err != nil {
		p.api.Log("threadkeeper: marshal: %v", err)
		return
	}
	p.api.SetConfig(guildID, "k:"+r.ChannelID, string(b))
}
