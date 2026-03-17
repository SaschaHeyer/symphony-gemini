# TECH SPEC: Symphony Go — Architecture & Data Models

**Status:** Draft v1
**Date:** 2026-03-17
**Feature ID:** SYMPHONY-GO-001
**Depends on:** FUNCTIONAL.md v1

---

## 1. Project Structure

```
symphony/go/
├── cmd/
│   └── symphony/
│       └── main.go                 # CLI entrypoint
├── internal/
│   ├── config/
│   │   ├── config.go               # Typed config struct + getters
│   │   ├── defaults.go             # Default values
│   │   ├── resolve.go              # $VAR expansion, ~ expansion, coercion
│   │   └── validate.go             # Dispatch preflight validation
│   ├── workflow/
│   │   ├── loader.go               # WORKFLOW.md parser (YAML front matter + prompt body)
│   │   └── watcher.go              # fsnotify file watcher + reload
│   ├── tracker/
│   │   ├── client.go               # Linear GraphQL HTTP client
│   │   ├── queries.go              # GraphQL query strings
│   │   ├── normalize.go            # Issue normalization from Linear payloads
│   │   └── errors.go               # Typed tracker errors
│   ├── orchestrator/
│   │   ├── orchestrator.go         # Main state machine + poll loop
│   │   ├── state.go                # Runtime state struct
│   │   ├── dispatch.go             # Candidate selection + dispatch logic
│   │   ├── reconcile.go            # Stall detection + tracker state refresh
│   │   ├── retry.go                # Retry queue + backoff calculation
│   │   └── metrics.go              # Token/runtime accounting
│   ├── workspace/
│   │   ├── manager.go              # Create/reuse/clean workspaces
│   │   ├── hooks.go                # Hook execution (sh -lc, timeout)
│   │   └── safety.go               # Path sanitization + containment checks
│   ├── agent/
│   │   ├── runner.go               # Agent runner: workspace + prompt + ACP client orchestration
│   │   ├── acp.go                  # ACP protocol client (JSON-RPC 2.0 over stdio)
│   │   ├── messages.go             # ACP message types (request/response/notification structs)
│   │   ├── session.go              # Session + turn lifecycle management
│   │   └── events.go               # Event emission to orchestrator
│   ├── prompt/
│   │   ├── render.go               # Liquid-compatible strict template rendering
│   │   └── context.go              # Template variable assembly (issue + attempt)
│   ├── server/
│   │   ├── server.go               # HTTP server (optional extension)
│   │   ├── api.go                  # /api/v1/* handlers
│   │   └── dashboard.go            # / handler (HTML dashboard)
│   └── logging/
│       └── logging.go              # slog configuration + structured context helpers
├── go.mod
├── go.sum
└── Makefile
```

---

## 2. Data Models

### 2.1 Issue (Domain Model)

```go
// internal/tracker/issue.go

type Issue struct {
    ID          string     `json:"id"`
    Identifier  string     `json:"identifier"`
    Title       string     `json:"title"`
    Description *string    `json:"description"`
    Priority    *int       `json:"priority"`
    State       string     `json:"state"`
    BranchName  *string    `json:"branch_name"`
    URL         *string    `json:"url"`
    Labels      []string   `json:"labels"`
    BlockedBy   []Blocker  `json:"blocked_by"`
    CreatedAt   *time.Time `json:"created_at"`
    UpdatedAt   *time.Time `json:"updated_at"`
}

type Blocker struct {
    ID         *string `json:"id"`
    Identifier *string `json:"identifier"`
    State      *string `json:"state"`
}
```

### 2.2 Workflow Definition

```go
// internal/workflow/loader.go

type WorkflowDefinition struct {
    Config         map[string]any `json:"config"`
    PromptTemplate string         `json:"prompt_template"`
}
```

### 2.3 Service Config (Typed)

