---
tracker:
  kind: linear
  api_key: $LINEAR_API_KEY
  project_slug: sympho-2a2c014d1423
gemini:
  command: "gemini --acp --model gemini-3-flash-preview"
  read_timeout_ms: 30000
server:
  port: 8080
---
Test the Linear MCP tools. 

1. Use `mcp_linear_list_issues` to list issues in the current project.
2. Tell me the identifier and title of the first issue you find.
3. If you can't find any issues, tell me "No issues found".
4. If the tool is missing, tell me exactly what error you see.

Then stop.
