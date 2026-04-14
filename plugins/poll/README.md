# poll

**Category:** Engagement

Reaction-based polls. The bot posts an embed with your question and options (up to 10), pre-reacts with A/B/C/... emoji, and users vote by clicking. When a timer expires (or you close it manually) the embed is replaced with results and a winner announcement is posted.

## Usage

```
!poll <question> | <option 1> | <option 2> [| <option 3>] ... [for <duration>]
```

Segments are separated with `|`. The first segment is the question, the rest are options. The optional trailing `for <duration>` closes the poll automatically after that interval. Duration format is the usual `10s`/`5m`/`2h`/`1d` used elsewhere.

### Examples

```
!poll Game night? | Yes | No | Maybe
!poll Pizza toppings? | Pepperoni | Mushroom | Pineapple for 10m
!poll Next sprint focus? | Bugs | Features | Docs for 2h
```

## Subcommands

| Command | Description |
|---------|-------------|
| `!poll end <messageID>` | Close a poll early (author or mod) |

## Notes

- Up to 10 options (limited by the letter-emoji set and reaction limits).
- Open-ended polls (no `for` clause) remain active until manually ended.
- Expired polls are processed every 30 seconds by a background ticker.
- The winner line highlights ties ("Winner: **Yes** (4), **No** (4)").

## Required intents / permissions

- `GuildMessageReactions` and `MessageContent`
- Send/embed/add-reaction permissions in the channel
