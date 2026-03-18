package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type AuditEntry struct {
	Time   string `json:"time"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Result string `json:"result"`
}

type AuditService struct {
	path string
}

func NewAuditService(path string) *AuditService {
	return &AuditService{path: path}
}

func (a *AuditService) Log(actor, action, result string) {
	if a == nil || a.path == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(a.path), 0755)
	f, err := os.OpenFile(a.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_ = json.NewEncoder(f).Encode(AuditEntry{
		Time:   time.Now().Format(time.RFC3339),
		Actor:  actor,
		Action: action,
		Result: result,
	})
}
