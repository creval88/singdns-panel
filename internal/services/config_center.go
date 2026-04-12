package services

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type ConfigCenterDraft struct {
	Version    int                     `json:"version"`
	Source     string                  `json:"source"`
	Outbounds  []ConfigCenterOutbound  `json:"outbounds"`
	RuleSets   []ConfigCenterRuleSet   `json:"rule_sets"`
	RouteRules []ConfigCenterRouteRule `json:"route_rules"`
	Raw        map[string]any          `json:"raw,omitempty"`
}

type ConfigCenterOutbound struct {
	ID        string         `json:"id"`
	Tag       string         `json:"tag"`
	Type      string         `json:"type"`
	Enabled   bool           `json:"enabled"`
	Members   []string       `json:"members,omitempty"`
	Reference int            `json:"reference_count"`
	Raw       map[string]any `json:"raw,omitempty"`
}

type ConfigCenterRuleSet struct {
	ID        string         `json:"id"`
	Tag       string         `json:"tag"`
	Type      string         `json:"type"`
	Format    string         `json:"format,omitempty"`
	Source    string         `json:"source,omitempty"`
	Enabled   bool           `json:"enabled"`
	Reference int            `json:"reference_count"`
	Raw       map[string]any `json:"raw,omitempty"`
}

type ConfigCenterRouteRule struct {
	ID        string         `json:"id"`
	Position  int            `json:"position"`
	Type      string         `json:"type,omitempty"`
	Outbound  string         `json:"outbound,omitempty"`
	Action    string         `json:"action,omitempty"`
	RuleSets  []string       `json:"rule_sets,omitempty"`
	Inbound   []string       `json:"inbound,omitempty"`
	Domain    []string       `json:"domain,omitempty"`
	DomainSuf []string       `json:"domain_suffix,omitempty"`
	IPCIDR    []string       `json:"ip_cidr,omitempty"`
	Summary   string         `json:"summary"`
	Enabled   bool           `json:"enabled"`
	Raw       map[string]any `json:"raw,omitempty"`
}

type ConfigCenterOverview struct {
	OK          bool                       `json:"ok"`
	Counts      ConfigCenterOverviewCounts `json:"counts"`
	Warnings    []string                   `json:"warnings"`
	CanApply    bool                       `json:"can_apply"`
	UpdatedAt   string                     `json:"updated_at,omitempty"`
	ConfigBytes int                        `json:"config_bytes"`
}

type ConfigCenterOverviewCounts struct {
	Outbounds  int `json:"outbounds"`
	RuleSets   int `json:"rule_sets"`
	RouteRules int `json:"route_rules"`
}

