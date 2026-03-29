package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s *SingBoxService) templateConfigPath() string {
	if p := strings.TrimSpace(s.cfg.TemplatePath); p != "" {
		return p
	}
	baseDir := filepath.Dir(strings.TrimSpace(s.cfg.ConfigPath))
	if baseDir == "" || baseDir == "." {
		baseDir = "/etc/sing-box"
	}
	return filepath.Join(baseDir, "template.json")
}

func (s *SingBoxService) ReadTemplateConfig() (string, error) {
	p := s.templateConfigPath()
	if b, err := os.ReadFile(p); err == nil {
		return string(b), nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	// 模板不存在时，回退显示当前完整配置，避免页面空白
	return s.ReadConfig()
}

func (s *SingBoxService) SaveTemplateConfig(content string) (*OperationResult, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("template config is empty")
	}
	var js any
	if err := json.Unmarshal([]byte(content), &js); err != nil {
		return nil, fmt.Errorf("template config is not valid json: %w", err)
	}
	norm, _ := json.MarshalIndent(js, "", "  ")
	if err := s.writeManagedFile(s.templateConfigPath(), string(norm)+"\n"); err != nil {
		return nil, err
	}
	return &OperationResult{Action: "template.save", Message: "模版配置已保存"}, nil
}

func (s *SingBoxService) ValidateTemplateConfig(content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("template config is empty")
	}
	var js any
	if err := json.Unmarshal([]byte(content), &js); err != nil {
		return fmt.Errorf("template config is not valid json: %w", err)
	}
	return nil
}
