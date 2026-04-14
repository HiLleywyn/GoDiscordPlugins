// Package highlight lets users subscribe to keywords and receive a DM
// whenever one of their keywords appears in a guild channel they're in.
//
// Think of it as a per-user mention list: "ping me whenever someone
// mentions Kubernetes". Users manage their own subscriptions with
// `!highlight add <word>`, and each user can also mute specific channels
// or users so they don't get DMed for noise.
//
// Commands:
//
//	!highlight add <word>        Subscribe to a word/phrase
//	!highlight remove <word>     Unsubscribe
//	!highlight list              Show your keywords, mutes, and cooldown
//	!highlight mute @user        Suppress highlights for this author
//	!highlight unmute @user      Remove a user mute
//	!highlight mute #channel     Suppress highlights in a channel
//	!highlight unmute #channel   Remove a channel mute
//	!highlight cooldown <secs>   Only DM at most once per N seconds per keyword
//	!highlight clear             Remove every keyword you have
package highlight

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

// userData is the per-user subscription record.
type userData struct {
	Keywords    []string `json:"keywords"`
	MutedUsers  []string `json:"muted_users,omitempty"`
	MutedChans  []string `json:"muted_channels,omitempty"`
	CooldownSec int      `json:"cooldown_sec,omitempty"`
}

