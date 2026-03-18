package services

import (
	"bufio"
	"encoding/json"
	"os"
)

func (a *AuditService) List(limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	f, err := os.Open(a.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var lines []AuditEntry
	s := bufio.NewScanner(f)
	for s.Scan() {
		var item AuditEntry
		if err := json.Unmarshal([]byte(s.Text()), &item); err == nil && item.Time != "" {
			lines = append(lines, item)
			continue
		}
		lines = append(lines, AuditEntry{Time: "", Action: s.Text()})
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	out := make([]AuditEntry, 0, len(lines))
	for i := len(lines) - 1; i >= 0; i-- {
		out = append(out, lines[i])
	}
	return out, nil
}
