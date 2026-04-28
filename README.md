# gpb — GitHub Project Board reporter for Bale Messenger

Sends GitHub ProjectsV2 board updates to a Bale group chat via GitHub Actions.

## How it works

Two workflows run the CLI:

| Workflow | Trigger | What it does |
|---|---|---|
| `daily-report` | `schedule` (9am UTC) or manual | Fetches the full board and sends a summary grouped by assignee with urgency flags |
| `project-events` | `project_v2_item` (created, edited, deleted) or manual | Sends a one-line notification about the changed item |

## Setup

### 1. Add secrets and variables to the repo

| Name | Type | Value |
|---|---|---|
| `BALE_TOKEN` | Secret | Bot token from BotFather |
| `PROJECT_ID` | Variable | GitHub ProjectsV2 node ID (`PVT_...`) |
| `CHAT_ID` | Variable | Bale group chat ID |
| `URGENCY_DAYS` | Variable | Days threshold for urgent flag (default: `2`) |

`GITHUB_TOKEN` is auto-provided — no setup needed.

### 2. Enable workflow permissions

In repo Settings → Actions → General → Workflow permissions, enable **Read and write permissions**.

## CLI

```bash
# Full board report
go run ./cmd/gpb report \
  --github-token=ghp_... \
  --project-id=PVT_... \
  --bale-token=... \
  --chat-id=g-...

# Single-item notification
go run ./cmd/gpb notify \
  --github-token=ghp_... \
  --item-id=PVTI_... \
  --event=created \
  --sender=alice \
  --bale-token=... \
  --chat-id=g-...
```

All flags also read from env vars (`GITHUB_TOKEN`, `PROJECT_ID`, `BALE_TOKEN`, `CHAT_ID`, `ITEM_ID`, `EVENT`, `SENDER`).

## Notification examples

```
🆕 alice added Fix login crash → In Progress (assigned: bob)
✏️ bob updated Review PR — due tomorrow → Done
🗑️ alice removed Old task from the board
```

## Daily report format

```
📋 *Board Report — Apr 28, 2026*

*@alice* (3 task(s), 🔴 1 urgent)
• 🔴 Fix login crash — due tomorrow
• Add rate limiter — due in 5 days
• Update docs — no due date

*@bob* (2 task(s))
• Review PR — due in 3 days
• DB migration — no due date

*Unassigned* (1 task(s))
• Sprint retro notes — no due date
```

## Develop

```bash
go vet ./...
go test -race ./...
go build ./...
```

Zero external dependencies — stdlib only.