// Plugin is the highlight plugin instance.
type Plugin struct {
	api pluginapi.API

	mu        sync.Mutex
	lastFired map[string]time.Time // key: guildID:userID:keyword
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "highlight",
		Version:     "1.0.0",
		Description: "DM users when their subscribed keywords appear.",
		Author:      "HiLleywyn",
		Commands:    []string{"highlight", "hl"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	p.lastFired = make(map[string]time.Time)
	api.AddCommand(&discord.Command{
		Name:        "highlight",
		Aliases:     []string{"hl"},
		Description: "Get DMed when a keyword appears in this guild.",
		Usage:       "add <word> | remove <word> | list | mute @user|#chan | unmute @user|#chan | cooldown <secs> | clear",
		Handler:     p.handleCmd,
	})
	api.OnMessage(p.onMessage)
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("highlight")
	return nil
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !highlight add <word> | remove <word> | list | mute @user|#chan | unmute @user|#chan | cooldown <secs> | clear")
		return
	}

	ud := p.load(ctx.GuildID, ctx.AuthorID)

	switch strings.ToLower(ctx.Args[0]) {
	case "add":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !highlight add <word>")
			return
		}
		word := strings.ToLower(strings.Join(ctx.Args[1:], " "))
		if len(word) < 2 || len(word) > 64 {
			ctx.Reply("Keyword must be 2-64 characters.")
			return
		}
		if contains(ud.Keywords, word) {
			ctx.Reply("Already subscribed to that keyword.")
			return
		}
		if len(ud.Keywords) >= 25 {
			ctx.Reply("You can only have 25 keywords. Remove some first.")
			return
		}
		ud.Keywords = append(ud.Keywords, word)
		p.save(ctx.GuildID, ctx.AuthorID, ud)
		ctx.Reply("Added highlight: `" + word + "`")

	case "remove", "delete", "del":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !highlight remove <word>")
			return
		}
		word := strings.ToLower(strings.Join(ctx.Args[1:], " "))
		before := len(ud.Keywords)
		ud.Keywords = removeValue(ud.Keywords, word)
		if len(ud.Keywords) == before {
			ctx.Reply("You weren't subscribed to that keyword.")
			return
		}
		p.save(ctx.GuildID, ctx.AuthorID, ud)
		ctx.Reply("Removed highlight: `" + word + "`")

	case "list":
		if len(ud.Keywords) == 0 && len(ud.MutedUsers) == 0 && len(ud.MutedChans) == 0 {
			ctx.Reply("You have no highlights configured.")
			return
		}
		var sb strings.Builder
		sb.WriteString("**Your highlights**\n")
		if len(ud.Keywords) > 0 {
			sb.WriteString("Keywords: `")
			sb.WriteString(strings.Join(ud.Keywords, "`, `"))
			sb.WriteString("`\n")
		}
		if len(ud.MutedUsers) > 0 {
			var mentions []string
			for _, u := range ud.MutedUsers {
				mentions = append(mentions, "<@"+u+">")
			}
			sb.WriteString("Muted users: " + strings.Join(mentions, " ") + "\n")
		}
		if len(ud.MutedChans) > 0 {
			var mentions []string
			for _, c := range ud.MutedChans {
				mentions = append(mentions, "<#"+c+">")
			}
			sb.WriteString("Muted channels: " + strings.Join(mentions, " ") + "\n")
		}
		if ud.CooldownSec > 0 {
			sb.WriteString(fmt.Sprintf("Cooldown: %ds\n", ud.CooldownSec))
		}
		ctx.Reply(sb.String())

	case "mute":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !highlight mute @user|#channel")
			return
		}
		arg := ctx.Args[1]
		if uid := discord.ParseUserID(arg); uid != "" {
			if !contains(ud.MutedUsers, uid) {
				ud.MutedUsers = append(ud.MutedUsers, uid)
			}
			p.save(ctx.GuildID, ctx.AuthorID, ud)
			ctx.Reply("Muted highlights from <@" + uid + ">.")
			return
		}
		if cid := discord.ParseChannelMention(arg); cid != "" {
			if !contains(ud.MutedChans, cid) {
				ud.MutedChans = append(ud.MutedChans, cid)
			}
			p.save(ctx.GuildID, ctx.AuthorID, ud)
			ctx.Reply("Muted highlights in <#" + cid + ">.")
			return
		}
		ctx.Reply("Mention a user or channel.")

	case "unmute":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !highlight unmute @user|#channel")
			return
		}
		arg := ctx.Args[1]
		if uid := discord.ParseUserID(arg); uid != "" {
			ud.MutedUsers = removeValue(ud.MutedUsers, uid)
			p.save(ctx.GuildID, ctx.AuthorID, ud)
			ctx.Reply("Unmuted <@" + uid + ">.")
			return
		}
		if cid := discord.ParseChannelMention(arg); cid != "" {
			ud.MutedChans = removeValue(ud.MutedChans, cid)
			p.save(ctx.GuildID, ctx.AuthorID, ud)
			ctx.Reply("Unmuted <#" + cid + ">.")
			return
		}
		ctx.Reply("Mention a user or channel.")

	case "cooldown":
		if len(ctx.Args) < 2 {
			ctx.Reply(fmt.Sprintf("Current cooldown: %ds", ud.CooldownSec))
			return
		}
		n, err := strconv.Atoi(ctx.Args[1])
		if err != nil || n < 0 || n > 3600 {
			ctx.Reply("Cooldown must be 0-3600 seconds.")
			return
		}
		ud.CooldownSec = n
		p.save(ctx.GuildID, ctx.AuthorID, ud)
		ctx.Reply(fmt.Sprintf("Cooldown set to %ds.", n))

	case "clear":
		p.api.DeleteConfig(ctx.GuildID, "u:"+ctx.AuthorID)
		ctx.Reply("All your highlights cleared.")

	default:
		ctx.Reply("Unknown subcommand. Try: add, remove, list, mute, unmute, cooldown, clear")
	}
}

// ---------------------------------------------------------------------------
// Message hook
// ---------------------------------------------------------------------------

