# GPB — GitHub Project Board Reporter for Bale Messenger

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      single binary                           │
│                                                              │
│  ┌──────────┐  ┌───────────┐  ┌──────────┐  ┌───────────┐  │
│  │ scheduler │  │  command   │  │ reporter  │  │  tenant    │  │
│  │ (cron)    │  │  handler   │  │ (format)  │  │  registry  │  │
│  └────┬─────┘  └─────┬─────┘  └────┬─────┘  └─────┬─────┘  │
│       │              │              │              │         │
│  ┌────┴──────────────┴──────────────┴──────────────┴────┐    │
│  │                   orchestration                       │    │
│  └────┬─────────────────────────────────────────────────┘    │
│       │                                                      │
│  ┌────┴─────┐  ┌────────────┐  ┌─────────────────┐          │
│  │  github   │  │   bale     │  │  authz           │          │
│  │  client   │  │   client   │  │  middleware      │          │
│  └──────────┘  └────────────┘  └─────────────────┘          │
└─────────────────────────────────────────────────────────────┘
```

**Data flow:**
1. Cron fires → orchestration layer looks up all configured tenants
2. For each tenant → GitHub client fetches project items (GraphQL) → reporter formats markdown → Bale client posts to group
3. Bale webhook arrives → authz middleware verifies group → command handler dispatches → same fetch→format→post pipeline (synchronous response + async follow-up if needed)

**Concurrency model:**
- One goroutine runs the cron scheduler (robfig/cron)
- One goroutine per tenant report generation, with a timeout context (prevents one slow tenant from blocking others)
- HTTP server in main goroutine for webhook ingestion
- Command-triggered reports run in their own goroutine so the webhook returns `200 OK` immediately

---

## Module Breakdown

### 1. `internal/github/` — GitHub Client

**Responsibility:** Query a GitHub ProjectsV2 board via GraphQL.

**Key GraphQL query:**

```graphql
query($projectId: ID!, $cursor: String) {
  node(id: $projectId) {
    ... on ProjectV2 {
      title
      items(first: 100, after: $cursor) {
        pageInfo { hasNextPage, endCursor }
        nodes {
          id
          type
          content {
            ... on Issue {
              title, number, url, state
              assignees(first: 5) { nodes { login } }
            }
            ... on PullRequest {
              title, number, url, state
              assignees(first: 5) { nodes { login } }
            }
            ... on DraftIssue {
              title
              assignees(first: 5) { nodes { login } }
            }
          }
          fieldValues(first: 50) {
            nodes {
              ... on ProjectV2ItemFieldDateValue { date field { ...name } }
              ... on ProjectV2ItemFieldSingleSelectValue { name field { ...name } }
            }
          }
        }
      }
    }
  }
}
```

**Public interface:**

```go
type Client interface {
    // GetProjectItems returns all open items with assignees and due dates.
    GetProjectItems(ctx context.Context, projectID string) ([]ProjectItem, error)
}

type ProjectItem struct {
    Title     string
    URL       string
    Type      string   // "Issue", "PullRequest", "DraftIssue"
    State     string
    Assignees []string
    DueDate   *time.Time
    Status    string   // column/status name
}
```

**Implementation notes:**
- Use `github.com/shurcooL/githubv4` (typed GraphQL client) or `github.com/cli/go-gh` for authenticated transport
- Paginate via `pageInfo.hasNextPage` / `endCursor`
- Retry on 502/503 with exponential backoff (see Error Handling section)
- Cache the field-name→ID mapping on first call (ProjectV2 field IDs are stable)

### 2. `internal/bale/` — Bale Client

**Responsibility:** Send messages to a group and parse incoming commands.

**Public interface:**

```go
type Client interface {
    SendMessage(ctx context.Context, chatID string, text string) error
}

type WebhookHandler interface {
    HandleUpdate(ctx context.Context, update json.RawMessage) error
}
```

**API calls used:**
- `POST /bot{token}/sendMessage` — `chat_id`, `text`, `parse_mode=Markdown`
- `POST /bot{token}/setWebhook` — register webhook URL at startup
- Incoming updates arrive as JSON to the webhook endpoint

**Group chat handling:**
- Bale group chat IDs start with `g-` or are negative integers (verify exact prefix from Bale docs)
- The bot should be added to the group by an admin; the bot can call `getChat` to verify membership
- Messages from the group include `chat.id` and `from.id`; the handler checks `chat.id` against the allowlist

### 3. `internal/scheduler/` — Scheduler

**Responsibility:** Run the daily report at a configured time.

Uses `github.com/robfig/cron/v3` with a cron expression derived from config.

```go
type Scheduler struct {
    cron *cron.Cron
    fn   func(context.Context) error
}

