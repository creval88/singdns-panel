package handlers

import (
	"encoding/json"
	"net/http"
)

func (a *App) respondAudited(w http.ResponseWriter, r *http.Request, action string, result interface{ AuditText() string }, err error, successMsg string) {
	if err != nil {
		a.auditFromRequest(r, action, err)
		respondMessage(w, err, successMsg)
		return
	}
	if result != nil {
		a.auditMessageFromRequest(r, action, result.AuditText())
		respondMessage(w, nil, result.AuditText())
		return
	}
	a.auditMessageFromRequest(r, action, successMsg)
	respondMessage(w, nil, successMsg)
}

// respondJSON is a tiny helper for handlers that need to return structured payloads
// with custom HTTP status while keeping the common ok/message shape elsewhere.
func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	if status > 0 {
		w.WriteHeader(status)
	}
	_ = json.NewEncoder(w).Encode(v)
}
