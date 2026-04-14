// Package slowmode auto-adjusts a channel's per-user rate limit based on
// observed message rate. When a tracked channel gets busy the plugin increases
// its slowmode; when the channel cools down the slowmode is lowered back.
//
// Commands:
//
//	!slowmode enable #channel          Start tracking the channel
//	!slowmode disable #channel         Stop tracking
//	!slowmode threshold <msgs/minute>  Msgs/min that triggers an increase
//	!slowmode max <seconds>            Cap on the applied slowmode
//	!slowmode show                     Show current settings and tracked channels
package slowmode

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

const (
	defaultThreshold = 20 // messages per minute before we raise slowmode
	defaultMaxSecs   = 30 // cap on applied slowmode
	minSlowmode      = 0
	adjustInterval   = 30 * time.Second
)

// channelStat tracks recent activity for a single channel.
type channelStat struct {
	guildID     string
	messages    []time.Time
	currentRate int // seconds currently applied
}

// Plugin is the slowmode plugin instance.
type Plugin struct {
	api    pluginapi.API
	cancel context.CancelFunc

	mu    sync.Mutex
	stats map[string]*channelStat // channelID -> stats
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "slowmode",
		Version:     "1.0.0",
		Description: "Auto-adjust channel slowmode based on messages per minute.",
		Author:      "HiLleywyn",
		Commands:    []string{"slowmode"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	p.stats = make(map[string]*channelStat)

	api.AddCommand(&discord.Command{
		Name:        "slowmode",
		Description: "Configure auto-slowmode.",
		Usage:       "enable #chan | disable #chan | threshold <msgs/min> | max <secs> | show",
		Handler:     p.handleCmd,
	})
	api.OnMessage(p.onMessage)

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go p.tick(ctx)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("slowmode")
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

// tick runs the adjustment loop.
func (p *Plugin) tick(ctx context.Context) {
	t := time.NewTicker(adjustInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.adjustAll()
		}
	}
}

// ---------------------------------------------------------------------------
// Message handler - track rate
// ---------------------------------------------------------------------------

func (p *Plugin) onMessage(bot *discord.Bot, msg *discord.Message) {
	if msg.GuildID == "" || msg.Author == nil || msg.Author.Bot {
		return
	}
	if p.api.GetConfig(msg.GuildID, "track:"+msg.ChannelID) != "true" {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.stats[msg.ChannelID]
	if s == nil {
		s = &channelStat{guildID: msg.GuildID}
		p.stats[msg.ChannelID] = s
	}
	s.messages = append(s.messages, time.Now())
}

// adjustAll runs through tracked channels and recomputes slowmode.
func (p *Plugin) adjustAll() {
	p.mu.Lock()
	// Snapshot of channel IDs so we can release the lock for REST calls.
	type snap struct {
		channelID string
		guildID   string
		rate      int
		target    int
	}
	var snaps []snap
	now := time.Now()
	cutoff := now.Add(-time.Minute)

	for chID, s := range p.stats {
		// Prune messages older than 1 minute.
		n := 0
		for _, t := range s.messages {
			if t.After(cutoff) {
				s.messages[n] = t
				n++
			}
		}
		s.messages = s.messages[:n]

		threshold := p.intConfig(s.guildID, "threshold", defaultThreshold)
		maxSecs := p.intConfig(s.guildID, "max", defaultMaxSecs)

		rate := len(s.messages) // messages in the last minute
		target := computeTarget(rate, threshold, maxSecs, s.currentRate)

		if target != s.currentRate {
			snaps = append(snaps, snap{
				channelID: chID,
				guildID:   s.guildID,
				rate:      rate,
				target:    target,
			})
			s.currentRate = target
		}
	}
	p.mu.Unlock()

	rest := p.api.Rest()
	for _, sn := range snaps {
		if err := rest.SetChannelSlowmode(sn.channelID, sn.target); err != nil {
			p.api.Log("slowmode: set %s=%d: %v", sn.channelID, sn.target, err)
			continue
		}
		p.api.Log("slowmode: %s rate=%d/min -> %ds", sn.channelID, sn.rate, sn.target)
	}
}

// computeTarget picks the new slowmode value for a channel given its observed
// rate, the configured trigger threshold, the cap, and the current value.
//
// Simple curve:
//   rate >= 2 * threshold -> max slowmode
//   rate >= threshold     -> scaled proportionally toward max
//   rate <  threshold / 2 -> decay current value toward 0 (step = -5)
//   otherwise             -> keep current value
func computeTarget(rate, threshold, maxSecs, current int) int {
	if threshold <= 0 {
		return 0
	}
	if rate >= 2*threshold {
		return maxSecs
	}
	if rate >= threshold {
		// Linear between threshold (1s) and 2*threshold (max).
		span := threshold
		offset := rate - threshold
		return clamp(1+(maxSecs-1)*offset/span, current, maxSecs)
	}
	if rate < threshold/2 {
		next := current - 5
		if next < 0 {
			next = 0
		}
		return next
	}
	return current
}

func clamp(v, floor, ceil int) int {
	if v < floor {
		v = floor
	}
	if v > ceil {
		v = ceil
	}
	return v
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !slowmode enable #chan | disable #chan | threshold <msgs/min> | max <secs> | show")
		return
	}
	switch strings.ToLower(ctx.Args[0]) {
	case "enable":
		if len(ctx.Args) < 2 {
			ctx.Reply("Provide a channel mention or ID.")
			return
		}
		chID := discord.ParseChannelMention(ctx.Args[1])
		p.api.SetConfig(ctx.GuildID, "track:"+chID, "true")
		ctx.Reply("Auto-slowmode enabled for <#" + chID + ">.")
	case "disable":
		if len(ctx.Args) < 2 {
			ctx.Reply("Provide a channel mention or ID.")
			return
		}
		chID := discord.ParseChannelMention(ctx.Args[1])
		p.api.DeleteConfig(ctx.GuildID, "track:"+chID)
		p.mu.Lock()
		delete(p.stats, chID)
		p.mu.Unlock()
		_ = p.api.Rest().SetChannelSlowmode(chID, 0)
		ctx.Reply("Auto-slowmode disabled for <#" + chID + ">.")
	case "threshold":
		if len(ctx.Args) < 2 {
			ctx.Reply(fmt.Sprintf("Current threshold: %d msgs/min", p.intConfig(ctx.GuildID, "threshold", defaultThreshold)))
			return
		}
		n, err := strconv.Atoi(ctx.Args[1])
		if err != nil || n < 1 {
			ctx.Reply("Threshold must be a positive integer.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "threshold", strconv.Itoa(n))
		ctx.Reply(fmt.Sprintf("Threshold set to %d msgs/min.", n))
	case "max":
		if len(ctx.Args) < 2 {
			ctx.Reply(fmt.Sprintf("Current max slowmode: %ds", p.intConfig(ctx.GuildID, "max", defaultMaxSecs)))
			return
		}
		n, err := strconv.Atoi(ctx.Args[1])
		if err != nil || n < 1 || n > 21600 {
			ctx.Reply("Max must be between 1 and 21600 seconds.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "max", strconv.Itoa(n))
		ctx.Reply(fmt.Sprintf("Max slowmode set to %ds.", n))
	case "show":
		p.showCmd(ctx)
	default:
		ctx.Reply("Unknown subcommand. Try: enable, disable, threshold, max, show")
	}
}

func (p *Plugin) showCmd(ctx *discord.CommandContext) {
	thr := p.intConfig(ctx.GuildID, "threshold", defaultThreshold)
	mx := p.intConfig(ctx.GuildID, "max", defaultMaxSecs)

	var tracked []string
	for k := range p.api.AllConfig(ctx.GuildID) {
		if strings.HasPrefix(k, "track:") {
			tracked = append(tracked, "<#"+strings.TrimPrefix(k, "track:")+">")
		}
	}
	if len(tracked) == 0 {
		tracked = []string{"(none)"}
	}
	ctx.Reply(fmt.Sprintf(
		"threshold: %d msgs/min | max: %ds\ntracked: %s",
		thr, mx, strings.Join(tracked, " "),
	))
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (p *Plugin) intConfig(guildID, key string, def int) int {
	v := p.api.GetConfig(guildID, key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
