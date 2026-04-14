// Package stats tracks message volume per channel and per user and
// produces weekly digest reports. A background ticker posts a summary
// every week (7 days from the first run, then every 7 days after).
//
// Data layout (per guild):
//
//	total:YYYY-WW              int - all messages in that ISO week
//	ch:<chanID>:YYYY-WW        int - messages in that channel that week
//	usr:<userID>:YYYY-WW       int - messages by that user that week
//	digest_channel             channel to post the weekly digest
//	last_digest                last digest week string (YYYY-WW)
//
// Commands:
//
//	!stats                     Summary for the current week
//	!stats week                Previous completed week
//	!stats channel #chan       This week for one channel
//	!stats user @user          This week for one user
//	!stats top channels        Top 5 channels this week
//	!stats top users           Top 5 users this week
//	!stats digest #chan        Admin: set digest channel
//	!stats digest off          Admin: stop weekly digests
package stats

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

// Plugin is the stats plugin instance.
type Plugin struct {
	api    pluginapi.API
	cancel context.CancelFunc
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "stats",
		Version:     "1.0.0",
		Description: "Server message activity stats with weekly digests.",
		Author:      "HiLleywyn",
		Commands:    []string{"stats"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "stats",
		Description: "Server activity statistics.",
		Usage:       "[week | channel #chan | user @user | top channels|users | digest #chan|off]",
		Handler:     p.handleCmd,
	})
	api.OnMessage(p.onMessage)

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go p.tick(ctx)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("stats")
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Message hook
// ---------------------------------------------------------------------------

func (p *Plugin) onMessage(bot *discord.Bot, msg *discord.Message) {
	if msg.GuildID == "" || msg.Author == nil || msg.Author.Bot {
		return
	}
	week := weekKey(time.Now())
	p.bumpCounter(msg.GuildID, "total:"+week)
	p.bumpCounter(msg.GuildID, "ch:"+msg.ChannelID+":"+week)
	p.bumpCounter(msg.GuildID, "usr:"+msg.Author.ID+":"+week)
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		p.cmdSummary(ctx, weekKey(time.Now()), "This week")
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "week":
		p.cmdSummary(ctx, weekKey(time.Now().AddDate(0, 0, -7)), "Last week")
	case "channel":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !stats channel #chan")
			return
		}
		chID := discord.ParseChannelMention(ctx.Args[1])
		if chID == "" {
			ctx.Reply("Provide a valid channel.")
			return
		}
		week := weekKey(time.Now())
		n := p.counter(ctx.GuildID, "ch:"+chID+":"+week)
		ctx.Reply(fmt.Sprintf("<#%s> this week: **%d** messages", chID, n))
	case "user":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !stats user @user")
			return
		}
		uid := discord.ParseUserID(ctx.Args[1])
		if uid == "" {
			ctx.Reply("Provide a valid user.")
			return
		}
		week := weekKey(time.Now())
		n := p.counter(ctx.GuildID, "usr:"+uid+":"+week)
		ctx.Reply(fmt.Sprintf("<@%s> this week: **%d** messages", uid, n))
	case "top":
		p.cmdTop(ctx)
	case "digest":
		p.cmdDigest(ctx)
	default:
		ctx.Reply("Usage: !stats | week | channel #chan | user @user | top channels|users | digest #chan|off")
	}
}

func (p *Plugin) cmdSummary(ctx *discord.CommandContext, week, label string) {
	total := p.counter(ctx.GuildID, "total:"+week)
	topCh := p.topN(ctx.GuildID, "ch:", week, 5)
	topUsr := p.topN(ctx.GuildID, "usr:", week, 5)

	lines := []string{fmt.Sprintf("Total messages: **%d**", total)}
	if len(topCh) > 0 {
		lines = append(lines, "\n**Top channels**")
		for _, e := range topCh {
			lines = append(lines, fmt.Sprintf("<#%s> - %d", e.id, e.count))
		}
	}
	if len(topUsr) > 0 {
		lines = append(lines, "\n**Top posters**")
		for _, e := range topUsr {
			lines = append(lines, fmt.Sprintf("<@%s> - %d", e.id, e.count))
		}
	}

	ctx.ReplyEmbed(discord.Embed{
		Title:       label + " (" + week + ")",
		Description: strings.Join(lines, "\n"),
		Color:       0x5865F2,
	})
}

