package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionUsernameRejectsExpiredToken(t *testing.T) {
	t.Parallel()

	sm := NewSessionManager("sid")
	sm.sessions["expired"] = sessionRecord{
		Username:  "admin",
		ExpiresAt: time.Now().Add(-time.Minute),
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "sid", Value: "expired"})

	if username, ok := sm.Username(req); ok || username != "" {
		t.Fatalf("expected expired session to be rejected, got username=%q ok=%v", username, ok)
	}
	if _, exists := sm.sessions["expired"]; exists {
		t.Fatalf("expired session was not removed")
	}
}

func TestSessionCreateCleansExpiredTokens(t *testing.T) {
	t.Parallel()

	sm := NewSessionManager("sid")
	sm.sessions["expired"] = sessionRecord{
		Username:  "old",
		ExpiresAt: time.Now().Add(-time.Minute),
	}

	rr := httptest.NewRecorder()
	if err := sm.Create(rr, "admin"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, exists := sm.sessions["expired"]; exists {
		t.Fatalf("expired session was not cleaned during create")
	}
	if len(sm.sessions) != 1 {
		t.Fatalf("expected one active session, got %d", len(sm.sessions))
	}
}
