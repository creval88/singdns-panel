package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cfgpkg "singdns-panel/internal/config"
)

func TestBuildConfigFromSubscription_MergesManualDraftNodes(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.json")
	templatePath := filepath.Join(tmp, "template.json")

	template := `{
  "outbounds": [
    {"type":"selector","tag":"默认代理","outbounds":["{all}","direct"]},
    {"type":"direct","tag":"direct"}
  ]
}`
	if err := os.WriteFile(templatePath, []byte(template), 0644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(template), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	s := &SingBoxService{cfg: cfgpkg.ServiceConfig{ConfigPath: configPath, TemplatePath: templatePath}}
	manualLine := "vless://ce1c7040-f250-4fc1-9a6e-555ab2d14ab7@sssss.ss888.online:29858?security=reality&sni=www.tesla.com&pbk=swvPMHYCpCCZionjEkjLnngAQyhrsHka4mZcCp4-0gY&sid=9054d0&fp=chrome&flow=xtls-rprx-vision#manual-node"
	if _, err := s.SaveManualNodesDraft(manualLine); err != nil {
		t.Fatalf("SaveManualNodesDraft: %v", err)
	}

	subscription := "ss://MjAyMi1ibGFrZTMtYWVzLTEyOC1nY206akdud1kwUGJzV3hUcUF6bmd3aWpkQT09OkFXRHRDYlBjVklBemJHc2UyRmJ0Tnc9PQ@b7e3d98.mnmjutnn.sbs:19300/?group=QW15VGVsZWNvbQ#sub-node"
	merged, summary, err := s.BuildConfigFromSubscription("", subscription)
	if err != nil {
		t.Fatalf("BuildConfigFromSubscription: %v", err)
	}
	if !strings.Contains(merged, "manual-node") {
		t.Fatalf("merged config missing manual node: %s", merged)
	}
	if !strings.Contains(merged, "sub-node") {
		t.Fatalf("merged config missing subscription node: %s", merged)
	}
	if summary == nil || summary.ParsedNodeCount < 2 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}

func TestListBackups_ExcludesSubscriptionRollbackSlot(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"outbounds":[]}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	s := &SingBoxService{cfg: cfgpkg.ServiceConfig{ConfigPath: configPath}}

	if _, err := s.CreateBackup(); err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if _, err := s.CreateSubscriptionRollbackBackup(); err != nil {
		t.Fatalf("CreateSubscriptionRollbackBackup: %v", err)
	}

	items, err := s.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected only manual backup in list, got %d: %#v", len(items), items)
	}
	if items[0].Name == s.subscriptionRollbackFileName() {
		t.Fatalf("rollback slot should be hidden from manual backup list: %#v", items)
	}
}
