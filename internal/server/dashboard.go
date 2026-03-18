package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	snapshot := s.orch.Snapshot()

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html><head>
<meta charset="utf-8">
<meta http-equiv="refresh" content="5">
<title>Symphony Go Dashboard</title>
<style>
body { font-family: monospace; background: #1a1a2e; color: #e0e0e0; padding: 20px; }
h1 { color: #00d4ff; }
h2 { color: #7b68ee; margin-top: 24px; }
table { border-collapse: collapse; width: 100%; margin: 8px 0; }
th, td { text-align: left; padding: 6px 12px; border: 1px solid #333; }
th { background: #16213e; color: #00d4ff; }
tr:nth-child(even) { background: #0f3460; }
.metric { display: inline-block; background: #16213e; padding: 8px 16px; margin: 4px; border-radius: 4px; }
.metric .value { font-size: 1.4em; color: #00d4ff; }
.metric .label { font-size: 0.8em; color: #888; }
</style>
</head><body>
`)

	b.WriteString(fmt.Sprintf("<h1>Symphony Go</h1>\n"))
	b.WriteString(fmt.Sprintf("<p>Generated: %s | Model: <strong>%s</strong> | Project: <strong>%s</strong></p>\n",
		snapshot.GeneratedAt.Format(time.RFC3339), snapshot.Config.AgentModel, snapshot.Config.ProjectSlug))

	// Metrics
	b.WriteString(`<div>`)
	b.WriteString(fmt.Sprintf(`<div class="metric"><div class="value">%d</div><div class="label">Running</div></div>`, snapshot.Counts.Running))
	b.WriteString(fmt.Sprintf(`<div class="metric"><div class="value">%d</div><div class="label">Retrying</div></div>`, snapshot.Counts.Retrying))
	b.WriteString(fmt.Sprintf(`<div class="metric"><div class="value">%d</div><div class="label">Total Tokens</div></div>`, snapshot.AgentTotals.TotalTokens))
	b.WriteString(fmt.Sprintf(`<div class="metric"><div class="value">%.0fs</div><div class="label">Runtime</div></div>`, snapshot.AgentTotals.SecondsRunning))
	b.WriteString(`</div>`)

	// Running
	b.WriteString("<h2>Running Sessions</h2>\n")
	if len(snapshot.Running) == 0 {
		b.WriteString("<p>No running sessions.</p>\n")
	} else {
		b.WriteString("<table><tr><th>Issue</th><th>State</th><th>Session</th><th>Turns</th><th>Last Event</th><th>Started</th><th>Tokens</th></tr>\n")
		for _, r := range snapshot.Running {
			elapsed := ""
			if !r.StartedAt.IsZero() {
				elapsed = time.Since(r.StartedAt).Truncate(time.Second).String()
			}
			b.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td><td>%d</td><td>%s</td><td>%s ago</td><td>%d</td></tr>\n",
				r.IssueIdentifier, r.State, r.SessionID, r.TurnCount, r.LastEvent, elapsed, r.Tokens.TotalTokens))
		}
		b.WriteString("</table>\n")
	}

	// Retrying
	b.WriteString("<h2>Retry Queue</h2>\n")
	if len(snapshot.Retrying) == 0 {
		b.WriteString("<p>No retries queued.</p>\n")
	} else {
		b.WriteString("<table><tr><th>Issue</th><th>Attempt</th><th>Due At</th><th>Error</th></tr>\n")
		for _, r := range snapshot.Retrying {
			b.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%d</td><td>%s</td><td>%s</td></tr>\n",
				r.IssueIdentifier, r.Attempt, r.DueAt.Format(time.RFC3339), r.Error))
		}
		b.WriteString("</table>\n")
	}

	b.WriteString("</body></html>")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(b.String()))
}
