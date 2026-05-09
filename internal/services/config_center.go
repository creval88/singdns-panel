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
	ID                        string         `json:"id"`
	Tag                       string         `json:"tag"`
	Type                      string         `json:"type"`
	Enabled                   bool           `json:"enabled"`
	Members                   []string       `json:"members,omitempty"`
	FilterInclude             []string       `json:"filter_include,omitempty"`
	FilterExclude             []string       `json:"filter_exclude,omitempty"`
	Interval                  string         `json:"interval,omitempty"`
	Tolerance                 int            `json:"tolerance,omitempty"`
	IdleTimeout               string         `json:"idle_timeout,omitempty"`
	InterruptExistConnections *bool          `json:"interrupt_exist_connections,omitempty"`
	Reference                 int            `json:"reference_count"`
	Raw                       map[string]any `json:"raw,omitempty"`
}

type ConfigCenterRuleSet struct {
	ID             string         `json:"id"`
	Tag            string         `json:"tag"`
	Type           string         `json:"type"`
	Format         string         `json:"format,omitempty"`
	Source         string         `json:"source,omitempty"`
	DownloadDetour string         `json:"download_detour,omitempty"`
	Enabled        bool           `json:"enabled"`
	Reference      int            `json:"reference_count"`
	Raw            map[string]any `json:"raw,omitempty"`
}

type ConfigCenterRouteRule struct {
	ID          string         `json:"id"`
	Position    int            `json:"position"`
	Type        string         `json:"type,omitempty"`
	Outbound    string         `json:"outbound,omitempty"`
	Action      string         `json:"action,omitempty"`
	ClashMode   string         `json:"clash_mode,omitempty"`
	RuleSets    []string       `json:"rule_sets,omitempty"`
	Inbound     []string       `json:"inbound,omitempty"`
	Domain      []string       `json:"domain,omitempty"`
	DomainSuf   []string       `json:"domain_suffix,omitempty"`
	IPCIDR      []string       `json:"ip_cidr,omitempty"`
	Network     []string       `json:"network,omitempty"`
	Port        string         `json:"port,omitempty"`
	IPIsPrivate *bool          `json:"ip_is_private,omitempty"`
	Summary     string         `json:"summary"`
	Enabled     bool           `json:"enabled"`
	Raw         map[string]any `json:"raw,omitempty"`
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
	OK       bool                       `json:"ok"`
	Errors   []string                   `json:"errors"`
	Warnings []string                   `json:"warnings"`
	CanApply bool                       `json:"can_apply"`
	Risk     *ConfigRiskReport          `json:"risk,omitempty"`
	Summary  *ConfigCenterChangeSummary `json:"summary,omitempty"`
}

type ConfigCenterChangeSummary struct {
	Changed          bool `json:"changed"`
	OldBytes         int  `json:"old_bytes"`
	NewBytes         int  `json:"new_bytes"`
	OutboundsBefore  int  `json:"outbounds_before"`
	OutboundsAfter   int  `json:"outbounds_after"`
	RuleSetsBefore   int  `json:"rule_sets_before"`
	RuleSetsAfter    int  `json:"rule_sets_after"`
	RouteRulesBefore int  `json:"route_rules_before"`
	RouteRulesAfter  int  `json:"route_rules_after"`
}

type ConfigCenterSaveResult struct {
	Action     string                     `json:"action"`
	Message    string                     `json:"message"`
	BackupName string                     `json:"backup_name,omitempty"`
	Bytes      int                        `json:"bytes"`
	Risk       *ConfigRiskReport          `json:"risk,omitempty"`
	Validation *ConfigCenterValidation    `json:"validation,omitempty"`
	Summary    *ConfigCenterChangeSummary `json:"summary,omitempty"`
}

