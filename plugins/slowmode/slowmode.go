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

// channelState tracks sliding-window message timestamps and current slowmode state.
type channelState struct {
	mu           sync.Mutex
	timestamps   []time.Time // message timestamps in the last 60 seconds
	slowActive   bool        // whether slowmode is currently applied
	quietSince   time.Time   // when the rate first dropped below threshold/2
}

// Plugin automatically adjusts channel slowmode based on message rate.
type Plugin struct {
	api    pluginapi.API
	cancel context.CancelFunc

	mu       sync.Mutex
	channels map[string]*channelState // keyed by channelID
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "slowmode",
		Version:     "1.0.0",
		Description: "Auto-adjust channel slowmode based on message rate.",
		Author:      "HiLleywyn",
		Commands:    []string{"slowmode"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	p.channels = make(map[string]*channelState)

	api.AddCommand(&discord.Command{
		Name:        "slowmode",
		Description: "Auto-slowmode based on message rate.",
		Usage:       "auto <threshold> | off | status",
		Handler:     p.handleCmd,
	})
	api.OnMessage(p.onMessage)

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go p.cleaner(ctx)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("slowmode")
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Background goroutine - cleans old state for channels no longer monitored
// ---------------------------------------------------------------------------

func (p *Plugin) cleaner(ctx context.Context) {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.mu.Lock()
			for id, st := range p.channels {
				st.mu.Lock()
				if !st.slowActive && len(st.timestamps) == 0 {
					delete(p.channels, id)
				}
				st.mu.Unlock()
			}
			p.mu.Unlock()
		}
	}
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !slowmode auto <msgs/min> | off | status")
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "auto":
		if len(ctx.Args) < 2 {
			ctx.Reply("Provide a message rate threshold (messages per minute).")
			return
		}
		n, err := strconv.Atoi(ctx.Args[1])
		if err != nil || n < 1 {
			ctx.Reply("Threshold must be a positive integer.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "auto:"+ctx.ChannelID, strconv.Itoa(n))
		ctx.Reply(fmt.Sprintf(
			"Auto slowmode enabled for <#%s>. Triggers at %d msgs/min.",
			ctx.ChannelID, n,
		))

	case "off":
		p.api.DeleteConfig(ctx.GuildID, "auto:"+ctx.ChannelID)
		// Remove any active slowmode.
		_, err := ctx.Bot.Rest.ModifyChannel(ctx.ChannelID, map[string]interface{}{
			"rate_limit_per_user": 0,
		})
		if err != nil {
			p.api.Log("slowmode: failed to clear slowmode on %s: %v", ctx.ChannelID, err)
		}
		// Clear local state.
		p.mu.Lock()
		delete(p.channels, ctx.ChannelID)
		p.mu.Unlock()
		ctx.Reply("Auto slowmode disabled for this channel.")

	case "status":
		v := p.api.GetConfig(ctx.GuildID, "auto:"+ctx.ChannelID)
		if v == "" {
			ctx.Reply("Auto slowmode is not enabled for this channel.")
			return
		}
		p.mu.Lock()
		st := p.channels[ctx.ChannelID]
		p.mu.Unlock()

		rate := 0
		active := false
		if st != nil {
			st.mu.Lock()
			rate = currentRate(st.timestamps)
			active = st.slowActive
			st.mu.Unlock()
		}
		ctx.Reply(fmt.Sprintf(
			"threshold: %s msgs/min | current rate: %d msgs/min | slowmode active: %v",
			v, rate, active,
		))

	default:
		ctx.Reply("Unknown subcommand. Try: auto, off, status")
	}
}

// ---------------------------------------------------------------------------
// Message handler
// ---------------------------------------------------------------------------

func (p *Plugin) onMessage(bot *discord.Bot, msg *discord.Message) {
	if msg.GuildID == "" {
		return
	}

	// Check if auto slowmode is configured for this channel.
	thresholdStr := p.api.GetConfig(msg.GuildID, "auto:"+msg.ChannelID)
	if thresholdStr == "" {
		return
	}
	threshold, err := strconv.Atoi(thresholdStr)
	if err != nil || threshold < 1 {
		return
	}

	// Get or create state for this channel.
	p.mu.Lock()
	st := p.channels[msg.ChannelID]
	if st == nil {
		st = &channelState{}
		p.channels[msg.ChannelID] = st
	}
	p.mu.Unlock()

	st.mu.Lock()
	defer st.mu.Unlock()

	now := time.Now()
	// Add this message.
	st.timestamps = append(st.timestamps, now)
	// Prune messages older than 60 seconds.
	cutoff := now.Add(-60 * time.Second)
	keep := 0
	for i, t := range st.timestamps {
		if t.After(cutoff) {
			keep = i
			break
		}
	}
	st.timestamps = st.timestamps[keep:]

	rate := len(st.timestamps) // messages in the last 60 seconds = msgs/min

	if !st.slowActive && rate >= threshold {
		// Activate slowmode: set to 5 seconds per user.
		_, err := bot.Rest.ModifyChannel(msg.ChannelID, map[string]interface{}{
			"rate_limit_per_user": 5,
		})
		if err != nil {
			p.api.Log("slowmode: failed to set slowmode on %s: %v", msg.ChannelID, err)
		} else {
			st.slowActive = true
			st.quietSince = time.Time{} // reset quiet timer
			p.api.Log("slowmode: activated on channel %s (rate=%d, threshold=%d)",
				msg.ChannelID, rate, threshold)
		}
	} else if st.slowActive && rate < threshold/2 {
		// Rate is below half threshold - track quiet time.
		if st.quietSince.IsZero() {
			st.quietSince = now
		} else if now.Sub(st.quietSince) >= 2*time.Minute {
			// Been quiet for 2 minutes - remove slowmode.
			_, err := bot.Rest.ModifyChannel(msg.ChannelID, map[string]interface{}{
				"rate_limit_per_user": 0,
			})
			if err != nil {
				p.api.Log("slowmode: failed to remove slowmode on %s: %v", msg.ChannelID, err)
			} else {
				st.slowActive = false
				st.quietSince = time.Time{}
				p.api.Log("slowmode: deactivated on channel %s", msg.ChannelID)
			}
		}
	} else if st.slowActive && rate >= threshold/2 {
		// Still busy, reset quiet timer.
		st.quietSince = time.Time{}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// currentRate returns the number of messages recorded in the last 60 seconds.
func currentRate(timestamps []time.Time) int {
	cutoff := time.Now().Add(-60 * time.Second)
	count := 0
	for _, t := range timestamps {
		if t.After(cutoff) {
			count++
		}
	}
	return count
}
