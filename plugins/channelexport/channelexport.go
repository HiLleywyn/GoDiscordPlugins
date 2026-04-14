// Package channelexport produces a text dump of recent channel
// activity. It maintains a rolling in-memory buffer of recent messages
// per channel, and exports the buffer on demand as a text file.
//
// The buffer only contains messages the bot saw after startup. History
// from before the bot came online is not included. If you need a
// complete archive of a channel, use Discord's official Data Export or
// a dedicated archiver - this plugin is meant for day-to-day "give me
// the last N messages" use cases (incident review, context handoff,
// quoting a discussion in a PR, etc).
//
// Commands:
//
//	!channelexport              Export current channel, default size
//	!channelexport #chan        Export that channel
//	!channelexport #chan 500    Export last 500 messages
//	!channelexport clear        Clear the buffer for this channel
package channelexport

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

const (
	defaultExport = 200
	maxExport     = 2000
	bufferCap     = 2000
)

func init() { pluginapi.Register(&Plugin{}) }

type bufferedMsg struct {
	ID        string
	AuthorID  string
	AuthorTag string
	Content   string
	Timestamp time.Time
}

// Plugin is the channelexport plugin instance.
type Plugin struct {
	api pluginapi.API

	mu      sync.Mutex
	buffers map[string][]bufferedMsg // keyed by channelID
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "channelexport",
		Version:     "1.0.0",
		Description: "Export recent channel messages as a text dump.",
		Author:      "HiLleywyn",
		Commands:    []string{"channelexport"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	p.buffers = make(map[string][]bufferedMsg)
	api.AddCommand(&discord.Command{
		Name:        "channelexport",
		Description: "Export recent channel messages.",
		Usage:       "[#chan] [count] | clear",
		Aliases:     []string{"export"},
		Handler:     p.handleCmd,
	})
	api.OnMessage(p.onMessage)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("channelexport")
	p.mu.Lock()
	p.buffers = nil
	p.mu.Unlock()
	return nil
}

// ---------------------------------------------------------------------------
// Buffer
// ---------------------------------------------------------------------------

func (p *Plugin) onMessage(bot *discord.Bot, msg *discord.Message) {
	if msg.GuildID == "" || msg.Author == nil {
		return
	}
	tag := msg.Author.Username
	if msg.Author.Discriminator != "" && msg.Author.Discriminator != "0" {
		tag = msg.Author.Username + "#" + msg.Author.Discriminator
	}
	bm := bufferedMsg{
		ID:        msg.ID,
		AuthorID:  msg.Author.ID,
		AuthorTag: tag,
		Content:   msg.Content,
		Timestamp: time.Now(),
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	buf := p.buffers[msg.ChannelID]
	buf = append(buf, bm)
	if len(buf) > bufferCap {
		// Drop the oldest entries to stay within cap.
		buf = buf[len(buf)-bufferCap:]
	}
	p.buffers[msg.ChannelID] = buf
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if ctx.Member == nil || !ctx.Member.HasPermission(discord.PermissionManageMessages) {
		ctx.Reply("You need Manage Messages for this.")
		return
	}

	chID := ctx.ChannelID
	count := defaultExport
	args := ctx.Args

	if len(args) >= 1 && strings.EqualFold(args[0], "clear") {
		p.mu.Lock()
		delete(p.buffers, chID)
		p.mu.Unlock()
		ctx.Reply("Buffer cleared for this channel.")
		return
	}

	if len(args) >= 1 {
		if id := discord.ParseChannelMention(args[0]); id != "" {
			chID = id
			args = args[1:]
		}
	}
	if len(args) >= 1 {
		if v, err := strconv.Atoi(args[0]); err == nil {
			if v < 1 {
				v = 1
			}
			if v > maxExport {
				v = maxExport
			}
			count = v
		}
	}

	p.mu.Lock()
	buf := p.buffers[chID]
	p.mu.Unlock()
	if len(buf) == 0 {
		ctx.Reply("No buffered messages for that channel. The buffer only contains messages received since the bot started.")
		return
	}

	if count > len(buf) {
		count = len(buf)
	}
	slice := buf[len(buf)-count:]
	body := p.render(chID, slice)

	// Try to send as a file attachment. Fall back to embed preview.
	name := fmt.Sprintf("channel-%s-%s.txt", chID, time.Now().Format("20060102-150405"))
	if _, err := p.api.Rest().SendFile(ctx.ChannelID, name, []byte(body), fmt.Sprintf("Exported **%d** messages from <#%s>.", count, chID)); err != nil {
		p.api.Log("channelexport: SendFile: %v", err)
		preview := body
		if len(preview) > 1800 {
			preview = preview[:1800] + "\n... (truncated; enable file uploads for full export)"
		}
		ctx.ReplyEmbed(discord.Embed{
			Title:       fmt.Sprintf("Export: last %d messages from #%s", count, chID),
			Description: "```\n" + preview + "\n```",
			Color:       0x5865F2,
		})
	}
}

func (p *Plugin) render(chID string, msgs []bufferedMsg) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Channel export for <#%s>\n", chID)
	fmt.Fprintf(&b, "Exported at %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "%d messages\n", len(msgs))
	b.WriteString(strings.Repeat("-", 60))
	b.WriteString("\n\n")

	for _, m := range msgs {
		ts := m.Timestamp.UTC().Format("2006-01-02 15:04:05")
		fmt.Fprintf(&b, "[%s] %s (%s):\n", ts, m.AuthorTag, m.AuthorID)
		if m.Content == "" {
			b.WriteString("  <no text content>\n")
		} else {
			for _, line := range strings.Split(m.Content, "\n") {
				b.WriteString("  ")
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}
