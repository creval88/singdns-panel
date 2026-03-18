package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

func NewCSRFToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func SetCSRFCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "singdns_csrf",
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
}

func CSRFFromRequest(r *http.Request) string {
	c, err := r.Cookie("singdns_csrf")
	if err != nil {
		return ""
	}
	return c.Value
}

func CheckCSRF(r *http.Request) bool {
	header := r.Header.Get("X-CSRF-Token")
	cookie := CSRFFromRequest(r)
	return header != "" && cookie != "" && header == cookie
}
