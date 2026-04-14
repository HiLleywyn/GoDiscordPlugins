# todo

**Category:** Utility

Personal per-user task lists. Each user gets up to 100 open tasks per
guild. Optional due dates. Check them off as you go.

## Commands

| Command | Description |
|---------|-------------|
| `!todo` | List your open tasks |
| `!todo show` | List open + completed |
| `!todo add <text>` | Add a task |
| `!todo add <text> \| <MM-DD>` | Add a task with a due date |
| `!todo done <n>` | Mark task #n done |
| `!todo undone <n>` | Re-open task #n |
| `!todo remove <n>` | Delete task #n |
| `!todo clear` | Delete every task you have |

## Example

```
!todo add Ship the PR | 12-20
!todo add Review @alice's design doc
!todo add Update runbook
!todo
  [ ] #1 Ship the PR (due 12-20)
  [ ] #2 Review @alice's design doc
  [ ] #3 Update runbook
!todo done 2
```

## Notes

- Storage is per-guild per-user, so your work todos and your hobby
  server todos stay separate.
- Task ids never get reused; they always count up. The counter resets
  only when you `!todo clear`.
- For time-based delivery (e.g. "ping me when this is due"), use the
  `reminder` plugin instead.
