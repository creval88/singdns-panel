package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var templateValidationDummyTags = []string{
	"__template_probe_hk__",
	"__template_probe_jp__",
	"__template_probe_sg__",
	"__template_probe_tw__",
	"__template_probe_us__",
	"__template_probe_reality__",
	"__template_probe_generic__",
}

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
	if err := s.ValidateTemplateConfig(content); err != nil {
		return nil, err
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
	binPath := strings.TrimSpace(s.cfg.BinPath)
	if binPath == "" {
		return nil
	}
	if st, err := os.Stat(binPath); err != nil || st.IsDir() {
		return nil
	}
	preview, err := normalizeTemplateConfigForValidation(content)
	if err != nil {
		return err
	}
	if err := s.ValidateConfig(preview); err != nil {
		return fmt.Errorf("模版配置与当前 sing-box 内核不兼容：%w", err)
	}
	return nil
}

func normalizeTemplateConfigForValidation(content string) (string, error) {
	var root map[string]any
	if err := json.Unmarshal([]byte(content), &root); err != nil {
		return "", fmt.Errorf("template config is not valid json: %w", err)
	}

	outbounds, _ := root["outbounds"].([]any)
	if len(outbounds) == 0 {
		return content, nil
	}

	normalized := make([]any, 0, len(outbounds)+len(templateValidationDummyTags))
	existingTags := map[string]struct{}{}
	for _, item := range outbounds {
		m, ok := item.(map[string]any)
		if !ok {
			normalized = append(normalized, item)
			continue
		}
		tag := strings.TrimSpace(asTrimmedString(m["tag"]))
		if tag != "" {
			existingTags[tag] = struct{}{}
		}
	}

	referencedDummyTags := map[string]struct{}{}
	fallbackDummy := templateValidationDummyTags[0]

	for _, item := range outbounds {
		m, ok := item.(map[string]any)
		if !ok {
			normalized = append(normalized, item)
			continue
		}

		next := cloneMap(m)
		typ := strings.TrimSpace(asTrimmedString(next["type"]))
		if typ == "selector" || typ == "urltest" {
			if expanded, changed := expandAllPlaceholder(next, templateValidationDummyTags); changed {
				for i, raw := range expanded {
					tag, _ := raw.(string)
					tag = strings.TrimSpace(tag)
					if tag == "direct" {
						if _, ok := existingTags["direct"]; !ok {
							expanded[i] = fallbackDummy
							referencedDummyTags[fallbackDummy] = struct{}{}
							continue
						}
					}
					if strings.HasPrefix(tag, "__template_probe_") {
						referencedDummyTags[tag] = struct{}{}
					}
				}
				next["outbounds"] = expanded
			}
			delete(next, "filter")
		}

		normalized = append(normalized, next)
	}

	for _, tag := range templateValidationDummyTags {
		if _, needed := referencedDummyTags[tag]; !needed {
			continue
		}
		if _, exists := existingTags[tag]; exists {
			continue
		}
		normalized = append(normalized, map[string]any{
			"type": "direct",
			"tag":  tag,
		})
	}

	root["outbounds"] = normalized
	buf, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", fmt.Errorf("template config normalize failed: %w", err)
	}
	return string(buf) + "\n", nil
}
