package giveaway

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

const giveawayEmoji = "\U0001f389" // party popper

// giveawayData holds the persisted state for a single giveaway.
type giveawayData struct {
	GuildID   string    `json:"guild_id"`
	ChannelID string    `json:"channel_id"`
	MessageID string    `json:"message_id"`
	EndTime   time.Time `json:"end_time"`
	Winners   int       `json:"winners"`
	Prize     string    `json:"prize"`
	Ended     bool      `json:"ended"`
}

// Plugin manages timed giveaways.
type Plugin struct {
	api    pluginapi.API
	cancel context.CancelFunc
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "giveaway",
		Version:     "1.0.0",
		Description: "Run timed giveaways with reaction-based entry.",
		Author:      "HiLleywyn",
		Commands:    []string{"giveaway"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "giveaway",
		Description: "Manage giveaways.",
		Usage:       "start <duration> <winners> <prize> | end <messageID> | reroll <messageID>",
		Handler:     p.handleCmd,
	})

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go p.ticker(ctx)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("giveaway")
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Background ticker - checks for expired giveaways every minute
// ---------------------------------------------------------------------------

func (p *Plugin) ticker(ctx context.Context) {
	t := time.NewTicker(time.Minute)
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
	for _, guildID := range p.api.GuildIDs() {
		all := p.api.AllConfig(guildID)
		for k, v := range all {
			if !strings.HasPrefix(k, "giveaway:") {
				continue
			}
			var gd giveawayData
			if err := json.Unmarshal([]byte(v), &gd); err != nil {
				continue
			}
			if gd.Ended {
				continue
			}
			if time.Now().After(gd.EndTime) {
				p.endGiveaway(guildID, &gd)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !giveaway start <duration> <winners> <prize> | end <messageID> | reroll <messageID>")
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "start":
		// !giveaway start <duration> <winners> <prize...>
		if len(ctx.Args) < 4 {
			ctx.Reply("Usage: !giveaway start <duration> <winners> <prize>")
			return
		}
		dur, err := parseDuration(ctx.Args[1])
		if err != nil {
			ctx.Reply("Invalid duration. Use s/m/h/d suffixes (e.g. 10m, 24h, 7d).")
			return
		}
		if dur > 30*24*time.Hour {
			ctx.Reply("Maximum giveaway duration is 30 days.")
			return
		}
		winners, err := strconv.Atoi(ctx.Args[2])
		if err != nil || winners < 1 {
			ctx.Reply("Winners must be a positive integer.")
			return
		}
		prize := strings.Join(ctx.Args[3:], " ")
		p.startGiveaway(ctx, dur, winners, prize)

	case "end":
		if len(ctx.Args) < 2 {
			ctx.Reply("Provide the giveaway message ID.")
			return
		}
		msgID := ctx.Args[1]
		v := p.api.GetConfig(ctx.GuildID, "giveaway:"+msgID)
		if v == "" {
			ctx.Reply("No giveaway found with that message ID.")
			return
		}
		var gd giveawayData
		if err := json.Unmarshal([]byte(v), &gd); err != nil {
			ctx.Reply("Failed to read giveaway data.")
			return
		}
		if gd.Ended {
			ctx.Reply("That giveaway has already ended.")
			return
		}
		p.endGiveaway(ctx.GuildID, &gd)
		ctx.Reply("Giveaway ended early.")

	case "reroll":
		if len(ctx.Args) < 2 {
			ctx.Reply("Provide the giveaway message ID.")
			return
		}
		msgID := ctx.Args[1]
		v := p.api.GetConfig(ctx.GuildID, "giveaway:"+msgID)
		if v == "" {
			ctx.Reply("No giveaway found with that message ID.")
			return
		}
		var gd giveawayData
		if err := json.Unmarshal([]byte(v), &gd); err != nil {
			ctx.Reply("Failed to read giveaway data.")
			return
		}
		p.pickWinnersAndAnnounce(ctx.Bot, &gd, true)

	default:
		ctx.Reply("Unknown subcommand. Try: start, end, reroll")
	}
}

// ---------------------------------------------------------------------------
// Giveaway logic
// ---------------------------------------------------------------------------

func (p *Plugin) startGiveaway(ctx *discord.CommandContext, dur time.Duration, winners int, prize string) {
	endTime := time.Now().Add(dur)
	endStr := fmt.Sprintf("<t:%d:R>", endTime.Unix())

	embed := discord.Embed{
		Title: giveawayEmoji + " Giveaway: " + prize,
		Description: fmt.Sprintf(
			"React with %s to enter!\n\nEnds %s\nWinners: %d",
			giveawayEmoji, endStr, winners,
		),
		Color: 0xFF6B6B,
		Footer: &discord.EmbedFooter{
			Text: fmt.Sprintf("Ends at"),
		},
		Timestamp: endTime.UTC().Format(time.RFC3339),
	}

	msg, err := ctx.Bot.Rest.SendEmbed(ctx.ChannelID, embed)
	if err != nil {
		p.api.Log("giveaway: failed to post embed: %v", err)
		ctx.Reply("Failed to post giveaway message.")
		return
	}

	// Add the entry reaction.
	_ = ctx.Bot.Rest.AddReaction(ctx.ChannelID, msg.ID, giveawayEmoji)

	gd := giveawayData{
		GuildID:   ctx.GuildID,
		ChannelID: ctx.ChannelID,
		MessageID: msg.ID,
		EndTime:   endTime,
		Winners:   winners,
		Prize:     prize,
		Ended:     false,
	}
	p.saveGiveaway(ctx.GuildID, &gd)
}

func (p *Plugin) endGiveaway(guildID string, gd *giveawayData) {
	gd.Ended = true
	p.saveGiveaway(guildID, gd)

	// We need a bot instance to call REST. Use a dummy bot if nil by fetching
	// the rest client through the API. Since pluginapi.API exposes Rest(),
	// we call it directly.
	rest := p.api.Rest()
	p.pickWinnersWithRest(rest, gd, false)
}

func (p *Plugin) pickWinnersAndAnnounce(bot *discord.Bot, gd *giveawayData, reroll bool) {
	p.pickWinnersWithRest(bot.Rest, gd, reroll)
}

func (p *Plugin) pickWinnersWithRest(rest *discord.RestClient, gd *giveawayData, reroll bool) {
	botID := p.api.BotID()

	// Fetch reactions - the emoji is a unicode character, URL-encoded by the REST client.
	users, err := rest.GetReactions(gd.ChannelID, gd.MessageID, giveawayEmoji, 100)
	if err != nil {
		p.api.Log("giveaway: failed to fetch reactions: %v", err)
		return
	}

	// Exclude the bot itself.
	var entrants []*discord.User
	for _, u := range users {
		if u.ID != botID && !u.Bot {
			entrants = append(entrants, u)
		}
	}

	label := "Giveaway over!"
	if reroll {
		label = "Reroll!"
	}

	if len(entrants) == 0 {
		_, _ = rest.SendMessage(gd.ChannelID, label+" No valid entries - no winners this time.")
		return
	}

	// Pick N random unique winners.
	count := gd.Winners
	if count > len(entrants) {
		count = len(entrants)
	}
	perm := rand.Perm(len(entrants))
	var mentions []string
	for i := 0; i < count; i++ {
		mentions = append(mentions, entrants[perm[i]].Mention())
	}

	result := fmt.Sprintf("%s Congratulations to %s! You won **%s**!",
		label, strings.Join(mentions, ", "), gd.Prize)
	_, err = rest.SendMessage(gd.ChannelID, result)
	if err != nil {
		p.api.Log("giveaway: failed to send result: %v", err)
	}
}

func (p *Plugin) saveGiveaway(guildID string, gd *giveawayData) {
	b, err := json.Marshal(gd)
	if err != nil {
		p.api.Log("giveaway: failed to marshal giveaway: %v", err)
		return
	}
	p.api.SetConfig(guildID, "giveaway:"+gd.MessageID, string(b))
}

// ---------------------------------------------------------------------------
// Duration parser: supports s, m, h, d suffixes
// ---------------------------------------------------------------------------

func parseDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("too short")
	}
	suffix := s[len(s)-1]
	numStr := s[:len(s)-1]
	n, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid number")
	}
	switch suffix {
	case 's':
		return time.Duration(n) * time.Second, nil
	case 'm':
		return time.Duration(n) * time.Minute, nil
	case 'h':
		return time.Duration(n) * time.Hour, nil
	case 'd':
		return time.Duration(n) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown suffix '%c', use s/m/h/d", suffix)
	}
}
