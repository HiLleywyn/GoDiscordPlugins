# leveling

Message XP and rank system. Users earn XP for sending messages, level up, and can earn role rewards at configured thresholds.

## Features

- XP per message with configurable cooldown (default 60s) to prevent spam farming
- Per-channel XP multipliers
- Role rewards at configurable level thresholds
- Level-up announcements in a configurable channel
- Rank cards and a server leaderboard

## Commands

| Command | Description |
|---|---|
| `!rank [@user]` | Show rank, level, and XP progress |
| `!leaderboard [page]` | Top 10 users by XP |
| `!xp give @user <amount>` | Give XP to a user |
| `!xp take @user <amount>` | Remove XP from a user |
| `!xp reset @user` | Reset a user's XP to 0 |
| `!xp setcooldown <seconds>` | Set XP grant cooldown (1-3600) |
| `!xp setpermsg <amount>` | Set base XP per message (1-1000) |
| `!xp setannounce #channel\|off` | Set level-up announcement channel |
| `!xp setmultiplier #channel <x>` | Set XP multiplier for a channel |
| `!xp rewardadd <level> @role` | Assign a role reward at a level |
| `!xp rewardremove <level>` | Remove a role reward |
| `!xp rewardlist` | List all role rewards |

## Level formula

Level thresholds increase linearly: level 1 = 100 XP, level 2 = 300 XP total, level 3 = 600 XP total, etc. (`sum of 100 * i for i = 1..level`)

## Install

```
!plugin install leveling
```
