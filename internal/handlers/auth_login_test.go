package handlers

import (
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	authpkg "singdns-panel/internal/auth"
	cfgpkg "singdns-panel/internal/config"
)

func TestLoginPost_SessionCreateErrorDoesNotLeakInternalMessage(t *testing.T) {
	old := sessionCreate
	sessionCreate = func(sm *authpkg.SessionManager, w http.ResponseWriter, username string) error {
		return errors.New("session backend broken: dial tcp 127.0.0.1:6379: connect: connection refused")
	}
	defer func() { sessionCreate = old }()

	tpl := template.Must(template.New("login.html").Parse("<html>login</html>"))
	hash, err := authpkg.HashPassword("secret")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	app := &App{
		Config: &cfgpkg.Config{Auth: cfgpkg.AuthConfig{
			Username:     "admin",
			PasswordHash: hash,
		}},
		Sessions:  authpkg.NewSessionManager("sid"),
		Limiter:   authpkg.NewLoginLimiter(5, time.Minute),
		Templates: tpl,
	}

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "secret")
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:12345"
	rr := httptest.NewRecorder()

	app.LoginPost(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d, body=%q", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, "connection refused") || strings.Contains(body, "session backend broken") {
		t.Fatalf("internal error leaked to client: %q", body)
	}
	if !strings.Contains(body, "internal server error") {
		t.Fatalf("expected generic server error message, got %q", body)
	}
}
