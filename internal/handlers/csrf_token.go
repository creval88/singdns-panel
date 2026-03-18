package handlers

import (
	"encoding/json"
	"net/http"

	"singdns-panel/internal/auth"
)

func (a *App) CSRFTokenAPI(w http.ResponseWriter, r *http.Request) {
	token, err := auth.NewCSRFToken()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.SetCSRFCookie(w, token)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "token": token})
}