```go
// internal/config/config.go

type Config struct {
    Tracker   TrackerConfig   `yaml:"tracker"`
    Polling   PollingConfig   `yaml:"polling"`
    Workspace WorkspaceConfig `yaml:"workspace"`
    Hooks     HooksConfig     `yaml:"hooks"`
    Agent     AgentConfig     `yaml:"agent"`
    Gemini    GeminiConfig    `yaml:"gemini"`
    Server    ServerConfig    `yaml:"server"`
}

type TrackerConfig struct {
    Kind           string   `yaml:"kind"`
    Endpoint       string   `yaml:"endpoint"`
    APIKey         string   `yaml:"api_key"`
    ProjectSlug    string   `yaml:"project_slug"`
    ActiveStates   []string `yaml:"active_states"`
    TerminalStates []string `yaml:"terminal_states"`
}

type PollingConfig struct {
    IntervalMs int `yaml:"interval_ms"`
}

type WorkspaceConfig struct {
    Root string `yaml:"root"`
}

type HooksConfig struct {
    AfterCreate  *string `yaml:"after_create"`
    BeforeRun    *string `yaml:"before_run"`
    AfterRun     *string `yaml:"after_run"`
    BeforeRemove *string `yaml:"before_remove"`
    TimeoutMs    int     `yaml:"timeout_ms"`
}

type AgentConfig struct {
    MaxConcurrentAgents        int            `yaml:"max_concurrent_agents"`
    MaxTurns                   int            `yaml:"max_turns"`
    MaxRetryBackoffMs          int            `yaml:"max_retry_backoff_ms"`
    MaxConcurrentAgentsByState map[string]int `yaml:"max_concurrent_agents_by_state"`
}

type GeminiConfig struct {
    Command          string `yaml:"command"`
    Model            string `yaml:"model"`
    TurnTimeoutMs    int    `yaml:"turn_timeout_ms"`
    ReadTimeoutMs    int    `yaml:"read_timeout_ms"`
    StallTimeoutMs   int    `yaml:"stall_timeout_ms"`
}

type ServerConfig struct {
    Port *int `yaml:"port"`
}
```

**Defaults:**

```go
// internal/config/defaults.go

var Defaults = Config{
    Tracker: TrackerConfig{
        Endpoint:       "https://api.linear.app/graphql",
        ActiveStates:   []string{"Todo", "In Progress"},
        TerminalStates: []string{"Closed", "Cancelled", "Canceled", "Duplicate", "Done"},
    },
    Polling: PollingConfig{
        IntervalMs: 30000,
    },
    Workspace: WorkspaceConfig{
        Root: filepath.Join(os.TempDir(), "symphony_workspaces"),
    },
    Hooks: HooksConfig{
        TimeoutMs: 60000,
    },
    Agent: AgentConfig{
        MaxConcurrentAgents:        10,
        MaxTurns:                   20,
        MaxRetryBackoffMs:          300000,
        MaxConcurrentAgentsByState: map[string]int{},
    },
    Gemini: GeminiConfig{
        Command:        "gemini --experimental-acp",
        Model:          "gemini-3.1-pro-preview",
        TurnTimeoutMs:  3600000,
        ReadTimeoutMs:  5000,
        StallTimeoutMs: 300000,
    },
}
```

### 2.4 Workspace

```go
// internal/workspace/manager.go

type Workspace struct {
    Path         string
    WorkspaceKey string
    CreatedNow   bool
}
```

### 2.5 Orchestrator Runtime State

