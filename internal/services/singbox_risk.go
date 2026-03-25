package services

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ConfigRiskReport struct {
	Level   string   `json:"level"`
	Summary string   `json:"summary"`
	Items   []string `json:"items"`
}

func (s *SingBoxService) ConfigRiskReport(content string) (*ConfigRiskReport, error) {
	newCfg := map[string]any{}
	if err := json.Unmarshal([]byte(content), &newCfg); err != nil {
		return nil, fmt.Errorf("草稿不是合法 JSON：%w", err)
	}

	oldCfg := map[string]any{}
	if oldText, err := s.ReadConfig(); err == nil {
		_ = json.Unmarshal([]byte(oldText), &oldCfg)
	}

	oldOutbounds := outboundTags(oldCfg)
	newOutbounds := outboundTags(newCfg)
	removedTags := diffStrings(oldOutbounds, newOutbounds)

	oldRouteFinal := asTrimmedString(routeField(oldCfg, "final"))
	newRouteFinal := asTrimmedString(routeField(newCfg, "final"))

	oldRouteRules := len(routeRules(oldCfg))
	newRouteRules := len(routeRules(newCfg))
	oldDNSServers := len(dnsServers(oldCfg))
	newDNSServers := len(dnsServers(newCfg))

	var high, warn []string

	if newDNSServers == 0 {
		high = append(high, "高风险：dns.servers 为空，解析将不可用")
	} else if oldDNSServers != newDNSServers {
		warn = append(warn, fmt.Sprintf("注意：dns.servers 数量从 %d 变为 %d", oldDNSServers, newDNSServers))
	}

	if len(newOutbounds) == 0 {
		high = append(high, "高风险：outbounds 为空，流量将无法转发")
	} else if len(oldOutbounds) != len(newOutbounds) {
		warn = append(warn, fmt.Sprintf("注意：outbounds 数量从 %d 变为 %d", len(oldOutbounds), len(newOutbounds)))
	}

	if len(removedTags) > 0 {
		high = append(high, "高风险：移除了出站 tag："+strings.Join(removedTags, ", "))
	}

	if oldRouteRules != newRouteRules {
		warn = append(warn, fmt.Sprintf("注意：route.rules 数量从 %d 变为 %d", oldRouteRules, newRouteRules))
	}

	if oldRouteFinal != newRouteFinal {
		warn = append(warn, fmt.Sprintf("注意：route.final 从 %q 变为 %q", blankAsDash(oldRouteFinal), blankAsDash(newRouteFinal)))
	}

	oldLogLevel := asTrimmedString(nestedString(oldCfg, "log", "level"))
	newLogLevel := asTrimmedString(nestedString(newCfg, "log", "level"))
	if oldLogLevel != newLogLevel {
		warn = append(warn, fmt.Sprintf("注意：log.level 从 %q 变为 %q", blankAsDash(oldLogLevel), blankAsDash(newLogLevel)))
	}

	report := &ConfigRiskReport{Level: "ok", Summary: "未发现明显高风险变更", Items: []string{"未发现高风险项"}}
	if len(warn) > 0 {
		report.Level = "warn"
		report.Summary = fmt.Sprintf("发现 %d 项变更，请确认", len(warn))
		report.Items = append([]string{}, warn...)
	}
	if len(high) > 0 {
		report.Level = "bad"
		report.Summary = fmt.Sprintf("发现 %d 项高风险变更，建议先复核", len(high))
		report.Items = append(append([]string{}, high...), warn...)
	}
	return report, nil
}

func routeField(cfg map[string]any, key string) any {
	r, _ := cfg["route"].(map[string]any)
	if r == nil {
		return nil
	}
	return r[key]
}

func routeRules(cfg map[string]any) []any {
	r, _ := cfg["route"].(map[string]any)
	if r == nil {
		return nil
	}
	arr, _ := r["rules"].([]any)
	return arr
}

func dnsServers(cfg map[string]any) []any {
	d, _ := cfg["dns"].(map[string]any)
	if d == nil {
		return nil
	}
	arr, _ := d["servers"].([]any)
	return arr
}

func outboundTags(cfg map[string]any) []string {
	arr, _ := cfg["outbounds"].([]any)
	if len(arr) == 0 {
		return nil
	}
	out := make([]string, 0, len(arr))
	seen := map[string]struct{}{}
	for _, item := range arr {
		m, _ := item.(map[string]any)
		tag := strings.TrimSpace(asTrimmedString(m["tag"]))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func nestedString(cfg map[string]any, section, key string) string {
	m, _ := cfg[section].(map[string]any)
	if m == nil {
		return ""
	}
	return asTrimmedString(m[key])
}

func asTrimmedString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func blankAsDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}

func diffStrings(oldList, newList []string) []string {
	m := map[string]struct{}{}
	for _, v := range newList {
		m[v] = struct{}{}
	}
	var out []string
	for _, v := range oldList {
		if _, ok := m[v]; !ok {
			out = append(out, v)
		}
	}
	return out
}