func (r ConfigCenterSaveResult) AuditText() string {
	return strings.TrimSpace(r.Message)
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
	if risk, err := s.ConfigRiskReport(content); err == nil {
		result.Risk = risk
	}
	if summary, err := s.configCenterChangeSummary(content); err == nil {
		result.Summary = summary
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
		delete(m, "filter")
		delete(m, "interval")
		delete(m, "tolerance")
		delete(m, "idle_timeout")
		delete(m, "interrupt_exist_connections")
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
		if len(item.FilterInclude) > 0 || len(item.FilterExclude) > 0 {
			filter := map[string]any{}
			if vals := stringsToAny(item.FilterInclude); len(vals) > 0 {
				filter["include"] = vals
			}
			if vals := stringsToAny(item.FilterExclude); len(vals) > 0 {
				filter["exclude"] = vals
			}
			if len(filter) > 0 {
				m["filter"] = filter
			}
		}
		if strings.TrimSpace(item.Interval) != "" {
			m["interval"] = strings.TrimSpace(item.Interval)
		}
		if item.Tolerance > 0 {
			m["tolerance"] = item.Tolerance
		}
		if strings.TrimSpace(item.IdleTimeout) != "" {
			m["idle_timeout"] = strings.TrimSpace(item.IdleTimeout)
		}
		if item.InterruptExistConnections != nil {
			m["interrupt_exist_connections"] = *item.InterruptExistConnections
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
		delete(m, "download_detour")
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
		if strings.TrimSpace(item.DownloadDetour) != "" {
			m["download_detour"] = strings.TrimSpace(item.DownloadDetour)
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
		delete(m, "clash_mode")
		delete(m, "network")
		delete(m, "port")
		delete(m, "ip_is_private")
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
		if strings.TrimSpace(item.ClashMode) != "" {
			m["clash_mode"] = strings.TrimSpace(item.ClashMode)
		}
		if vals := stringsToAny(item.Network); len(vals) == 1 {
			m["network"] = vals[0]
		} else if len(vals) > 1 {
			m["network"] = vals
		}
		if portVal := portValueFromString(item.Port); portVal != nil {
			m["port"] = portVal
		}
		if item.IPIsPrivate != nil {
			m["ip_is_private"] = *item.IPIsPrivate
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
	result, err := s.SaveConfigCenterDraftDetailed(draft)
	if err != nil {
		return nil, err
	}
	return &OperationResult{Action: result.Action, Message: result.Message}, nil
}

func (s *SingBoxService) SaveConfigCenterDraftDetailed(draft *ConfigCenterDraft) (*ConfigCenterSaveResult, error) {
	validation := ValidateConfigCenterDraft(draft)
	if !validation.OK {
		return nil, fmt.Errorf("草稿校验失败: %s", strings.Join(validation.Errors, "; "))
	}
	content, err := s.BuildConfigCenterContentFromDraft(draft)
	if err != nil {
		return nil, err
	}
	if err := s.ValidateConfig(content); err != nil {
		return nil, err
	}
	risk, _ := s.ConfigRiskReport(content)
	summary, _ := s.configCenterChangeSummary(content)
	validation.Risk = risk
	validation.Summary = summary

	backupName, err := s.CreateBackup()
	if err != nil {
		return nil, err
	}
	if err := s.writeConfigFile(content); err != nil {
		return nil, err
	}
	s.PruneBackups(20)
	msg := fmt.Sprintf("配置中心草稿已保存，写入 %d 字节", len(content))
	if backupName != "" {
		msg += "，已备份为 " + backupName
	}
	return &ConfigCenterSaveResult{
		Action:     "config_center.save",
		Message:    msg,
		BackupName: backupName,
		Bytes:      len(content),
		Risk:       risk,
		Validation: validation,
		Summary:    summary,
	}, nil
}

func (s *SingBoxService) configCenterChangeSummary(newContent string) (*ConfigCenterChangeSummary, error) {
	oldContent, err := s.ReadConfig()
	if err != nil {
		return nil, err
	}
	oldDraft, err := s.ParseConfigCenterDraft(oldContent)
	if err != nil {
		return nil, err
	}
	newDraft, err := s.ParseConfigCenterDraft(newContent)
	if err != nil {
		return nil, err
	}
	return &ConfigCenterChangeSummary{
		Changed:          normalizeConfigText(oldContent) != normalizeConfigText(newContent),
		OldBytes:         len(oldContent),
		NewBytes:         len(newContent),
		OutboundsBefore:  len(oldDraft.Outbounds),
		OutboundsAfter:   len(newDraft.Outbounds),
		RuleSetsBefore:   len(oldDraft.RuleSets),
		RuleSetsAfter:    len(newDraft.RuleSets),
		RouteRulesBefore: len(oldDraft.RouteRules),
		RouteRulesAfter:  len(newDraft.RouteRules),
	}, nil
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
				ID:                        fmt.Sprintf("ob-%d", i+1),
				Tag:                       tag,
				Type:                      typeName,
				Enabled:                   true,
				Members:                   members,
				FilterInclude:             trimStrings(parseFilterInclude(m["filter"])),
				FilterExclude:             trimStrings(parseFilterExclude(m["filter"])),
				Interval:                  stringValue(m["interval"]),
				Tolerance:                 intValue(m["tolerance"]),
				IdleTimeout:               stringValue(m["idle_timeout"]),
				InterruptExistConnections: boolPtrValue(m["interrupt_exist_connections"]),
				Raw:                       cloneMap(m),
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
				ID:             fmt.Sprintf("rs-%d", i+1),
				Tag:            tag,
				Type:           stringValue(m["type"]),
				Format:         stringValue(m["format"]),
				Source:         source,
				DownloadDetour: stringValue(m["download_detour"]),
				Enabled:        true,
				Raw:            cloneMap(m),
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
				ID:          fmt.Sprintf("rr-%d", i+1),
				Position:    i + 1,
				Type:        stringValue(m["type"]),
				Outbound:    outbound,
				Action:      firstNonEmpty(stringValue(m["action"]), stringValue(m["method"])),
				ClashMode:   stringValue(m["clash_mode"]),
				RuleSets:    ruleSets,
				Inbound:     stringOrSlice(m["inbound"]),
				Domain:      stringOrSlice(m["domain"]),
				DomainSuf:   stringOrSlice(m["domain_suffix"]),
				IPCIDR:      stringOrSlice(m["ip_cidr"]),
				Network:     stringOrSlice(m["network"]),
				Port:        scalarString(m["port"]),
				IPIsPrivate: boolPtrValue(m["ip_is_private"]),
				Enabled:     true,
				Raw:         cloneMap(m),
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
			if strings.TrimSpace(item.Type) == "" {
				result.Warnings = append(result.Warnings, "存在未命名且未声明类型的 outbound")
			}
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
	if strings.TrimSpace(rule.ClashMode) != "" {
		parts = append(parts, "clash_mode="+rule.ClashMode)
	}
	if len(rule.Network) > 0 {
		parts = append(parts, "network="+strings.Join(rule.Network, ", "))
	}
	if strings.TrimSpace(rule.Port) != "" {
		parts = append(parts, "port="+rule.Port)
	}
	if rule.IPIsPrivate != nil {
		parts = append(parts, fmt.Sprintf("ip_is_private=%t", *rule.IPIsPrivate))
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

func trimStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
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

func scalarString(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case json.Number:
		return strings.TrimSpace(x.String())
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return strings.TrimSpace(fmt.Sprintf("%v", x))
	case float32:
		if x == float32(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return strings.TrimSpace(fmt.Sprintf("%v", x))
	case int, int8, int16, int32, int64:
		return strings.TrimSpace(fmt.Sprintf("%d", x))
	case uint, uint8, uint16, uint32, uint64:
		return strings.TrimSpace(fmt.Sprintf("%d", x))
	default:
		return ""
	}
}

func intValue(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int8:
		return int(x)
	case int16:
		return int(x)
	case int32:
		return int(x)
	case int64:
		return int(x)
	case uint:
		return int(x)
	case uint8:
		return int(x)
	case uint16:
		return int(x)
	case uint32:
		return int(x)
	case uint64:
		return int(x)
	case float64:
		return int(x)
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	default:
		return 0
	}
}

func boolPtrValue(v any) *bool {
	b, ok := v.(bool)
	if !ok {
		return nil
	}
	out := b
	return &out
}

func parseFilterInclude(v any) []string {
	includes, _ := parseFilterRules(v)
	return includes
}

func parseFilterExclude(v any) []string {
	_, excludes := parseFilterRules(v)
	return excludes
}

func portValueFromString(v string) any {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	if !strings.ContainsAny(v, "-,: ") {
		if n := intValue(json.Number(v)); n > 0 {
			return n
		}
	}
	return v
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
