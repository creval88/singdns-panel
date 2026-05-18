package services

import (
	"encoding/json"
	"testing"

	cfgpkg "singdns-panel/internal/config"
)

func TestInjectMonitorTestRoutingAddsManagedLocalSocksRoutes(t *testing.T) {
	t.Parallel()

	content := `{
  "inbounds": [{"type": "socks", "tag": "main-in", "listen": "0.0.0.0", "listen_port": 7891}],
  "outbounds": [
    {"type": "selector", "tag": "日本手动", "outbounds": ["jp-1"]},
    {"type": "selector", "tag": "自建节点", "outbounds": ["self-1"]},
    {"type": "direct", "tag": "direct"}
  ],
  "route": {"rules": [{"inbound": "main-in", "outbound": "direct"}]}
}`
	out, changed, err := InjectMonitorTestRouting(content, cfgpkg.MonitorConfig{
		Enabled:             true,
		QualityCheckEnabled: true,
		DownloadTestURL:     "https://proof.ovh.net/files/100Mb.dat",
		PrimaryGroup:        "日本手动",
		FallbackGroup:       "自建节点",
	})
	if err != nil {
		t.Fatalf("InjectMonitorTestRouting err: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed")
	}

	var cfg map[string]any
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	inbounds, _ := cfg["inbounds"].([]any)
	if !hasInbound(inbounds, monitorPrimaryInboundTag, monitorPrimaryPort) || !hasInbound(inbounds, monitorFallbackInboundTag, monitorFallbackPort) {
		t.Fatalf("managed monitor inbounds missing: %#v", inbounds)
	}
	route, _ := cfg["route"].(map[string]any)
	rules, _ := route["rules"].([]any)
	if len(rules) < 2 {
		t.Fatalf("managed route rules missing: %#v", rules)
	}
	if !hasInboundRoute(rules, monitorPrimaryInboundTag, "日本手动") || !hasInboundRoute(rules, monitorFallbackInboundTag, "自建节点") {
		t.Fatalf("managed route rules not found: %#v", rules)
	}
}

func TestInjectMonitorTestRoutingUpdatesManagedRoutesAfterGroupChange(t *testing.T) {
	t.Parallel()

	content := `{
  "inbounds": [
    {"type": "socks", "tag": "singdns-monitor-primary-in", "listen": "127.0.0.1", "listen_port": 7892},
    {"type": "socks", "tag": "singdns-monitor-fallback-in", "listen": "127.0.0.1", "listen_port": 7893}
  ],
  "outbounds": [
    {"type": "selector", "tag": "新主组", "outbounds": ["a"]},
    {"type": "selector", "tag": "新备组", "outbounds": ["b"]}
  ],
  "route": {"rules": [
    {"inbound": "singdns-monitor-primary-in", "outbound": "旧主组"},
    {"inbound": "singdns-monitor-fallback-in", "outbound": "旧备组"}
  ]}
}`
	out, _, err := InjectMonitorTestRouting(content, cfgpkg.MonitorConfig{
		Enabled:             true,
		QualityCheckEnabled: true,
		DownloadTestURL:     "https://proof.ovh.net/files/100Mb.dat",
		PrimaryGroup:        "新主组",
		FallbackGroup:       "新备组",
	})
	if err != nil {
		t.Fatalf("InjectMonitorTestRouting err: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	route, _ := cfg["route"].(map[string]any)
	rules, _ := route["rules"].([]any)
	if !hasInboundRoute(rules, monitorPrimaryInboundTag, "新主组") || !hasInboundRoute(rules, monitorFallbackInboundTag, "新备组") {
		t.Fatalf("managed route rules not updated: %#v", rules)
	}
}

func hasInbound(items []any, tag string, port int) bool {
	for _, item := range items {
		m, _ := item.(map[string]any)
		if m == nil {
			continue
		}
		if m["tag"] == tag && int(m["listen_port"].(float64)) == port && m["listen"] == "127.0.0.1" {
			return true
		}
	}
	return false
}

func hasInboundRoute(items []any, inbound, outbound string) bool {
	for _, item := range items {
		m, _ := item.(map[string]any)
		if m == nil {
			continue
		}
		if m["inbound"] == inbound && m["outbound"] == outbound {
			return true
		}
	}
	return false
}
