# giveaway

Timed giveaways where users react to enter and winners are picked randomly.

## Commands

| Command | Description |
|---------|-------------|
| `!giveaway start <duration> <winners> <prize>` | Start a giveaway |
| `!giveaway end <messageID>` | End a giveaway early and pick winners |
| `!giveaway reroll <messageID>` | Reroll winners for a completed giveaway |

### Duration format

Use a number followed by `s`, `m`, `h`, or `d`:

- `30s` - 30 seconds
- `10m` - 10 minutes
- `24h` - 24 hours
- `7d` - 7 days (max: 30d)

### Examples

```
!giveaway start 24h 1 Discord Nitro
!giveaway start 7d 3 Steam gift card
!giveaway end 1234567890123456789
!giveaway reroll 1234567890123456789
```

## How it works

1. `!giveaway start` posts an embed with a party-popper reaction (the bot adds it automatically).
2. Users react with the party-popper to enter.
3. When the timer expires (checked every minute) or `!giveaway end` is used, the bot fetches all reactors, excludes bots, and picks N random winners.
4. The results are posted in the same channel mentioning the winners.

## Config keys

| Key | Value |
|-----|-------|
| `giveaway:<messageID>` | JSON blob with giveaway state |

## Required intents

- `GuildMessageReactions` - to let users react
- `GuildMessages` - to post results
