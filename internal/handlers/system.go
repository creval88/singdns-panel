package handlers

import (
	"encoding/json"
	"net/http"
)

func (a *App) HealthAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (a *App) SystemPage(w http.ResponseWriter, r *http.Request) {
	panel, _ := a.Panel.LatestLocalRelease()
	a.render(w, "system.html", map[string]any{
		"Title":           "System Settings",
		"ActiveNav":       "system",
		"PageTitle":       "系统设置与升级",
		"Eyebrow":         "System",
		"SidebarSubtitle": "sing-box / mosdns 控制台",
		"PanelVersion":    a.Panel.CurrentVersion(),
		"PanelRelease":    panel,
		"Arch":            "linux/amd64",
		"Listen":          a.Config.Listen,
	})
}
