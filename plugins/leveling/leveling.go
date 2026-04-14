// Package leveling provides a message XP and rank system.
//
// Users earn XP for sending messages (with a 60-second cooldown per user to
// prevent spam farming). When a user reaches a new level, the bot announces
// it in the configured announcement channel (or the message channel if none
// is set). Admins and mods can configure XP per message, the cooldown,
// level-up announcements, role rewards, and XP multipliers per channel.
//
// Commands (mod/admin only unless noted):
//   !rank [@user]          - show your (or another user's) rank card
//   !leaderboard [page]    - top 10 users by XP
//   !xp give @user <amount>
//   !xp take @user <amount>
//   !xp reset @user
//   !xp setcooldown <seconds>
//   !xp setpermsg <amount>
//   !xp setannounce [#channel|off]
//   !xp setmultiplier <#channel> <multiplier>
//   !xp rewardadd <level> @role
//   !xp rewardremove <level>
//   !xp rewardlist
package leveling

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
	defaultXPPerMsg  = 15  // base XP awarded per message
	defaultCooldown  = 60  // seconds between XP grants for the same user
	xpPerLevel       = 100 // XP needed per level (multiplied by level number)
)

// levelThreshold returns the total XP required to reach a given level.
// Level 1 = 100 XP, level 2 = 300 XP, level 3 = 600 XP, ...
// Formula: sum of (xpPerLevel * i) for i = 1..level
func levelThreshold(level int) int {
	total := 0
	for i := 1; i <= level; i++ {
		total += xpPerLevel * i
	}
	return total
}

// xpToLevel converts a total XP value to the current level.
func xpToLevel(xp int) int {
	level := 0
	for xp >= levelThreshold(level+1) {
		level++
	}
	return level
}

// cooldownKey is used as the map key for per-user cooldowns.
type cooldownKey struct {
	guildID string
	userID  string
}

// Plugin is the leveling plugin instance.
type Plugin struct {
	api    pluginapi.API
	cancel context.CancelFunc

	mu        sync.Mutex
	cooldowns map[cooldownKey]time.Time // last XP grant time
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "leveling",
		Version:     "1.0.0",
		Description: "Message XP, levels, rank cards, and role rewards.",
		Author:      "HiLleywyn",
		Commands:    []string{"rank", "leaderboard", "xp"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	p.cooldowns = make(map[cooldownKey]time.Time)

	api.AddCommand(&discord.Command{
		Name:        "rank",
		Description: "Show your rank and XP.",
		Handler:     p.cmdRank,
	})
	api.AddCommand(&discord.Command{
		Name:        "leaderboard",
		Aliases:     []string{"lb", "top"},
		Description: "Top 10 users by XP.",
		Handler:     p.cmdLeaderboard,
	})
	api.AddCommand(&discord.Command{
		Name:        "xp",
		Description: "XP management (mod/admin).",
		Handler:     p.cmdXP,
	})

	api.OnMessage(p.onMessage)

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go p.cooldownSweep(ctx)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("rank")
	p.api.RemoveCommand("leaderboard")
	p.api.RemoveCommand("xp")
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Background sweep - purge stale cooldown entries
// ---------------------------------------------------------------------------

func (p *Plugin) cooldownSweep(ctx context.Context) {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			cutoff := time.Now().Add(-5 * time.Minute)
			p.mu.Lock()
			for k, ts := range p.cooldowns {
				if ts.Before(cutoff) {
					delete(p.cooldowns, k)
				}
			}
			p.mu.Unlock()
		}
	}
}

// ---------------------------------------------------------------------------
// Message handler - award XP
// ---------------------------------------------------------------------------

