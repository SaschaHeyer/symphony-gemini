# Implementation Plan: cmux Session Visibility

**Date:** 2026-03-18
**Feature ID:** SYMPHONY-GO-004

---

## Delivery Strategy

Four vertical slices, each building on the previous. Each step produces testable, compilable code.

## Execution Checklist

- [x] **Step 1** — Config + cmux Manager foundation (CmuxConfig, New, disabled no-op, binary detection)
- [x] **Step 2** — cmux CLI exec + workspace management (run helper, Init, mock binary, workspace create/reuse)
- [x] **Step 3** — Surface lifecycle + log writing (CreateSurface, CloseSurface, WriteEvent, LogWriter)
- [x] **Step 4** — Orchestrator + agent runner + main.go integration (full wiring, end-to-end flow)

## Dependency Graph

```
Step 1: Config + Manager foundation
   │
   ▼
Step 2: CLI exec + workspace management
   │
   ▼
Step 3: Surface lifecycle + log writing
   │
   ▼
Step 4: Orchestrator + runner + main.go integration
```

All steps are sequential — each builds on the previous.
