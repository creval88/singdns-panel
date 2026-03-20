package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveReplacesExistingFileAtomically(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(cfgDir, 0750); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}

	path := filepath.Join(cfgDir, "panel.json")
	if err := os.WriteFile(path, []byte(`{"listen":":1111"}`), 0400); err != nil {
		t.Fatalf("seed panel.json: %v", err)
	}

	cfg := &Config{
		Listen:     ":9999",
		SessionKey: "k",
		AuditLog:   "logs/audit.log",
		Auth: AuthConfig{
			Username:     "admin",
			PasswordHash: "hash",
		},
	}

	if err := cfg.Save(path); err != nil {
		t.Fatalf("save config: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read panel.json: %v", err)
	}
	if !strings.Contains(string(b), `"listen": ":9999"`) {
		t.Fatalf("unexpected content: %s", string(b))
	}

	entries, err := os.ReadDir(cfgDir)
	if err != nil {
		t.Fatalf("read configs dir: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".panel.json.tmp-") {
			t.Fatalf("temp file not cleaned: %s", entry.Name())
		}
	}
}