func (p *Plugin) onMessage(bot *discord.Bot, msg *discord.Message) {
	if msg.GuildID == "" || msg.Author == nil || msg.Author.Bot {
		return
	}

	// Respect per-guild cooldown setting.
	cooldownSecs := p.intConfig(msg.GuildID, "cooldown", defaultCooldown)
	key := cooldownKey{msg.GuildID, msg.Author.ID}

	p.mu.Lock()
	last := p.cooldowns[key]
	if time.Since(last) < time.Duration(cooldownSecs)*time.Second {
		p.mu.Unlock()
		return
	}
	p.cooldowns[key] = time.Now()
	p.mu.Unlock()

	// Apply channel multiplier (stored as "multi:<channelID>" = "1.5" etc.)
	base := p.intConfig(msg.GuildID, "permsg", defaultXPPerMsg)
	xpGain := base
	if multiStr := p.api.GetConfig(msg.GuildID, "multi:"+msg.ChannelID); multiStr != "" {
		if f, err := strconv.ParseFloat(multiStr, 64); err == nil && f > 0 {
			xpGain = int(float64(base) * f)
		}
	}

	// Read and update stored XP.
	xpKey := "xp:" + msg.Author.ID
	oldXP := p.intConfig(msg.GuildID, xpKey, 0)
	newXP := oldXP + xpGain
	p.api.SetConfig(msg.GuildID, xpKey, strconv.Itoa(newXP))

	oldLevel := xpToLevel(oldXP)
	newLevel := xpToLevel(newXP)
	if newLevel > oldLevel {
		p.onLevelUp(bot, msg, newLevel)
	}
}

// onLevelUp announces a level-up and grants role rewards.
func (p *Plugin) onLevelUp(bot *discord.Bot, msg *discord.Message, level int) {
	// Role rewards.
	if roleID := p.api.GetConfig(msg.GuildID, fmt.Sprintf("reward:%d", level)); roleID != "" {
		if err := bot.Rest.AddMemberRole(msg.GuildID, msg.Author.ID, roleID); err != nil {
			p.api.Log("leveling: add role reward %s to %s: %v", roleID, msg.Author.ID, err)
		}
	}

	// Announcement.
	announce := p.api.GetConfig(msg.GuildID, "announce_channel")
	channelID := msg.ChannelID
	if announce != "" && announce != "off" {
		channelID = announce
	}

	text := fmt.Sprintf("<@%s> reached **level %d**!", msg.Author.ID, level)
	if _, err := bot.Rest.SendMessage(channelID, text); err != nil {
		p.api.Log("leveling: announce level-up: %v", err)
	}
}

// ---------------------------------------------------------------------------
// !rank
// ---------------------------------------------------------------------------

func (p *Plugin) cmdRank(ctx *discord.CommandContext) {
	targetID := ctx.AuthorID
	if len(ctx.Args) > 0 {
		if parsed := discord.ParseUserID(ctx.Args[0]); parsed != "" {
			targetID = parsed
		}
	}

	xpKey := "xp:" + targetID
	xp := p.intConfig(ctx.GuildID, xpKey, 0)
	level := xpToLevel(xp)

	next := levelThreshold(level + 1)
	current := levelThreshold(level)
	progress := xp - current
	needed := next - current

	bar := progressBar(progress, needed, 10)
	ctx.ReplyEmbed(discord.Embed{
		Title: "Rank",
		Description: fmt.Sprintf(
			"<@%s>\n**Level:** %d\n**XP:** %d / %d to level %d\n`%s`",
			targetID, level, progress, needed, level+1, bar,
		),
		Color: 0x5865F2,
	})
}

// ---------------------------------------------------------------------------
// !leaderboard
// ---------------------------------------------------------------------------

