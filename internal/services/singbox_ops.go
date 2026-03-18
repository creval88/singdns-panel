package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"singdns-panel/internal/utils"
)

type OperationResult struct {
	Action  string
	Message string
}

func (r OperationResult) AuditText() string {
	msg := strings.TrimSpace(r.Message)
	if msg == "" {
		return "ok"
	}
	return msg
}

func (s *SingBoxService) runCtl(timeout time.Duration, args ...string) error {
	if s.cfg.CtlPath == "" {
		return fmt.Errorf("control script is not configured")
	}
	_, err := utils.Run(timeout, "sudo", append([]string{s.cfg.CtlPath}, args...)...)
	return err
}

func (s *SingBoxService) writePrivilegedFile(content, targetPath string) error {
	tmp, err := os.CreateTemp("", "singdns-panel-write-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return s.runCtl(15*time.Second, "write-file", tmpName, targetPath)
}

func (s *SingBoxService) copyPrivilegedFile(srcPath, targetPath string) error {
	return s.runCtl(15*time.Second, "copy-file", srcPath, targetPath)
}

func (s *SingBoxService) deletePrivilegedFile(targetPath string) error {
	return s.runCtl(10*time.Second, "delete-file", targetPath)
}

func (s *SingBoxService) writeTextFile(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".singdns-panel-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func (s *SingBoxService) writeManagedFile(path, content string) error {
	if err := s.writeTextFile(path, content); err == nil {
		return nil
	}
	return s.writePrivilegedFile(content, path)
}

func (s *SingBoxService) runCommandWithInput(timeout time.Duration, input string, name string, args ...string) (*utils.CommandResult, error) {
	ctxTimeout := timeout
	if ctxTimeout <= 0 {
		ctxTimeout = 10 * time.Second
	}
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()

	select {
	case err := <-done:
		res := &utils.CommandResult{Stdout: stdout.String(), Stderr: stderr.String()}
		if err != nil {
			return res, fmt.Errorf("run %s %v: %w", name, args, err)
		}
		return res, nil
	case <-time.After(ctxTimeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return &utils.CommandResult{Stdout: stdout.String(), Stderr: stderr.String()}, fmt.Errorf("run %s %v: timeout after %s", name, args, ctxTimeout)
	}
}
