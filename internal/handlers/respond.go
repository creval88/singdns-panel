package handlers

import "net/http"

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
