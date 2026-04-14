// Package poll implements reaction-based polls.
//
//	!poll <question> | <opt 1> | <opt 2> [| <opt 3>] ... [for <duration>]
//
// The plugin posts an embed listing each option next to an auto-assigned
// regional-indicator emoji (A, B, C, ...). Users vote by reacting. When the
// configured duration elapses the plugin tallies the votes and posts a result
// embed replacing the original. Polls without a duration stay open until
// manually closed with `!poll end <messageID>`.
package poll

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

// letterEmoji maps an option index 0..9 to its regional-indicator emoji.
var letterEmoji = []string{
	"\U0001F1E6", // A
	"\U0001F1E7", // B
	"\U0001F1E8", // C
	"\U0001F1E9", // D
	"\U0001F1EA", // E
	"\U0001F1EB", // F
	"\U0001F1EC", // G
	"\U0001F1ED", // H
	"\U0001F1EE", // I
	"\U0001F1EF", // J
}

const maxOptions = 10

// pollData is the persisted record for a single poll.
type pollData struct {
	GuildID   string    `json:"guild_id"`
	ChannelID string    `json:"channel_id"`
	MessageID string    `json:"message_id"`
	Question  string    `json:"question"`
	Options   []string  `json:"options"`
	EndTime   time.Time `json:"end_time"` // zero = open ended
	Ended     bool      `json:"ended"`
	AuthorID  string    `json:"author_id"`
}

// Plugin is the poll plugin instance.
type Plugin struct {
	api    pluginapi.API
	cancel context.CancelFunc
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "poll",
		Version:     "1.0.0",
		Description: "Reaction-based polls with optional timed closing.",
		Author:      "HiLleywyn",
		Commands:    []string{"poll"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "poll",
		Description: "Run a reaction-based poll.",
		Usage:       `"Question" "opt 1" "opt 2" [opt 3] ... [for <duration>]  |  end <messageID>`,
		Handler:     p.handleCmd,
	})

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go p.tick(ctx)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("poll")
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Background ticker
// ---------------------------------------------------------------------------

func (p *Plugin) tick(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.checkExpired()
		}
	}
}

func (p *Plugin) checkExpired() {
	now := time.Now()
	for _, guildID := range p.api.GuildIDs() {
		for k, v := range p.api.AllConfig(guildID) {
			if !strings.HasPrefix(k, "poll:") {
				continue
			}
			var pd pollData
			if err := json.Unmarshal([]byte(v), &pd); err != nil {
				continue
			}
			if pd.Ended || pd.EndTime.IsZero() || pd.EndTime.After(now) {
				continue
			}
			p.finish(&pd)
		}
	}
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	// "end <messageID>" is handled differently from starting a poll.
	if len(ctx.Args) >= 2 && strings.EqualFold(ctx.Args[0], "end") {
		p.cmdEnd(ctx, ctx.Args[1])
		return
	}
	p.cmdStart(ctx)
}

func (p *Plugin) cmdStart(ctx *discord.CommandContext) {
	// Segments are separated with `|`. The first segment is the question,
	// the rest are the options. An optional trailing `for <duration>` word
	// pair on the final segment closes the poll automatically.
	raw := strings.Join(ctx.Args, " ")
	if raw == "" {
		ctx.Reply("Usage: !poll <question> | <option 1> | <option 2> [| <option 3> ...] [for <duration>]")
		return
	}

	parts := strings.Split(raw, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	// Detect a trailing `for <duration>` clause on the last segment.
	var duration time.Duration
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		words := strings.Fields(last)
		if len(words) >= 2 && strings.EqualFold(words[len(words)-2], "for") {
			if d, err := parseDuration(words[len(words)-1]); err == nil {
				duration = d
				last = strings.TrimSpace(strings.Join(words[:len(words)-2], " "))
				parts[len(parts)-1] = last
			}
		}
	}

	// Drop any empty trailing segment produced by the duration trim.
	if n := len(parts); n > 0 && parts[n-1] == "" {
		parts = parts[:n-1]
	}

	if len(parts) < 3 {
		ctx.Reply("Need a question and at least 2 options. Example: !poll Pizza? | Yes | No")
		return
	}
	question := parts[0]
	options := parts[1:]
	if len(options) > maxOptions {
		ctx.Reply(fmt.Sprintf("Maximum %d options.", maxOptions))
		return
	}

	pd := pollData{
		GuildID:   ctx.GuildID,
		ChannelID: ctx.ChannelID,
		Question:  question,
		Options:   options,
		AuthorID:  ctx.AuthorID,
	}
	if duration > 0 {
		pd.EndTime = time.Now().Add(duration)
	}

	msg, err := ctx.Bot.Rest.SendEmbed(ctx.ChannelID, renderPoll(&pd, nil))
	if err != nil || msg == nil {
		ctx.Reply("Failed to post poll: " + errStr(err))
		return
	}
	pd.MessageID = msg.ID

	for i := range options {
		_ = ctx.Bot.Rest.AddReaction(ctx.ChannelID, msg.ID, letterEmoji[i])
	}
	p.save(&pd)
}

