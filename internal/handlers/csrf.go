package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"singdns-panel/internal/auth"
)

func (a *App) CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if auth.CheckCSRF(r) || sameOrigin(r) {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "csrf 校验失败，请刷新页面后重试"})
			return
		}
		http.Error(w, "invalid csrf token", http.StatusForbidden)
	})
}

func sameOrigin(r *http.Request) bool {
	host := r.Host
	origin := r.Header.Get("Origin")
	referer := r.Header.Get("Referer")
	return (origin != "" && strings.Contains(origin, host)) || (referer != "" && strings.Contains(referer, host))
}
