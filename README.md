# gpb — GitHub Project Board reporter for Bale Messenger

Sends GitHub ProjectsV2 board updates to a Bale group chat via GitHub Actions.

## How it works

Two workflows watch your project board:

| Workflow | Trigger | What it does |
|---|---|---|
| `daily-report` | `schedule` (9am weekdays) or manual | Fetches the full board and sends a summary grouped by assignee with urgency flags |
| `project-events` | `schedule` (every 5 min) or manual | Polls the board via GraphQL, diffs against cached state, and sends one-line notifications for changes |

The CLI has three commands:

| Command | Purpose |
|---|---|
| `report` | Full board summary with assignee grouping and urgency flags |
| `poll` | Compare current board against cached state, notify on diffs |
| `notify` | Single-item notification (used by manual dispatch) |

## Setup for your organization

### Quick start

1. **Copy the workflow files** from [`scaffold/`](scaffold/) into your repo's `.github/workflows/` directory:
   - `daily-report.yaml`
   - `project-events.yaml`
   - `project-events-manual.yaml` (optional)

2. **Set secrets and variables** via CLI (or use repo Settings → Secrets and variables → Actions):

   ```bash
   gh secret set BALE_TOKEN --repo your-org/gpb
   # paste your Bale bot token from BotFather

   gh secret set GH_PAT --repo your-org/gpb
   # paste a GitHub PAT with repo and project scopes

   gh variable set PROJECT_ID --repo your-org/gpb
   # paste your ProjectsV2 node ID (PVT_...)

   gh variable set CHAT_ID --repo your-org/gpb
   # paste your Bale group chat ID

   gh variable set URGENCY_DAYS --repo your-org/gpb
   # optional, defaults to 2
   ```

3. **Enable write permissions** (required for the polling state file):
   ```bash
   gh api repos/your-org/gpb/actions/permissions/workflow \
     -X PUT -f default_workflow_permissions=write
   ```

4. **Push to the default branch** — scheduled workflows only run from the default branch.

The first scheduled run may take up to an hour to start. You can trigger a manual run from the Actions tab immediately.

### PAT scopes

The polling workflow commits a state file back to your repo, so the PAT needs:

| Scope | Why |
|---|---|
| `repo` | Push state file commits back to the repo |
| `project` | Read project items via GraphQL API |

If you only need the daily report (no polling), `project` scope alone is sufficient and you can use the auto-provided `GITHUB_TOKEN` (same org only).

## Notification examples

```
🆕 alice added Fix login crash → In Progress (assigned: bob)
✏️ bob updated Review PR — due tomorrow → Done
↔️ alice moved Clean up config → In Review
🗑️ bob removed Old task from the board
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

## CLI reference

All flags also read from env vars (`GITHUB_TOKEN`, `PROJECT_ID`, `BALE_TOKEN`, `CHAT_ID`, etc.).

### `gpb report`

```bash
go run ./cmd/gpb report \
  --github-token=ghp_... \
  --project-id=PVT_... \
  --bale-token=... \
  --chat-id=g-... \
  --urgency-days=2
```

| Flag | Env | Default | Description |
|---|---|---|---|
| `--github-token` | `GITHUB_TOKEN` | — | GitHub PAT |
| `--project-id` | `PROJECT_ID` | — | ProjectsV2 node ID |
| `--bale-token` | `BALE_TOKEN` | — | Bale bot token |
| `--chat-id` | `CHAT_ID` | — | Bale group chat ID |
| `--urgency-days` | `URGENCY_DAYS` | `2` | Urgent threshold in days |

### `gpb poll`

```bash
go run ./cmd/gpb poll \
  --github-token=ghp_... \
  --project-id=PVT_... \
  --bale-token=... \
  --chat-id=g-... \
  --state-file=.gpb-state.json
```

| Flag | Env | Default | Description |
|---|---|---|---|
| `--github-token` | `GITHUB_TOKEN` | — | GitHub PAT |
| `--project-id` | `PROJECT_ID` | — | ProjectsV2 node ID |
| `--bale-token` | `BALE_TOKEN` | — | Bale bot token |
| `--chat-id` | `CHAT_ID` | — | Bale group chat ID |
| `--state-file` | — | `.gpb-state.json` | Path to state cache |

### `gpb notify`

```bash
go run ./cmd/gpb notify \
  --github-token=ghp_... \
  --item-id=PVTI_... \
  --event=created \
  --sender=alice \
  --bale-token=... \
  --chat-id=g-...
```

| Flag | Env | Default | Description |
|---|---|---|---|
| `--github-token` | `GITHUB_TOKEN` | — | GitHub PAT |
| `--item-id` | `ITEM_ID` | — | Project item node ID (`PVTI_...`) |
| `--event` | `EVENT` | — | `created`, `edited`, `moved`, `deleted` |
| `--sender` | `SENDER` | — | GitHub username |
| `--bale-token` | `BALE_TOKEN` | — | Bale bot token |
| `--chat-id` | `CHAT_ID` | — | Bale group chat ID |

## Develop

```bash
go vet ./...
go test -race ./...
go build ./...
```

Zero external dependencies — stdlib only.
