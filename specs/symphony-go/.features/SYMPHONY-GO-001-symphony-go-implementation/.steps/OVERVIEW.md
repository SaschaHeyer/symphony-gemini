# Implementation Plan: Symphony Go

**Date:** 2026-03-17
**Feature ID:** SYMPHONY-GO-001

---

## Delivery Strategy

8 vertical slices, each producing testable, runnable code. Early steps build foundational packages; later steps compose them into the orchestrator and CLI. Each step ends with passing tests.

---

## Execution Checklist

- [x] **Step 1** — Project scaffold + Workflow Loader + Config Layer
- [x] **Step 2** — Linear GraphQL Client
- [x] **Step 3** — Workspace Manager (create, reuse, hooks, safety)
- [x] **Step 4** — Prompt Renderer
- [x] **Step 5** — ACP Client (Gemini CLI JSON-RPC protocol)
- [x] **Step 6** — Agent Runner (workspace + prompt + ACP composition)
- [x] **Step 7** — Orchestrator (poll loop, dispatch, reconciliation, retry)
- [x] **Step 8** — CLI Entrypoint + HTTP Server Extension + Integration Tests

---

## Dependency Graph

```
Step 1 (workflow + config)
  │
  ├── Step 2 (Linear client)     ── uses config for endpoint/auth
  ├── Step 3 (workspace)         ── uses config for root/hooks
  ├── Step 4 (prompt)            ── uses workflow template
  │
  └── Step 5 (ACP client)        ── uses config for gemini settings
        │
        └── Step 6 (agent runner) ── composes workspace + prompt + ACP
              │
              └── Step 7 (orchestrator) ── composes all: tracker + runner + config
                    │
                    └── Step 8 (CLI + HTTP + integration)
```

Steps 2, 3, 4, 5 can be built in any order after Step 1. Steps 6–8 are sequential.
