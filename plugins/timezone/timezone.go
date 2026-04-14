// Package timezone stores per-user timezones and renders them as
// Discord-native timestamps that each viewer sees in their own locale.
//
// Commands:
//
//	!tz set <zone>        Save your timezone (IANA, e.g. "America/New_York")
//	!tz clear             Remove your timezone
//	!tz @user             Show a user's current local time
//	!tz                   Show your current local time
//	!when <time>          Broadcast a time: each viewer sees it in their TZ
//	                      (uses Discord <t:unix:F> timestamps)
package timezone

import (
	"fmt"
	"strings"
	"time"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

// Plugin is the timezone plugin instance.
type Plugin struct {
	api pluginapi.API
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "timezone",
		Version:     "1.0.0",
		Description: "Per-user timezones and locale-aware time formatting.",
		Author:      "HiLleywyn",
		Commands:    []string{"tz", "when"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "tz",
		Aliases:     []string{"timezone"},
		Description: "Manage and query user timezones.",
		Usage:       "[set <zone> | clear | @user]",
		Handler:     p.cmdTZ,
	})
	api.AddCommand(&discord.Command{
		Name:        "when",
		Description: "Broadcast a time in a way that renders locally for every viewer.",
		Usage:       "<time>   e.g. 15:30, 3pm, tomorrow 9am",
		Handler:     p.cmdWhen,
	})
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("tz")
	p.api.RemoveCommand("when")
	return nil
}

// ---------------------------------------------------------------------------
// !tz
// ---------------------------------------------------------------------------

func (p *Plugin) cmdTZ(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		// Self
		zone := p.api.GetConfig(ctx.GuildID, "u:"+ctx.AuthorID)
		if zone == "" {
			ctx.Reply("You haven't set a timezone. Try `!tz set America/New_York`.")
			return
		}
		loc, err := time.LoadLocation(zone)
		if err != nil {
			ctx.Reply("Your timezone `" + zone + "` is invalid; reset with `!tz set <zone>`.")
			return
		}
		now := time.Now().In(loc)
		ctx.Reply(fmt.Sprintf("Your local time: **%s** (%s)", now.Format("Mon 15:04 MST"), zone))
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "set":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !tz set <IANA zone>, e.g. America/New_York")
			return
		}
		zone := ctx.Args[1]
		if _, err := time.LoadLocation(zone); err != nil {
			ctx.Reply("Invalid timezone `" + zone + "`. Use IANA format, e.g. `Europe/Berlin`.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "u:"+ctx.AuthorID, zone)
		ctx.Reply("Timezone set to `" + zone + "`.")
	case "clear":
		p.api.DeleteConfig(ctx.GuildID, "u:"+ctx.AuthorID)
		ctx.Reply("Timezone cleared.")
	default:
		// @user lookup
		uid := discord.ParseUserID(ctx.Args[0])
		if uid == "" {
			ctx.Reply("Usage: !tz set <zone> | clear | @user")
			return
		}
		zone := p.api.GetConfig(ctx.GuildID, "u:"+uid)
		if zone == "" {
			ctx.Reply("That user hasn't set a timezone.")
			return
		}
		loc, err := time.LoadLocation(zone)
		if err != nil {
			ctx.Reply("That user's timezone is invalid.")
			return
		}
		now := time.Now().In(loc)
		ctx.Reply(fmt.Sprintf("<@%s> local time: **%s** (%s)",
			uid, now.Format("Mon 15:04 MST"), zone))
	}
}

// ---------------------------------------------------------------------------
// !when
// ---------------------------------------------------------------------------

func (p *Plugin) cmdWhen(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !when <time>   e.g. `!when 15:30`, `!when 3pm`, `!when tomorrow 9am`")
		return
	}
	zone := p.api.GetConfig(ctx.GuildID, "u:"+ctx.AuthorID)
	loc := time.UTC
	if zone != "" {
		if l, err := time.LoadLocation(zone); err == nil {
			loc = l
		}
	}

	input := strings.Join(ctx.Args, " ")
	t, err := parseTime(input, loc)
	if err != nil {
		ctx.Reply("Couldn't parse that time. Try `15:30`, `3pm`, or `tomorrow 9am`.")
		return
	}

	// Discord timestamp - each viewer sees it in their locale.
	ts := t.Unix()
	ctx.Reply(fmt.Sprintf("<t:%d:F> (<t:%d:R>)", ts, ts))
}

// ---------------------------------------------------------------------------
// Parsing
// ---------------------------------------------------------------------------

// parseTime handles a few common shorthand formats.
// Returns a time in the provided location.
func parseTime(s string, loc *time.Location) (time.Time, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	now := time.Now().In(loc)

	// "tomorrow <time>" prefix
	addDay := 0
	if strings.HasPrefix(s, "tomorrow ") {
		addDay = 1
		s = strings.TrimPrefix(s, "tomorrow ")
	} else if strings.HasPrefix(s, "today ") {
		s = strings.TrimPrefix(s, "today ")
	}

	// "3pm", "3:30pm", "15:30", "15:30:00"
	formats := []string{"3pm", "3:04pm", "15:04", "15:04:05"}
	var parsed time.Time
	var err error
	for _, f := range formats {
		parsed, err = time.ParseInLocation(f, s, loc)
		if err == nil {
			break
		}
	}
	if err != nil {
		return time.Time{}, err
	}

	year, month, day := now.Date()
	day += addDay
	result := time.Date(year, month, day,
		parsed.Hour(), parsed.Minute(), parsed.Second(), 0, loc)

	// If the result is in the past (e.g. "3pm" posted at 5pm with no
	// "tomorrow"), roll forward a day.
	if addDay == 0 && result.Before(now) {
		result = result.Add(24 * time.Hour)
	}
	return result, nil
}
