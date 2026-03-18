package handlers

import (
	"encoding/json"
	"net/http"

	"singdns-panel/internal/services"
)

type serviceRuntime interface {
	Status() (*services.ServiceStatus, error)
	Action(action string) (*services.ServiceActionResult, error)
}

func (a *App) serviceStatusAPI(w http.ResponseWriter, svc serviceRuntime) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(must(svc.Status()))
}

func (a *App) serviceActionAPI(w http.ResponseWriter, r *http.Request, auditPrefix string, svc serviceRuntime, successMsg string) {
	action := r.PathValue("action")
	res, err := svc.Action(action)
	a.respondAudited(w, r, auditPrefix+action, res, err, successMsg)
}
