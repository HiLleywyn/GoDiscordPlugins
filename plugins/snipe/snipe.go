// Package snipe keeps an in-memory buffer of recently deleted and edited
// messages per channel. Mods (or anyone with read access) can pull them back
// out with `!snipe` and `!editsnipe`.
//
// Nothing is persisted - buffers clear on restart and entries auto-expire
// after 10 minutes. This is intentional: snipe is meant to answer "what did
// that message say a second ago", not to build a permanent audit trail.
// For long-term logging, pair with modlog or an audit plugin.
package snipe

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

const (
	bufferSize = 10               // how many entries to keep per channel
	maxAge     = 10 * time.Minute // auto-expiry
)

// entry is one captured deleted/edited message.
type entry struct {
	authorID   string
	authorName string
	content    string
	oldContent string // for edits
	when       time.Time
}

// buffer is a bounded ring of entries for a single channel.
type buffer struct {
	entries []entry
}

func (b *buffer) push(e entry) {
	b.entries = append(b.entries, e)
	if len(b.entries) > bufferSize {
		b.entries = b.entries[len(b.entries)-bufferSize:]
	}
}

// get returns the nth most recent entry (1-indexed). nil if not present.
func (b *buffer) get(n int) *entry {
	if n < 1 || n > len(b.entries) {
		return nil
	}
	return &b.entries[len(b.entries)-n]
}

// prune drops entries older than cutoff.
func (b *buffer) prune(cutoff time.Time) {
	n := 0
	for _, e := range b.entries {
		if e.when.After(cutoff) {
			b.entries[n] = e
			n++
		}
	}
	b.entries = b.entries[:n]
}

// Plugin is the snipe plugin instance.
type Plugin struct {
	api    pluginapi.API
	cancel context.CancelFunc

	mu      sync.Mutex
	deletes map[string]*buffer // channelID -> buffer of deletes
	edits   map[string]*buffer // channelID -> buffer of edits
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "snipe",
		Version:     "1.0.0",
		Description: "Recover recently deleted or edited messages.",
		Author:      "HiLleywyn",
		Commands:    []string{"snipe", "editsnipe"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	p.deletes = make(map[string]*buffer)
	p.edits = make(map[string]*buffer)

	api.AddCommand(&discord.Command{
		Name:        "snipe",
		Aliases:     []string{"s"},
		Description: "Show a recently deleted message in this channel.",
		Usage:       "[n]",
		Handler:     p.cmdSnipe,
	})
	api.AddCommand(&discord.Command{
		Name:        "editsnipe",
		Aliases:     []string{"es"},
		Description: "Show the previous version of a recently edited message.",
		Usage:       "[n]",
		Handler:     p.cmdEditSnipe,
	})

	api.OnMessageDelete(p.onDelete)
	api.OnMessageUpdate(p.onEdit)

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go p.sweep(ctx)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("snipe")
	p.api.RemoveCommand("editsnipe")
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

// sweep periodically drops expired entries.
func (p *Plugin) sweep(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			cutoff := time.Now().Add(-maxAge)
			p.mu.Lock()
			for _, b := range p.deletes {
				b.prune(cutoff)
			}
			for _, b := range p.edits {
				b.prune(cutoff)
			}
			p.mu.Unlock()
		}
	}
}

// ---------------------------------------------------------------------------
// Event handlers
// ---------------------------------------------------------------------------

func (p *Plugin) onDelete(bot *discord.Bot, ev *discord.MessageDeleteEvent) {
	if ev.CachedMessage == nil || ev.CachedMessage.Author == nil {
		return
	}
	if ev.CachedMessage.Author.Bot {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	b := p.deletes[ev.ChannelID]
	if b == nil {
		b = &buffer{}
		p.deletes[ev.ChannelID] = b
	}
	b.push(entry{
		authorID:   ev.CachedMessage.Author.ID,
		authorName: ev.CachedMessage.Author.Tag(),
		content:    ev.CachedMessage.Content,
		when:       time.Now(),
	})
}

func (p *Plugin) onEdit(bot *discord.Bot, ev *discord.MessageUpdateEvent) {
	if ev.OldMessage == nil || ev.NewMessage == nil {
		return
	}
	if ev.NewMessage.Author == nil || ev.NewMessage.Author.Bot {
		return
	}
	if ev.OldMessage.Content == ev.NewMessage.Content {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	b := p.edits[ev.ChannelID]
	if b == nil {
		b = &buffer{}
		p.edits[ev.ChannelID] = b
	}
	b.push(entry{
		authorID:   ev.NewMessage.Author.ID,
		authorName: ev.NewMessage.Author.Tag(),
		content:    ev.NewMessage.Content,
		oldContent: ev.OldMessage.Content,
		when:       time.Now(),
	})
}

// ---------------------------------------------------------------------------
// Command handlers
// ---------------------------------------------------------------------------

func (p *Plugin) cmdSnipe(ctx *discord.CommandContext) {
	n := parseN(ctx.Args)
	p.mu.Lock()
	b := p.deletes[ctx.ChannelID]
	var e *entry
	if b != nil {
		e = b.get(n)
	}
	p.mu.Unlock()

	if e == nil {
		ctx.Reply("Nothing to snipe.")
		return
	}
	ctx.ReplyEmbed(discord.Embed{
		Author: &discord.EmbedAuthor{Name: e.authorName},
		Description: e.content,
		Color:       0xED4245,
		Footer: &discord.EmbedFooter{
			Text: fmt.Sprintf("Deleted %s ago", shortDuration(time.Since(e.when))),
		},
	})
}

func (p *Plugin) cmdEditSnipe(ctx *discord.CommandContext) {
	n := parseN(ctx.Args)
	p.mu.Lock()
	b := p.edits[ctx.ChannelID]
	var e *entry
	if b != nil {
		e = b.get(n)
	}
	p.mu.Unlock()

	if e == nil {
		ctx.Reply("No recent edits to snipe.")
		return
	}
	ctx.ReplyEmbed(discord.Embed{
		Author: &discord.EmbedAuthor{Name: e.authorName},
		Color:  0xFEE75C,
		Fields: []discord.EmbedField{
			{Name: "Before", Value: truncate(e.oldContent, 1024)},
			{Name: "After", Value: truncate(e.content, 1024)},
		},
		Footer: &discord.EmbedFooter{
			Text: fmt.Sprintf("Edited %s ago", shortDuration(time.Since(e.when))),
		},
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func parseN(args []string) int {
	if len(args) == 0 {
		return 1
	}
	n, err := strconv.Atoi(args[0])
	if err != nil || n < 1 {
		return 1
	}
	if n > bufferSize {
		return bufferSize
	}
	return n
}

func truncate(s string, max int) string {
	if s == "" {
		return "*(empty)*"
	}
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func shortDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}
