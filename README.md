# Symphony Go

A Go implementation of the [Symphony specification](../SPEC.md) — a long-running automation service that reads work from Linear, creates isolated workspaces, and runs AI coding agents for each issue.

Symphony supports two agent backends: **Gemini CLI** and **Claude Code**. You choose which backend to use per workflow.

## Architecture

```
WORKFLOW.md (config + prompt)
      │
      ▼
┌─────────────┐     ┌──────────────┐     ┌────────────────────┐
│ Orchestrator │────▶│ Linear API   │     │ Gemini CLI (ACP)   │
│              │     │ (GraphQL)    │     │ JSON-RPC over stdio│
│ poll/dispatch│     └──────────────┘     └────────────────────┘
│ reconcile   │                           ┌────────────────────┐
│ retry       │──── workspace ── prompt ─▶│ Claude Code (NDJSON)│
└─────────────┘                           │ CLI per turn       │
                                          └────────────────────┘
```

## Agent Backends

Symphony supports two agent backends. Set the `backend` field in your WORKFLOW.md to choose:

| | Gemini CLI | Claude Code |
|---|---|---|
| **Config key** | `backend: gemini` (default) | `backend: claude` |
| **Protocol** | ACP — JSON-RPC 2.0 over stdio (long-running process) | NDJSON stream — one CLI invocation per turn |
| **Session model** | Single process, session persists in-memory | `--resume <session_id>` across invocations, persisted to `.symphony-session-id` |
| **Tool access** | Client-side injection (ACP fs/terminal requests) | MCP servers (configured externally via `.mcp.json` or user config) |
| **Permission handling** | ACP `session/request_permission` auto-approve | `--permission-mode bypassPermissions` flag |
| **Default model** | `gemini-3.1-pro-preview` | `claude-sonnet-4-6` |
| **TTY requirement** | None | Requires pseudo-TTY (`script -q /dev/null` wrapper, handled automatically) |

### Gemini CLI Setup

```bash
npm install -g @google/gemini-cli
gemini auth login
```

For Linear integration, install the MCP extension:
```bash
gemini extensions install @google/mcp-linear
```

### Claude Code Setup

```bash
npm install -g @anthropic-ai/claude-code
```

For Linear integration, add the MCP server globally:
```bash
claude mcp add -s user --transport http linear-server https://mcp.linear.app/mcp
```

This makes the Linear MCP server available in all workspaces. Alternatively, write a `.mcp.json` file in the workspace via the `after_create` hook for a self-contained setup.

## Workflows & Customization

Symphony is driven by `WORKFLOW.md` files. You can create different workflow files for different strategies and backends.

### Included Workflows

| Workflow | File | Backend | Strategy | Best For |
|---|---|---|---|---|
| **Autonomous** | `WORKFLOW.md` | Gemini | Full automation | Rapid prototyping, bug fixes, trusted tasks |
| **Planning First** | `WORKFLOW-PLAN.md` | Gemini | Human-in-the-loop | Complex features, production changes |
| **Planning First (Claude)** | `WORKFLOW-PLAN-CLAUDE.md` | Claude Code | Human-in-the-loop | Same as above, using Claude Code as the agent |

#### Strategy Comparison

| Feature | Autonomous | Planning First |
|---|---|---|
| **Initial Action** | Moves to `In Progress` immediately | Analyzes code and creates a technical plan |
| **Approval Gate** | None — proceeds to implementation | Stops in `Plan Review` for human feedback |
| **Execution** | Continuous turn loop until PR | Only starts coding after move to `Plan Approved` |
| **Risk Profile** | High speed, less oversight | Higher quality, safe for sensitive codebases |

Both strategies work with either backend. The backend determines *which AI agent* runs. The workflow strategy determines *how* it runs (autonomous vs. human-gated).

### Creating Custom Workflows

You can tailor Symphony to any organizational need by creating a new `.md` file with a YAML header.

