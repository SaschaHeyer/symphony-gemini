# Implementation Plan: Jira Tracker Support

**Feature:** SYMPHONY-GO-003
**Total Steps:** 4

---

## Execution Checklist

- [x] **STEP-1**: Config — Add `Email` field to `TrackerConfig`, update validation for `jira` kind, update `ResolveConfig` with Jira env fallbacks + tests
- [x] **STEP-2**: Jira Client & Normalization — `JiraClient` implementing `TrackerClient`, ADF text extraction, issue normalization, error types + tests
- [x] **STEP-3**: Tracker Factory — `NewTrackerClient()` factory in tracker package + tests
- [x] **STEP-4**: main.go Integration — Replace `NewLinearClient()` with `NewTrackerClient()`, update README

---

## Dependency Order

```
STEP-1 (Config) ─────────►─┐
                             ├─► STEP-3 (Factory) ──► STEP-4 (Integration)
STEP-2 (JiraClient+Norm) ──┘
```

STEP-1 and STEP-2 are independent.
STEP-3 depends on both (factory imports both clients + config).
STEP-4 wires everything into main.go.
