package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cfgpkg "singdns-panel/internal/config"
)

func TestValidateTemplateConfigRejectsIncompatibleCurrentCore(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	binPath := filepath.Join(tmp, "sing-box")
	script := "#!/bin/sh\nif [ \"$1\" = \"check\" ]; then\n  echo 'inbounds[0]: unknown inbound type: xdp' 1>&2\n  exit 1\nfi\nexit 0\n"
	if err := os.WriteFile(binPath, []byte(script), 0755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}

	svc := &SingBoxService{cfg: cfgpkg.ServiceConfig{BinPath: binPath}}
	template := `{"inbounds":[{"type":"xdp"}],"outbounds":[{"type":"direct"}]}`

	err := svc.ValidateTemplateConfig(template)
	if err == nil {
		t.Fatal("expected compatibility validation to fail")
	}
	if !strings.Contains(err.Error(), "模版配置与当前 sing-box 内核不兼容") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "unknown inbound type: xdp") {
		t.Fatalf("expected xdp compatibility detail, got: %v", err)
	}
}

func TestValidateTemplateConfigAllowsTemplateFilterSyntax(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	binPath := filepath.Join(tmp, "sing-box")
	script := "#!/bin/sh\ncfg=\"$3\"\nif grep -q '\"filter\"' \"$cfg\"; then\n  echo 'filter should have been removed before validation' 1>&2\n  exit 1\nfi\nif grep -q '\"{all}\"' \"$cfg\"; then\n  echo 'placeholder should have been expanded before validation' 1>&2\n  exit 1\nfi\nexit 0\n"
	if err := os.WriteFile(binPath, []byte(script), 0755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}

	svc := &SingBoxService{cfg: cfgpkg.ServiceConfig{BinPath: binPath}}
	template := `{
  "outbounds": [
    {
      "type": "selector",
      "tag": "香港手动",
      "outbounds": ["{all}"],
      "filter": [
        {"action":"include","keywords":["HK|香港"]}
      ]
    },
    {
      "type": "selector",
      "tag": "默认代理",
      "outbounds": ["香港手动"]
    }
  ]
}`

	if err := svc.ValidateTemplateConfig(template); err != nil {
		t.Fatalf("expected template filter syntax to pass, got: %v", err)
	}
}

func TestValidateTemplateConfigSkipsCompatibilityWhenBinaryMissing(t *testing.T) {
	t.Parallel()

	svc := &SingBoxService{cfg: cfgpkg.ServiceConfig{BinPath: filepath.Join(t.TempDir(), "missing-sing-box")}}
	template := `{"inbounds":[{"type":"xdp"}],"outbounds":[{"type":"direct"}]}`

	if err := svc.ValidateTemplateConfig(template); err != nil {
		t.Fatalf("expected missing binary to skip compatibility check, got: %v", err)
	}
}
