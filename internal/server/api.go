package server

import (
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) handleGetState(w http.ResponseWriter, r *http.Request) {
	snapshot := s.orch.Snapshot()
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleGetIssue(w http.ResponseWriter, r *http.Request) {
	identifier := r.PathValue("identifier")
	if identifier == "" {
		writeError(w, http.StatusBadRequest, "missing_identifier", "identifier path parameter is required")
		return
	}

	running, retry := s.orch.FindIssueByIdentifier(identifier)
	if running == nil && retry == nil {
		writeError(w, http.StatusNotFound, "issue_not_found",
			"issue "+identifier+" is not tracked in current state")
		return
	}

	resp := map[string]any{
		"issue_identifier": identifier,
	}

	if running != nil {
		resp["status"] = "running"
		resp["issue_id"] = running.IssueID
		resp["running"] = map[string]any{
			"session_id":    running.SessionID,
			"turn_count":    running.TurnCount,
			"state":         running.State,
			"started_at":    running.StartedAt,
			"last_event":    running.LastEvent,
			"last_message":  running.LastMessage,
			"last_event_at": running.LastEventAt,
			"tokens": map[string]any{
				"input_tokens":  running.InputTokens,
				"output_tokens": running.OutputTokens,
				"total_tokens":  running.TotalTokens,
			},
		}
		resp["retry"] = nil
	}

	if retry != nil {
		resp["status"] = "retrying"
		resp["issue_id"] = retry.IssueID
		resp["running"] = nil
		resp["retry"] = map[string]any{
			"attempt": retry.Attempt,
			"due_at":  retry.DueAt,
			"error":   retry.Error,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handlePostRefresh(w http.ResponseWriter, r *http.Request) {
	select {
	case s.orch.RefreshCh() <- struct{}{}:
		writeJSON(w, http.StatusAccepted, map[string]any{
			"queued":       true,
			"coalesced":    false,
			"requested_at": time.Now().UTC(),
			"operations":   []string{"poll", "reconcile"},
		})
	default:
		// Channel full — coalesced
		writeJSON(w, http.StatusAccepted, map[string]any{
			"queued":       true,
			"coalesced":    true,
			"requested_at": time.Now().UTC(),
			"operations":   []string{"poll", "reconcile"},
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}