```go
// internal/orchestrator/state.go

type State struct {
    mu sync.Mutex

    PollIntervalMs      int
    MaxConcurrentAgents int

    Running       map[string]*RunningEntry  // issue_id -> entry
    Claimed       map[string]struct{}        // issue_id set
    RetryAttempts map[string]*RetryEntry     // issue_id -> entry
    Completed     map[string]struct{}        // issue_id set (bookkeeping)

    GeminiTotals  TokenTotals
    RateLimits    *RateLimitSnapshot
}

type RunningEntry struct {
    IssueID        string
    Identifier     string
    Issue          *Issue
    WorkerCancel   context.CancelFunc
    SessionID      string
    GeminiPID      string
    LastMessage     string
    LastEvent       string
    LastEventAt     *time.Time
    StartedAt       time.Time
    TurnCount       int
    RetryAttempt    int

    // Token accounting
    InputTokens              int64
    OutputTokens             int64
    TotalTokens              int64
    LastReportedInputTokens  int64
    LastReportedOutputTokens int64
    LastReportedTotalTokens  int64
}

type RetryEntry struct {
    IssueID     string
    Identifier  string
    Attempt     int
    DueAt       time.Time
    TimerCancel context.CancelFunc
    Error       string
}

type TokenTotals struct {
    InputTokens    int64
    OutputTokens   int64
    TotalTokens    int64
    SecondsRunning float64
}

type RateLimitSnapshot struct {
    // Implementation-defined; store latest rate limit data from agent events
    Raw map[string]any
}
```

---

## 3. ACP Protocol Messages

### 3.1 Request/Response Types

```go
// internal/agent/messages.go

// JSON-RPC 2.0 base types
type JSONRPCRequest struct {
    JSONRPC string `json:"jsonrpc"`
    ID      int    `json:"id,omitempty"`
    Method  string `json:"method"`
    Params  any    `json:"params,omitempty"`
}

type JSONRPCResponse struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      int             `json:"id,omitempty"`
    Result  json.RawMessage `json:"result,omitempty"`
    Error   *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCNotification struct {
    JSONRPC string `json:"jsonrpc"`
    Method  string `json:"method"`
    Params  any    `json:"params,omitempty"`
}

type JSONRPCError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
    Data    any    `json:"data,omitempty"`
}

// ACP-specific message params

type InitializeParams struct {
    ProtocolVersion    int                `json:"protocolVersion"`
    ClientInfo         ClientInfo         `json:"clientInfo"`
    ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
}

type ClientInfo struct {
    Name    string `json:"name"`
    Version string `json:"version"`
}

type ClientCapabilities struct {
    FS       *FSCapabilities `json:"fs,omitempty"`
    Terminal bool            `json:"terminal,omitempty"`
}

type FSCapabilities struct {
    ReadTextFile  bool `json:"readTextFile,omitempty"`
    WriteTextFile bool `json:"writeTextFile,omitempty"`
}

type InitializeResult struct {
    ProtocolVersion    int              `json:"protocolVersion"`
    AgentInfo          AgentInfo        `json:"agentInfo"`
    AgentCapabilities  AgentCapabilities `json:"agentCapabilities"`
}

type AgentInfo struct {
    Name    string `json:"name"`
    Version string `json:"version"`
}

type AgentCapabilities struct {
    LoadSession        bool                `json:"loadSession,omitempty"`
    PromptCapabilities *PromptCapabilities `json:"promptCapabilities,omitempty"`
}

type PromptCapabilities struct {
    Image bool `json:"image,omitempty"`
    Audio bool `json:"audio,omitempty"`
}

type SessionNewParams struct {
    CWD        string      `json:"cwd"`
    MCPServers []MCPServer `json:"mcpServers"`
}

type MCPServer struct {
    Transport string            `json:"transport"`
    Command   string            `json:"command,omitempty"`
    Args      []string          `json:"args,omitempty"`
    Env       map[string]string `json:"env,omitempty"`
}

type SessionNewResult struct {
    SessionID string `json:"sessionId"`
}

type SessionPromptParams struct {
    SessionID string         `json:"sessionId"`
    Prompt    []ContentBlock `json:"prompt"`
}

type ContentBlock struct {
    Type string `json:"type"`
    Text string `json:"text,omitempty"`
}

type SessionPromptResult struct {
    StopReason string `json:"stopReason"`
}

// Agent → Client notifications

type SessionUpdateParams struct {
    SessionID string        `json:"sessionId"`
    Update    SessionUpdate `json:"update"`
}

type SessionUpdate struct {
    SessionUpdate string          `json:"sessionUpdate"`  // "message_chunk", "tool_call", "tool_call_update", "plan", etc.
    ToolCallID    string          `json:"toolCallId,omitempty"`
    Title         string          `json:"title,omitempty"`
    Kind          string          `json:"kind,omitempty"`
    Status        string          `json:"status,omitempty"`
    Content       json.RawMessage `json:"content,omitempty"`
    // Message chunk fields
    Role          string          `json:"role,omitempty"`    // "agent", "thought"
    Text          string          `json:"text,omitempty"`
}

// Agent → Client requests (permissions)

type RequestPermissionParams struct {
    SessionID string             `json:"sessionId"`
    ToolCall  ToolCallRef        `json:"toolCall"`
    Options   []PermissionOption `json:"options"`
}

type ToolCallRef struct {
    ToolCallID string `json:"toolCallId"`
}

type PermissionOption struct {
    OptionID string `json:"optionId"`
    Name     string `json:"name"`
    Kind     string `json:"kind"` // "allow_once", "allow_always", "reject_once", etc.
}

type RequestPermissionResult struct {
    Outcome PermissionOutcome `json:"outcome"`
}

type PermissionOutcome struct {
    Outcome  string `json:"outcome"`  // "selected" or "cancelled"
    OptionID string `json:"optionId,omitempty"`
}
```

