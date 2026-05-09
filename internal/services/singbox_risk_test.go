package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cfgpkg "singdns-panel/internal/config"
)

func TestConfigRiskReportAllowsUntaggedOutbound(t *testing.T) {
	t.Parallel()

	currentConfig := `{
  "dns": {
    "servers": [
      { "tag": "local", "address": "local" }
    ]
  },
  "outbounds": [
    { "type": "direct" }
  ]
}`

	configPath := writeRiskConfigFixture(t, currentConfig)
	svc := &SingBoxService{cfg: cfgpkg.ServiceConfig{ConfigPath: configPath}}

	report, err := svc.ConfigRiskReport(currentConfig)
	if err != nil {
		t.Fatalf("ConfigRiskReport returned error: %v", err)
	}
	if report.Level != "ok" {
		t.Fatalf("expected ok level, got %q with items %v", report.Level, report.Items)
	}
	for _, item := range report.Items {
		if strings.Contains(item, "outbounds 为空") {
			t.Fatalf("expected no empty outbounds warning, got items %v", report.Items)
		}
	}
}

func TestConfigRiskReportFlagsRemovedTaggedOutbound(t *testing.T) {
	t.Parallel()

	oldConfig := `{
  "dns": {
    "servers": [
      { "tag": "local", "address": "local" }
    ]
  },
  "outbounds": [
    { "type": "direct", "tag": "direct" },
    { "type": "block", "tag": "block" }
  ]
}`
	newConfig := `{
  "dns": {
    "servers": [
      { "tag": "local", "address": "local" }
    ]
  },
  "outbounds": [
    { "type": "direct", "tag": "direct" }
  ]
}`

	configPath := writeRiskConfigFixture(t, oldConfig)
	svc := &SingBoxService{cfg: cfgpkg.ServiceConfig{ConfigPath: configPath}}

	report, err := svc.ConfigRiskReport(newConfig)
	if err != nil {
		t.Fatalf("ConfigRiskReport returned error: %v", err)
	}
	if report.Level != "bad" {
		t.Fatalf("expected bad level, got %q with items %v", report.Level, report.Items)
	}
	if !containsRiskItem(report.Items, "移除了出站 tag：block") {
		t.Fatalf("expected removed outbound tag warning, got items %v", report.Items)
	}
}

func writeRiskConfigFixture(t *testing.T, content string) string {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	return configPath
}

func containsRiskItem(items []string, want string) bool {
	for _, item := range items {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}
