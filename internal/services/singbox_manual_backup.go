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

func (s *SingBoxService) subscriptionRollbackFileName() string {
	return filepath.Base(s.cfg.ConfigPath) + ".subscription.rollback"
}

func (s *SingBoxService) copyConfigToPath(target string) error {
	content, err := os.ReadFile(s.cfg.ConfigPath)
	if err == nil {
		if err := s.writeTextFile(target, string(content)); err == nil {
			return nil
		}
	}
	if s.cfg.CtlPath != "" {
		if err := s.copyPrivilegedFile(s.cfg.ConfigPath, target); err == nil {
			return nil
		} else {
			return err
		}
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("copy config failed")
}

func (s *SingBoxService) restoreConfigFromPath(source string) error {
	content, err := os.ReadFile(source)
	if err == nil {
		return s.writeConfigFile(string(content))
	}
	if s.cfg.CtlPath != "" {
		if err := s.copyPrivilegedFile(source, s.cfg.ConfigPath); err == nil {
			return nil
		} else {
			return err
		}
	}
	return err
}

func (s *SingBoxService) CreateBackup() (string, error) {
	name := s.backupFileName()
	target := s.backupPath(name)
	if err := s.copyConfigToPath(target); err != nil {
		return "", err
	}
	s.PruneBackups(20)
	return name, nil
}

func (s *SingBoxService) CreateSubscriptionRollbackBackup() (string, error) {
	name := s.subscriptionRollbackFileName()
	target := s.backupPath(name)
	if err := s.copyConfigToPath(target); err != nil {
		return "", err
	}
	return name, nil
}

func (s *SingBoxService) RestoreSubscriptionRollbackBackup() error {
	return s.restoreConfigFromPath(s.backupPath(s.subscriptionRollbackFileName()))
}
