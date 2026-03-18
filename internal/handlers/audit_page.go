package handlers

import (
	"encoding/json"
	"net/http"
)

func (a *App) AuditPage(w http.ResponseWriter, r *http.Request) {
	items, _ := a.Audit.List(200)
	a.render(w, "audit.html", map[string]any{"Title": "审计日志", "ActiveNav": "audit", "PageTitle": "审计日志", "Eyebrow": "Audit", "SidebarSubtitle": "审计日志", "Items": items})
}

func (a *App) AuditAPI(w http.ResponseWriter, r *http.Request) {
	items, err := a.Audit.List(200)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
}
