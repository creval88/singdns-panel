package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	cfgpkg "singdns-panel/internal/config"
)

func TestValidateConfigCenterDraftAllowsUnnamedOutboundWithType(t *testing.T) {
	t.Parallel()

	draft := &ConfigCenterDraft{
		Version: 1,
		Source:  "config",
		Outbounds: []ConfigCenterOutbound{
			{Type: "direct", Enabled: true},
		},
		RouteRules: []ConfigCenterRouteRule{
			{Position: 1, Action: "hijack-dns", Enabled: true},
		},
	}

	result := ValidateConfigCenterDraft(draft)
	if !result.OK {
		t.Fatalf("expected draft to be valid, got errors %v", result.Errors)
	}
	if !result.CanApply {
		t.Fatalf("expected draft to be applicable, got warnings %v", result.Warnings)
	}
}

func TestValidateConfigCenterDraftWarnsUnnamedOutboundWithoutType(t *testing.T) {
	t.Parallel()

	draft := &ConfigCenterDraft{
		Version: 1,
		Source:  "config",
		Outbounds: []ConfigCenterOutbound{
			{Enabled: true},
		},
	}

	result := ValidateConfigCenterDraft(draft)
	if !result.OK {
		t.Fatalf("expected draft without type to remain warning-only, got errors %v", result.Errors)
	}
	if !containsRiskItem(result.Warnings, "未命名且未声明类型的 outbound") {
		t.Fatalf("expected warning about unnamed outbound without type, got warnings %v", result.Warnings)
	}
}

