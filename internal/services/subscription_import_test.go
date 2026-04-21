package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cfgpkg "singdns-panel/internal/config"
)

func TestMergeSubscriptionNodesIntoConfig_FiltersNodesByTemplateRules(t *testing.T) {
	svc := &SingBoxService{cfg: cfgpkg.ServiceConfig{ConfigPath: "/tmp/config.json"}}
	base := `{
	  "outbounds": [
	    {
	      "type": "selector",
	      "tag": "proxy",
	      "outbounds": ["{all}", "direct"],
	      "filter": {
	        "include": ["HK", "JP"],
	        "exclude": ["倍率"]
	      }
	    },
	    {
	      "type": "direct",
	      "tag": "direct"
	    }
	  ]
	}`
	nodes := []map[string]any{
		{"type": "vmess", "tag": "HK-01", "server": "hk.example.com", "server_port": 443, "uuid": "u1"},
		{"type": "vmess", "tag": "JP-01", "server": "jp.example.com", "server_port": 443, "uuid": "u2"},
		{"type": "vmess", "tag": "US-01", "server": "us.example.com", "server_port": 443, "uuid": "u3"},
		{"type": "vmess", "tag": "HK-倍率", "server": "hk2.example.com", "server_port": 443, "uuid": "u4"},
	}

	merged, summary, err := svc.mergeSubscriptionNodesIntoConfig(base, nodes)
	if err != nil {
		t.Fatalf("mergeSubscriptionNodesIntoConfig error: %v", err)
	}
	if summary == nil {
		t.Fatalf("summary is nil")
	}
	if summary.ParsedNodeCount != 4 {
		t.Fatalf("ParsedNodeCount = %d, want 4", summary.ParsedNodeCount)
	}
	if got := strings.Join(summary.ManagedTags, ","); got != "HK-01,JP-01" {
		t.Fatalf("ManagedTags = %s, want HK-01,JP-01", got)
	}

	var cfg map[string]any
	if err := json.Unmarshal([]byte(merged), &cfg); err != nil {
		t.Fatalf("unmarshal merged config: %v", err)
	}
	outbounds, _ := cfg["outbounds"].([]any)
	if len(outbounds) != 4 {
		t.Fatalf("outbounds len = %d, want 4", len(outbounds))
	}

	var selector map[string]any
	seenTags := map[string]bool{}
	for _, item := range outbounds {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := m["tag"].(string)
		if tag != "" {
			seenTags[tag] = true
		}
		if tag == "proxy" {
			selector = m
		}
	}
	if selector == nil {
		t.Fatalf("selector proxy not found")
	}
	if seenTags["US-01"] {
		t.Fatalf("unexpected unfiltered node US-01 present in final outbounds")
	}
	if seenTags["HK-倍率"] {
		t.Fatalf("unexpected excluded node HK-倍率 present in final outbounds")
	}
	if !seenTags["HK-01"] || !seenTags["JP-01"] {
		t.Fatalf("expected filtered nodes HK-01 and JP-01 to be present")
	}

	selectorOutbounds, _ := selector["outbounds"].([]any)
	gotOrder := make([]string, 0, len(selectorOutbounds))
	for _, item := range selectorOutbounds {
		if s, ok := item.(string); ok {
			gotOrder = append(gotOrder, s)
		}
	}
	wantOrder := []string{"HK-01", "JP-01", "direct"}
	if strings.Join(gotOrder, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("selector outbounds = %v, want %v", gotOrder, wantOrder)
	}
}

