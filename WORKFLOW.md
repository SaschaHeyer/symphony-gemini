---
tracker:
  kind: linear
  api_key: $LINEAR_API_KEY
  project_slug: sympho-2a2c014d1423
  active_states:
    - Todo
    - In Progress
    - Rework
gemini:
  command: "gemini --acp --model gemini-3-flash-preview"
  model: gemini-3-flash-preview
  read_timeout_ms: 30000
agent:
  max_concurrent_agents: 3
  max_turns: 10
workspace:
  root: ~/symphony_workspaces
hooks:
  after_create: |
    git clone https://github.com/SaschaHeyer/nfc-cards.git .
    npm install
  timeout_ms: 180000
  before_run: |
    git stash --include-untracked 2>/dev/null; git checkout main && git pull
server:
  port: 8080
---
You are working on issue {{ issue.identifier }}: {{ issue.title }}.
You are working in a checkout of https://github.com/SaschaHeyer/nfc-cards.

This is an unattended orchestration session. Never ask a human to perform follow-up actions.
Only stop early for a true blocker (missing auth/permissions/secrets). If blocked, record it in a comment on the Linear issue.
Work only in the provided repository copy. Do not touch any other path.

{% if issue.description %}
## Description
{{ issue.description }}
{% endif %}

{% if attempt %}
## Continuation Context
- This is retry attempt #{{ attempt }} because the ticket is still in an active state.
- Resume from the current workspace state instead of restarting from scratch.
- Do not repeat already-completed investigation or validation unless needed for new code changes.
- Do not end the turn while the issue remains in an active state unless you are blocked.
{% endif %}

## Status Routing

Determine the current ticket state and follow the matching flow:

- **Todo**: Move the issue to `In Progress` immediately, then start work.
- **In Progress**: Continue implementation from current state.
- **Rework**: Re-read the full issue and all comments. Close the existing PR. Create a fresh branch from `origin/main`. Start over.
- **Human Review**: Do nothing. Wait for human approval.
- **Done**: Do nothing. Shut down.

To transition issue states, use curl with the Linear GraphQL API:
```bash
curl -s -X POST https://api.linear.app/graphql \
  -H "Content-Type: application/json" \
  -H "Authorization: $LINEAR_API_KEY" \
  -d '{"query":"mutation { issueUpdate(id: \"{{ issue.id }}\", input: { stateId: \"STATE_ID\" }) { success } }"}'
```

To find state IDs, query the team's workflow states first:
```bash
curl -s -X POST https://api.linear.app/graphql \
  -H "Content-Type: application/json" \
  -H "Authorization: $LINEAR_API_KEY" \
  -d '{"query":"{ workflowStates { nodes { id name } } }"}' | jq '.data.workflowStates.nodes'
```

## Workpad Comment

Create a single persistent comment on the Linear issue as your progress tracker. Use this exact structure:

```markdown
## Workpad

### Plan
- [ ] 1. Analyze codebase and reproduce/understand the issue
- [ ] 2. Implement changes
- [ ] 3. Validate changes
- [ ] 4. Commit, push, create PR
- [ ] 5. Move issue to Human Review

### Acceptance Criteria
- [ ] (derived from issue description)

### Validation
- [ ] targeted proof of the change

### Notes
- (progress updates)
```

To create a comment on the issue:
```bash
curl -s -X POST https://api.linear.app/graphql \
  -H "Content-Type: application/json" \
  -H "Authorization: $LINEAR_API_KEY" \
  -d '{"query":"mutation { commentCreate(input: { issueId: \"{{ issue.id }}\", body: \"COMMENT_BODY\" }) { success } }"}'
```

Update the same comment as you make progress. To update a comment, use `commentUpdate` with the comment ID.

## Execution Steps

1. **Reproduce first**: Before implementing, confirm the current behavior or understand the request. Record what you find in the workpad.
2. **Implement**: Make the code changes needed. Keep changes focused and minimal.
3. **Validate**: Run a targeted proof that demonstrates the change works. If there are tests, run them.
4. **Branch and commit**:
   - `git checkout -b {{ issue.identifier }}`
   - Commit with a clear message referencing the issue.
   - `git push origin {{ issue.identifier }}`
5. **Create a pull request**:
   - `gh pr create --title "{{ issue.identifier }}: {{ issue.title }}" --body "Resolves {{ issue.identifier }}"`
   - Print the PR URL.
6. **PR feedback sweep** (if PR already existed):
   - Read all PR comments: `gh pr view --comments`
   - Read inline review comments: `gh api repos/SaschaHeyer/nfc-cards/pulls/$(gh pr view --json number -q .number)/comments`
   - Address every actionable comment with code changes or explicit pushback.
   - Push updates and repeat until no outstanding comments remain.
7. **Move issue to Human Review** once PR is created and validated.

## Guardrails

- If the branch PR is already closed or merged, create a fresh branch from `origin/main` and restart.
- When meaningful out-of-scope improvements are discovered, note them in the workpad but do NOT expand scope.
- Do not edit the Linear issue body/description. Use the workpad comment only.
- Final message must report completed actions and blockers only. Do not include "next steps for user".
- Keep all work inside the workspace directory.

## Completion Bar (before moving to Human Review)

- All plan items checked off in workpad comment.
- Acceptance criteria met.
- Validation proof recorded.
- PR created, pushed, and linked.
- No outstanding PR feedback.
- Branch is up to date with main.