### 3.2 ACP Message Flow (Sequence)

```
Symphony (Client)                        Gemini CLI (Agent)
     │                                         │
     │──── initialize ────────────────────────>│
     │<─── initialize result ─────────────────│
     │                                         │
     │──── session/new {cwd} ─────────────────>│
     │<─── session/new result {sessionId} ────│
     │                                         │
     │──── session/prompt {prompt} ───────────>│
     │                                         │
     │<─── session/update {message_chunk} ────│  (streaming)
     │<─── session/update {tool_call} ────────│
     │                                         │
     │<─── session/request_permission ────────│  (if needed)
     │──── permission result {approved} ──────>│
     │                                         │
     │<─── session/update {tool_call_update} ─│
     │<─── session/update {message_chunk} ────│
     │                                         │
     │<─── session/prompt result {stopReason} ─│  (turn done)
     │                                         │
     │  [if continuing: send another session/prompt]
     │                                         │
     │──── session/cancel ────────────────────>│  (on shutdown/stop)
     │                                         │
```

---

## 4. Concurrency Architecture

### 4.1 Goroutine Model

```
main goroutine
  ├── Orchestrator goroutine (owns all mutable state)
  │     ├── poll ticker (time.Ticker)
  │     ├── receives events via channels
  │     └── single-threaded state mutations
  │
  ├── Workflow watcher goroutine (fsnotify)
  │     └── sends reload events to orchestrator
  │
  ├── Worker goroutines (one per running issue)
  │     ├── workspace setup
  │     ├── ACP subprocess management
  │     ├── stdout line reader goroutine
  │     ├── turn loop
  │     └── sends events/results back via channel
  │
  ├── Retry timer goroutines (time.AfterFunc per retry)
  │     └── sends retry-fire events to orchestrator
  │
  └── HTTP server goroutine (optional)
        └── reads state snapshot via sync channel or mutex
```

### 4.2 Channel Design

```go
// internal/orchestrator/orchestrator.go

type Orchestrator struct {
    state  *State
    config *config.Config  // atomic pointer for hot reload

    // Inbound event channels — all state mutations flow through these
    events     chan Event          // worker events (codex updates, completion)
    retryFired chan RetryFire      // retry timer expirations
    reloadCh   chan *config.Config // workflow config reloads
    refreshCh  chan struct{}       // manual refresh trigger (HTTP API)
    stopCh     chan struct{}       // graceful shutdown signal
}

type Event struct {
    Type    EventType
    IssueID string
    Payload any // typed per EventType
}

type EventType int

const (
    EventWorkerDone EventType = iota
    EventWorkerFailed
    EventAgentUpdate
)

type RetryFire struct {
    IssueID string
    Entry   *RetryEntry
}
```