func (p *Plugin) cmdTop(ctx *discord.CommandContext) {
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !stats top channels | users")
		return
	}
	week := weekKey(time.Now())
	prefix := ""
	label := ""
	switch strings.ToLower(ctx.Args[1]) {
	case "channels":
		prefix, label = "ch:", "Top channels"
	case "users", "posters":
		prefix, label = "usr:", "Top posters"
	default:
		ctx.Reply("Usage: !stats top channels | users")
		return
	}
	rows := p.topN(ctx.GuildID, prefix, week, 10)
	if len(rows) == 0 {
		ctx.Reply("No data yet this week.")
		return
	}
	var lines []string
	for i, e := range rows {
		if prefix == "ch:" {
			lines = append(lines, fmt.Sprintf("%d. <#%s> - %d", i+1, e.id, e.count))
		} else {
			lines = append(lines, fmt.Sprintf("%d. <@%s> - %d", i+1, e.id, e.count))
		}
	}
	ctx.ReplyEmbed(discord.Embed{
		Title:       label + " (" + week + ")",
		Description: strings.Join(lines, "\n"),
		Color:       0x5865F2,
	})
}

func (p *Plugin) cmdDigest(ctx *discord.CommandContext) {
	if ctx.Member == nil || !ctx.Member.HasPermission(discord.PermissionManageMessages) {
		ctx.Reply("You need Manage Messages for this.")
		return
	}
	if len(ctx.Args) < 2 {
		ch := p.api.GetConfig(ctx.GuildID, "digest_channel")
		if ch == "" {
			ctx.Reply("Weekly digest is off. Set a channel with `!stats digest #chan`.")
		} else {
			ctx.Reply("Weekly digest is posted to <#" + ch + ">.")
		}
		return
	}
	if strings.EqualFold(ctx.Args[1], "off") {
		p.api.DeleteConfig(ctx.GuildID, "digest_channel")
		ctx.Reply("Weekly digest disabled.")
		return
	}
	chID := discord.ParseChannelMention(ctx.Args[1])
	if chID == "" {
		ctx.Reply("Provide a valid channel.")
		return
	}
	p.api.SetConfig(ctx.GuildID, "digest_channel", chID)
	ctx.Reply("Weekly digest will be posted to <#" + chID + ">.")
}

// ---------------------------------------------------------------------------
// Background weekly digest
// ---------------------------------------------------------------------------

func (p *Plugin) tick(ctx context.Context) {
	t := time.NewTicker(1 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.maybeDigest()
		}
	}
}

func (p *Plugin) maybeDigest() {
	rest := p.api.Rest()
	// The "current" week is always incomplete; we digest the *previous* week.
	lastCompleted := weekKey(time.Now().AddDate(0, 0, -7))

	for _, guildID := range p.api.GuildIDs() {
		chID := p.api.GetConfig(guildID, "digest_channel")
		if chID == "" {
			continue
		}
		if p.api.GetConfig(guildID, "last_digest") == lastCompleted {
			continue
		}

		total := p.counter(guildID, "total:"+lastCompleted)
		if total == 0 {
			p.api.SetConfig(guildID, "last_digest", lastCompleted)
			continue
		}
		topCh := p.topN(guildID, "ch:", lastCompleted, 5)
		topUsr := p.topN(guildID, "usr:", lastCompleted, 5)

		lines := []string{fmt.Sprintf("Total messages: **%d**", total)}
		if len(topCh) > 0 {
			lines = append(lines, "\n**Top channels**")
			for _, e := range topCh {
				lines = append(lines, fmt.Sprintf("<#%s> - %d", e.id, e.count))
			}
		}
		if len(topUsr) > 0 {
			lines = append(lines, "\n**Top posters**")
			for _, e := range topUsr {
				lines = append(lines, fmt.Sprintf("<@%s> - %d", e.id, e.count))
			}
		}

		embed := discord.Embed{
			Title:       "Weekly digest - " + lastCompleted,
			Description: strings.Join(lines, "\n"),
			Color:       0x5865F2,
		}
		if _, err := rest.SendEmbed(chID, embed); err != nil {
			p.api.Log("stats: digest send: %v", err)
			continue
		}
		p.api.SetConfig(guildID, "last_digest", lastCompleted)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (p *Plugin) bumpCounter(guildID, key string) {
	n := p.counter(guildID, key)
	p.api.SetConfig(guildID, key, strconv.Itoa(n+1))
}

func (p *Plugin) counter(guildID, key string) int {
	v := p.api.GetConfig(guildID, key)
	if v == "" {
		return 0
	}
	n, _ := strconv.Atoi(v)
	return n
}

type topEntry struct {
	id    string
	count int
}

func (p *Plugin) topN(guildID, prefix, week string, n int) []topEntry {
	suffix := ":" + week
	var rows []topEntry
	for k, v := range p.api.AllConfig(guildID) {
		if !strings.HasPrefix(k, prefix) || !strings.HasSuffix(k, suffix) {
			continue
		}
		id := strings.TrimPrefix(k, prefix)
		id = strings.TrimSuffix(id, suffix)
		c, _ := strconv.Atoi(v)
		rows = append(rows, topEntry{id: id, count: c})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].count > rows[j].count })
	if len(rows) > n {
		rows = rows[:n]
	}
	return rows
}

func weekKey(t time.Time) string {
	y, w := t.ISOWeek()
	return fmt.Sprintf("%04d-%02d", y, w)
}
