package services

import (
	"os"
	"path/filepath"
	"testing"

	cfgpkg "singdns-panel/internal/config"
)

func TestExpandAllPlaceholderFallbackDirectWhenEmpty(t *testing.T) {
	group := map[string]any{
		"type":      "selector",
		"outbounds": []any{"{all}"},
		"filter": []any{
			map[string]any{"action": "include", "keywords": []any{"HK"}},
		},
	}
	out, changed := expandAllPlaceholder(group, []string{"US-1"})
	if !changed {
		t.Fatalf("expected changed")
	}
	if len(out) != 1 || out[0] != "direct" {
		t.Fatalf("out=%#v, want [direct]", out)
	}
}

func TestReadManagedTagsStateFallsBackToLegacyPath(t *testing.T) {
	tmp := t.TempDir()
	svc := &SingBoxService{cfg: cfgpkg.ServiceConfig{ConfigPath: filepath.Join(tmp, "config.json")}}
	if err := os.WriteFile(svc.legacyManagedTagsStatePath(), []byte("[\"a\",\"b\"]"), 0644); err != nil {
		t.Fatalf("write legacy managed tags: %v", err)
	}

	got := svc.readManagedTagsState()
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected tags: %#v", got)
	}
}

func TestTemplateDirectFallbackTagPrefersTaggedDirectOutbound(t *testing.T) {
	outbounds := []any{
		map[string]any{"tag": "proxy", "type": "selector", "outbounds": []any{"{all}"}},
		map[string]any{"tag": "🎯 全球直连", "type": "direct"},
	}
	fallbackTag := templateDirectFallbackTag(outbounds)
	if fallbackTag != "🎯 全球直连" {
		t.Fatalf("unexpected fallback tag: %q", fallbackTag)
	}

	group := map[string]any{
		"type":      "selector",
		"outbounds": []any{"{all}"},
		"filter": []any{
			map[string]any{"action": "include", "keywords": []any{"Reality"}},
		},
	}
	out, changed := expandAllPlaceholderWithFallback(group, []string{"普通节点"}, fallbackTag)
	if !changed {
		t.Fatalf("expected changed")
	}
	if len(out) != 1 || out[0] != "🎯 全球直连" {
		t.Fatalf("out=%#v, want [🎯 全球直连]", out)
	}
}
