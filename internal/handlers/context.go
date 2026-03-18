package handlers

import (
	"html/template"
	"net/http"

	"singdns-panel/internal/auth"
	cfgpkg "singdns-panel/internal/config"
	"singdns-panel/internal/services"
)

type App struct {
	Config       *cfgpkg.Config
	ConfigPath   string
	Sessions     *auth.SessionManager
	Limiter      *auth.LoginLimiter
	Templates    *template.Template
	SingBox      *services.SingBoxService
	MosDNS       *services.MosDNSService
	Audit        *services.AuditService
	Panel        *services.PanelService
	PanelVersion string
}

func (a *App) render(w http.ResponseWriter, name string, data any) {
	if token, err := auth.NewCSRFToken(); err == nil {
		auth.SetCSRFCookie(w, token)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.Templates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