func New(spec string, fn func(context.Context) error) (*Scheduler, error)
func (s *Scheduler) Start(ctx context.Context) error
func (s *Scheduler) Stop()  // graceful: waits for in-flight job
```

- Cron spec: `"0 9 * * *"` (9 AM daily). Configurable.
- Each tenant gets its own cron entry; a tenant add/remove operation updates the cron table at runtime.
- The job function spawns a goroutine per tenant with a 60-second timeout.

### 4. `internal/reporter/` — Reporter

**Responsibility:** Turn `[]github.ProjectItem` into a human-readable markdown string.

**Output format:**

```markdown
📋 *Project Board Report — {date}*

*@alice* (2 tasks, 🔴 1 urgent)
• [Fix login crash](#url) — due tomorrow
• Update docs — due in 5 days

*@bob* (1 task)
• Review PR #42 — no due date

*Unassigned* (3 items)
• Migrate DB — due Apr 30 🔴
• ...
```

**Logic:**
- Group items by assignee; items with no assignee go into "Unassigned"
- Sort each group: items with due dates first, then by proximity; urgent items flagged
- Urgency window: configurable (default 2 days). Items past due are marked separately.
- Respect Bale's message length limit (~4096 chars); split into multiple messages if needed.

### 5. `internal/command/` — Command Handler

**Responsibility:** Parse `/status` and any future slash commands from incoming Bale messages.

```go
type Router struct {
    handlers map[string]Handler
}

type Handler func(ctx context.Context, cmd Command) (string, error)

type Command struct {
    Name   string   // e.g., "status"
    Args   []string
    ChatID string
    UserID string
}
```

- Strip bot mention suffix if present (`/status@gpb_bot` → `/status`)
- `/status` triggers an immediate report for the tenant matching the chat
- Unknown commands reply with "Unknown command. Try /status."
- Handler runs report generation in a goroutine so the webhook acknowledges quickly

### 6. `internal/authz/` — Authorization Middleware

**Responsibility:** All three authz dimensions.

```
                    ┌─────────────────┐
                    │   authz pkg     │
                    │                 │
  GitHub token ────►│ github_auth.go  │──► graphql transport
  Bale token ──────►│ bale_auth.go    │──► http client
  group allowlist ─►│ group_guard.go  │──► webhook middleware
  tenant isolation─►│ tenant_registry │──► orchestrator
                    └─────────────────┘
```

**GitHub auth:**
- Token passed as `Authorization: Bearer ghp_...` header
- Scopes needed: `read:project`, `read:org` (for org projects) or `read:user` (for user projects)
- Fine-grained PATs: repository read on the target repo(s) + organization projects read

**Bale auth:**
- Bot token from BotFather, passed in URL path: `/bot{token}/...`
- Webhook endpoint protected by a shared secret in a custom header; the incoming request must include `X-Bot-Token: <secret>` (or verify via Bale's own signature mechanism if available)

**Group verification:**
- On webhook receive: extract `chat.id`, check against a tenant-configured allowlist of group IDs
- Unknown groups get a logged warning and no response (prevents cross-tenant leaks)
- Config maps `group_id` → `tenant_name`

**Secret storage:**
| Environment | Method |
|---|---|
| Local dev | `.env` file (gitignored) or `export` in shell |
| CI | GitHub Actions secrets |
| Production | Environment variables injected by orchestrator (Docker `--env-file`, Kubernetes Secrets, or HashiCorp Vault) |

Secrets are loaded once at startup into a typed config struct; never logged.

**Tenant isolation:**

```go
type Tenant struct {
    Name          string
    GitHubToken   string
    GitHubProjectID string
    BaleToken     string
    GroupChatID   string
    CronSpec      string
    UrgencyDays   int
}
```

- A `TenantRegistry` holds a `map[string]*Tenant` keyed by group chat ID
- The orchestrator iterates tenants; each tenant's GitHub and Bale calls use that tenant's tokens
- A tenant can never see another tenant's project data or post to another tenant's group
- Config is loaded at startup and optionally hot-reloaded on SIGHUP

---

## API Surface

### Inbound (this binary exposes)

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/webhook` | Receive Bale updates |
| `GET`  | `/health` | Liveness probe |
| `GET`  | `/ready` | Readiness probe (GitHub + Bale reachable) |

### Outbound (this binary calls)

| Target | Protocol | Endpoint |
|---|---|---|
| GitHub | GraphQL | `https://api.github.com/graphql` |
| Bale | REST | `https://tapi.bale.ai/bot{token}/sendMessage` |
| Bale | REST | `https://tapi.bale.ai/bot{token}/setWebhook` |

---

## Configuration

Single YAML file (or env vars), path set via `GPB_CONFIG_PATH` (default `./config.yaml`):

```yaml
server:
  listen: ":8080"
  webhook_path: "/webhook"
  webhook_secret: "${WEBHOOK_SECRET}"  # env-var interpolation

tenants:
  - name: "team-alpha"
    github_token: "${GITHUB_TOKEN_ALPHA}"
    github_project_id: "PVT_kwDOA..."
    bale_token: "${BALE_TOKEN_ALPHA}"
    group_chat_id: "g-123456"
    cron_spec: "0 9 * * *"
    urgency_days: 2

  - name: "team-beta"
    github_token: "${GITHUB_TOKEN_BETA}"
    github_project_id: "PVT_kwDOB..."
    bale_token: "${BALE_TOKEN_BETA}"
    group_chat_id: "g-789012"
    cron_spec: "0 10 * * *"
    urgency_days: 3

logging:
  level: "info"      # debug, info, warn, error
  format: "json"     # json or text
```

Env-var interpolation: any config value matching `${VAR}` is replaced at load time from the environment. This keeps secrets out of the config file.

---

## Implementation Steps (in order)

### Phase 1: Scaffolding (day 1)
1. `go mod init github.com/<org>/gpb`
2. Directory layout: `cmd/gpb/main.go`, `internal/{github,bale,scheduler,reporter,command,authz,config}`
3. Config loading with env-var interpolation (`internal/config/`)
4. Structured logging via `log/slog`
5. Health/readiness HTTP server
6. Dockerfile (multi-stage, scratch final image)

### Phase 2: Core Integration (days 2–4)
1. GitHub client: authenticate, query ProjectsV2, parse response, paginate
2. Bale client: sendMessage, setWebhook, parse incoming update JSON
3. Reporter: format markdown string from `[]ProjectItem`, handle edge cases (no assignee, no due date, empty board)
4. Manual integration test: run the binary, trigger a report via a hardcoded `main()` call

### Phase 3: Scheduling & Commands (days 5–6)
1. Integrate `robfig/cron/v3`; spawn per-tenant cron entries
2. Webhook handler: parse message text, extract command, dispatch
3. `/status` command: trigger synchronous fetch→format→post
4. Graceful shutdown: stop cron, drain in-flight HTTP requests, exit

### Phase 4: AuthZ Hardening (day 7)
1. Group allowlist check in webhook handler
2. Webhook secret verification
3. Tenant registry with token-per-tenant
4. Ensure tokens/group IDs never appear in logs or error messages

### Phase 5: Testing & Hardening (days 8–10)
1. Unit tests for reporter, config, command router
2. Integration tests with `httptest` for webhook handler
3. Mock GitHub and Bale servers for table-driven tests
4. Error handling review: network failures, API errors, empty data, malformed config
5. CI/CD pipeline (GitHub Actions): lint, test, build, push Docker image

---

## Error Handling & Retry Strategy

| Scenario | Behaviour |
|---|---|
| GitHub API timeout/5xx | Retry 3× with exponential backoff (1s, 2s, 4s). If all fail, post "⚠️ Could not fetch project data. Will retry tomorrow." to the group |
| GitHub returns empty project | Post "📋 No items on the board today." |
| Item has no assignee | Grouped under "Unassigned" |
| Item has no due date | Listed without urgency flag |
| Bale sendMessage fails | Retry 3×. If all fail, log error at ERROR level; no user-visible output (avoids spam) |
| Cron job overlaps (previous still running) | `robfig/cron` `SkipIfStillRunning` chain; log a WARN |
| Malformed webhook request | Log DEBUG, return 400 |
| Unauthorized group | Log WARN with group ID, return 200 (don't leak existence of the bot) |
| Config parse error at startup | Log FATAL and exit with code 1 |

---

## Testing Strategy

### Unit tests (`*_test.go` alongside source)
- **Reporter:** table-driven. Input: `[]ProjectItem`. Output: expected markdown string. Cover: empty list, no assignees, all urgent, mixed states, message splitting at char limit.
- **Config:** parse valid YAML, invalid YAML, missing fields, env-var interpolation edge cases.
- **Command router:** valid `/status`, unknown command, `/status@botname`, empty message.
- **Authz group guard:** allowed group, denied group, malformed update.

### Integration tests
- **Webhook handler:** use `httptest.Server` to simulate Bale sending an update; verify the handler calls the Bale client with correct parameters.
- **GitHub client:** spin up an `httptest.Server` that returns canned GraphQL responses; verify pagination and field parsing.
- **Scheduler:** use a short cron interval (every 2 seconds), verify the function fires at least twice, then stop.

### Mocking
- Interfaces for `github.Client`, `bale.Client` allow swapping real implementations for mocks
- Use `gomock` or hand-rolled test doubles
- CI runs `go test -race ./...`

---

## Deployment

### Docker

```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /gpb ./cmd/gpb

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /gpb /gpb
ENTRYPOINT ["/gpb"]
```

### Running

```bash
docker run -d \
  --name gpb \
  -p 8080:8080 \
  --env-file .env \
  -v $(pwd)/config.yaml:/config.yaml \
  gpb:latest
```

### Cron in production
- The binary's internal cron is sufficient for single-instance deployments
- For HA: run a single instance (the cron path is not leader-elected); if redundancy is needed, use an external scheduler (Kubernetes CronJob that hits a `/trigger` endpoint) and run multiple stateless webhook-handler replicas

---

## CI/CD (GitHub Actions)

```yaml
name: ci
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.24" }
      - run: go vet ./...
      - run: go test -race -coverprofile=coverage.out ./...
      - run: go build -o /dev/null ./cmd/gpb
```

Optional: on tag push, build Docker image and push to registry.

---

## Potential Pitfalls & Mitigations

| Pitfall | Mitigation |
|---|---|
| GitHub ProjectsV2 field IDs change if the project schema is edited | Detect unknown field IDs and log a clear error; the admin must update config |
| Bale message length limit (~4096 chars) | Reporter splits output into multiple `sendMessage` calls when close to limit |
| Large project boards (200+ items) | Pagination handles this; set a per-tenant max-items config to avoid runaway reports |
| Bot removed from group | `sendMessage` returns a specific error code; log and disable that tenant until re-enabled |
| Clock skew between cron container and real time | Always run container with TZ set; use `TZ=Asia/Tehran` or UTC consistently |
| GitHub rate limiting (5000 points/hr for PAT) | GraphQL is efficient (1 point per query); caching field-name→ID mappings reduces extra calls |
| Draft issues have no URL | Reporter omits the link for draft issues; shows title only |
| Multiple assignees on one item | Item appears under each assignee's section (deduplication not needed — it accurately reflects the board) |

---

## Day-in-the-Life Example

**09:00** — Cron fires. The orchestrator iterates over tenant "team-alpha".

1. GitHub client fetches project `PVT_kwDOA...` items
2. Items returned: 3 assigned to @alice, 2 to @bob, 1 unassigned
3. Reporter formats:

```
📋 *Board Report — Apr 28, 2026*

*@alice* (3 tasks, 🔴 1 urgent)
• Fix login crash — due Apr 29 🔴
• Add rate limiter — due May 3
• Update onboarding docs — no due date

*@bob* (2 tasks)
• Review PR #142 — due Apr 29 🔴
• Database migration — due May 10

*Unassigned* (1 item)
• Sprint retro notes — no due date
```

4. Bale client posts to `g-123456`. Members see it.

**14:30** — @alice types `/status` in the group.

1. Webhook received. Authz checks: group `g-123456` is in allowlist ✓. Webhook secret matches ✓.
2. Command router dispatches to status handler.
3. Handler spawns a goroutine for the fetch→format→post pipeline, returns immediately.
4. Bale client posts the same-formatted report to the group.

**Error scenario (GitHub down):**
1. GitHub API returns 503 after 3 retries.
2. Error sentry logs: `level=ERROR msg="github fetch failed" tenant=team-alpha err="GET /graphql: 503 Service Unavailable"`
3. Bale client posts: `⚠️ Could not fetch project data. Will retry at the next scheduled time.`

---

## Directory Layout

```
gpb/
├── cmd/
│   └── gpb/
│       └── main.go              # entrypoint: load config, wire deps, start server
├── internal/
│   ├── authz/
│   │   ├── group_guard.go
│   │   ├── group_guard_test.go
│   │   └── tenant.go
│   ├── bale/
│   │   ├── client.go            # sendMessage, setWebhook
│   │   └── client_test.go
│   ├── command/
│   │   ├── router.go
│   │   └── router_test.go
│   ├── config/
│   │   ├── config.go
│   │   └── config_test.go
│   ├── github/
│   │   ├── client.go            # ProjectsV2 GraphQL queries
│   │   ├── client_test.go
│   │   └── models.go
│   ├── orchestrator/
│   │   └── orchestrator.go      # wires scheduler + github + reporter + bale per tenant
│   ├── reporter/
│   │   ├── reporter.go
│   │   └── reporter_test.go
│   └── scheduler/
│       ├── scheduler.go
│       └── scheduler_test.go
├── config.yaml
├── Dockerfile
├── go.mod
├── go.sum
└── .github/
    └── workflows/
        └── ci.yaml
```