**Common customization ideas:**
- **Security Auditor**: A workflow that only runs security scans and reports findings to Linear comments.
- **Documentation Agent**: A workflow that focuses on updating `README` and `DOCS` based on code changes.
- **Issue Triage**: A workflow that analyzes new issues, adds labels, and suggests a priority without writing code.

To use a custom workflow:
```bash
./bin/symphony my-custom-workflow.md
```

## Prerequisites

1. **Go 1.25+** — [install](https://go.dev/dl/)

2. **An agent backend** — at least one of:
   - **Gemini CLI** — `npm install -g @google/gemini-cli && gemini auth login`
   - **Claude Code** — `npm install -g @anthropic-ai/claude-code` (requires Anthropic API key or Claude subscription)

3. **Linear project slug** — from your project URL:
   `https://linear.app/yourteam/project/my-project-abc123` → slug is `my-project-abc123`

4. **Linear MCP** (for agent access to Linear) — see [Agent Backends](#agent-backends) for backend-specific setup

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

### Minimal WORKFLOW.md (Gemini)

```yaml
---
tracker:
  kind: linear
  project_slug: my-project-slug
gemini:
  command: "gemini --acp"
  model: gemini-3.1-pro-preview
---
You are working on issue {{ issue.identifier }}: {{ issue.title }}.

{{ issue.description }}
```

### Minimal WORKFLOW.md (Claude Code)

```yaml
---
backend: claude
tracker:
  kind: linear
  project_slug: my-project-slug
claude:
  command: claude
  model: claude-sonnet-4-6
---
You are working on issue {{ issue.identifier }}: {{ issue.title }}.

{{ issue.description }}
```

### Full WORKFLOW.md reference

```yaml
---
backend: gemini                           # "gemini" (default) or "claude"

tracker:
  kind: linear                          # required, only "linear" supported
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
  max_turns: 10                         # default: 20, orchestrator-level turn loop
  max_retry_backoff_ms: 300000          # default: 300000 (5 min)
  max_concurrent_agents_by_state:       # optional per-state caps
    todo: 2
    in progress: 5

# --- Gemini backend config (used when backend: gemini) ---
gemini:
  command: "gemini --acp"               # default
  model: gemini-3.1-pro-preview         # default
  turn_timeout_ms: 3600000              # default: 3600000 (1 hour)
  read_timeout_ms: 5000                 # default: 5000 (5s)
  stall_timeout_ms: 300000              # default: 300000 (5 min), 0 disables

# --- Claude Code backend config (used when backend: claude) ---
claude:
  command: claude                       # default
  model: claude-sonnet-4-6              # default
  permission_mode: bypassPermissions    # default, auto-approves all tool use
  allowed_tools:                        # default: ["Read", "Write", "Edit", "Bash"]
    - Read
    - Write
    - Edit
    - Bash
    - "Bash(git *)"
  max_turns: 25                         # default: 25, per-invocation Claude turns
  turn_timeout_ms: 600000               # default: 600000 (10 min per invocation)
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
| `LINEAR_API_KEY` | (Optional) Used by the backend for polling if `tracker.api_key` is not specified. |

Use `$VAR_NAME` syntax in path fields to reference environment variables.

## Run

```bash
# Default: looks for ./WORKFLOW.md
./bin/symphony

# Explicit workflow path
./bin/symphony WORKFLOW-PLAN.md

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
3. **Worker** — Create workspace → run hooks → launch agent (Gemini or Claude) → multi-turn session
4. **Reconcile** — Each tick, check tracker state for running issues (stop on terminal, update on active)
5. **Retry** — Normal exit → 1s continuation retry; failure → exponential backoff (10s base, capped)
6. **Reload** — `WORKFLOW.md` changes are detected and applied without restart (backend change requires restart)

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

### Agent protocols

**Gemini CLI (ACP)** — Long-running JSON-RPC 2.0 process over stdio:

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

**Claude Code (NDJSON)** — One CLI invocation per turn with streaming JSON output:

```
Symphony ── claude -p "<prompt>" --output-format stream-json ──▶ Claude Code
         ◀── {"type":"system","subtype":"init","session_id":"..."} ──
         ◀── {"type":"assistant","message":{...}} ──  (streaming)
         ◀── {"type":"result","subtype":"success",...} ──
         (process exits)

Next turn:
Symphony ── claude -p "<prompt>" --resume <session_id> ──▶ Claude Code
         ◀── ... ──
```

Session continuity is maintained via `--resume <session_id>`. The session ID is persisted to `.symphony-session-id` in the workspace directory.

Claude Code requires a TTY to produce `stream-json` output. Symphony wraps the process in `script -q /dev/null` to allocate a pseudo-TTY automatically.

## Writing the Prompt (Instructing the Agent)

The prompt in `WORKFLOW.md` is the **only way you control what the agent does**. Symphony is a scheduler — it picks up issues, creates workspaces, and launches the agent. Everything else is determined by your prompt.

The same prompt works with both Gemini and Claude Code. The agents have different capabilities, but both can read/write files, run shell commands, and use MCP tools.

### What to include in your prompt

1. **What it's working on** — use template variables like `{{ issue.identifier }}` and `{{ issue.title }}`
2. **Where the code is** — mention the repo so the agent understands the context
3. **What steps to follow** — be explicit about branching, committing, pushing, PR creation
4. **What tools to use** — both backends support Linear MCP tools and shell commands
5. **What to do when done** — move issue to review, create a PR, etc.

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
2. Use the Linear MCP tools to move the issue to `In Progress`.
3. Create a new branch: `git checkout -b {{ issue.identifier }}`
4. Commit your changes with a clear message referencing the issue.
5. Push the branch: `git push origin {{ issue.identifier }}`
6. Create a pull request:
   `gh pr create --title "{{ issue.identifier }}: {{ issue.title }}" --body "Resolves {{ issue.identifier }}"`
7. Add the PR link to the issue as a comment.
8. Move the issue to `Human Review`.

When you are done, do NOT leave the issue in Todo.
The issue will be picked up again if it stays active.

{% if attempt %}
This is retry attempt {{ attempt }}. Check previous work and continue.
{% endif %}
```

### Key principles

- **Be explicit.** The agent does what you tell it. If you don't say "create a PR", it won't.
- **Use Linear MCP tools.** Both backends discover MCP tools automatically — Gemini via extensions, Claude Code via user-scoped or workspace `.mcp.json` config.
- **Use `gh` CLI for PRs.** If `gh` is installed and authenticated on the machine, the agent can create PRs directly.
- **Use Linear's GitHub integration for linking.** Including `Resolves AIE-123` in a PR body auto-links it in Linear.
- **Handle retries.** Use `{% if attempt %}` to give different instructions on retry.

### Workspace location

Each issue gets its own directory under `workspace.root`:

```
~/symphony_workspaces/
  AIE-7/          ← cloned repo for issue AIE-7
  AIE-8/          ← cloned repo for issue AIE-8
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
├── cmd/symphony/main.go          # CLI entrypoint
├── internal/
│   ├── config/                   # Typed config + defaults + validation
│   ├── workflow/                 # WORKFLOW.md parser + file watcher
│   ├── tracker/                  # Linear GraphQL client
│   ├── orchestrator/             # Poll loop, dispatch, reconcile, retry
│   ├── workspace/                # Directory lifecycle + hooks + safety
│   ├── agent/                    # Backend runners + protocol clients
│   │   ├── runner.go             # AgentLauncher interface + factory
│   │   ├── acp.go                # Gemini ACP client (JSON-RPC over stdio)
│   │   ├── claude_runner.go      # Claude Code runner (NDJSON streaming)
│   │   ├── ndjson.go             # NDJSON line-accumulator parser
│   │   └── events.go             # Event types for orchestrator
│   ├── prompt/                   # Liquid template rendering
│   ├── server/                   # HTTP dashboard + JSON API
│   └── logging/                  # slog JSON setup
├── Makefile
├── go.mod
└── go.sum
```
