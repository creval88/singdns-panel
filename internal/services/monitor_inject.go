package services

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	cfgpkg "singdns-panel/internal/config"
)

const (
	monitorPrimaryInboundTag  = "singdns-monitor-primary-in"
	monitorFallbackInboundTag = "singdns-monitor-fallback-in"
	monitorPrimaryPort        = 7892
	monitorFallbackPort       = 7893
)

func monitorPrimaryProxyURL() string {
	return fmt.Sprintf("socks5://127.0.0.1:%d", monitorPrimaryPort)
}

func monitorFallbackProxyURL() string {
	return fmt.Sprintf("socks5://127.0.0.1:%d", monitorFallbackPort)
}

func (s *SingBoxService) prepareConfigForWrite(content string) (string, error) {
	monitorCfg, ok := s.monitorConfigForInjection()
	if !ok {
		return content, nil
	}
	out, _, err := InjectMonitorTestRouting(content, monitorCfg)
	return out, err
}

func (s *SingBoxService) monitorConfigForInjection() (cfgpkg.MonitorConfig, bool) {
	if strings.TrimSpace(s.panelConfigPath) == "" {
		return cfgpkg.MonitorConfig{}, false
	}
	cfg, err := cfgpkg.Load(s.panelConfigPath)
	if err != nil || cfg == nil {
		return cfgpkg.MonitorConfig{}, false
	}
	return cfg.Monitor, true
}

func (s *SingBoxService) EnsureMonitorTestRouting() (bool, error) {
	monitorCfg, ok := s.monitorConfigForInjection()
	if !ok {
		return false, nil
	}
	current, err := s.ReadConfig()
	if err != nil {
		return false, err
	}
	next, changed, err := InjectMonitorTestRouting(current, monitorCfg)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	if err := s.ValidateConfig(next); err != nil {
		return false, err
	}
	if err := s.writeManagedFile(s.cfg.ConfigPath, next); err != nil {
		return false, err
	}
	return true, nil
}

func InjectMonitorTestRouting(content string, monitorCfg cfgpkg.MonitorConfig) (string, bool, error) {
	var root map[string]any
	if err := json.Unmarshal([]byte(content), &root); err != nil {
		return "", false, err
	}
	outbounds, _ := root["outbounds"].([]any)
	knownOutbounds := collectOutboundTags(outbounds)
	primaryTag := strings.TrimSpace(monitorCfg.PrimaryGroup)
	fallbackTag := strings.TrimSpace(monitorCfg.FallbackGroup)
	shouldInject := monitorCfg.Enabled && monitorCfg.QualityCheckEnabled && strings.TrimSpace(monitorCfg.DownloadTestURL) != ""

	inbounds, _ := root["inbounds"].([]any)
	inbounds = filterMonitorInbounds(inbounds)
	if shouldInject && primaryTag != "" {
		if _, ok := knownOutbounds[primaryTag]; ok {
			inbounds = append(inbounds, monitorSOCKSInbound(monitorPrimaryInboundTag, monitorPrimaryPort))
		}
	}
	if shouldInject && fallbackTag != "" {
		if _, ok := knownOutbounds[fallbackTag]; ok {
			inbounds = append(inbounds, monitorSOCKSInbound(monitorFallbackInboundTag, monitorFallbackPort))
		}
	}
	if len(inbounds) > 0 {
		root["inbounds"] = inbounds
	} else {
		delete(root, "inbounds")
	}

	route, _ := root["route"].(map[string]any)
	if route == nil {
		route = map[string]any{}
	}
	rules, _ := route["rules"].([]any)
	rules = filterMonitorRouteRules(rules)
	managedRules := make([]any, 0, 2)
	if shouldInject && primaryTag != "" {
		if _, ok := knownOutbounds[primaryTag]; ok {
			managedRules = append(managedRules, monitorRouteRule(monitorPrimaryInboundTag, primaryTag))
		}
	}
	if shouldInject && fallbackTag != "" {
		if _, ok := knownOutbounds[fallbackTag]; ok {
			managedRules = append(managedRules, monitorRouteRule(monitorFallbackInboundTag, fallbackTag))
		}
	}
	if len(managedRules) > 0 {
		rules = append(managedRules, rules...)
	}
	if len(rules) > 0 {
		route["rules"] = rules
		root["route"] = route
	} else {
		delete(route, "rules")
		if len(route) > 0 {
			root["route"] = route
		} else {
			delete(root, "route")
		}
	}

	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", false, err
	}
	next := string(b) + "\n"
	return next, strings.TrimSpace(next) != strings.TrimSpace(content), nil
}

func monitorSOCKSInbound(tag string, port int) map[string]any {
	return map[string]any{
		"type":        "socks",
		"tag":         tag,
		"listen":      "127.0.0.1",
		"listen_port": port,
	}
}

func monitorRouteRule(inboundTag, outboundTag string) map[string]any {
	return map[string]any{
		"inbound":  inboundTag,
		"outbound": outboundTag,
	}
}

func filterMonitorInbounds(items []any) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			if isMonitorInboundTag(strings.TrimSpace(anyString(m["tag"]))) {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func filterMonitorRouteRules(items []any) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok && monitorRuleUsesManagedInbound(m) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func monitorRuleUsesManagedInbound(rule map[string]any) bool {
	switch v := rule["inbound"].(type) {
	case string:
		return isMonitorInboundTag(strings.TrimSpace(v))
	case []any:
		for _, item := range v {
			if isMonitorInboundTag(strings.TrimSpace(anyString(item))) {
				return true
			}
		}
	}
	return false
}

func isMonitorInboundTag(tag string) bool {
	return tag == monitorPrimaryInboundTag || tag == monitorFallbackInboundTag
}

func anyString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}
