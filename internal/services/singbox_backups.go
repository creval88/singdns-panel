package services

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (s *SingBoxService) ListBackups() ([]BackupInfo, error) {
	dir := filepath.Dir(s.cfg.ConfigPath)
	base := filepath.Base(s.cfg.ConfigPath) + ".backup."
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	currentConfig, _ := s.ReadConfig()
	currentDigest := sha256.Sum256([]byte(currentConfig))
	var out []BackupInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), base) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		st, err := e.Info()
		if err != nil {
			continue
		}
		info := BackupInfo{
			Name:     e.Name(),
			Path:     p,
			ModTime:  st.ModTime().Format("2006-01-02 15:04:05"),
			Size:     st.Size(),
			SizeText: humanSize(st.Size()),
			AgeText:  time.Since(st.ModTime()).Round(time.Minute).String() + " 前",
		}
		if b, err := os.ReadFile(p); err == nil {
			info.IsCurrent = sha256.Sum256(b) == currentDigest
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	for i := range out {
		out[i].IsLatest = i == 0
	}
	return out, nil
}

func humanSize(v int64) string {
	units := []string{"B", "KB", "MB", "GB"}
	fv := float64(v)
	i := 0
	for fv >= 1024 && i < len(units)-1 {
		fv /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d %s", v, units[i])
	}
	return fmt.Sprintf("%.1f %s", fv, units[i])
}

func (s *SingBoxService) RestoreBackup(name string) (*OperationResult, error) {
	b, err := os.ReadFile(s.backupPath(name))
	if err != nil {
		return nil, err
	}
	res, err := s.SaveConfig(string(b))
	if err != nil {
		return nil, err
	}
	msg := "已回滚配置"
	if res != nil && res.Message != "" {
		msg = "已回滚配置：" + res.Message
	}
	return &OperationResult{Action: "backup.restore", Message: msg}, nil
}

func (s *SingBoxService) DeleteBackup(name string) (*OperationResult, error) {
	target := s.backupPath(name)
	if err := os.Remove(target); err == nil || os.IsNotExist(err) {
		return &OperationResult{Action: "backup.delete", Message: "备份已删除：" + name}, nil
	}
	if s.cfg.CtlPath != "" {
		if err := s.deletePrivilegedFile(target); err != nil {
			return nil, err
		}
		return &OperationResult{Action: "backup.delete", Message: "备份已删除：" + name}, nil
	}
	if err := os.Remove(target); err != nil {
		return nil, err
	}
	return &OperationResult{Action: "backup.delete", Message: "备份已删除：" + name}, nil
}