func (p *Plugin) onMessage(bot *discord.Bot, msg *discord.Message) {
	if msg.GuildID == "" || msg.Author == nil || msg.Author.Bot || msg.Content == "" {
		return
	}
	// Skip command invocations from our own plugin.
	if strings.HasPrefix(msg.Content, "!highlight") || strings.HasPrefix(msg.Content, "!hl") {
		return
	}

	lower := strings.ToLower(msg.Content)

	// Walk every user with a subscription in this guild.
	for k, v := range p.api.AllConfig(msg.GuildID) {
		if !strings.HasPrefix(k, "u:") {
			continue
		}
		userID := strings.TrimPrefix(k, "u:")
		if userID == msg.Author.ID {
			continue // don't highlight your own messages
		}
		// Don't highlight someone the author already mentioned directly.
		if mentioned(msg, userID) {
			continue
		}

		var ud userData
		if err := json.Unmarshal([]byte(v), &ud); err != nil {
			continue
		}
		if len(ud.Keywords) == 0 {
			continue
		}
		if contains(ud.MutedUsers, msg.Author.ID) {
			continue
		}
		if contains(ud.MutedChans, msg.ChannelID) {
			continue
		}

		// Find the first matching keyword.
		var matched string
		for _, kw := range ud.Keywords {
			if containsWord(lower, kw) {
				matched = kw
				break
			}
		}
		if matched == "" {
			continue
		}

		// Cooldown check.
		cd := time.Duration(ud.CooldownSec) * time.Second
		if cd > 0 {
			key := msg.GuildID + ":" + userID + ":" + matched
			p.mu.Lock()
			last, ok := p.lastFired[key]
			if ok && time.Since(last) < cd {
				p.mu.Unlock()
				continue
			}
			p.lastFired[key] = time.Now()
			p.mu.Unlock()
		}

		p.fire(bot, msg, userID, matched)
	}
}

func (p *Plugin) fire(bot *discord.Bot, msg *discord.Message, userID, keyword string) {
	snippet := msg.Content
	if len(snippet) > 400 {
		snippet = snippet[:400] + "..."
	}
	text := fmt.Sprintf(
		"Highlight **%s** in <#%s>\nFrom <@%s>: %s",
		keyword, msg.ChannelID, msg.Author.ID, snippet,
	)
	if err := bot.Rest.SendDM(userID, text); err != nil {
		p.api.Log("highlight: dm %s: %v", userID, err)
	}
}

// ---------------------------------------------------------------------------
// Persistence helpers
// ---------------------------------------------------------------------------

func (p *Plugin) load(guildID, userID string) *userData {
	raw := p.api.GetConfig(guildID, "u:"+userID)
	if raw == "" {
		return &userData{}
	}
	var ud userData
	if err := json.Unmarshal([]byte(raw), &ud); err != nil {
		return &userData{}
	}
	return &ud
}

func (p *Plugin) save(guildID, userID string, ud *userData) {
	// If the record is empty, drop the key entirely.
	if len(ud.Keywords) == 0 && len(ud.MutedUsers) == 0 &&
		len(ud.MutedChans) == 0 && ud.CooldownSec == 0 {
		p.api.DeleteConfig(guildID, "u:"+userID)
		return
	}
	b, err := json.Marshal(ud)
	if err != nil {
		p.api.Log("highlight: marshal: %v", err)
		return
	}
	p.api.SetConfig(guildID, "u:"+userID, string(b))
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func removeValue(s []string, v string) []string {
	out := s[:0]
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

func mentioned(msg *discord.Message, userID string) bool {
	for _, u := range msg.Mentions {
		if u != nil && u.ID == userID {
			return true
		}
	}
	return false
}

// containsWord returns true if needle appears in haystack as a whole word
// (bounded by non-alphanumeric characters). Both must already be lowercase.
func containsWord(haystack, needle string) bool {
	if needle == "" {
		return false
	}
	// Multi-word phrases: do a plain substring match.
	if strings.ContainsAny(needle, " \t") {
		return strings.Contains(haystack, needle)
	}
	idx := 0
	for {
		i := strings.Index(haystack[idx:], needle)
		if i < 0 {
			return false
		}
		start := idx + i
		end := start + len(needle)
		leftOK := start == 0 || !isWordChar(haystack[start-1])
		rightOK := end == len(haystack) || !isWordChar(haystack[end])
		if leftOK && rightOK {
			return true
		}
		idx = end
	}
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '_'
}
