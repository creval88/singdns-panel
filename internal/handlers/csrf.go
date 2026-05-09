package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
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
	origin := r.Header.Get("Origin")
	referer := r.Header.Get("Referer")
	if origin != "" {
		return requestHostMatchesURL(r.Host, origin)
	}
	if referer != "" {
		return requestHostMatchesURL(r.Host, referer)
	}
	return false
}

func requestHostMatchesURL(host, rawURL string) bool {
	host = strings.TrimSpace(host)
	if host == "" || strings.TrimSpace(rawURL) == "" {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return strings.EqualFold(u.Host, host)
}