### 4.3 Orchestrator Main Loop

```go
func (o *Orchestrator) Run(ctx context.Context) error {
    // Startup validation
    if err := o.validateConfig(); err != nil {
        return fmt.Errorf("startup validation: %w", err)
    }
    o.startupTerminalCleanup(ctx)

    ticker := time.NewTicker(time.Duration(o.state.PollIntervalMs) * time.Millisecond)
    defer ticker.Stop()

    // Immediate first tick
    o.tick(ctx)

    for {
        select {
        case <-ctx.Done():
            o.shutdown()
            return nil
        case <-ticker.C:
            o.tick(ctx)
        case ev := <-o.events:
            o.handleEvent(ev)
        case rf := <-o.retryFired:
            o.handleRetry(ctx, rf)
        case cfg := <-o.reloadCh:
            o.applyConfig(cfg, ticker)
        case <-o.refreshCh:
            o.tick(ctx)
        case <-o.stopCh:
            o.shutdown()
            return nil
        }
    }
}
```

**Key invariant:** All state mutations happen in this single goroutine. Workers communicate results via channels only.

---

## 5. Key Component Contracts

### 5.1 Workflow Loader

```go
func LoadWorkflow(path string) (*WorkflowDefinition, error)
```

- Reads file, splits on `---` delimiters
- Returns typed errors: `ErrMissingWorkflowFile`, `ErrWorkflowParseError`, `ErrFrontMatterNotMap`

### 5.2 Config Resolution

```go
func ResolveConfig(raw map[string]any) (*Config, error)
```

- Merges raw map onto defaults
- Resolves `$VAR` in `tracker.api_key` and path fields
- Expands `~` in path fields
- Coerces string integers to int
- Normalizes per-state concurrency keys to lowercase
- Aliases `codex` key to `gemini` for backward compatibility

### 5.3 Dispatch Preflight Validation

```go
func ValidateDispatchConfig(cfg *Config) error
```

Checks:
- `tracker.kind` present and is `"linear"`
- `tracker.api_key` present and non-empty after resolution
- `tracker.project_slug` present
- `gemini.command` present and non-empty

### 5.4 Linear Client

```go
type LinearClient struct {
    endpoint string
    apiKey   string
    httpClient *http.Client  // 30s timeout
}

func (c *LinearClient) FetchCandidateIssues(ctx context.Context, projectSlug string, activeStates []string) ([]Issue, error)
func (c *LinearClient) FetchIssueStatesByIDs(ctx context.Context, ids []string) ([]Issue, error)
func (c *LinearClient) FetchIssuesByStates(ctx context.Context, projectSlug string, states []string) ([]Issue, error)
```

GraphQL queries:
- Candidate fetch: filter `project: {slugId: {eq: $projectSlug}}`, states filter, paginate with `after` cursor, page size 50
- State refresh: filter `id: {in: $ids}` with `[ID!]` variable type
- Terminal fetch: filter project + states

### 5.5 Workspace Manager

```go
func (m *Manager) CreateForIssue(identifier string) (*Workspace, error)
func (m *Manager) CleanWorkspace(identifier string) error
func (m *Manager) RunHook(hookName string, script string, workspacePath string) error
```

Safety checks (called before every agent launch):
```go
func ValidateWorkspacePath(workspacePath, workspaceRoot string) error
```
- Both paths resolved to absolute
- `workspacePath` has `workspaceRoot` as directory prefix
- Workspace key contains only `[A-Za-z0-9._-]`

### 5.6 Agent Runner

