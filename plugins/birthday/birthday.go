// Package birthday tracks user birthdays and announces them with an
// optional birthday role for the day.
//
// Users self-register with `!birthday set MM-DD` (or `YYYY-MM-DD` to
// include the year for age counting). A background ticker runs every
// hour and, once per guild-day, posts an announcement and grants the
// configured birthday role to anyone whose birthday is today.
// Yesterday's recipients have the role removed.
//
// Commands:
//
//	!birthday set <MM-DD | YYYY-MM-DD>   Set your birthday
//	!birthday clear                      Remove your birthday
//	!birthday @user                      Show a user's birthday
//	!birthday upcoming                   List next 7 days
//	!birthday channel #chan              Admin: announcement channel
//	!birthday role @role                 Admin: birthday role (optional)
//	!birthday message <text>             Admin: announcement template
package birthday

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

const defaultMessage = "Happy birthday <@{user}>!"

// Plugin is the birthday plugin instance.
type Plugin struct {
	api    pluginapi.API
	cancel context.CancelFunc
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "birthday",
		Version:     "1.0.0",
		Description: "Track user birthdays, announce them, grant a birthday role.",
		Author:      "HiLleywyn",
		Commands:    []string{"birthday", "bday"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "birthday",
		Aliases:     []string{"bday"},
		Description: "Manage birthdays.",
		Usage:       "set <MM-DD|YYYY-MM-DD> | clear | @user | upcoming | channel #chan | role @role | message <text>",
		Handler:     p.handleCmd,
	})
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go p.tick(ctx)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("birthday")
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !birthday set <MM-DD> | clear | @user | upcoming | channel #chan | role @role | message <text>")
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "set":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !birthday set <MM-DD> or <YYYY-MM-DD>")
			return
		}
		m, d, y, err := parseDate(ctx.Args[1])
		if err != nil {
			ctx.Reply("Couldn't parse that date. Use `MM-DD` or `YYYY-MM-DD`.")
			return
		}
		val := fmt.Sprintf("%04d-%02d-%02d", y, m, d)
		p.api.SetConfig(ctx.GuildID, "u:"+ctx.AuthorID, val)
		ctx.Reply("Birthday set to **" + formatDate(m, d, y) + "**.")

	case "clear":
		p.api.DeleteConfig(ctx.GuildID, "u:"+ctx.AuthorID)
		ctx.Reply("Birthday cleared.")

	case "upcoming":
		p.cmdUpcoming(ctx)

	case "channel":
		if !isAdmin(ctx) {
			ctx.Reply("You need Manage Messages for this.")
			return
		}
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !birthday channel #chan")
			return
		}
		chID := discord.ParseChannelMention(ctx.Args[1])
		if chID == "" {
			ctx.Reply("Provide a valid channel.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "channel", chID)
		ctx.Reply("Announcements will be posted in <#" + chID + ">.")

	case "role":
		if !isAdmin(ctx) {
			ctx.Reply("You need Manage Messages for this.")
			return
		}
		if len(ctx.Args) < 2 {
			p.api.DeleteConfig(ctx.GuildID, "role")
			ctx.Reply("Birthday role cleared.")
			return
		}
		roleID := discord.ParseRoleMention(ctx.Args[1])
		if roleID == "" {
			ctx.Reply("Provide a valid role.")
			return
		}
		p.api.SetConfig(ctx.GuildID, "role", roleID)
		ctx.Reply("Birthday role set to <@&" + roleID + ">.")

	case "message":
		if !isAdmin(ctx) {
			ctx.Reply("You need Manage Messages for this.")
			return
		}
		if len(ctx.Args) < 2 {
			ctx.Reply("Current message: `" + p.message(ctx.GuildID) + "`")
			return
		}
		tmpl := strings.Join(ctx.Args[1:], " ")
		p.api.SetConfig(ctx.GuildID, "message", tmpl)
		ctx.Reply("Announcement template set.")

	default:
		// @user lookup
		uid := discord.ParseUserID(ctx.Args[0])
		if uid == "" {
			ctx.Reply("Unknown subcommand.")
			return
		}
		val := p.api.GetConfig(ctx.GuildID, "u:"+uid)
		if val == "" {
			ctx.Reply("That user hasn't set a birthday.")
			return
		}
		m, d, y, _ := parseDate(val)
		ctx.Reply(fmt.Sprintf("<@%s>'s birthday: **%s**", uid, formatDate(m, d, y)))
	}
}

func (p *Plugin) cmdUpcoming(ctx *discord.CommandContext) {
	type entry struct {
		userID string
		month  int
		day    int
		year   int
		days   int
	}
	var rows []entry
	now := time.Now()
	for k, v := range p.api.AllConfig(ctx.GuildID) {
		if !strings.HasPrefix(k, "u:") {
			continue
		}
		m, d, y, err := parseDate(v)
		if err != nil {
			continue
		}
		days := daysUntil(now, m, d)
		if days >= 0 && days <= 7 {
			rows = append(rows, entry{
				userID: strings.TrimPrefix(k, "u:"),
				month:  m, day: d, year: y, days: days,
			})
		}
	}
	if len(rows) == 0 {
		ctx.Reply("No birthdays in the next 7 days.")
		return
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].days < rows[j].days })
	var lines []string
	for _, r := range rows {
		when := "today"
		if r.days == 1 {
			when = "tomorrow"
		} else if r.days > 1 {
			when = fmt.Sprintf("in %d days", r.days)
		}
		lines = append(lines, fmt.Sprintf("%s - <@%s> (%s)",
			formatDate(r.month, r.day, 0), r.userID, when))
	}
	ctx.ReplyEmbed(discord.Embed{
		Title:       "Upcoming birthdays",
		Description: strings.Join(lines, "\n"),
		Color:       0xFEE75C,
	})
}

