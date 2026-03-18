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
	sessions   map[string]string
}

func NewSessionManager(cookieName string) *SessionManager {
	return &SessionManager{cookieName: cookieName, sessions: map[string]string{}}
}

func (s *SessionManager) Create(w http.ResponseWriter, username string) error {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return err
	}
	token := hex.EncodeToString(buf)
	s.mu.Lock()
	s.sessions[token] = username
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})
	return nil
}

func (s *SessionManager) Username(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(s.cookieName)
	if err != nil {
		return "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.sessions[cookie.Value]
	return u, ok
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
