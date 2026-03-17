# Step 4: Prompt Renderer

**Covers:** FR-7 (Prompt Rendering)
**Package:** `internal/prompt`

---

## 1. Tasks

### 1.1 Template Context Assembly

- [ ] `internal/prompt/context.go`:
  - `BuildContext(issue *tracker.Issue, attempt *int) map[string]any`:
    - Convert `Issue` struct to `map[string]any` with string keys for template compatibility
    - Preserve nested arrays (labels) and structs (blocked_by) as iterable collections
    - Include `attempt` key: nil if first run, integer if retry/continuation
    - `*time.Time` fields → ISO-8601 string or nil

### 1.2 Renderer

- [ ] `internal/prompt/render.go`:
  - `RenderPrompt(templateStr string, issue *tracker.Issue, attempt *int) (string, error)`:
    1. If `templateStr` is empty after trim → return fallback: `"You are working on an issue from Linear."`
    2. Create `liquid.Engine` with strict variables enabled (`engine.StrictVariables = true`)
    3. Build template context via `BuildContext`
    4. Parse template → on error, return `ErrTemplateParseError`
    5. Render with context → on error (unknown var/filter), return `ErrTemplateRenderError`
    6. Return rendered string

- [ ] Error types in `internal/prompt/render.go`:
  - `ErrTemplateParseError`
  - `ErrTemplateRenderError`

### 1.3 Tests

- [ ] `internal/prompt/render_test.go`:
  - Test: renders `{{ issue.identifier }}` and `{{ issue.title }}`
  - Test: renders `{{ issue.labels }}` as iterable (e.g., `{% for label in issue.labels %}{{ label }}{% endfor %}`)
  - Test: renders `{{ issue.description }}` when nil → empty string
  - Test: `attempt` is nil on first run → `{{ attempt }}` renders as empty/nil
  - Test: `attempt` is integer on retry → renders as number
  - Test: unknown variable `{{ issue.nonexistent }}` → `ErrTemplateRenderError`
  - Test: unknown filter `{{ issue.title | nonexistent }}` → `ErrTemplateRenderError`
  - Test: empty template → fallback prompt
  - Test: complex template with conditionals and loops renders correctly

---

## 2. Dependencies to Add

```
github.com/osteele/liquid
```

---

## 3. Definition of Done

- [ ] `go test ./internal/prompt/...` — all pass
- [ ] Strict mode rejects unknown variables and filters
- [ ] All issue fields accessible in templates
- [ ] Fallback prompt works for empty templates
