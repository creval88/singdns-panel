package handlers

import "net/http"

func (a *App) auditFromRequest(r *http.Request, action string, err error) {
	result := "ok"
	if err != nil {
		result = err.Error()
	}
	a.auditMessageFromRequest(r, action, result)
}

func (a *App) auditMessageFromRequest(r *http.Request, action, result string) {
	if a.Audit == nil {
		return
	}
	user, _ := a.Sessions.Username(r)
	a.Audit.Log(user, action, result)
}
