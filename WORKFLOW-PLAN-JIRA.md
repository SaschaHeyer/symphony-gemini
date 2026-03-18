---
backend: claude
tracker:
  kind: jira
  endpoint: $JIRA_ENDPOINT
  api_key: $JIRA_API_TOKEN
  email: $JIRA_EMAIL
  project_slug: PROJ
  active_states:
    - To Do
    - Plan Approved
    - In Progress
    - Rework
  terminal_states:
    - Done
    - Cancelled
claude:
  command: claude
  model: claude-sonnet-4-6
  permission_mode: bypassPermissions
  allowed_tools:
    - Read
    - Write
    - Edit
    - Bash
  max_turns: 25
  turn_timeout_ms: 600000
agent:
  max_concurrent_agents: 3
  max_turns: 15
workspace:
  root: ~/symphony_workspaces
hooks:
  after_create: |
    git clone https://github.com/your-org/your-repo.git .
    npm install
  timeout_ms: 180000
  before_run: |
    git fetch origin main 2>/dev/null
    CURRENT_BRANCH=$(git branch --show-current 2>/dev/null)
    if [ "$CURRENT_BRANCH" = "main" ] || [ -z "$CURRENT_BRANCH" ]; then
      git stash --include-untracked 2>/dev/null
      git checkout main 2>/dev/null
      git pull 2>/dev/null
    fi
server:
  port: 8080
---
You are working on issue {{ issue.identifier }}: {{ issue.title }}.
You are working in a checkout of https://github.com/your-org/your-repo.

## Status Routing (Planning First)

Determine the current ticket state and follow the matching flow:

- **To Do**:
  1. Analyze the codebase and the issue.
  2. Create a detailed technical plan.
  3. Create/Update the `## Workpad` comment with this plan.
  4. Move the issue to `Plan Review` (this state is for human approval).
  5. STOP. Do not implement yet.

- **Plan Approved**:
  1. Move the issue to `In Progress`.
  2. Continue to implementation steps below.

- **In Progress**:
  1. Read the approved plan from the `## Workpad`.
  2. Implement the changes.
  3. Move the issue to `Human Review` when done.

- **Rework**:
  1. Re-read the full issue and all comments.
  2. Close the existing PR.
  3. Create a fresh branch from `origin/main`.
  4. Restart from the planning phase (Move to `To Do`).

- **Plan Review**: Do nothing. Wait for human approval.

## Jira Integration

Always use the Jira MCP tools for ALL Jira operations. Do NOT use curl for Jira API calls.

- To transition issue status: use the Jira MCP to update the issue status (e.g., "Plan Review", "In Progress").
- To manage comments: use the Jira MCP to list, create, and update comments on the issue.
- To read issue details: use the Jira MCP to fetch the full issue including description and existing comments.

## Workpad Structure

Always maintain a single `## Workpad` comment on the Jira issue.

IMPORTANT: Before creating a workpad, ALWAYS search for an existing one first.
Use the Jira MCP to list all comments on the issue, then check if any start with "## Workpad".
If one exists, update it instead of creating a duplicate.

### For Planning (in `To Do`):
Your plan must include:
- **Reproduction**: How you verified the current behavior.
- **Proposed Changes**: Files to modify and specific logic changes.
- **Verification Plan**: How you will prove the fix works (tests/scripts).

### For Execution (in `In Progress`):
Check off items as you complete them.

## Execution Steps

### Phase 1: Planning (To Do)
1. **Analyze**: Explore the code. Identify the relevant components.
2. **Draft Plan**: Write a step-by-step implementation plan.
3. **Submit**: Update the Workpad and move the ticket to `Plan Review`.
4. **Exit**: End your turn.

### Phase 2: Implementation (In Progress)
1. **Sync**: Ensure your workspace is clean and up to date.
2. **Implement**: Follow the plan in the Workpad.
3. **Validate**: Run tests or a reproduction script.
4. **PR**: Create a branch `{{ issue.identifier }}`, push, and `gh pr create`.
5. **Complete**: Move to `Human Review`.

## Guardrails
- NEVER start implementation while the ticket is in `To Do`.
- NEVER ask a human for follow-up actions during the turn.
- If blocked by missing secrets or permissions, note it in the Workpad and stay in `Plan Review`.
