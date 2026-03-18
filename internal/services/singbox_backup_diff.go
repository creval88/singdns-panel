package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s *SingBoxService) BackupDiff(name string) (string, error) {
	current, err := s.ReadConfig()
	if err != nil {
		return "", err
	}
	backupPath := filepath.Join(filepath.Dir(s.cfg.ConfigPath), name)
	b, err := os.ReadFile(backupPath)
	if err != nil {
		return "", err
	}
	curLines := strings.Split(current, "\n")
	bakLines := strings.Split(string(b), "\n")
	max := len(curLines)
	if len(bakLines) > max {
		max = len(bakLines)
	}
	var out []string
	for i := 0; i < max; i++ {
		var c, d string
		if i < len(curLines) {
			c = curLines[i]
		}
		if i < len(bakLines) {
			d = bakLines[i]
		}
		if c == d {
			continue
		}
		if d != "" {
			out = append(out, fmt.Sprintf("- %s", d))
		}
		if c != "" {
			out = append(out, fmt.Sprintf("+ %s", c))
		}
		if len(out) >= 120 {
			break
		}
	}
	if len(out) == 0 {
		return "无差异", nil
	}
	return strings.Join(out, "\n"), nil
}