func (p *Plugin) cmdEnd(ctx *discord.CommandContext, msgID string) {
	pd := p.load(ctx.GuildID, msgID)
	if pd == nil {
		ctx.Reply("No poll with that message ID.")
		return
	}
	if pd.Ended {
		ctx.Reply("That poll has already ended.")
		return
	}
	if pd.AuthorID != ctx.AuthorID && !hasManage(ctx) {
		ctx.Reply("Only the poll creator (or a mod) can end this poll early.")
		return
	}
	p.finish(pd)
	ctx.Reply("Poll ended.")
}

// ---------------------------------------------------------------------------
// Poll rendering and tallying
// ---------------------------------------------------------------------------

func renderPoll(pd *pollData, tallies []int) discord.Embed {
	var body strings.Builder
	for i, opt := range pd.Options {
		if tallies != nil {
			body.WriteString(fmt.Sprintf("%s %s - **%d**\n", letterEmoji[i], opt, tallies[i]))
		} else {
			body.WriteString(fmt.Sprintf("%s %s\n", letterEmoji[i], opt))
		}
	}

	title := pd.Question
	desc := body.String()
	if tallies != nil {
		title = "Results: " + title
	} else if !pd.EndTime.IsZero() {
		desc += fmt.Sprintf("\nCloses <t:%d:R>", pd.EndTime.Unix())
	}

	color := 0x5865F2
	if tallies != nil {
		color = 0x57F287
	}
	return discord.Embed{
		Title:       title,
		Description: desc,
		Color:       color,
	}
}

func (p *Plugin) finish(pd *pollData) {
	rest := p.api.Rest()
	botID := p.api.BotID()

	tallies := make([]int, len(pd.Options))
	for i := range pd.Options {
		users, err := rest.GetReactions(pd.ChannelID, pd.MessageID, letterEmoji[i], 100)
		if err != nil {
			p.api.Log("poll: get reactions: %v", err)
			continue
		}
		for _, u := range users {
			if u.ID != botID && !u.Bot {
				tallies[i]++
			}
		}
	}

	if err := rest.EditEmbed(pd.ChannelID, pd.MessageID, renderPoll(pd, tallies)); err != nil {
		p.api.Log("poll: edit embed: %v", err)
	}
	pd.Ended = true
	p.save(pd)

	// Announce the winner(s) below the poll for clarity.
	winners := findWinners(tallies)
	if len(winners) > 0 && tallies[winners[0]] > 0 {
		var labels []string
		for _, w := range winners {
			labels = append(labels, fmt.Sprintf("**%s** (%d)", pd.Options[w], tallies[w]))
		}
		_, _ = rest.SendMessage(pd.ChannelID,
			"Poll closed. Winner: "+strings.Join(labels, ", "))
	} else {
		_, _ = rest.SendMessage(pd.ChannelID, "Poll closed with no votes.")
	}
}

func findWinners(tallies []int) []int {
	best := -1
	for _, t := range tallies {
		if t > best {
			best = t
		}
	}
	if best <= 0 {
		return nil
	}
	var out []int
	for i, t := range tallies {
		if t == best {
			out = append(out, i)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

func (p *Plugin) load(guildID, msgID string) *pollData {
	raw := p.api.GetConfig(guildID, "poll:"+msgID)
	if raw == "" {
		return nil
	}
	var pd pollData
	if err := json.Unmarshal([]byte(raw), &pd); err != nil {
		return nil
	}
	return &pd
}

func (p *Plugin) save(pd *pollData) {
	b, err := json.Marshal(pd)
	if err != nil {
		p.api.Log("poll: marshal: %v", err)
		return
	}
	p.api.SetConfig(pd.GuildID, "poll:"+pd.MessageID, string(b))
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func parseDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("too short")
	}
	suffix := s[len(s)-1]
	num, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
	if err != nil || num <= 0 {
		return 0, fmt.Errorf("invalid number")
	}
	switch suffix {
	case 's':
		return time.Duration(num) * time.Second, nil
	case 'm':
		return time.Duration(num) * time.Minute, nil
	case 'h':
		return time.Duration(num) * time.Hour, nil
	case 'd':
		return time.Duration(num) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown duration suffix")
	}
}

func hasManage(ctx *discord.CommandContext) bool {
	if ctx.Member == nil {
		return false
	}
	return ctx.Member.HasPermission(discord.PermissionManageMessages)
}

func errStr(err error) string {
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}