type ConfigCenterValidation struct {
	OK       bool     `json:"ok"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
	CanApply bool     `json:"can_apply"`
}

func (s *SingBoxService) ConfigCenterDraftFromCurrent() (*ConfigCenterDraft, error) {
	content, err := s.ReadConfig()
	if err != nil {
		return nil, err
	}
	return s.ParseConfigCenterDraft(content)
}

func (s *SingBoxService) ConfigCenterOverview() (*ConfigCenterOverview, error) {
	content, err := s.ReadConfig()
	if err != nil {
		return nil, err
	}
	draft, err := s.ParseConfigCenterDraft(content)
	if err != nil {
		return nil, err
	}
	validation := ValidateConfigCenterDraft(draft)
	updatedAt, _ := s.ConfigUpdatedAt()
	return &ConfigCenterOverview{
		OK: true,
		Counts: ConfigCenterOverviewCounts{
			Outbounds:  len(draft.Outbounds),
			RuleSets:   len(draft.RuleSets),
			RouteRules: len(draft.RouteRules),
		},
		Warnings:    append([]string{}, validation.Warnings...),
		CanApply:    validation.CanApply,
		UpdatedAt:   updatedAt,
		ConfigBytes: len(content),
	}, nil
}

func (s *SingBoxService) ValidateConfigCenterContent(content string) (*ConfigCenterValidation, error) {
	draft, err := s.ParseConfigCenterDraft(content)
	if err != nil {
		return &ConfigCenterValidation{OK: false, Errors: []string{err.Error()}, CanApply: false}, nil
	}
	result := ValidateConfigCenterDraft(draft)
	// 如果 sing-box 二进制不存在，则提示引导去系统页安装，避免硬错误
	if st, err := os.Stat(strings.TrimSpace(s.cfg.BinPath)); err != nil || st.IsDir() {
		msg := fmt.Sprintf("未检测到 sing-box 可执行文件：%s；请先到“系统设置”安装后再校验/保存", strings.TrimSpace(s.cfg.BinPath))
		result.Errors = append(result.Errors, msg)
		result.OK = false
		result.CanApply = false
		return result, nil
	}
	if err := s.ValidateConfig(content); err != nil {
		result.Errors = append(result.Errors, err.Error())
		result.OK = false
		result.CanApply = false
	}
	return result, nil
}

func (s *SingBoxService) BuildConfigCenterContentFromDraft(draft *ConfigCenterDraft) (string, error) {
	if draft == nil {
		return "", fmt.Errorf("草稿为空")
	}
	content, err := s.ReadConfig()
	if err != nil {
		return "", err
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(content), &root); err != nil {
		return "", fmt.Errorf("读取现有配置失败: %w", err)
	}

	outbounds := make([]any, 0, len(draft.Outbounds))
	for _, item := range draft.Outbounds {
		m := cloneMap(item.Raw)
		delete(m, "tag")
		delete(m, "type")
		delete(m, "outbounds")
		if strings.TrimSpace(item.Tag) != "" {
			m["tag"] = strings.TrimSpace(item.Tag)
		}
		if strings.TrimSpace(item.Type) != "" {
			m["type"] = strings.TrimSpace(item.Type)
		}
		if len(item.Members) > 0 {
			vals := make([]any, 0, len(item.Members))
			for _, member := range item.Members {
				member = strings.TrimSpace(member)
				if member != "" {
					vals = append(vals, member)
				}
			}
			if len(vals) > 0 {
				m["outbounds"] = vals
			}
		}
		outbounds = append(outbounds, m)
	}
	root["outbounds"] = outbounds

	routeMap, _ := root["route"].(map[string]any)
	if routeMap == nil {
		routeMap = map[string]any{}
	}
	ruleSets := make([]any, 0, len(draft.RuleSets))
	for _, item := range draft.RuleSets {
		m := cloneMap(item.Raw)
		delete(m, "tag")
		delete(m, "type")
		delete(m, "format")
		delete(m, "url")
		delete(m, "path")
		if strings.TrimSpace(item.Tag) != "" {
			m["tag"] = strings.TrimSpace(item.Tag)
		}
		if strings.TrimSpace(item.Type) != "" {
			m["type"] = strings.TrimSpace(item.Type)
		}
		if strings.TrimSpace(item.Format) != "" {
			m["format"] = strings.TrimSpace(item.Format)
		}
		if src := strings.TrimSpace(item.Source); src != "" {
			if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
				m["url"] = src
			} else {
				m["path"] = src
			}
		}
		ruleSets = append(ruleSets, m)
	}
	routeMap["rule_set"] = ruleSets

	rules := make([]any, 0, len(draft.RouteRules))
	for _, item := range draft.RouteRules {
		m := cloneMap(item.Raw)
		delete(m, "type")
		delete(m, "rule_set")
		delete(m, "inbound")
		delete(m, "domain")
		delete(m, "domain_suffix")
		delete(m, "ip_cidr")
		delete(m, "outbound")
		delete(m, "action")
		delete(m, "method")
		if strings.TrimSpace(item.Type) != "" {
			m["type"] = strings.TrimSpace(item.Type)
		}
		if vals := stringsToAny(item.RuleSets); len(vals) == 1 {
			m["rule_set"] = vals[0]
		} else if len(vals) > 1 {
			m["rule_set"] = vals
		}
		if vals := stringsToAny(item.Inbound); len(vals) == 1 {
			m["inbound"] = vals[0]
		} else if len(vals) > 1 {
			m["inbound"] = vals
		}
		if vals := stringsToAny(item.Domain); len(vals) == 1 {
			m["domain"] = vals[0]
		} else if len(vals) > 1 {
			m["domain"] = vals
		}
		if vals := stringsToAny(item.DomainSuf); len(vals) == 1 {
			m["domain_suffix"] = vals[0]
		} else if len(vals) > 1 {
			m["domain_suffix"] = vals
		}
		if vals := stringsToAny(item.IPCIDR); len(vals) == 1 {
			m["ip_cidr"] = vals[0]
		} else if len(vals) > 1 {
			m["ip_cidr"] = vals
		}
		if strings.TrimSpace(item.Outbound) != "" {
			m["outbound"] = strings.TrimSpace(item.Outbound)
		}
		if strings.TrimSpace(item.Action) != "" {
			m["action"] = strings.TrimSpace(item.Action)
		}
		rules = append(rules, m)
	}
	routeMap["rules"] = rules
	root["route"] = routeMap

	buf, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", fmt.Errorf("生成配置失败: %w", err)
	}
	return string(buf) + "\n", nil
}

func (s *SingBoxService) SaveConfigCenterDraft(draft *ConfigCenterDraft) (*OperationResult, error) {
	validation := ValidateConfigCenterDraft(draft)
	if !validation.OK {
		return nil, fmt.Errorf("草稿校验失败: %s", strings.Join(validation.Errors, "; "))
	}
	content, err := s.BuildConfigCenterContentFromDraft(draft)
	if err != nil {
		return nil, err
	}
	return s.SaveConfig(content)
}

func (s *SingBoxService) ParseConfigCenterDraft(content string) (*ConfigCenterDraft, error) {
	var root map[string]any
	if err := json.Unmarshal([]byte(content), &root); err != nil {
		return nil, fmt.Errorf("配置 JSON 解析失败: %w", err)
	}
	draft := &ConfigCenterDraft{
		Version: 1,
		Source:  "config",
		Raw:     map[string]any{},
	}

	outboundRef := map[string]int{}
	ruleSetRef := map[string]int{}

	if outbounds, ok := root["outbounds"].([]any); ok {
		for i, item := range outbounds {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			tag := stringValue(m["tag"])
			typeName := stringValue(m["type"])
			members := stringSliceValue(m["outbounds"])
			draft.Outbounds = append(draft.Outbounds, ConfigCenterOutbound{
				ID:      fmt.Sprintf("ob-%d", i+1),
				Tag:     tag,
				Type:    typeName,
				Enabled: true,
				Members: members,
				Raw:     cloneMap(m),
			})
			for _, member := range members {
				if strings.TrimSpace(member) != "" && member != "{all}" {
					outboundRef[member]++
				}
			}
		}
	}

	routeMap, _ := root["route"].(map[string]any)
	if ruleSets, ok := routeMap["rule_set"].([]any); ok {
		for i, item := range ruleSets {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			tag := stringValue(m["tag"])
			source := stringValue(m["url"])
			if source == "" {
				source = stringValue(m["path"])
			}
			draft.RuleSets = append(draft.RuleSets, ConfigCenterRuleSet{
				ID:      fmt.Sprintf("rs-%d", i+1),
				Tag:     tag,
				Type:    stringValue(m["type"]),
				Format:  stringValue(m["format"]),
				Source:  source,
				Enabled: true,
				Raw:     cloneMap(m),
			})
		}
	}
	if rules, ok := routeMap["rules"].([]any); ok {
		for i, item := range rules {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			ruleSets := stringOrSlice(m["rule_set"])
			outbound := stringValue(m["outbound"])
			if strings.TrimSpace(outbound) != "" {
				outboundRef[outbound]++
			}
			for _, tag := range ruleSets {
				if strings.TrimSpace(tag) != "" {
					ruleSetRef[tag]++
				}
			}
			rule := ConfigCenterRouteRule{
				ID:        fmt.Sprintf("rr-%d", i+1),
				Position:  i + 1,
				Type:      stringValue(m["type"]),
				Outbound:  outbound,
				Action:    firstNonEmpty(stringValue(m["action"]), stringValue(m["method"])),
				RuleSets:  ruleSets,
				Inbound:   stringOrSlice(m["inbound"]),
				Domain:    stringOrSlice(m["domain"]),
				DomainSuf: stringOrSlice(m["domain_suffix"]),
				IPCIDR:    stringOrSlice(m["ip_cidr"]),
				Enabled:   true,
				Raw:       cloneMap(m),
			}
			rule.Summary = summarizeRouteRule(rule)
			draft.RouteRules = append(draft.RouteRules, rule)
		}
	}

	for i := range draft.Outbounds {
		draft.Outbounds[i].Reference = outboundRef[draft.Outbounds[i].Tag]
	}
	for i := range draft.RuleSets {
		draft.RuleSets[i].Reference = ruleSetRef[draft.RuleSets[i].Tag]
	}
	sort.Slice(draft.Outbounds, func(i, j int) bool { return draft.Outbounds[i].Tag < draft.Outbounds[j].Tag })
	sort.Slice(draft.RuleSets, func(i, j int) bool { return draft.RuleSets[i].Tag < draft.RuleSets[j].Tag })
	return draft, nil
}

func ValidateConfigCenterDraft(draft *ConfigCenterDraft) *ConfigCenterValidation {
	result := &ConfigCenterValidation{OK: true, Errors: []string{}, Warnings: []string{}, CanApply: true}
	if draft == nil {
		result.OK = false
		result.CanApply = false
		result.Errors = append(result.Errors, "草稿为空")
		return result
	}
	outboundTags := map[string]struct{}{}
	ruleSetTags := map[string]struct{}{}
	for _, item := range draft.Outbounds {
		tag := strings.TrimSpace(item.Tag)
		if tag == "" {
			result.Errors = append(result.Errors, "存在未命名 outbound")
			continue
		}
		if _, ok := outboundTags[tag]; ok {
			result.Errors = append(result.Errors, fmt.Sprintf("outbound tag 重复: %s", tag))
			continue
		}
		outboundTags[tag] = struct{}{}
		if (item.Type == "selector" || item.Type == "urltest") && len(item.Members) == 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("outbound %s 没有成员", tag))
		}
	}
	for _, item := range draft.RuleSets {
		tag := strings.TrimSpace(item.Tag)
		if tag == "" {
			result.Errors = append(result.Errors, "存在未命名 rule_set")
			continue
		}
		if _, ok := ruleSetTags[tag]; ok {
			result.Errors = append(result.Errors, fmt.Sprintf("rule_set tag 重复: %s", tag))
			continue
		}
		ruleSetTags[tag] = struct{}{}
	}
	for _, rule := range draft.RouteRules {
		if tag := strings.TrimSpace(rule.Outbound); tag != "" {
			if _, ok := outboundTags[tag]; !ok {
				result.Errors = append(result.Errors, fmt.Sprintf("规则 #%d 引用了不存在的 outbound: %s", rule.Position, tag))
			}
		}
		for _, tag := range rule.RuleSets {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			if _, ok := ruleSetTags[tag]; !ok {
				result.Errors = append(result.Errors, fmt.Sprintf("规则 #%d 引用了不存在的 rule_set: %s", rule.Position, tag))
			}
		}
	}
	if len(draft.Outbounds) == 0 {
		result.Warnings = append(result.Warnings, "未提取到 outbound")
	}
	if len(result.Errors) > 0 {
		result.OK = false
		result.CanApply = false
	}
	return result
}

func summarizeRouteRule(rule ConfigCenterRouteRule) string {
	parts := []string{}
	if len(rule.RuleSets) > 0 {
		parts = append(parts, "rule_set="+strings.Join(rule.RuleSets, ", "))
	}
	if len(rule.DomainSuf) > 0 {
		parts = append(parts, "domain_suffix="+strings.Join(rule.DomainSuf, ", "))
	}
	if len(rule.Domain) > 0 {
		parts = append(parts, "domain="+strings.Join(rule.Domain, ", "))
	}
	if len(rule.IPCIDR) > 0 {
		parts = append(parts, "ip_cidr="+strings.Join(rule.IPCIDR, ", "))
	}
	if len(rule.Inbound) > 0 {
		parts = append(parts, "inbound="+strings.Join(rule.Inbound, ", "))
	}
	if len(parts) == 0 && rule.Type != "" {
		parts = append(parts, "type="+rule.Type)
	}
	if len(parts) == 0 {
		parts = append(parts, "通用规则")
	}
	action := firstNonEmpty(rule.Outbound, rule.Action, "无动作")
	return strings.Join(parts, " | ") + " -> " + action
}

func stringValue(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func stringSliceValue(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s := stringValue(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func stringOrSlice(v any) []string {
	if s := stringValue(v); s != "" {
		return []string{s}
	}
	return stringSliceValue(v)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func stringsToAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
