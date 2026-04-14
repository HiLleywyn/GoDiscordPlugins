// Package reminder implements personal reminders.
//
// Users can schedule reminders with `!remindme <duration> <note>`. The note is
// delivered either as a channel reply pinging the requester, or by DM (see
// `!reminders dm on`). Reminders are persisted per-guild and checked every
// minute by a background ticker.
package reminder

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

// reminderData is the persisted state for a single reminder.
type reminderData struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	ChannelID string    `json:"channel_id"`
	Note      string    `json:"note"`
	Due       time.Time `json:"due"`
	DM        bool      `json:"dm"`
}

// Plugin is the reminder plugin instance.
type Plugin struct {
	api    pluginapi.API
	cancel context.CancelFunc
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "reminder",
		Version:     "1.0.0",
		Description: "Personal reminders delivered in channel or by DM.",
		Author:      "HiLleywyn",
		Commands:    []string{"remindme", "reminders"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api

	api.AddCommand(&discord.Command{
		Name:        "remindme",
		Aliases:     []string{"remind"},
		Description: "Schedule a personal reminder.",
		Usage:       "<duration> <note>   e.g. 30m check the oven",
		Handler:     p.cmdRemindMe,
	})
	api.AddCommand(&discord.Command{
		Name:        "reminders",
		Description: "List and manage your reminders.",
		Usage:       "[list] | cancel <id> | dm on|off",
		Handler:     p.cmdReminders,
	})

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go p.tick(ctx)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("remindme")
	p.api.RemoveCommand("reminders")
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
			p.checkDue()
		}
	}
}

func (p *Plugin) checkDue() {
	now := time.Now()
	rest := p.api.Rest()
	for _, guildID := range p.api.GuildIDs() {
		for k, v := range p.api.AllConfig(guildID) {
			if !strings.HasPrefix(k, "rem:") {
				continue
			}
			var r reminderData
			if err := json.Unmarshal([]byte(v), &r); err != nil {
				continue
			}
			if r.Due.After(now) {
				continue
			}
			p.deliver(rest, guildID, &r)
			p.api.DeleteConfig(guildID, k)
		}
	}
}

func (p *Plugin) deliver(rest *discord.RestClient, guildID string, r *reminderData) {
	text := fmt.Sprintf("<@%s> reminder: %s", r.UserID, r.Note)

	if r.DM {
		if _, err := rest.SendDM(r.UserID, text); err == nil {
			return
		}
		// Fall through to channel delivery if DM failed (user has DMs off).
		p.api.Log("reminder: DM to %s failed, falling back to channel", r.UserID)
	}
	if _, err := rest.SendMessage(r.ChannelID, text); err != nil {
		p.api.Log("reminder: channel deliver %s: %v", r.UserID, err)
	}
}

// ---------------------------------------------------------------------------
// !remindme <duration> <note>
// ---------------------------------------------------------------------------

func (p *Plugin) cmdRemindMe(ctx *discord.CommandContext) {
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !remindme <duration> <note>    (e.g. !remindme 2h tea time)")
		return
	}
	dur, err := parseDuration(ctx.Args[0])
	if err != nil {
		ctx.Reply("Invalid duration. Use s/m/h/d suffixes, e.g. 10m, 3h, 2d.")
		return
	}
	if dur < time.Minute {
		ctx.Reply("Minimum reminder duration is 1 minute.")
		return
	}
	if dur > 365*24*time.Hour {
		ctx.Reply("Maximum reminder duration is 365 days.")
		return
	}

	note := strings.Join(ctx.Args[1:], " ")
	if len(note) > 500 {
		note = note[:500] + "..."
	}

	due := time.Now().Add(dur)
	id := newReminderID(ctx.AuthorID, due)

	r := reminderData{
		ID:        id,
		UserID:    ctx.AuthorID,
		ChannelID: ctx.ChannelID,
		Note:      note,
		Due:       due,
		DM:        p.api.GetConfig(ctx.GuildID, "dm:"+ctx.AuthorID) == "true",
	}
	b, _ := json.Marshal(&r)
	p.api.SetConfig(ctx.GuildID, "rem:"+id, string(b))

	ctx.Reply(fmt.Sprintf("Reminder set for <t:%d:R>. (id: `%s`)", due.Unix(), id))
}

// ---------------------------------------------------------------------------
// !reminders list | cancel <id> | dm on|off
// ---------------------------------------------------------------------------

func (p *Plugin) cmdReminders(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 || strings.EqualFold(ctx.Args[0], "list") {
		p.listReminders(ctx)
		return
	}
	switch strings.ToLower(ctx.Args[0]) {
	case "cancel":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !reminders cancel <id>")
			return
		}
		id := ctx.Args[1]
		key := "rem:" + id
		raw := p.api.GetConfig(ctx.GuildID, key)
		if raw == "" {
			ctx.Reply("No such reminder.")
			return
		}
		var r reminderData
		_ = json.Unmarshal([]byte(raw), &r)
		if r.UserID != ctx.AuthorID {
			ctx.Reply("That reminder isn't yours.")
			return
		}
		p.api.DeleteConfig(ctx.GuildID, key)
		ctx.Reply("Reminder cancelled.")

	case "dm":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !reminders dm on|off")
			return
		}
		switch strings.ToLower(ctx.Args[1]) {
		case "on":
			p.api.SetConfig(ctx.GuildID, "dm:"+ctx.AuthorID, "true")
			ctx.Reply("Future reminders will be delivered by DM (falls back to channel if DMs are closed).")
		case "off":
			p.api.DeleteConfig(ctx.GuildID, "dm:"+ctx.AuthorID)
			ctx.Reply("Future reminders will be delivered in the channel where they were set.")
		default:
			ctx.Reply("Usage: !reminders dm on|off")
		}

	default:
		ctx.Reply("Usage: !reminders [list] | cancel <id> | dm on|off")
	}
}

func (p *Plugin) listReminders(ctx *discord.CommandContext) {
	type entry struct {
		id  string
		due time.Time
		txt string
	}

	var mine []entry
	for k, v := range p.api.AllConfig(ctx.GuildID) {
		if !strings.HasPrefix(k, "rem:") {
			continue
		}
		var r reminderData
		if err := json.Unmarshal([]byte(v), &r); err != nil {
			continue
		}
		if r.UserID != ctx.AuthorID {
			continue
		}
		mine = append(mine, entry{
			id:  r.ID,
			due: r.Due,
			txt: r.Note,
		})
	}

	if len(mine) == 0 {
		ctx.Reply("You have no reminders set here.")
		return
	}

	sort.Slice(mine, func(i, j int) bool { return mine[i].due.Before(mine[j].due) })

	var lines []string
	for _, e := range mine {
		lines = append(lines, fmt.Sprintf(
			"`%s` <t:%d:R> - %s",
			e.id, e.due.Unix(), truncate(e.txt, 120),
		))
	}
	ctx.ReplyEmbed(discord.Embed{
		Title:       "Your reminders",
		Description: strings.Join(lines, "\n"),
		Color:       0x5865F2,
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// parseDuration supports s/m/h/d suffixes.
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
		return 0, fmt.Errorf("unknown suffix '%c', use s/m/h/d", suffix)
	}
}

// newReminderID builds a short unique-ish id from user and due time.
// Users only ever see their own reminders, so collisions across users are ok.
func newReminderID(userID string, due time.Time) string {
	ts := strconv.FormatInt(due.UnixNano(), 36)
	suffix := userID
	if len(suffix) > 4 {
		suffix = suffix[len(suffix)-4:]
	}
	return ts + suffix
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
