# Symphony Go

A Go implementation of the [Symphony specification](../SPEC.md) — a long-running automation service that reads work from Linear, creates isolated workspaces, and runs a Gemini CLI coding-agent session for each issue.

This implementation uses **Gemini CLI** (via the [Agent Client Protocol](https://agentclientprotocol.com)) instead of Codex as the coding agent.

## Architecture

```
WORKFLOW.md (config + prompt)
      │
      ▼
┌─────────────┐     ┌──────────────┐     ┌────────────────────┐
│ Orchestrator │────▶│ Linear API   │     │ Gemini CLI (ACP)   │
│              │     │ (GraphQL)    │     │ JSON-RPC over stdio│
│ poll/dispatch│     └──────────────┘     └────────────────────┘
│ reconcile   │                                    ▲
│ retry       │──── workspace ──── prompt ─────────┘
└─────────────┘
```

**Key components:**

| Component | Package | Purpose |
|---|---|---|
| Workflow Loader | `internal/workflow` | Parses `WORKFLOW.md` (YAML front matter + Liquid prompt) |
| Config Layer | `internal/config` | Typed config with defaults, `$VAR` resolution, hot reload |
| Linear Client | `internal/tracker` | GraphQL client for fetching/refreshing issues |
| Orchestrator | `internal/orchestrator` | Poll loop, dispatch, concurrency, reconciliation, retry |
| Workspace Manager | `internal/workspace` | Per-issue directory lifecycle + hooks |
| Agent Runner | `internal/agent` | ACP protocol client, multi-turn session management |
| Prompt Renderer | `internal/prompt` | Liquid-compatible strict template rendering |
| HTTP Server | `internal/server` | Optional dashboard + JSON API |

## Prerequisites

1. **Go 1.25+** — [install](https://go.dev/dl/)

2. **Gemini CLI** — installed and authenticated
   ```bash
   npm install -g @google/gemini-cli
   gemini auth login
   ```
   Verify: `gemini --version`

3. **Linear API key** — get one from [Linear Settings → API → Personal API keys](https://linear.app/settings/api)

4. **Linear project slug** — from your project URL:
   `https://linear.app/yourteam/project/my-project-abc123` → slug is `my-project-abc123`

## Build

```bash
cd go/
make build
```

This produces `bin/symphony`.

## Configuration

All configuration lives in a single `WORKFLOW.md` file. The file has two parts:

1. **YAML front matter** (between `---` delimiters) — runtime settings
2. **Markdown body** — the prompt template sent to the agent for each issue

### Minimal WORKFLOW.md

```yaml
---
tracker:
  kind: linear
  api_key: $LINEAR_API_KEY
  project_slug: my-project-slug
gemini:
  command: "gemini --experimental-acp"
  model: gemini-3.1-pro-preview
---
You are working on issue {{ issue.identifier }}: {{ issue.title }}.

{{ issue.description }}
```

### Full WORKFLOW.md reference

```yaml
---
tracker:
  kind: linear                          # required, only "linear" supported
  api_key: $LINEAR_API_KEY              # required, supports $VAR env resolution
  project_slug: my-project              # required for linear
  endpoint: https://api.linear.app/graphql  # default
  active_states:                        # default: ["Todo", "In Progress"]
    - Todo
    - In Progress
  terminal_states:                      # default: ["Closed", "Cancelled", "Canceled", "Duplicate", "Done"]
    - Done
    - Closed
    - Cancelled

polling:
  interval_ms: 30000                    # default: 30000 (30s)

workspace:
  root: ~/symphony_workspaces           # default: <system-temp>/symphony_workspaces
                                        # supports ~ and $VAR

hooks:
  after_create: |                       # runs once when workspace dir is first created
    git clone git@github.com:org/repo.git .
  before_run: |                         # runs before each agent attempt
    git checkout main && git pull
  after_run: |                          # runs after each attempt (failures ignored)
    echo "run complete"
  before_remove: |                      # runs before workspace deletion (failures ignored)
    echo "cleaning up"
  timeout_ms: 60000                     # default: 60000 (60s), applies to all hooks

agent:
  max_concurrent_agents: 5              # default: 10
  max_turns: 10                         # default: 20
  max_retry_backoff_ms: 300000          # default: 300000 (5 min)
  max_concurrent_agents_by_state:       # optional per-state caps
    todo: 2
    in progress: 5

gemini:
  command: "gemini --experimental-acp"  # default
  model: gemini-3.1-pro-preview         # default
  turn_timeout_ms: 3600000              # default: 3600000 (1 hour)
  read_timeout_ms: 5000                 # default: 5000 (5s)
  stall_timeout_ms: 300000              # default: 300000 (5 min), 0 disables

server:
  port: 8080                            # optional, enables HTTP dashboard
---
You are working on issue {{ issue.identifier }}: {{ issue.title }}.

{% if issue.description %}
## Description
{{ issue.description }}
{% endif %}

## Labels
{% for label in issue.labels %}- {{ label }}
{% endfor %}

{% if attempt %}
This is retry attempt {{ attempt }}. Check previous work and continue.
{% endif %}
```

### Template variables

The prompt body is rendered with [Liquid](https://shopify.github.io/liquid/) syntax. Available variables:

| Variable | Type | Description |
|---|---|---|
| `issue.id` | string | Linear internal ID |
| `issue.identifier` | string | Human-readable key (e.g., `MT-123`) |
| `issue.title` | string | Issue title |
| `issue.description` | string | Issue description (empty if none) |
| `issue.state` | string | Current tracker state name |
| `issue.priority` | int or nil | Priority (1=urgent, 4=low, nil=none) |
| `issue.url` | string | Linear issue URL |
| `issue.labels` | list of strings | Lowercase label names |
| `issue.branch_name` | string | Suggested branch name |
| `issue.blocked_by` | list of objects | Blocking issues (each has `.id`, `.identifier`, `.state`) |
| `issue.created_at` | string | ISO-8601 timestamp |
| `issue.updated_at` | string | ISO-8601 timestamp |
| `attempt` | int or nil | nil on first run, integer on retry/continuation |

### Environment variables

| Variable | Purpose |
|---|---|
| `LINEAR_API_KEY` | Linear API authentication token |

Use `$VAR_NAME` syntax in `tracker.api_key` and path fields to reference environment variables.

## Run

```bash
# Default: looks for ./WORKFLOW.md
./bin/symphony

# Explicit workflow path
./bin/symphony /path/to/WORKFLOW.md

# With HTTP dashboard
./bin/symphony --port 8080

# CLI --port overrides server.port in WORKFLOW.md
./bin/symphony --port 9090 /path/to/WORKFLOW.md

# Version
./bin/symphony --version
```

The service runs until stopped with `Ctrl+C` (SIGINT) or `SIGTERM`.

## HTTP Dashboard & API

When a port is configured (via `--port` flag or `server.port` in config):

| Endpoint | Method | Description |
|---|---|---|
| `/` | GET | HTML dashboard (auto-refreshes every 5s) |
| `/api/v1/state` | GET | JSON system state: running sessions, retry queue, token totals |
| `/api/v1/{identifier}` | GET | JSON detail for a specific issue (e.g., `/api/v1/MT-123`) |
| `/api/v1/refresh` | POST | Trigger an immediate poll + reconciliation cycle |

The server binds to `127.0.0.1` (localhost only).

## How it works

1. **Poll** — Every `polling.interval_ms`, fetch candidate issues from Linear
2. **Dispatch** — Sort by priority, check eligibility (concurrency, blockers), launch workers
3. **Worker** — Create workspace → run hooks → start Gemini CLI via ACP → multi-turn session
4. **Reconcile** — Each tick, check tracker state for running issues (stop on terminal, update on active)
5. **Retry** — Normal exit → 1s continuation retry; failure → exponential backoff (10s base, capped)
6. **Reload** — `WORKFLOW.md` changes are detected and applied without restart

### Workspace lifecycle

```
<workspace.root>/
  MT-123/          ← one directory per issue
  MT-124/
  MT-125/
```

- Created on first dispatch, reused on retries
- `after_create` hook runs once (e.g., git clone)
- `before_run` hook runs before each attempt (e.g., git pull)
- Cleaned up when issue enters a terminal state

### Agent protocol

Symphony communicates with Gemini CLI using the [Agent Client Protocol (ACP)](https://agentclientprotocol.com) — JSON-RPC 2.0 over stdio:

```
Symphony ──initialize──▶ Gemini CLI
         ◀──result──────
         ──session/new──▶
         ◀──sessionId───
         ──session/prompt──▶
         ◀──session/update── (streaming)
         ◀──prompt result──
```

Permission requests (`session/request_permission`) are auto-approved (high-trust mode).

## Writing the Prompt (Instructing the Agent)

The prompt in `WORKFLOW.md` is the **only way you control what Gemini does**. Symphony is a scheduler — it picks up issues, creates workspaces, and launches the agent. Everything else is determined by your prompt.

### What to include in your prompt

The prompt should tell Gemini:

1. **What it's working on** — use template variables like `{{ issue.identifier }}` and `{{ issue.title }}`
2. **Where the code is** — mention the repo so Gemini understands the context
3. **What steps to follow** — be explicit about branching, committing, pushing, PR creation
4. **What tools to use** — Gemini can run shell commands, so tell it to use `git`, `gh`, `npm`, etc.
5. **What to do when done** — create a PR, link to Linear, etc.

### Example: Full workflow prompt

```markdown
You are working on issue {{ issue.identifier }}: {{ issue.title }}.
You are working in a checkout of https://github.com/your-org/your-repo.

{% if issue.description %}
## Description
{{ issue.description }}
{% endif %}

## Instructions
1. Make the code changes needed to resolve this issue.
2. Create a new branch: `git checkout -b {{ issue.identifier }}`
3. Commit your changes with a clear message referencing the issue.
4. Push the branch: `git push origin {{ issue.identifier }}`
5. Create a pull request:
   `gh pr create --title "{{ issue.identifier }}: {{ issue.title }}" --body "Resolves {{ issue.identifier }}"`
6. Print the PR URL so it is visible in the logs.

When you are done, do NOT leave the issue in Todo.
The issue will be picked up again if it stays active.

{% if attempt %}
This is retry attempt {{ attempt }}. Check previous work and continue.
{% endif %}
```

### Key principles

- **Be explicit.** Gemini does what you tell it. If you don't say "create a PR", it won't.
- **Symphony doesn't write to Linear.** It only reads issues. Ticket state transitions (Todo → Done) must be done by Gemini via tools, by you manually, or via Linear's GitHub integration.
- **Use `gh` CLI for PRs.** If `gh` is installed and authenticated on the machine, Gemini can create PRs directly.
- **Use Linear's GitHub integration for linking.** Including `Resolves AIE-123` in a PR body auto-links it in Linear when the [GitHub integration](https://linear.app/settings/integrations/github) is enabled.
- **Handle retries.** Use `{% if attempt %}` to give different instructions on retry (e.g., "check what was already done").
- **Hooks handle repo setup.** The prompt assumes the workspace already has code — use `after_create` to clone and `before_run` to pull.

### Prompt + hooks work together

| Concern | Where to configure |
|---|---|
| Clone the repo | `hooks.after_create` |
| Pull latest before each run | `hooks.before_run` |
| What code changes to make | Prompt body |
| Branching strategy | Prompt body |
| PR creation | Prompt body (using `gh`) |
| Linking PR to Linear | Prompt body (`Resolves {{ issue.identifier }}`) |
| Moving ticket state | Prompt body or manual in Linear |
| Cleanup after terminal | Automatic (Symphony deletes workspace) |

### Workspace location

Each issue gets its own directory under `workspace.root`:

```
~/symphony_workspaces/
  AIE-7/          ← cloned repo for issue AIE-7
  AIE-8/          ← cloned repo for issue AIE-8
```

You can inspect what the agent is doing at any time:

```bash
cd ~/symphony_workspaces/AIE-7
git log --oneline -5    # see what was committed
git diff                # see uncommitted changes
ls -la                  # see workspace contents
```

## Development

```bash
# Run tests
make test

# Build
make build

# Run directly
make run
```

### Project structure

```
go/
├── cmd/symphony/main.go          # CLI entrypoint
├── internal/
│   ├── config/                   # Typed config + defaults + validation
│   ├── workflow/                 # WORKFLOW.md parser + file watcher
│   ├── tracker/                  # Linear GraphQL client
│   ├── orchestrator/             # Poll loop, dispatch, reconcile, retry
│   ├── workspace/                # Directory lifecycle + hooks + safety
│   ├── agent/                    # ACP client + runner + events
│   ├── prompt/                   # Liquid template rendering
│   ├── server/                   # HTTP dashboard + JSON API
│   └── logging/                  # slog JSON setup
├── Makefile
├── go.mod
└── go.sum
```
