// Package todo gives each user a personal task list per guild. Tasks
// are checked off, listed, and optionally scheduled with a due date.
//
// Commands:
//
//	!todo add <text>            Add a task
//	!todo add <text> | <MM-DD>  Add a task with a due date
//	!todo                       List your open tasks
//	!todo done <n>              Mark task #n done
//	!todo undone <n>            Unmark task #n
//	!todo remove <n>            Delete a task
//	!todo clear                 Delete all of your tasks
//	!todo show                  Show done + open
package todo

import (
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

// taskList is the per-user record.
type taskList struct {
	Counter int     `json:"counter"`
	Tasks   []*task `json:"tasks"`
}

type task struct {
	ID      int       `json:"id"`
	Text    string    `json:"text"`
	Done    bool      `json:"done"`
	AddedAt time.Time `json:"added_at"`
	Due     string    `json:"due,omitempty"` // MM-DD or YYYY-MM-DD
}

// Plugin is the todo plugin instance.
type Plugin struct {
	api pluginapi.API
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "todo",
		Version:     "1.0.0",
		Description: "Personal task lists per user.",
		Author:      "HiLleywyn",
		Commands:    []string{"todo"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "todo",
		Description: "Personal task list.",
		Usage:       "[add <text> [| due] | done <n> | undone <n> | remove <n> | clear | show]",
		Handler:     p.handleCmd,
	})
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("todo")
	return nil
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if len(ctx.Args) == 0 {
		p.cmdList(ctx, false)
		return
	}

	switch strings.ToLower(ctx.Args[0]) {
	case "add":
		p.cmdAdd(ctx)
	case "done", "do", "check":
		p.cmdMark(ctx, true)
	case "undone", "undo", "uncheck":
		p.cmdMark(ctx, false)
	case "remove", "rm", "del", "delete":
		p.cmdRemove(ctx)
	case "clear":
		p.api.DeleteConfig(ctx.GuildID, "u:"+ctx.AuthorID)
		ctx.Reply("All your tasks cleared.")
	case "show":
		p.cmdList(ctx, true)
	case "list":
		p.cmdList(ctx, false)
	default:
		ctx.Reply("Usage: !todo add <text> | done <n> | undone <n> | remove <n> | clear | show")
	}
}

func (p *Plugin) cmdAdd(ctx *discord.CommandContext) {
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !todo add <text> [| <MM-DD>]")
		return
	}
	raw := strings.Join(ctx.Args[1:], " ")
	text := raw
	due := ""
	if idx := strings.LastIndex(raw, "|"); idx >= 0 {
		text = strings.TrimSpace(raw[:idx])
		due = strings.TrimSpace(raw[idx+1:])
	}
	if len(text) == 0 || len(text) > 500 {
		ctx.Reply("Task text must be 1-500 chars.")
		return
	}

	tl := p.load(ctx)
	tl.Counter++
	tl.Tasks = append(tl.Tasks, &task{
		ID:      tl.Counter,
		Text:    text,
		AddedAt: time.Now(),
		Due:     due,
	})
	if len(tl.Tasks) > 100 {
		ctx.Reply("You already have 100 tasks. Clear some out first.")
		return
	}
	p.save(ctx, tl)
	msg := fmt.Sprintf("Added task #%d: %s", tl.Counter, text)
	if due != "" {
		msg += " (due " + due + ")"
	}
	ctx.Reply(msg)
}

func (p *Plugin) cmdMark(ctx *discord.CommandContext, done bool) {
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !todo done <n>")
		return
	}
	n, err := strconv.Atoi(ctx.Args[1])
	if err != nil {
		ctx.Reply("Id must be a number.")
		return
	}
	tl := p.load(ctx)
	for _, t := range tl.Tasks {
		if t.ID == n {
			t.Done = done
			p.save(ctx, tl)
			if done {
				ctx.Reply("Marked #" + ctx.Args[1] + " done.")
			} else {
				ctx.Reply("Re-opened #" + ctx.Args[1] + ".")
			}
			return
		}
	}
	ctx.Reply("No task with that id.")
}

func (p *Plugin) cmdRemove(ctx *discord.CommandContext) {
	if len(ctx.Args) < 2 {
		ctx.Reply("Usage: !todo remove <n>")
		return
	}
	n, err := strconv.Atoi(ctx.Args[1])
	if err != nil {
		ctx.Reply("Id must be a number.")
		return
	}
	tl := p.load(ctx)
	out := tl.Tasks[:0]
	removed := false
	for _, t := range tl.Tasks {
		if t.ID == n {
			removed = true
			continue
		}
		out = append(out, t)
	}
	if !removed {
		ctx.Reply("No task with that id.")
		return
	}
	tl.Tasks = out
	p.save(ctx, tl)
	ctx.Reply("Task #" + ctx.Args[1] + " removed.")
}

func (p *Plugin) cmdList(ctx *discord.CommandContext, includeDone bool) {
	tl := p.load(ctx)
	if len(tl.Tasks) == 0 {
		ctx.Reply("Your todo list is empty. Try `!todo add <text>`.")
		return
	}

	var open []*task
	var done []*task
	for _, t := range tl.Tasks {
		if t.Done {
			done = append(done, t)
		} else {
			open = append(open, t)
		}
	}
	sort.Slice(open, func(i, j int) bool { return open[i].ID < open[j].ID })
	sort.Slice(done, func(i, j int) bool { return done[i].ID < done[j].ID })

	var lines []string
	if len(open) == 0 {
		lines = append(lines, "_No open tasks._")
	}
	for _, t := range open {
		line := fmt.Sprintf("`[ ]` **#%d** %s", t.ID, t.Text)
		if t.Due != "" {
			line += " (due " + t.Due + ")"
		}
		lines = append(lines, line)
	}
	if includeDone && len(done) > 0 {
		lines = append(lines, "")
		for _, t := range done {
			lines = append(lines, fmt.Sprintf("`[x]` **#%d** ~~%s~~", t.ID, t.Text))
		}
	}

	ctx.ReplyEmbed(discord.Embed{
		Title:       "Your todo list",
		Description: strings.Join(lines, "\n"),
		Color:       0x5865F2,
	})
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

func (p *Plugin) load(ctx *discord.CommandContext) *taskList {
	raw := p.api.GetConfig(ctx.GuildID, "u:"+ctx.AuthorID)
	if raw == "" {
		return &taskList{}
	}
	var tl taskList
	if err := json.Unmarshal([]byte(raw), &tl); err != nil {
		return &taskList{}
	}
	return &tl
}

func (p *Plugin) save(ctx *discord.CommandContext, tl *taskList) {
	if len(tl.Tasks) == 0 {
		p.api.DeleteConfig(ctx.GuildID, "u:"+ctx.AuthorID)
		return
	}
	b, err := json.Marshal(tl)
	if err != nil {
		p.api.Log("todo: marshal: %v", err)
		return
	}
	p.api.SetConfig(ctx.GuildID, "u:"+ctx.AuthorID, string(b))
}
