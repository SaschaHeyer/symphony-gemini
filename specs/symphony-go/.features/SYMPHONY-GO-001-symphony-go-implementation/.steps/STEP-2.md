# Step 2: Linear GraphQL Client

**Covers:** FR-3 (Issue Tracker Client)
**Package:** `internal/tracker`

---

## 1. Tasks

### 1.1 Domain Types

- [ ] `internal/tracker/issue.go`:
  - `Issue` struct per TECH.md Section 2.1
  - `Blocker` struct

### 1.2 GraphQL Queries

- [ ] `internal/tracker/queries.go`:
  - Constant strings for each GraphQL query:
    - `candidateIssuesQuery`: fetch issues filtered by project `slugId` and state names, paginated (page size 50). Fields: id, identifier, title, description, priority, state{name}, branchName, url, labels{nodes{name}}, relations{nodes{type, relatedIssue{id, identifier, state{name}}}}, createdAt, updatedAt. Pagination via `after` cursor.
    - `issueStatesByIDsQuery`: fetch issues by `id: {in: $ids}` with variable type `[ID!]`. Fields: id, identifier, state{name}.
    - `issuesByStatesQuery`: fetch issues by project `slugId` and state names (for terminal cleanup). Same fields as candidate query.

### 1.3 HTTP Client

- [ ] `internal/tracker/client.go`:
  - `LinearClient` struct: endpoint, apiKey, `*http.Client` (30s timeout)
  - `NewLinearClient(endpoint, apiKey string) *LinearClient`
  - `FetchCandidateIssues(ctx, projectSlug, activeStates) ([]Issue, error)`:
    - POST GraphQL to endpoint with `Authorization: Bearer <apiKey>`
    - Paginate: follow `endCursor` while `hasNextPage` is true
    - Return all issues across pages in order
  - `FetchIssueStatesByIDs(ctx, ids) ([]Issue, error)`:
    - Short-circuit: empty ids → return empty
    - Single query, no pagination needed for state refresh
  - `FetchIssuesByStates(ctx, projectSlug, states) ([]Issue, error)`:
    - Short-circuit: empty states → return empty
    - Paginate like candidate fetch

### 1.4 Normalization

- [ ] `internal/tracker/normalize.go`:
  - `normalizeIssue(raw linearIssueNode) Issue`:
    - `labels` → lowercase
    - `blocked_by` → extract from `relations` where `relation.type == "blocks"` (inverse: the related issue blocks this one)
    - `priority` → `*int`, non-integer becomes nil
    - `created_at`, `updated_at` → parse ISO-8601, nil on failure
    - `state` → `raw.state.name`

### 1.5 Error Types

- [ ] `internal/tracker/errors.go`:
  - `TrackerError` struct with `Kind` and `Cause`
  - Error kind constants: `ErrUnsupportedTrackerKind`, `ErrMissingAPIKey`, `ErrMissingProjectSlug`, `ErrLinearAPIRequest`, `ErrLinearAPIStatus`, `ErrLinearGraphQLErrors`, `ErrLinearUnknownPayload`, `ErrLinearMissingEndCursor`

### 1.6 Interface

- [ ] `internal/tracker/client.go`:
  - Define `TrackerClient` interface:
    ```go
    type TrackerClient interface {
        FetchCandidateIssues(ctx context.Context, projectSlug string, activeStates []string) ([]Issue, error)
        FetchIssueStatesByIDs(ctx context.Context, ids []string) ([]Issue, error)
        FetchIssuesByStates(ctx context.Context, projectSlug string, states []string) ([]Issue, error)
    }
    ```
  - `LinearClient` implements `TrackerClient`

### 1.7 Tests

- [ ] `internal/tracker/client_test.go`:
  - Use `httptest.NewServer` to mock Linear GraphQL responses
  - Test: candidate fetch constructs correct query with `slugId` filter
  - Test: state refresh uses `[ID!]` variable type
  - Test: empty ID list returns empty without HTTP call
  - Test: empty state list returns empty without HTTP call
  - Test: pagination follows `endCursor` across 2+ pages
  - Test: HTTP error → `ErrLinearAPIRequest`
  - Test: non-200 → `ErrLinearAPIStatus`
  - Test: GraphQL errors in response → `ErrLinearGraphQLErrors`
  - Test: missing `endCursor` with `hasNextPage=true` → `ErrLinearMissingEndCursor`

- [ ] `internal/tracker/normalize_test.go`:
  - Test: labels lowercased
  - Test: blockers from inverse `blocks` relations
  - Test: non-integer priority → nil
  - Test: ISO-8601 timestamps parsed
  - Test: missing optional fields → nil

---

## 2. Dependencies

No new dependencies. Uses `net/http` and `encoding/json` from stdlib.

---

## 3. Definition of Done

- [ ] `go test ./internal/tracker/...` — all pass
- [ ] Client correctly paginates mock Linear responses
- [ ] Normalization produces correct `Issue` structs from raw Linear payloads
- [ ] All error types mapped and tested
- [ ] `TrackerClient` interface defined for mock injection
