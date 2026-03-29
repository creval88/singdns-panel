package services

import (
	"encoding/json"
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
