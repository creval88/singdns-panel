package auth

import (
	"sync"
	"time"
)

type attemptInfo struct {
	Count int
	Until time.Time
}

type LoginLimiter struct {
	mu       sync.Mutex
	attempts map[string]attemptInfo
	maxTries int
	window   time.Duration
}

func NewLoginLimiter(maxTries int, window time.Duration) *LoginLimiter {
	return &LoginLimiter{attempts: map[string]attemptInfo{}, maxTries: maxTries, window: window}
}

func (l *LoginLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	info, ok := l.attempts[key]
	if !ok {
		return true
	}
	if time.Now().After(info.Until) {
		delete(l.attempts, key)
		return true
	}
	return info.Count < l.maxTries
}

func (l *LoginLimiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	info := l.attempts[key]
	info.Count++
	info.Until = time.Now().Add(l.window)
	l.attempts[key] = info
}

func (l *LoginLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}
