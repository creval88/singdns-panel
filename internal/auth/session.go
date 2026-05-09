package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

type SessionManager struct {
	cookieName string
	mu         sync.RWMutex
	sessions   map[string]sessionRecord
}

type sessionRecord struct {
	Username  string
	ExpiresAt time.Time
}

func NewSessionManager(cookieName string) *SessionManager {
	return &SessionManager{cookieName: cookieName, sessions: map[string]sessionRecord{}}
}

func (s *SessionManager) Create(w http.ResponseWriter, username string) error {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return err
	}
	token := hex.EncodeToString(buf)
	expiresAt := time.Now().Add(24 * time.Hour)
	s.mu.Lock()
	s.cleanupExpiredLocked(time.Now())
	s.sessions[token] = sessionRecord{Username: username, ExpiresAt: expiresAt}
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
		MaxAge:   int((24 * time.Hour).Seconds()),
	})
	return nil
}

func (s *SessionManager) Username(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(s.cookieName)
	if err != nil {
		return "", false
	}
	s.mu.RLock()
	rec, ok := s.sessions[cookie.Value]
	s.mu.RUnlock()
	if !ok {
		return "", false
	}
	if time.Now().After(rec.ExpiresAt) {
		s.mu.Lock()
		delete(s.sessions, cookie.Value)
		s.mu.Unlock()
		return "", false
	}
	return rec.Username, true
}

func (s *SessionManager) Destroy(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(s.cookieName)
	if err == nil {
		s.mu.Lock()
		delete(s.sessions, cookie.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: s.cookieName, Value: "", Path: "/", Expires: time.Unix(0, 0), MaxAge: -1})
}

func (s *SessionManager) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.Username(r); !ok {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *SessionManager) cleanupExpiredLocked(now time.Time) {
	for token, rec := range s.sessions {
		if now.After(rec.ExpiresAt) {
			delete(s.sessions, token)
		}
	}
}