```go
func RunAgent(ctx context.Context, params RunParams, eventCh chan<- Event) error

type RunParams struct {
    Issue          *Issue
    Attempt        *int
    WorkspacePath  string
    Config         *config.GeminiConfig
    AgentConfig    *config.AgentConfig
    PromptTemplate string
    Workflow       *WorkflowDefinition
}
```

Lifecycle:
1. Create/reuse workspace
2. Run `before_run` hook
3. Launch Gemini CLI subprocess (`bash -lc <command>`)
4. ACP handshake: `initialize` → `session/new`
5. Turn loop: `session/prompt` → stream `session/update` → check issue state → continue or exit
6. On exit: stop subprocess, run `after_run` hook

### 5.7 ACP Client

```go
type ACPClient struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    stdout *bufio.Scanner
    nextID int
}

func NewACPClient(command string, cwd string) (*ACPClient, error)
func (c *ACPClient) Initialize(ctx context.Context) (*InitializeResult, error)
func (c *ACPClient) SessionNew(ctx context.Context, cwd string) (string, error)
func (c *ACPClient) SessionPrompt(ctx context.Context, sessionID string, prompt []ContentBlock, handler UpdateHandler) (*SessionPromptResult, error)
func (c *ACPClient) SessionCancel(sessionID string) error
func (c *ACPClient) Close() error

// UpdateHandler is called for each session/update notification during a turn
type UpdateHandler func(update *SessionUpdateParams)
```

The `SessionPrompt` method blocks until a response (StopReason) is received, calling `handler` for each `session/update` notification along the way. Incoming `session/request_permission` requests are auto-approved inline.

### 5.8 Prompt Renderer

```go
func RenderPrompt(template string, issue *Issue, attempt *int) (string, error)
```

- Strict mode: unknown variables → error, unknown filters → error
- Uses `github.com/osteele/liquid` for Liquid-compatible rendering
- Fallback prompt when template is empty: `"You are working on an issue from Linear."`

---

## 6. Error Handling Strategy

### 6.1 Error Types

```go
// Workflow errors
var (
    ErrMissingWorkflowFile    = errors.New("missing_workflow_file")
    ErrWorkflowParseError     = errors.New("workflow_parse_error")
    ErrFrontMatterNotMap      = errors.New("workflow_front_matter_not_a_map")
    ErrTemplateParseError     = errors.New("template_parse_error")
    ErrTemplateRenderError    = errors.New("template_render_error")
)

// Tracker errors
type TrackerError struct {
    Kind    string // unsupported_tracker_kind, missing_tracker_api_key, etc.
    Message string
    Cause   error
}

// Agent errors
type AgentError struct {
    Kind    string // response_timeout, turn_timeout, port_exit, etc.
    Message string
    Cause   error
}
```

### 6.2 Recovery Behavior

| Failure | Action |
|---|---|
| Startup config validation | Fail startup, exit nonzero |
| Per-tick config validation | Skip dispatch, continue reconciliation |
| Candidate fetch failure | Log, skip dispatch this tick |
| State refresh failure | Log, keep workers running |
| Worker failure | Remove from running, schedule retry with backoff |
| Worker normal exit | Remove from running, schedule continuation retry (1s) |
| Hook failure (after_create) | Fatal to workspace creation, fail attempt |
| Hook failure (before_run) | Fatal to attempt |
| Hook failure (after_run) | Log and ignore |
| Hook failure (before_remove) | Log and ignore |
| Prompt render failure | Fail attempt |
| ACP handshake failure | Fail attempt |
| Turn timeout | Fail attempt |
| Stall detected | Kill worker, schedule retry |
| Log sink failure | Continue running |

---

## 7. Dependencies