func (p *Plugin) cmdLeaderboard(ctx *discord.CommandContext) {
	page := 1
	if len(ctx.Args) > 0 {
		if n, err := strconv.Atoi(ctx.Args[0]); err == nil && n > 0 {
			page = n
		}
	}

	all := p.api.AllConfig(ctx.GuildID)
	type entry struct {
		userID string
		xp     int
	}
	var rows []entry
	for k, v := range all {
		if !strings.HasPrefix(k, "xp:") {
			continue
		}
		uid := strings.TrimPrefix(k, "xp:")
		xp, _ := strconv.Atoi(v)
		rows = append(rows, entry{uid, xp})
	}

	// Sort descending by XP.
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			if rows[j].xp > rows[i].xp {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}

	perPage := 10
	start := (page - 1) * perPage
	if start >= len(rows) {
		ctx.Reply("No entries on that page.")
		return
	}
	end := start + perPage
	if end > len(rows) {
		end = len(rows)
	}

	var lines []string
	for i, r := range rows[start:end] {
		lines = append(lines, fmt.Sprintf(
			"**%d.** <@%s> - Level %d (%d XP)",
			start+i+1, r.userID, xpToLevel(r.xp), r.xp,
		))
	}

	ctx.ReplyEmbed(discord.Embed{
		Title:       fmt.Sprintf("Leaderboard - Page %d", page),
		Description: strings.Join(lines, "\n"),
		Color:       0x5865F2,
	})
}

// ---------------------------------------------------------------------------
// !xp <sub> ...
// ---------------------------------------------------------------------------

func (p *Plugin) cmdXP(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: `!xp give|take|reset|setcooldown|setpermsg|setannounce|setmultiplier|rewardadd|rewardremove|rewardlist`")
		return
	}

	sub := strings.ToLower(ctx.Args[0])
	args := ctx.Args[1:]

	switch sub {
	case "give":
		p.xpAdjust(ctx, args, +1)
	case "take":
		p.xpAdjust(ctx, args, -1)
	case "reset":
		p.xpReset(ctx, args)
	case "setcooldown":
		p.xpSetInt(ctx, args, "cooldown", "cooldown", 1, 3600)
	case "setpermsg":
		p.xpSetInt(ctx, args, "permsg", "XP per message", 1, 1000)
	case "setannounce":
		p.xpSetAnnounce(ctx, args)
	case "setmultiplier":
		p.xpSetMultiplier(ctx, args)
	case "rewardadd":
		p.xpRewardAdd(ctx, args)
	case "rewardremove":
		p.xpRewardRemove(ctx, args)
	case "rewardlist":
		p.xpRewardList(ctx)
	default:
		ctx.Reply(fmt.Sprintf("Unknown subcommand `%s`.", sub))
	}
}

func (p *Plugin) xpAdjust(ctx *discord.CommandContext, args []string, sign int) {
	if len(args) < 2 {
		ctx.Reply("Usage: `!xp give|take @user <amount>`")
		return
	}
	targetID := discord.ParseUserID(args[0])
	if targetID == "" {
		ctx.Reply("Provide a valid user mention or ID.")
		return
	}
	amount, err := strconv.Atoi(args[1])
	if err != nil || amount <= 0 {
		ctx.Reply("Amount must be a positive integer.")
		return
	}
	xpKey := "xp:" + targetID
	xp := p.intConfig(ctx.GuildID, xpKey, 0)
	xp += sign * amount
	if xp < 0 {
		xp = 0
	}
	p.api.SetConfig(ctx.GuildID, xpKey, strconv.Itoa(xp))
	verb := "Gave"
	if sign < 0 {
		verb = "Took"
	}
	ctx.Reply(fmt.Sprintf("%s %d XP to/from <@%s>. They now have %d XP (level %d).",
		verb, amount, targetID, xp, xpToLevel(xp)))
}

func (p *Plugin) xpReset(ctx *discord.CommandContext, args []string) {
	if len(args) == 0 {
		ctx.Reply("Usage: `!xp reset @user`")
		return
	}
	targetID := discord.ParseUserID(args[0])
	if targetID == "" {
		ctx.Reply("Provide a valid user mention or ID.")
		return
	}
	p.api.DeleteConfig(ctx.GuildID, "xp:"+targetID)
	ctx.Reply(fmt.Sprintf("Reset XP for <@%s>.", targetID))
}