func TestConfigCenterRoundTripKeepsTemplateFields(t *testing.T) {
	t.Parallel()

	config := `{
  "outbounds": [
    {
      "tag": "proxy",
      "type": "selector",
      "outbounds": ["{all}", "direct"],
      "filter": {
        "include": ["HK", "JP"],
        "exclude": ["倍率"]
      }
    },
    {
      "tag": "auto",
      "type": "urltest",
      "outbounds": ["{all}"],
      "interval": "10m",
      "tolerance": 100,
      "idle_timeout": "30m",
      "interrupt_exist_connections": false
    },
    {
      "tag": "direct",
      "type": "direct"
    }
  ],
  "route": {
    "rule_set": [
      {
        "tag": "geosite-ai",
        "type": "remote",
        "format": "binary",
        "url": "https://example.com/ai.srs",
        "download_detour": "proxy"
      }
    ],
    "rules": [
      {
        "clash_mode": "global",
        "outbound": "proxy"
      },
      {
        "network": "udp",
        "port": 443,
        "action": "reject"
      },
      {
        "ip_is_private": true,
        "outbound": "direct"
      }
    ]
  }
}`

	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	svc := &SingBoxService{cfg: cfgpkg.ServiceConfig{ConfigPath: configPath}}
	draft, err := svc.ParseConfigCenterDraft(config)
	if err != nil {
		t.Fatalf("ParseConfigCenterDraft: %v", err)
	}
	proxyOutbound := findOutboundByTag(draft.Outbounds, "proxy")
	autoOutbound := findOutboundByTag(draft.Outbounds, "auto")
	if proxyOutbound == nil || autoOutbound == nil {
		t.Fatalf("expected proxy/auto outbounds, got %#v", draft.Outbounds)
	}
	if got := joinListForTest(proxyOutbound.FilterInclude); got != "HK,JP" {
		t.Fatalf("unexpected filter include: %s", got)
	}
	if got := joinListForTest(proxyOutbound.FilterExclude); got != "倍率" {
		t.Fatalf("unexpected filter exclude: %s", got)
	}
	if autoOutbound.Interval != "10m" || autoOutbound.Tolerance != 100 || autoOutbound.IdleTimeout != "30m" {
		t.Fatalf("unexpected urltest settings: %#v", autoOutbound)
	}
	if autoOutbound.InterruptExistConnections == nil || *autoOutbound.InterruptExistConnections {
		t.Fatalf("unexpected interrupt_exist_connections: %#v", autoOutbound.InterruptExistConnections)
	}
	if draft.RuleSets[0].DownloadDetour != "proxy" {
		t.Fatalf("unexpected download detour: %#v", draft.RuleSets[0])
	}
	if draft.RouteRules[0].ClashMode != "global" {
		t.Fatalf("unexpected clash mode: %#v", draft.RouteRules[0])
	}
	if got := joinListForTest(draft.RouteRules[1].Network); got != "udp" || draft.RouteRules[1].Port != "443" {
		t.Fatalf("unexpected network/port: %#v", draft.RouteRules[1])
	}
	if draft.RouteRules[2].IPIsPrivate == nil || !*draft.RouteRules[2].IPIsPrivate {
		t.Fatalf("unexpected ip_is_private: %#v", draft.RouteRules[2].IPIsPrivate)
	}

	rebuilt, err := svc.BuildConfigCenterContentFromDraft(draft)
	if err != nil {
		t.Fatalf("BuildConfigCenterContentFromDraft: %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal([]byte(rebuilt), &root); err != nil {
		t.Fatalf("unmarshal rebuilt config: %v", err)
	}
	outbounds, _ := root["outbounds"].([]any)
	first := findMapByTag(outbounds, "proxy")
	second := findMapByTag(outbounds, "auto")
	if first == nil || second == nil {
		t.Fatalf("expected rebuilt proxy/auto outbounds, got %#v", outbounds)
	}
	filter, _ := first["filter"].(map[string]any)
	if got := joinAnyListForTest(filter["include"]); got != "HK,JP" {
		t.Fatalf("unexpected rebuilt include filter: %s", got)
	}
	if got := joinAnyListForTest(filter["exclude"]); got != "倍率" {
		t.Fatalf("unexpected rebuilt exclude filter: %s", got)
	}
	if second["interval"] != "10m" || intValue(second["tolerance"]) != 100 || second["idle_timeout"] != "30m" {
		t.Fatalf("unexpected rebuilt urltest settings: %#v", second)
	}
	if v, ok := second["interrupt_exist_connections"].(bool); !ok || v {
		t.Fatalf("unexpected rebuilt interrupt_exist_connections: %#v", second["interrupt_exist_connections"])
	}

	routeMap, _ := root["route"].(map[string]any)
	ruleSets, _ := routeMap["rule_set"].([]any)
	ruleSet, _ := ruleSets[0].(map[string]any)
	if ruleSet["download_detour"] != "proxy" {
		t.Fatalf("unexpected rebuilt download_detour: %#v", ruleSet)
	}

	rules, _ := routeMap["rules"].([]any)
	rule0, _ := rules[0].(map[string]any)
	if rule0["clash_mode"] != "global" {
		t.Fatalf("unexpected rebuilt clash_mode: %#v", rule0)
	}
	rule1, _ := rules[1].(map[string]any)
	if rule1["network"] != "udp" || intValue(rule1["port"]) != 443 {
		t.Fatalf("unexpected rebuilt network/port: %#v", rule1)
	}
	rule2, _ := rules[2].(map[string]any)
	if v, ok := rule2["ip_is_private"].(bool); !ok || !v {
		t.Fatalf("unexpected rebuilt ip_is_private: %#v", rule2["ip_is_private"])
	}
}

func TestConfigCenterChangeSummaryCountsEditableSections(t *testing.T) {
	t.Parallel()

	config := `{
  "outbounds": [
    {"tag": "proxy", "type": "selector", "outbounds": ["direct"]},
    {"tag": "direct", "type": "direct"}
  ],
  "route": {
    "rule_set": [{"tag": "geo", "type": "remote", "url": "https://example.com/geo.srs"}],
    "rules": [{"outbound": "proxy"}]
  }
}`

	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	svc := &SingBoxService{cfg: cfgpkg.ServiceConfig{ConfigPath: configPath}}
	draft, err := svc.ParseConfigCenterDraft(config)
	if err != nil {
		t.Fatalf("ParseConfigCenterDraft: %v", err)
	}
	draft.Outbounds = append(draft.Outbounds, ConfigCenterOutbound{Tag: "block", Type: "block", Enabled: true, Raw: map[string]any{}})
	draft.RouteRules = append(draft.RouteRules, ConfigCenterRouteRule{Position: 2, Outbound: "direct", Enabled: true, Raw: map[string]any{}})

	rebuilt, err := svc.BuildConfigCenterContentFromDraft(draft)
	if err != nil {
		t.Fatalf("BuildConfigCenterContentFromDraft: %v", err)
	}
	summary, err := svc.configCenterChangeSummary(rebuilt)
	if err != nil {
		t.Fatalf("configCenterChangeSummary: %v", err)
	}
	if !summary.Changed {
		t.Fatalf("expected changed summary")
	}
	if summary.OutboundsBefore != 2 || summary.OutboundsAfter != 3 {
		t.Fatalf("unexpected outbound counts: %#v", summary)
	}
	if summary.RuleSetsBefore != 1 || summary.RuleSetsAfter != 1 {
		t.Fatalf("unexpected rule set counts: %#v", summary)
	}
	if summary.RouteRulesBefore != 1 || summary.RouteRulesAfter != 2 {
		t.Fatalf("unexpected route rule counts: %#v", summary)
	}
	if summary.OldBytes == 0 || summary.NewBytes == 0 {
		t.Fatalf("expected byte counts: %#v", summary)
	}
}

func joinListForTest(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := values[0]
	for i := 1; i < len(values); i++ {
		out += "," + values[i]
	}
	return out
}

func joinAnyListForTest(v any) string {
	items, _ := v.([]any)
	values := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok && s != "" {
			values = append(values, s)
		}
	}
	return joinListForTest(values)
}

func findOutboundByTag(items []ConfigCenterOutbound, tag string) *ConfigCenterOutbound {
	for i := range items {
		if items[i].Tag == tag {
			return &items[i]
		}
	}
	return nil
}

func findMapByTag(items []any, tag string) map[string]any {
	for _, item := range items {
		m, _ := item.(map[string]any)
		if m == nil {
			continue
		}
		if m["tag"] == tag {
			return m
		}
	}
	return nil
}