| Dependency | Purpose | Version |
|---|---|---|
| `gopkg.in/yaml.v3` | YAML front matter parsing | latest |
| `github.com/fsnotify/fsnotify` | Workflow file watching | v1.x |
| `github.com/osteele/liquid` | Liquid template rendering (strict mode) | latest |
| `log/slog` (stdlib) | Structured logging | Go 1.21+ |
| `net/http` (stdlib) | HTTP server + Linear API client | stdlib |
| `os/exec` (stdlib) | Subprocess management for Gemini CLI | stdlib |
| `encoding/json` (stdlib) | JSON-RPC message serialization | stdlib |
| `context` (stdlib) | Cancellation + timeout propagation | stdlib |

No heavy frameworks. No ORM. No GraphQL client library.

---

## 8. Configuration Hot Reload

```
WORKFLOW.md change detected (fsnotify)
    │
    ▼
Parse workflow file
    │
    ├── Parse error → log error, keep last good config
    │
    └── Parse OK → resolve config
            │
            ├── Resolve error → log error, keep last good config
            │
            └── Resolve OK → send new Config to orchestrator via reloadCh
                    │
                    ▼
              Orchestrator applies:
                - Update poll ticker interval
                - Update max_concurrent_agents
                - Update all runtime settings
                - Future dispatches use new config
                - In-flight workers NOT restarted
```

---

## 9. Graceful Shutdown

On SIGINT/SIGTERM:
1. Cancel orchestrator context
2. Stop accepting new dispatches
3. Cancel all worker contexts (sends `session/cancel` to each ACP subprocess)
4. Wait for workers to exit (with timeout)
5. Run `after_run` hooks for interrupted workers
6. Exit 0

---

## 10. HTTP API (Extension)

### Endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/` | HTML dashboard |
| GET | `/api/v1/state` | System state JSON |
| GET | `/api/v1/{identifier}` | Issue detail JSON |
| POST | `/api/v1/refresh` | Trigger immediate poll |

### State Snapshot

The HTTP handlers acquire a read-consistent snapshot from the orchestrator. Two approaches:

**Option A (chosen):** The orchestrator exposes a `Snapshot() StateSnapshot` method protected by `sync.RWMutex`. The HTTP handler calls this to get a point-in-time copy.

**Option B:** Send a request/response pair through a channel to the orchestrator loop. Higher latency but zero shared memory.

We use Option A because the state is read-heavy (dashboard polling) and the mutex contention is minimal.

---

## 11. Build & Run

```makefile
# Makefile
.PHONY: build test run

build:
	go build -o bin/symphony ./cmd/symphony

test:
	go test ./...

run: build
	./bin/symphony

run-with-port: build
	./bin/symphony --port 8080

run-custom-workflow: build
	./bin/symphony /path/to/WORKFLOW.md
```

Binary name: `symphony`
Module path: `github.com/user/symphony-go` (or appropriate module path)

---

## 12. Mapping: FUNCTIONAL.md → Implementation

| FR | Primary Package | Key File(s) |
|---|---|---|
| FR-1: Workflow Loader | `internal/workflow` | `loader.go` |
| FR-2: Config Layer | `internal/config` | `config.go`, `resolve.go`, `validate.go` |
| FR-3: Linear Client | `internal/tracker` | `client.go`, `queries.go`, `normalize.go` |
| FR-4: Orchestrator | `internal/orchestrator` | `orchestrator.go`, `dispatch.go`, `reconcile.go`, `retry.go` |
| FR-5: Workspace Manager | `internal/workspace` | `manager.go`, `hooks.go`, `safety.go` |
| FR-6: Agent Runner (ACP) | `internal/agent` | `runner.go`, `acp.go`, `session.go` |
| FR-7: Prompt Rendering | `internal/prompt` | `render.go`, `context.go` |
| FR-8: Logging | `internal/logging` | `logging.go` |
| FR-9: CLI | `cmd/symphony` | `main.go` |
| EXT-1: HTTP Server | `internal/server` | `server.go`, `api.go` |
| EXT-2: Linear GraphQL Tool | `internal/agent` | embedded in ACP session handling |