// ---------------------------------------------------------------------------
// Background ticker
// ---------------------------------------------------------------------------

func (p *Plugin) tick(ctx context.Context) {
	t := time.NewTicker(1 * time.Hour)
	defer t.Stop()
	// Run once at startup too.
	p.runDaily()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.runDaily()
		}
	}
}

func (p *Plugin) runDaily() {
	rest := p.api.Rest()
	today := time.Now().UTC().Format("2006-01-02")
	for _, guildID := range p.api.GuildIDs() {
		// Once per guild per day.
		lastRun := p.api.GetConfig(guildID, "lastrun")
		if lastRun == today {
			continue
		}

		chID := p.api.GetConfig(guildID, "channel")
		roleID := p.api.GetConfig(guildID, "role")
		tmpl := p.message(guildID)

		now := time.Now().UTC()
		m, d := int(now.Month()), now.Day()

		// Remove role from yesterday's birthday people.
		if roleID != "" {
			for _, uid := range p.yesterdayUsers(guildID) {
				_ = rest.RemoveMemberRole(guildID, uid, roleID)
			}
			p.api.DeleteConfig(guildID, "yesterday")
		}

		// Today's birthday people.
		var todaysUsers []string
		for k, v := range p.api.AllConfig(guildID) {
			if !strings.HasPrefix(k, "u:") {
				continue
			}
			bm, bd, by, err := parseDate(v)
			if err != nil {
				continue
			}
			if bm != m || bd != d {
				continue
			}
			uid := strings.TrimPrefix(k, "u:")
			todaysUsers = append(todaysUsers, uid)

			if roleID != "" {
				if err := rest.AddMemberRole(guildID, uid, roleID); err != nil {
					p.api.Log("birthday: role add %s: %v", uid, err)
				}
			}
			if chID != "" {
				text := strings.ReplaceAll(tmpl, "{user}", uid)
				if by > 0 {
					age := now.Year() - by
					text = strings.ReplaceAll(text, "{age}", strconv.Itoa(age))
				} else {
					text = strings.ReplaceAll(text, "{age}", "")
				}
				if _, err := rest.SendMessage(chID, text); err != nil {
					p.api.Log("birthday: send %s: %v", chID, err)
				}
			}
		}

		if len(todaysUsers) > 0 {
			p.api.SetConfig(guildID, "yesterday", strings.Join(todaysUsers, ","))
		}
		p.api.SetConfig(guildID, "lastrun", today)
	}
}

func (p *Plugin) yesterdayUsers(guildID string) []string {
	v := p.api.GetConfig(guildID, "yesterday")
	if v == "" {
		return nil
	}
	return strings.Split(v, ",")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (p *Plugin) message(guildID string) string {
	v := p.api.GetConfig(guildID, "message")
	if v == "" {
		return defaultMessage
	}
	return v
}

func parseDate(s string) (month, day, year int, err error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, "-")
	switch len(parts) {
	case 2:
		m, e1 := strconv.Atoi(parts[0])
		d, e2 := strconv.Atoi(parts[1])
		if e1 != nil || e2 != nil || !validMonthDay(m, d) {
			return 0, 0, 0, fmt.Errorf("bad date")
		}
		return m, d, 0, nil
	case 3:
		y, e1 := strconv.Atoi(parts[0])
		m, e2 := strconv.Atoi(parts[1])
		d, e3 := strconv.Atoi(parts[2])
		if e1 != nil || e2 != nil || e3 != nil || !validMonthDay(m, d) || y < 1900 || y > 2100 {
			return 0, 0, 0, fmt.Errorf("bad date")
		}
		return m, d, y, nil
	}
	return 0, 0, 0, fmt.Errorf("bad date")
}

func validMonthDay(m, d int) bool {
	if m < 1 || m > 12 || d < 1 || d > 31 {
		return false
	}
	return true
}

func formatDate(m, d, y int) string {
	months := []string{"", "Jan", "Feb", "Mar", "Apr", "May", "Jun",
		"Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	if y > 0 {
		return fmt.Sprintf("%s %d, %d", months[m], d, y)
	}
	return fmt.Sprintf("%s %d", months[m], d)
}

func daysUntil(now time.Time, month, day int) int {
	year := now.Year()
	target := time.Date(year, time.Month(month), day, 0, 0, 0, 0, now.Location())
	if target.Before(now.Truncate(24 * time.Hour)) {
		target = time.Date(year+1, time.Month(month), day, 0, 0, 0, 0, now.Location())
	}
	return int(target.Sub(now.Truncate(24*time.Hour)).Hours() / 24)
}

func isAdmin(ctx *discord.CommandContext) bool {
	if ctx.Member == nil {
		return false
	}
	return ctx.Member.HasPermission(discord.PermissionManageMessages)
}
