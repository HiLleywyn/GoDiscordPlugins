// Package embed lets admins compose rich embeds through the command
// interface without hand-writing JSON. Embeds are built up
// field-by-field in a "draft" that's stored per-user, then sent to a
// channel or edited into an existing message.
//
// Commands (Manage Messages):
//
//	!embed new                      Start a new draft (clears any existing)
//	!embed title <text>             Set the title
//	!embed desc <text>              Set the description
//	!embed color <#hex|integer>     Set the color
//	!embed author <text>            Set the author line
//	!embed footer <text>            Set the footer
//	!embed image <url>              Set the image URL
//	!embed thumbnail <url>          Set the thumbnail URL
//	!embed field <name> | <value>   Add a field (pipe-separated)
//	!embed field inline <name> | <value>   Add an inline field
//	!embed fields clear             Clear all fields
//	!embed preview                  Preview the current draft
//	!embed send #chan               Send the draft to a channel
//	!embed edit #chan <msgID>       Edit an existing embed message
//	!embed drop                     Discard the current draft
package embed

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hilleywyn/carlos/pluginapi"
	discord "github.com/hilleywyn/godiscord"
)

func init() { pluginapi.Register(&Plugin{}) }

// draft mirrors discord.Embed enough to serialize cleanly to config.
type draft struct {
	Title       string               `json:"title,omitempty"`
	Description string               `json:"description,omitempty"`
	Color       int                  `json:"color,omitempty"`
	Author      string               `json:"author,omitempty"`
	Footer      string               `json:"footer,omitempty"`
	ImageURL    string               `json:"image_url,omitempty"`
	ThumbnailURL string              `json:"thumbnail_url,omitempty"`
	Fields      []discord.EmbedField `json:"fields,omitempty"`
}

// Plugin is the embed plugin instance.
type Plugin struct {
	api pluginapi.API
}

func (p *Plugin) Manifest() pluginapi.PluginManifest {
	return pluginapi.PluginManifest{
		Name:        "embed",
		Version:     "1.0.0",
		Description: "Interactively build and post rich embeds.",
		Author:      "HiLleywyn",
		Commands:    []string{"embed"},
	}
}

func (p *Plugin) Load(api pluginapi.API) error {
	p.api = api
	api.AddCommand(&discord.Command{
		Name:        "embed",
		Description: "Build and send rich embeds.",
		Usage:       "new | title <t> | desc <t> | color <hex> | author <t> | footer <t> | image <url> | thumbnail <url> | field [inline] <name>|<val> | fields clear | preview | send #chan | edit #chan <msgID> | drop",
		Handler:     p.handleCmd,
	})
	return nil
}

func (p *Plugin) Unload() error {
	p.api.RemoveCommand("embed")
	return nil
}

// ---------------------------------------------------------------------------
// Command handler
// ---------------------------------------------------------------------------