func TestMergeSubscriptionNodesIntoConfig_MatchesManualRealityNodeByMetadata(t *testing.T) {
	svc := &SingBoxService{cfg: cfgpkg.ServiceConfig{ConfigPath: "/tmp/config.json"}}
	base := `{
	  "outbounds": [
	    {
	      "type": "selector",
	      "tag": "自建节点",
	      "outbounds": ["{all}"],
	      "filter": {
	        "include": ["Reality"]
	      }
	    },
	    {
	      "type": "direct",
	      "tag": "🎯 全球直连"
	    }
	  ]
	}`
	nodes := []map[string]any{
		{
			"type":        "vless",
			"tag":         "manual-node",
			"server":      "example.com",
			"server_port": 443,
			"uuid":        "u1",
			"tls": map[string]any{
				"enabled": true,
				"reality": map[string]any{"enabled": true},
			},
		},
	}

	merged, _, err := svc.mergeSubscriptionNodesIntoConfig(base, nodes)
	if err != nil {
		t.Fatalf("mergeSubscriptionNodesIntoConfig error: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal([]byte(merged), &cfg); err != nil {
		t.Fatalf("unmarshal merged config: %v", err)
	}
	outbounds, _ := cfg["outbounds"].([]any)
	var selfBuilt map[string]any
	for _, item := range outbounds {
		m, _ := item.(map[string]any)
		if m == nil {
			continue
		}
		if m["tag"] == "自建节点" {
			selfBuilt = m
			break
		}
	}
	if selfBuilt == nil {
		t.Fatalf("self-built group not found")
	}
	selected, _ := selfBuilt["outbounds"].([]any)
	if len(selected) != 1 || selected[0] != "manual-node" {
		t.Fatalf("unexpected self-built outbounds: %#v", selected)
	}
}

func TestBuildConfigFromSubscription_SanitizesFullConfigProviderFields(t *testing.T) {
	tmp := t.TempDir()
	binPath := filepath.Join(tmp, "sing-box")
	script := "#!/bin/sh\nif [ \"$1\" = \"check\" ]; then\n  echo 'FATAL decode config at /tmp/check.json: outbounds[0].excludes: json: unknown field \"excludes\"' 1>&2\n  exit 1\nfi\nexit 0\n"
	if err := os.WriteFile(binPath, []byte(script), 0755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	svc := &SingBoxService{cfg: cfgpkg.ServiceConfig{ConfigPath: filepath.Join(tmp, "config.json"), BinPath: binPath}}

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
		  "outbounds": [
		    {"type": "vmess", "tag": "HK-01", "server": "hk.example.com", "server_port": 443, "uuid": "u1"},
		    {"type": "vmess", "tag": "JP-01", "server": "jp.example.com", "server_port": 443, "uuid": "u2"},
		    {"type": "vmess", "tag": "HK-倍率", "server": "hk2.example.com", "server_port": 443, "uuid": "u3"}
		  ]
		}`))
	}))
	defer provider.Close()

	fullConfig := `{
	  "log": {"level": "info"},
	  "outbound_providers": [
	    {"tag": "demo", "type": "remote", "url": "` + provider.URL + `", "path": "./demo.yaml"}
	  ],
	  "outbounds": [
	    {"type": "urltest", "tag": "auto", "providers": ["demo"], "includes": ["HK|JP"], "excludes": ["倍率"]},
	    {"type": "direct", "tag": "🎯 全球直连"}
	  ]
	}`

	got, summary, err := svc.BuildConfigFromSubscription("https://example.com/config.json", fullConfig)
	if err != nil {
		t.Fatalf("BuildConfigFromSubscription error: %v", err)
	}
	if summary == nil || summary.Mode != "full_config" {
		t.Fatalf("summary = %#v, want full_config", summary)
	}
	if len(summary.Compatibility) == 0 {
		t.Fatalf("expected compatibility note, got %#v", summary)
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(got), &raw); err != nil {
		t.Fatalf("unmarshal sanitized config: %v", err)
	}
	if _, ok := raw["outbound_providers"]; ok {
		t.Fatalf("outbound_providers should be removed: %#v", raw)
	}
	outbounds, ok := raw["outbounds"].([]any)
	if !ok {
		t.Fatalf("outbounds missing after sanitize: %#v", raw)
	}
	if len(outbounds) != 5 {
		t.Fatalf("outbounds len = %d, want 5", len(outbounds))
	}

	var auto map[string]any
	seenTags := map[string]bool{}
	for _, item := range outbounds {
		m, _ := item.(map[string]any)
		if m == nil {
			continue
		}
		tag, _ := m["tag"].(string)
		if tag != "" {
			seenTags[tag] = true
		}
		if tag == "auto" {
			auto = m
		}
	}
	if auto == nil {
		t.Fatalf("auto outbound missing: %#v", outbounds)
	}
	if _, ok := auto["providers"]; ok {
		t.Fatalf("providers should be removed after expansion: %#v", auto)
	}
	if _, ok := auto["includes"]; ok {
		t.Fatalf("includes should be removed after expansion: %#v", auto)
	}
	if _, ok := auto["excludes"]; ok {
		t.Fatalf("excludes should be removed after expansion: %#v", auto)
	}

	groupOutbounds, _ := auto["outbounds"].([]any)
	gotTags := make([]string, 0, len(groupOutbounds))
	for _, item := range groupOutbounds {
		if s, ok := item.(string); ok {
			gotTags = append(gotTags, s)
		}
	}
	if strings.Join(gotTags, ",") != "HK-01,JP-01" {
		t.Fatalf("group outbounds = %v, want [HK-01 JP-01]", gotTags)
	}
	if !seenTags["HK-01"] || !seenTags["JP-01"] || !seenTags["HK-倍率"] {
		t.Fatalf("expected provider nodes appended to outbounds, got %#v", seenTags)
	}
	if !strings.Contains(strings.Join(summary.Compatibility, "；"), "已展开 provider demo：3 个节点") {
		t.Fatalf("expected provider expansion note, got %#v", summary.Compatibility)
	}
}

func TestBuildConfigFromSubscription_KeepsFullConfigProviderFieldsWhenCoreSupportsThem(t *testing.T) {
	tmp := t.TempDir()
	binPath := filepath.Join(tmp, "sing-box")
	script := "#!/bin/sh\nif [ \"$1\" = \"check\" ]; then\n  exit 0\nfi\nexit 0\n"
	if err := os.WriteFile(binPath, []byte(script), 0755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	svc := &SingBoxService{cfg: cfgpkg.ServiceConfig{ConfigPath: filepath.Join(tmp, "config.json"), BinPath: binPath}}

	fullConfig := `{
	  "log": {"level": "info"},
	  "outbound_providers": [
	    {"tag": "demo", "type": "remote", "url": "http://127.0.0.1:1/unreachable", "path": "./demo.yaml"}
	  ],
	  "outbounds": [
	    {"type": "urltest", "tag": "auto", "providers": ["demo"], "includes": ["HK|JP"], "excludes": ["倍率"]},
	    {"type": "direct", "tag": "🎯 全球直连"}
	  ]
	}`

	got, summary, err := svc.BuildConfigFromSubscription("https://example.com/config.json", fullConfig)
	if err != nil {
		t.Fatalf("BuildConfigFromSubscription error: %v", err)
	}
	if summary == nil || summary.Mode != "full_config" {
		t.Fatalf("summary = %#v, want full_config", summary)
	}
	if !strings.Contains(strings.Join(summary.Compatibility, "；"), "当前内核原生支持 provider 扩展字段") {
		t.Fatalf("expected native provider support note, got %#v", summary.Compatibility)
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(got), &raw); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if _, ok := raw["outbound_providers"]; !ok {
		t.Fatalf("outbound_providers should be kept for compatible core: %#v", raw)
	}
	outbounds, _ := raw["outbounds"].([]any)
	if len(outbounds) != 2 {
		t.Fatalf("outbounds len = %d, want 2", len(outbounds))
	}
	first, _ := outbounds[0].(map[string]any)
	if first == nil {
		t.Fatalf("first outbound missing")
	}
	if _, ok := first["providers"]; !ok {
		t.Fatalf("providers should be kept for compatible core: %#v", first)
	}
	if _, ok := first["includes"]; !ok {
		t.Fatalf("includes should be kept for compatible core: %#v", first)
	}
	if _, ok := first["excludes"]; !ok {
		t.Fatalf("excludes should be kept for compatible core: %#v", first)
	}
}

func TestClearManagedTagsState_AllowsFullConfigMode(t *testing.T) {
	tmp := t.TempDir()
	svc := &SingBoxService{cfg: cfgpkg.ServiceConfig{ConfigPath: filepath.Join(tmp, "config.json")}}
	if err := os.WriteFile(svc.cfg.ConfigPath, []byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := svc.writeManagedTagsState([]string{"old-node"}); err != nil {
		t.Fatalf("write managed tags: %v", err)
	}
	if err := svc.clearManagedTagsState(); err != nil {
		t.Fatalf("clearManagedTagsState error: %v", err)
	}

	summary, err := svc.buildSubscriptionSourceSummary("https://example.com/full.json", []string{"https://example.com/nodes"})
	if err != nil {
		t.Fatalf("buildSubscriptionSourceSummary error: %v", err)
	}
	if summary.ActiveMode != "full_config" {
		t.Fatalf("ActiveMode = %q, want full_config", summary.ActiveMode)
	}
}