func (p *Plugin) xpSetInt(ctx *discord.CommandContext, args []string, key, label string, min, max int) {
	if len(args) == 0 {
		ctx.Reply(fmt.Sprintf("Current %s: %d", label, p.intConfig(ctx.GuildID, key, 0)))
		return
	}
	n, err := strconv.Atoi(args[0])
	if err != nil || n < min || n > max {
		ctx.Reply(fmt.Sprintf("%s must be between %d and %d.", label, min, max))
		return
	}
	p.api.SetConfig(ctx.GuildID, key, strconv.Itoa(n))
	ctx.Reply(fmt.Sprintf("Set %s to %d.", label, n))
}

func (p *Plugin) xpSetAnnounce(ctx *discord.CommandContext, args []string) {
	if len(args) == 0 {
		ctx.Reply("Usage: `!xp setannounce #channel|off`")
		return
	}
	val := args[0]
	if val == "off" {
		p.api.SetConfig(ctx.GuildID, "announce_channel", "off")
		ctx.Reply("Level-up announcements disabled.")
		return
	}
	chanID := discord.ParseChannelMention(val)
	p.api.SetConfig(ctx.GuildID, "announce_channel", chanID)
	ctx.Reply(fmt.Sprintf("Level-up announcements will go to <#%s>.", chanID))
}

func (p *Plugin) xpSetMultiplier(ctx *discord.CommandContext, args []string) {
	if len(args) < 2 {
		ctx.Reply("Usage: `!xp setmultiplier #channel <multiplier>`")
		return
	}
	chanID := discord.ParseChannelMention(args[0])
	f, err := strconv.ParseFloat(args[1], 64)
	if err != nil || f <= 0 {
		ctx.Reply("Multiplier must be a positive number (e.g. 1.5, 2).")
		return
	}
	p.api.SetConfig(ctx.GuildID, "multi:"+chanID, strconv.FormatFloat(f, 'f', 2, 64))
	ctx.Reply(fmt.Sprintf("XP multiplier for <#%s> set to %.2fx.", chanID, f))
}

func (p *Plugin) xpRewardAdd(ctx *discord.CommandContext, args []string) {
	if len(args) < 2 {
		ctx.Reply("Usage: `!xp rewardadd <level> @role`")
		return
	}
	level, err := strconv.Atoi(args[0])
	if err != nil || level < 1 {
		ctx.Reply("Level must be a positive integer.")
		return
	}
	roleID := discord.ParseRoleMention(args[1])
	p.api.SetConfig(ctx.GuildID, fmt.Sprintf("reward:%d", level), roleID)
	ctx.Reply(fmt.Sprintf("Role <@&%s> will be awarded at level %d.", roleID, level))
}

func (p *Plugin) xpRewardRemove(ctx *discord.CommandContext, args []string) {
	if len(args) == 0 {
		ctx.Reply("Usage: `!xp rewardremove <level>`")
		return
	}
	level, err := strconv.Atoi(args[0])
	if err != nil {
		ctx.Reply("Provide the level number.")
		return
	}
	p.api.DeleteConfig(ctx.GuildID, fmt.Sprintf("reward:%d", level))
	ctx.Reply(fmt.Sprintf("Removed role reward for level %d.", level))
}

func (p *Plugin) xpRewardList(ctx *discord.CommandContext) {
	all := p.api.AllConfig(ctx.GuildID)
	var lines []string
	for k, v := range all {
		if !strings.HasPrefix(k, "reward:") {
			continue
		}
		lvl := strings.TrimPrefix(k, "reward:")
		lines = append(lines, fmt.Sprintf("Level %s -> <@&%s>", lvl, v))
	}
	if len(lines) == 0 {
		ctx.Reply("No role rewards configured.")
		return
	}
	ctx.ReplyEmbed(discord.Embed{
		Title:       "Role Rewards",
		Description: strings.Join(lines, "\n"),
		Color:       0x5865F2,
	})
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

func progressBar(current, total, width int) string {
	if total <= 0 {
		return strings.Repeat("-", width)
	}
	filled := current * width / total
	if filled > width {
		filled = width
	}
	return strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
}
