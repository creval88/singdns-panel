package services

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func (s *SingBoxService) backupPath(name string) string {
	return filepath.Join(filepath.Dir(s.cfg.ConfigPath), name)
}

func (s *SingBoxService) backupFileName() string {
	return filepath.Base(s.cfg.ConfigPath) + ".backup." + time.Now().Format("20060102-150405")
}

func (s *SingBoxService) CreateBackup() (string, error) {
	name := s.backupFileName()
	target := s.backupPath(name)
	content, err := os.ReadFile(s.cfg.ConfigPath)
	if err == nil {
		if err := s.writeTextFile(target, string(content)); err == nil {
			s.PruneBackups(20)
			return name, nil
		}
	}
	if s.cfg.CtlPath != "" {
		if err := s.copyPrivilegedFile(s.cfg.ConfigPath, target); err == nil {
			s.PruneBackups(20)
			return name, nil
		} else {
			return "", err
		}
	}
	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("create backup failed")
}