func (p *Plugin) handleCmd(ctx *discord.CommandContext) {
	if ctx.Member == nil || !ctx.Member.HasPermission(discord.PermissionManageMessages) {
		ctx.Reply("You need Manage Messages to use the embed builder.")
		return
	}
	if len(ctx.Args) == 0 {
		ctx.Reply("Usage: !embed new | title | desc | color | author | footer | image | thumbnail | field | fields clear | preview | send #chan | edit #chan <msgID> | drop")
		return
	}

	sub := strings.ToLower(ctx.Args[0])

	// Commands that don't require a loaded draft.
	switch sub {
	case "new":
		p.save(ctx, &draft{})
		ctx.Reply("New empty draft.")
		return
	case "drop":
		p.api.DeleteConfig(ctx.GuildID, "d:"+ctx.AuthorID)
		ctx.Reply("Draft dropped.")
		return
	}

	d := p.load(ctx)
	if d == nil {
		ctx.Reply("You don't have a draft yet. Start one with `!embed new`.")
		return
	}

	switch sub {
	case "title":
		d.Title = strings.Join(ctx.Args[1:], " ")
	case "desc", "description":
		d.Description = strings.Join(ctx.Args[1:], " ")
	case "color", "colour":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !embed color <#hex|integer>")
			return
		}
		c, err := parseColor(ctx.Args[1])
		if err != nil {
			ctx.Reply("Bad color. Use `#5865F2` or a decimal number.")
			return
		}
		d.Color = c
	case "author":
		d.Author = strings.Join(ctx.Args[1:], " ")
	case "footer":
		d.Footer = strings.Join(ctx.Args[1:], " ")
	case "image":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !embed image <url>")
			return
		}
		d.ImageURL = ctx.Args[1]
	case "thumbnail", "thumb":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !embed thumbnail <url>")
			return
		}
		d.ThumbnailURL = ctx.Args[1]
	case "field":
		inline := false
		args := ctx.Args[1:]
		if len(args) > 0 && strings.EqualFold(args[0], "inline") {
			inline = true
			args = args[1:]
		}
		raw := strings.Join(args, " ")
		parts := strings.SplitN(raw, "|", 2)
		if len(parts) != 2 {
			ctx.Reply("Usage: !embed field [inline] <name> | <value>")
			return
		}
		d.Fields = append(d.Fields, discord.EmbedField{
			Name:   strings.TrimSpace(parts[0]),
			Value:  strings.TrimSpace(parts[1]),
			Inline: inline,
		})
	case "fields":
		if len(ctx.Args) >= 2 && strings.EqualFold(ctx.Args[1], "clear") {
			d.Fields = nil
		} else {
			ctx.Reply("Usage: !embed fields clear")
			return
		}
	case "preview":
		ctx.ReplyEmbed(toEmbed(d))
		return
	case "send":
		if len(ctx.Args) < 2 {
			ctx.Reply("Usage: !embed send #chan")
			return
		}
		chID := discord.ParseChannelMention(ctx.Args[1])
		if chID == "" {
			ctx.Reply("Provide a valid channel.")
			return
		}
		if _, err := ctx.Bot.Rest.SendEmbed(chID, toEmbed(d)); err != nil {
			ctx.Reply("Send failed: " + err.Error())
			return
		}
		ctx.Reply("Sent to <#" + chID + ">.")
		return
	case "edit":
		if len(ctx.Args) < 3 {
			ctx.Reply("Usage: !embed edit #chan <msgID>")
			return
		}
		chID := discord.ParseChannelMention(ctx.Args[1])
		if chID == "" {
			ctx.Reply("Provide a valid channel.")
			return
		}
		msgID := ctx.Args[2]
		if err := ctx.Bot.Rest.EditEmbed(chID, msgID, toEmbed(d)); err != nil {
			ctx.Reply("Edit failed: " + err.Error())
			return
		}
		ctx.Reply("Embed edited.")
		return
	default:
		ctx.Reply("Unknown subcommand.")
		return
	}

	p.save(ctx, d)
	ctx.Reply("Draft updated. Use `!embed preview` to see it.")
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

func (p *Plugin) load(ctx *discord.CommandContext) *draft {
	raw := p.api.GetConfig(ctx.GuildID, "d:"+ctx.AuthorID)
	if raw == "" {
		return nil
	}
	var d draft
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return nil
	}
	return &d
}

func (p *Plugin) save(ctx *discord.CommandContext, d *draft) {
	b, err := json.Marshal(d)
	if err != nil {
		p.api.Log("embed: marshal: %v", err)
		return
	}
	p.api.SetConfig(ctx.GuildID, "d:"+ctx.AuthorID, string(b))
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func toEmbed(d *draft) discord.Embed {
	e := discord.Embed{
		Title:       d.Title,
		Description: d.Description,
		Color:       d.Color,
		Fields:      d.Fields,
	}
	if d.Author != "" {
		e.Author = &discord.EmbedAuthor{Name: d.Author}
	}
	if d.Footer != "" {
		e.Footer = &discord.EmbedFooter{Text: d.Footer}
	}
	if d.ImageURL != "" {
		e.Image = &discord.EmbedImage{URL: d.ImageURL}
	}
	if d.ThumbnailURL != "" {
		e.Thumbnail = &discord.EmbedThumbnail{URL: d.ThumbnailURL}
	}
	return e
}

func parseColor(s string) (int, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "#")
	if n, err := strconv.ParseInt(s, 16, 64); err == nil {
		return int(n), nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n, nil
	}
	return 0, fmt.Errorf("bad color")
}
