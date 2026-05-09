package services

import (
	"os"
	"path/filepath"
	"testing"

	cfgpkg "singdns-panel/internal/config"
)

func TestBuildSubscriptionSourceSummary_ClassifiesManagedNodes(t *testing.T) {
	tmp := t.TempDir()
	svc := &SingBoxService{cfg: cfgpkg.ServiceConfig{ConfigPath: filepath.Join(tmp, "config.json")}}

	configText := `{
	  "outbounds": [
	    {"type": "selector", "tag": "🚀 自建节点", "outbounds": ["manual-node", "🎯 全球直连"]},
	    {"type": "selector", "tag": "🌐 全部节点", "outbounds": ["manual-node", "sub-node-1", "🎯 全球直连"]},
	    {"type": "direct", "tag": "🎯 全球直连"},
	    {"type": "vless", "tag": "manual-node", "server": "manual.example.com"},
	    {"type": "vmess", "tag": "sub-node-1", "server": "sub.example.com"}
	  ]
	}`
	if err := os.WriteFile(svc.cfg.ConfigPath, []byte(configText), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(svc.managedTagsStatePath()), 0755); err != nil {
		t.Fatalf("mkdir managed state dir: %v", err)
	}
	if err := os.WriteFile(svc.managedTagsStatePath(), []byte("[\"manual-node\",\"sub-node-1\"]"), 0644); err != nil {
		t.Fatalf("write managed tags: %v", err)
	}
	if err := os.WriteFile(svc.manualNodesPath(), []byte("vless://11111111-1111-1111-1111-111111111111@manual.example.com:443?security=reality&sni=example.com&pbk=abcdef&type=tcp#manual-node\n"), 0644); err != nil {
		t.Fatalf("write manual nodes draft: %v", err)
	}

	summary, err := svc.buildSubscriptionSourceSummary("", []string{"https://example.com/sub"})
	if err != nil {
		t.Fatalf("buildSubscriptionSourceSummary error: %v", err)
	}
	if summary.ActiveMode != "nodes_template" {
		t.Fatalf("ActiveMode = %q, want nodes_template", summary.ActiveMode)
	}
	if summary.ManagedNodeCount != 2 {
		t.Fatalf("ManagedNodeCount = %d, want 2", summary.ManagedNodeCount)
	}
	if summary.ManualNodeCount != 1 {
		t.Fatalf("ManualNodeCount = %d, want 1", summary.ManualNodeCount)
	}
	if summary.SubscriptionCount != 1 {
		t.Fatalf("SubscriptionCount = %d, want 1", summary.SubscriptionCount)
	}
	if summary.SelfBuiltCount != 1 {
		t.Fatalf("SelfBuiltCount = %d, want 1", summary.SelfBuiltCount)
	}
	if summary.MatchedGroupCount != 2 {
		t.Fatalf("MatchedGroupCount = %d, want 2", summary.MatchedGroupCount)
	}
	if len(summary.Nodes) != 2 {
		t.Fatalf("len(Nodes) = %d, want 2", len(summary.Nodes))
	}

	nodeByTag := map[string]SubscriptionSourceNode{}
	for _, node := range summary.Nodes {
		nodeByTag[node.Tag] = node
	}
	if nodeByTag["manual-node"].Source != "manual" {
		t.Fatalf("manual-node source = %q, want manual", nodeByTag["manual-node"].Source)
	}
	if !nodeByTag["manual-node"].SelfBuilt {
		t.Fatalf("manual-node should be marked self-built")
	}
	if nodeByTag["sub-node-1"].Source != "subscription" {
		t.Fatalf("sub-node-1 source = %q, want subscription", nodeByTag["sub-node-1"].Source)
	}
}

func TestBuildSubscriptionSourceSummary_FullConfigOnlyAddsNote(t *testing.T) {
	tmp := t.TempDir()
	svc := &SingBoxService{cfg: cfgpkg.ServiceConfig{ConfigPath: filepath.Join(tmp, "config.json")}}
	if err := os.WriteFile(svc.cfg.ConfigPath, []byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	summary, err := svc.buildSubscriptionSourceSummary("https://example.com/config.json", nil)
	if err != nil {
		t.Fatalf("buildSubscriptionSourceSummary error: %v", err)
	}
	if summary.ActiveMode != "full_config" {
		t.Fatalf("ActiveMode = %q, want full_config", summary.ActiveMode)
	}
	if summary.ManagedNodeCount != 0 {
		t.Fatalf("ManagedNodeCount = %d, want 0", summary.ManagedNodeCount)
	}
	if len(summary.Notes) == 0 {
		t.Fatalf("expected full config note")
	}
}
