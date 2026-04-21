package services

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ImportSummary struct {
	ParsedNodeCount int
	ExpandedGroups  []string
	ManagedTags     []string
	Mode            string
	Compatibility   []string
}

func (s *SingBoxService) readSubscriptionBaseConfig() (string, error) {
	templatePath := strings.TrimSpace(s.cfg.TemplatePath)
	if templatePath != "" {
		if b, err := os.ReadFile(templatePath); err == nil {
			return string(b), nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("read subscription template: %w", err)
		}
	}

	baseText, err := s.ReadConfig()
	if err != nil {
		if templatePath != "" {
			return "", fmt.Errorf("read subscription template fallback config: %w", err)
		}
		return "", fmt.Errorf("read local config: %w", err)
	}
	return baseText, nil
}

func (s *SingBoxService) BuildConfigFromSubscription(_ string, content string) (string, *ImportSummary, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "", nil, fmt.Errorf("subscription content is empty")
	}

	// 兼容旧行为：如果订阅本身返回完整 sing-box 配置，则直接采用该配置。
	if isSingboxConfigJSON(trimmed) {
		cleaned, notes, err := s.sanitizeFullConfigSubscription(trimmed)
		if err != nil {
			return "", nil, err
		}
		return cleaned, &ImportSummary{ParsedNodeCount: 0, Mode: "full_config", Compatibility: notes}, nil
	}

	var nodes []map[string]any
	if nodesFromJSON, ok := parseSingboxOutboundsOnly(trimmed); ok {
		nodes = nodesFromJSON
	} else {
		parsedNodes, err := parseSubscriptionNodes(trimmed)
		if err != nil {
			return "", nil, err
		}
		if len(parsedNodes) == 0 {
			return "", nil, fmt.Errorf("no supported nodes parsed from subscription")
		}
		nodes = parsedNodes
	}

	manualRaw, err := s.ReadManualNodesDraft()
	if err != nil {
		return "", nil, err
	}
	manualRaw = strings.TrimSpace(manualRaw)
	if manualRaw != "" {
		manualNodes, _, err := parseManualNodeLines(manualRaw)
		if err != nil {
			return "", nil, fmt.Errorf("parse manual nodes draft: %w", err)
		}
		nodes = append(nodes, manualNodes...)
	}

	baseText, err := s.readSubscriptionBaseConfig()
	if err != nil {
		return "", nil, err
	}
	merged, summary, err := s.mergeSubscriptionNodesIntoConfig(baseText, nodes)
	if err != nil {
		return "", nil, err
	}
	if summary != nil {
		summary.ParsedNodeCount = len(nodes)
		summary.Mode = "nodes_template"
	}
	return merged, summary, nil
}

func (s *SingBoxService) sanitizeFullConfigSubscription(content string) (string, []string, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return "", nil, fmt.Errorf("parse full config json: %w", err)
	}

	compatNotes := make([]string, 0, 4)
	outbounds, _ := raw["outbounds"].([]any)
	fallbackTag := templateDirectFallbackTag(outbounds)
	providerNodes, providerNotes, err := s.expandOutboundProviders(raw, fallbackTag)
	if err != nil {
		return "", nil, err
	}
	compatNotes = append(compatNotes, providerNotes...)
	if _, ok := raw["outbound_providers"]; ok {
		delete(raw, "outbound_providers")
		compatNotes = append(compatNotes, "已移除 R 核心扩展字段 outbound_providers，避免 stock sing-box 校验失败")
	}
	compatNotes = append(compatNotes, "完整配置模式会直接覆盖当前配置，不会自动合并模板订阅节点、手动节点草稿或配置中心未保存草稿")
	if len(providerNodes) > 0 {
		existingTags := map[string]struct{}{}
		for _, item := range outbounds {
			m, ok := item.(map[string]any)
			if !ok || m == nil {
				continue
			}
			tag := strings.TrimSpace(stringValue(m["tag"]))
			if tag != "" {
				existingTags[tag] = struct{}{}
			}
		}
		for _, node := range providerNodes {
			tag := strings.TrimSpace(stringValue(node["tag"]))
			if tag == "" {
				continue
			}
			if _, exists := existingTags[tag]; exists {
				return "", nil, fmt.Errorf("provider 节点标签与现有 outbound 冲突: %s", tag)
			}
			existingTags[tag] = struct{}{}
			outbounds = append(outbounds, node)
		}
		raw["outbounds"] = outbounds
	}

	b, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return "", nil, fmt.Errorf("marshal sanitized full config: %w", err)
	}
	return string(b), compatNotes, nil
}

type outboundProviderDef struct {
	Tag string
	URL string
}

func (s *SingBoxService) expandOutboundProviders(raw map[string]any, fallbackTag string) ([]map[string]any, []string, error) {
	rawProviders, ok := raw["outbound_providers"]
	if !ok {
		return nil, nil, nil
	}
	providers := parseOutboundProviderDefs(rawProviders)
	if len(providers) == 0 {
		return nil, nil, nil
	}

	providerNodes := make(map[string][]map[string]any, len(providers))
	compatNotes := make([]string, 0, len(providers))
	allNodes := make([]map[string]any, 0)
	for _, provider := range providers {
		if provider.URL == "" {
			return nil, nil, fmt.Errorf("provider %s 缺少 url", provider.Tag)
		}
		content, err := s.DownloadSubscription(provider.URL)
		if err != nil {
			return nil, nil, fmt.Errorf("下载 provider %s 失败: %w", provider.Tag, err)
		}
		nodes, err := parseSubscriptionNodes(content)
		if err != nil {
			return nil, nil, fmt.Errorf("解析 provider %s 失败: %w", provider.Tag, err)
		}
		if len(nodes) == 0 {
			return nil, nil, fmt.Errorf("provider %s 未解析到可用节点", provider.Tag)
		}
		providerNodes[provider.Tag] = nodes
		allNodes = append(allNodes, nodes...)
		compatNotes = append(compatNotes, fmt.Sprintf("已展开 provider %s：%d 个节点", provider.Tag, len(nodes)))
	}

	outbounds, _ := raw["outbounds"].([]any)
	for _, item := range outbounds {
		m, ok := item.(map[string]any)
		if !ok || m == nil {
			continue
		}
		providerTags := wholeStringSlice(m["providers"])
		if len(providerTags) == 0 {
			continue
		}
		groupNodes := make([]map[string]any, 0)
		for _, tag := range providerTags {
			groupNodes = append(groupNodes, providerNodes[tag]...)
		}
		includes := wholeStringSlice(m["includes"])
		excludes := wholeStringSlice(m["excludes"])
		selectedTags := filterNodeTagsByPatterns(groupNodes, includes, excludes)
		outboundList := make([]any, 0, len(selectedTags))
		seen := map[string]struct{}{}
		for _, tag := range selectedTags {
			if _, ok := seen[tag]; ok {
				continue
			}
			seen[tag] = struct{}{}
			outboundList = append(outboundList, tag)
		}
		if len(outboundList) == 0 && strings.TrimSpace(fallbackTag) != "" {
			outboundList = append(outboundList, strings.TrimSpace(fallbackTag))
		}
		m["outbounds"] = outboundList
		delete(m, "providers")
		delete(m, "includes")
		delete(m, "excludes")
	}
	return allNodes, compatNotes, nil
}

func parseOutboundProviderDefs(v any) []outboundProviderDef {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]outboundProviderDef, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok || m == nil {
			continue
		}
		tag := strings.TrimSpace(stringValue(m["tag"]))
		url := strings.TrimSpace(stringValue(m["url"]))
		if tag == "" || url == "" {
			continue
		}
		out = append(out, outboundProviderDef{Tag: tag, URL: url})
	}
	return out
}

func wholeStringSlice(v any) []string {
	switch x := v.(type) {
	case string:
		x = strings.TrimSpace(x)
		if x == "" {
			return nil
		}
		return []string{x}
	case []string:
		out := make([]string, 0, len(x))
		for _, item := range x {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func filterNodeTagsByPatterns(nodes []map[string]any, includes, excludes []string) []string {
	out := make([]string, 0, len(nodes))
	seen := map[string]struct{}{}
	for _, node := range nodes {
		tag := strings.TrimSpace(stringValue(node["tag"]))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		searchText := nodeSearchText(node)
		if len(includes) > 0 && !matchAnyPattern(searchText, includes) {
			continue
		}
		if len(excludes) > 0 && matchAnyPattern(searchText, excludes) {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func matchAnyPattern(text string, patterns []string) bool {
	textLower := strings.ToLower(text)
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if re, err := regexp.Compile("(?i)" + pattern); err == nil {
			if re.MatchString(text) {
				return true
			}
			continue
		}
		if strings.Contains(textLower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

func selectedNodeTagsFromTemplate(outbounds []any, nodes []map[string]any) (map[string]struct{}, []string) {
	selected := make(map[string]struct{})
	expanded := make([]string, 0)
	fallbackTag := templateDirectFallbackTag(outbounds)
	for _, item := range outbounds {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := m["type"].(string)
		if typ != "selector" && typ != "urltest" {
			continue
		}
		ob, changed := expandAllPlaceholderForNodes(m, nodes, fallbackTag)
		if !changed {
			continue
		}
		m["outbounds"] = ob
		delete(m, "filter") // 已展开，不再依赖占位+filter
		for _, item := range ob {
			tag, _ := item.(string)
			tag = strings.TrimSpace(tag)
			if tag == "" || tag == "direct" || tag == "block" || tag == "dns-out" {
				continue
			}
			selected[tag] = struct{}{}
		}
		if tag, _ := m["tag"].(string); strings.TrimSpace(tag) != "" {
			expanded = append(expanded, tag)
		}
	}
	return selected, expanded
}

func isSingboxConfigJSON(content string) bool {
	var raw map[string]any
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return false
	}
	if _, ok := raw["outbounds"]; !ok {
		return false
	}
	// 仅 outbounds 视为节点片段，不走完整配置直通。
	if len(raw) == 1 {
		return false
	}
	return true
}

func (s *SingBoxService) mergeSubscriptionNodesIntoConfig(baseConfig string, nodes []map[string]any) (string, *ImportSummary, error) {
	var cfg map[string]any
	if err := json.Unmarshal([]byte(baseConfig), &cfg); err != nil {
		return "", nil, fmt.Errorf("parse local config json: %w", err)
	}

	outbounds, _ := cfg["outbounds"].([]any)
	if outbounds == nil {
		return "", nil, fmt.Errorf("local config missing outbounds array")
	}

	previousManaged := s.readManagedTagsState()
	previousManagedSet := make(map[string]struct{}, len(previousManaged))
	for _, tag := range previousManaged {
		previousManagedSet[tag] = struct{}{}
	}

	preserved := make([]any, 0, len(outbounds))
	for _, item := range outbounds {
		m, ok := item.(map[string]any)
		if !ok {
			preserved = append(preserved, item)
			continue
		}
		tag, _ := m["tag"].(string)
		if _, managed := previousManagedSet[tag]; managed {
			continue
		}
		preserved = append(preserved, m)
	}

	allNodeTags := make([]string, 0, len(nodes))
	seenNodeTag := map[string]struct{}{}
	nodeByTag := map[string]map[string]any{}
	for _, node := range nodes {
		tag, _ := node["tag"].(string)
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seenNodeTag[tag]; ok {
			continue
		}
		seenNodeTag[tag] = struct{}{}
		allNodeTags = append(allNodeTags, tag)
		nodeByTag[tag] = node
	}

	selectedTags, expanded := selectedNodeTagsFromTemplate(preserved, nodes)
	finalManagedTags := make([]string, 0, len(allNodeTags))
	for _, tag := range allNodeTags {
		if _, ok := selectedTags[tag]; !ok {
			continue
		}
		node, exists := nodeByTag[tag]
		if !exists {
			continue
		}
		preserved = append(preserved, node)
		finalManagedTags = append(finalManagedTags, tag)
	}

	cfg["outbounds"] = preserved
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", nil, fmt.Errorf("marshal merged config: %w", err)
	}
	_ = s.writeManagedTagsState(finalManagedTags)

	summary := &ImportSummary{ParsedNodeCount: len(allNodeTags), ExpandedGroups: expanded, ManagedTags: finalManagedTags}
	return string(b), summary, nil
}

func (s *SingBoxService) managedTagsStatePath() string {
	return filepath.Join(s.managedStateDir(), "subscription-managed-tags.json")
}

func (s *SingBoxService) legacyManagedTagsStatePath() string {
	baseDir := filepath.Dir(strings.TrimSpace(s.cfg.ConfigPath))
	if baseDir == "" || baseDir == "." {
		baseDir = "/etc/sing-box"
	}
	return filepath.Join(baseDir, "subscription-managed-tags.json")
}

func (s *SingBoxService) readManagedTagsState() []string {
	paths := []string{s.managedTagsStatePath(), s.legacyManagedTagsStatePath()}
	var b []byte
	var err error
	found := false
	for _, path := range paths {
		b, err = osReadFile(path)
		if err == nil {
			found = true
			break
		}
		if os.IsNotExist(err) {
			continue
		}
		return nil
	}
	if !found {
		return nil
	}
	var tags []string
	if err := json.Unmarshal(b, &tags); err != nil {
		return nil
	}
	out := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func (s *SingBoxService) writeManagedTagsState(tags []string) error {
	path := s.managedTagsStatePath()
	clean := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		clean = append(clean, t)
	}
	sort.Strings(clean)
	b, _ := json.MarshalIndent(clean, "", "  ")
	return s.writeManagedFile(path, string(b)+"\n")
}

func (s *SingBoxService) clearManagedTagsState() error {
	return s.writeManagedTagsState(nil)
}

// 通过变量包装，便于单测替换。
var osReadFile = func(path string) ([]byte, error) { return os.ReadFile(path) }

func expandAllPlaceholder(group map[string]any, nodeTags []string) ([]any, bool) {
	return expandAllPlaceholderWithFallback(group, nodeTags, "direct")
}

func expandAllPlaceholderWithFallback(group map[string]any, nodeTags []string, fallbackTag string) ([]any, bool) {
	rawList, _ := group["outbounds"].([]any)
	if len(rawList) == 0 {
		return rawList, false
	}
	containsAll := false
	for _, item := range rawList {
		if s, ok := item.(string); ok && strings.TrimSpace(s) == "{all}" {
			containsAll = true
			break
		}
	}
	if !containsAll {
		return rawList, false
	}

	includes, excludes := parseFilterRules(group["filter"])
	selected := filterTags(nodeTags, includes, excludes)

	out := make([]any, 0, len(rawList)+len(selected))
	seen := map[string]struct{}{}
	appendTag := func(tag string) {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return
		}
		if _, ok := seen[tag]; ok {
			return
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}

	for _, item := range rawList {
		tag, ok := item.(string)
		if !ok {
			continue
		}
		tag = strings.TrimSpace(tag)
		if tag == "{all}" {
			for _, t := range selected {
				appendTag(t)
			}
			continue
		}
		appendTag(tag)
	}
	if len(out) == 0 {
		appendTag(strings.TrimSpace(fallbackTag))
	}
	return out, true
}

func expandAllPlaceholderForNodes(group map[string]any, nodes []map[string]any, fallbackTag string) ([]any, bool) {
	rawList, _ := group["outbounds"].([]any)
	if len(rawList) == 0 {
		return rawList, false
	}
	containsAll := false
	for _, item := range rawList {
		if s, ok := item.(string); ok && strings.TrimSpace(s) == "{all}" {
			containsAll = true
			break
		}
	}
	if !containsAll {
		return rawList, false
	}

	includes, excludes := parseFilterRules(group["filter"])
	selected := filterNodeTags(nodes, includes, excludes)

	out := make([]any, 0, len(rawList)+len(selected))
	seen := map[string]struct{}{}
	appendTag := func(tag string) {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return
		}
		if _, ok := seen[tag]; ok {
			return
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}

	for _, item := range rawList {
		tag, ok := item.(string)
		if !ok {
			continue
		}
		tag = strings.TrimSpace(tag)
		if tag == "{all}" {
			for _, t := range selected {
				appendTag(t)
			}
			continue
		}
		appendTag(tag)
	}
	if len(out) == 0 {
		appendTag(strings.TrimSpace(fallbackTag))
	}
	return out, true
}

func templateDirectFallbackTag(outbounds []any) string {
	for _, item := range outbounds {
		m, ok := item.(map[string]any)
		if !ok || m == nil {
			continue
		}
		tag := strings.TrimSpace(stringValue(m["tag"]))
		if tag == "direct" {
			return tag
		}
	}
	for _, item := range outbounds {
		m, ok := item.(map[string]any)
		if !ok || m == nil {
			continue
		}
		if strings.TrimSpace(stringValue(m["type"])) != "direct" {
			continue
		}
		if tag := strings.TrimSpace(stringValue(m["tag"])); tag != "" {
			return tag
		}
	}
	return "direct"
}

func parseFilterRules(v any) (includes []string, excludes []string) {
	// 兼容两种格式：
	// 1) map: {"include": [...], "exclude": [...]}
	// 2) array: [{"action":"include|exclude","keywords":[...]}]
	if m, ok := v.(map[string]any); ok && m != nil {
		return toRuleList(m["include"]), toRuleList(m["exclude"])
	}

	rules, ok := v.([]any)
	if !ok {
		return nil, nil
	}

	includes = make([]string, 0, len(rules))
	excludes = make([]string, 0, len(rules))
	for _, item := range rules {
		rule, ok := item.(map[string]any)
		if !ok || rule == nil {
			continue
		}
		action, _ := rule["action"].(string)
		action = strings.ToLower(strings.TrimSpace(action))
		keywords := toRuleList(rule["keywords"])
		switch action {
		case "include":
			includes = append(includes, keywords...)
		case "exclude":
			excludes = append(excludes, keywords...)
		}
	}
	return includes, excludes
}

func toRuleList(v any) []string {
	addRule := func(out []string, raw string) []string {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return out
		}
		parts := strings.Split(raw, "|")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			out = append(out, p)
		}
		return out
	}

	toStringSlice := func(arr []any) []string {
		out := make([]string, 0, len(arr))
		for _, item := range arr {
			s, ok := item.(string)
			if !ok {
				continue
			}
			out = addRule(out, s)
		}
		return out
	}

	switch x := v.(type) {
	case string:
		var out []string
		return addRule(out, x)
	case []any:
		return toStringSlice(x)
	default:
		return nil
	}
}

func filterTags(tags, includes, excludes []string) []string {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if len(includes) > 0 && !matchAnyRule(tag, includes) {
			continue
		}
		if len(excludes) > 0 && matchAnyRule(tag, excludes) {
			continue
		}
		out = append(out, tag)
	}
	return out
}

func filterNodeTags(nodes []map[string]any, includes, excludes []string) []string {
	out := make([]string, 0, len(nodes))
	seen := map[string]struct{}{}
	for _, node := range nodes {
		tag := strings.TrimSpace(stringValue(node["tag"]))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		searchText := nodeSearchText(node)
		if len(includes) > 0 && !matchAnyRule(searchText, includes) {
			continue
		}
		if len(excludes) > 0 && matchAnyRule(searchText, excludes) {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func nodeSearchText(node map[string]any) string {
	parts := []string{
		strings.TrimSpace(stringValue(node["tag"])),
		strings.TrimSpace(stringValue(node["type"])),
		strings.TrimSpace(stringValue(node["server"])),
	}
	if tls, _ := node["tls"].(map[string]any); tls != nil {
		if enabled, _ := tls["enabled"].(bool); enabled {
			parts = append(parts, "tls")
		}
		if serverName := strings.TrimSpace(stringValue(tls["server_name"])); serverName != "" {
			parts = append(parts, serverName)
		}
		if reality, _ := tls["reality"].(map[string]any); reality != nil {
			if enabled, _ := reality["enabled"].(bool); enabled {
				parts = append(parts, "reality")
			}
		}
		if utls, _ := tls["utls"].(map[string]any); utls != nil {
			if enabled, _ := utls["enabled"].(bool); enabled {
				parts = append(parts, "utls")
				if fp := strings.TrimSpace(stringValue(utls["fingerprint"])); fp != "" {
					parts = append(parts, fp)
				}
			}
		}
	}
	return strings.Join(parts, " ")
}

func matchAnyRule(tag string, rules []string) bool {
	tagLower := strings.ToLower(tag)
	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		if strings.Contains(tagLower, strings.ToLower(rule)) {
			return true
		}
	}
	return false
}

func parseSubscriptionNodes(content string) ([]map[string]any, error) {
	decoded := maybeDecodeSubscriptionBody(content)
	if nodes, ok := parseSingboxOutboundsOnly(decoded); ok {
		return nodes, nil
	}
	if nodes, ok := parseClashProxiesContent(decoded); ok {
		return nodes, nil
	}
	lines := splitSubscriptionLines(decoded)
	out := make([]map[string]any, 0, len(lines))
	for i, line := range lines {
		node, err := parseNodeLine(line, i+1)
		if err != nil {
			continue
		}
		if node != nil {
			out = append(out, node)
		}
	}
	return out, nil
}

func maybeDecodeSubscriptionBody(content string) string {
	raw := strings.TrimSpace(content)
	if raw == "" {
		return raw
	}
	if strings.Contains(raw, "://") || strings.Contains(raw, "\n") {
		return raw
	}
	clean := strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t', ' ':
			return -1
		default:
			return r
		}
	}, raw)
	if clean == "" {
		return raw
	}
	data, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(clean)
		if err != nil {
			return raw
		}
	}
	decoded := strings.TrimSpace(string(data))
	if strings.Contains(decoded, "://") {
		return decoded
	}
	return raw
}

func decodeBase64Flexible(s string) (string, bool) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", false
	}
	clean := strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t', ' ':
			return -1
		default:
			return r
		}
	}, trimmed)
	if clean == "" {
		return "", false
	}
	try := []string{clean}
	if m := len(clean) % 4; m != 0 {
		try = append(try, clean+strings.Repeat("=", 4-m))
	}
	for _, c := range try {
		for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
			b, err := enc.DecodeString(c)
			if err == nil {
				return string(b), true
			}
		}
	}
	return "", false
}

func splitSubscriptionLines(content string) []string {
	normalized := strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(normalized, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func parseNodeLine(line string, idx int) (map[string]any, error) {
	switch {
	case strings.HasPrefix(line, "vmess://"):
		return parseVMessLine(line)
	case strings.HasPrefix(line, "vless://"):
		return parseVLESSLine(line)
	case strings.HasPrefix(line, "trojan://"):
		return parseTrojanLine(line)
	case strings.HasPrefix(line, "ss://"):
		return parseSSLine(line)
	default:
		return nil, fmt.Errorf("unsupported protocol at line %d", idx)
	}
}

func parseVMessLine(line string) (map[string]any, error) {
	raw := strings.TrimPrefix(strings.TrimSpace(line), "vmess://")
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		b, err = base64.RawStdEncoding.DecodeString(raw)
		if err != nil {
			return nil, err
		}
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	server, _ := asStringField(v["add"])
	port, _ := asIntField(v["port"])
	uuid, _ := asStringField(v["id"])
	tag, _ := asStringField(v["ps"])
	if tag == "" {
		tag = server
	}
	if server == "" || port == 0 || uuid == "" {
		return nil, fmt.Errorf("invalid vmess node")
	}
	node := map[string]any{
		"type":        "vmess",
		"tag":         tag,
		"server":      server,
		"server_port": port,
		"uuid":        uuid,
		"security":    "auto",
	}
	if aid, _ := asIntField(v["aid"]); aid > 0 {
		node["alter_id"] = aid
	}
	applyCommonTLSAndTransport(node, v)
	return node, nil
}

func parseVLESSLine(line string) (map[string]any, error) {
	u, err := url.Parse(strings.TrimSpace(line))
	if err != nil {
		return nil, err
	}
	user := ""
	if u.User != nil {
		user = u.User.Username()
	}
	host, port, err := splitHostPortOrDefault(u.Host, "443")
	if err != nil {
		return nil, err
	}
	tag := tagFromURLFragment(u, host)
	node := map[string]any{
		"type":        "vless",
		"tag":         tag,
		"server":      host,
		"server_port": port,
		"uuid":        user,
	}
	applyQueryTLSAndTransport(node, u)
	return node, nil
}

func parseTrojanLine(line string) (map[string]any, error) {
	u, err := url.Parse(strings.TrimSpace(line))
	if err != nil {
		return nil, err
	}
	pass := ""
	if u.User != nil {
		pass = u.User.Username()
	}
	host, port, err := splitHostPortOrDefault(u.Host, "443")
	if err != nil {
		return nil, err
	}
	tag := tagFromURLFragment(u, host)
	node := map[string]any{
		"type":        "trojan",
		"tag":         tag,
		"server":      host,
		"server_port": port,
		"password":    pass,
	}
	applyQueryTLSAndTransport(node, u)
	return node, nil
}

func parseSSLine(line string) (map[string]any, error) {
	raw := strings.TrimPrefix(strings.TrimSpace(line), "ss://")
	frag := ""
	if idx := strings.Index(raw, "#"); idx >= 0 {
		frag = raw[idx+1:]
		raw = raw[:idx]
	}
	query := ""
	if idx := strings.Index(raw, "?"); idx >= 0 {
		query = raw[idx+1:]
		raw = raw[:idx]
	}
	_ = query

	userinfoHost := raw
	if !strings.Contains(raw, "@") {
		decoded, ok := decodeBase64Flexible(raw)
		if !ok {
			return nil, fmt.Errorf("invalid ss node")
		}
		userinfoHost = decoded
	}
	parts := strings.SplitN(userinfoHost, "@", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid ss node")
	}
	methodPass := parts[0]
	hostPort := strings.TrimSpace(parts[1])
	hostPort = strings.Trim(hostPort, "/")
	if strings.Contains(methodPass, "%") {
		if unescaped, err := url.QueryUnescape(methodPass); err == nil {
			methodPass = unescaped
		}
	}
	if !strings.Contains(methodPass, ":") {
		if decoded, ok := decodeBase64Flexible(methodPass); ok {
			methodPass = decoded
		}
	}
	mp := strings.SplitN(methodPass, ":", 2)
	if len(mp) != 2 {
		return nil, fmt.Errorf("invalid ss method/password")
	}
	host, port, err := splitHostPortOrDefault(hostPort, "8388")
	if err != nil {
		return nil, err
	}
	tag := host
	if frag != "" {
		if unescaped, err := url.QueryUnescape(frag); err == nil {
			tag = strings.TrimSpace(unescaped)
		}
	}
	node := map[string]any{
		"type":        "shadowsocks",
		"tag":         tag,
		"server":      host,
		"server_port": port,
		"method":      mp[0],
		"password":    mp[1],
	}
	return node, nil
}

func applyCommonTLSAndTransport(node map[string]any, source map[string]any) {
	netVal, _ := asStringField(source["net"])
	tlsVal, _ := asStringField(source["tls"])
	sni, _ := asStringField(source["sni"])
	host, _ := asStringField(source["host"])
	path, _ := asStringField(source["path"])
	if strings.EqualFold(tlsVal, "tls") || strings.EqualFold(tlsVal, "true") || sni != "" {
		tls := map[string]any{"enabled": true}
		if sni != "" {
			tls["server_name"] = sni
		}
		node["tls"] = tls
	}
	if strings.EqualFold(netVal, "ws") {
		transport := map[string]any{"type": "ws"}
		if path != "" {
			transport["path"] = path
		}
		if host != "" {
			transport["headers"] = map[string]any{"Host": host}
		}
		node["transport"] = transport
	}
}

func applyQueryTLSAndTransport(node map[string]any, u *url.URL) {
	q := u.Query()
	security := strings.ToLower(strings.TrimSpace(q.Get("security")))
	sni := strings.TrimSpace(q.Get("sni"))
	if sni == "" {
		sni = strings.TrimSpace(q.Get("servername"))
	}
	if flow := strings.TrimSpace(q.Get("flow")); flow != "" {
		node["flow"] = flow
	}
	if security == "tls" || security == "reality" || sni != "" {
		tls := map[string]any{"enabled": true}
		if sni != "" {
			tls["server_name"] = sni
		}
		if insecure := strings.TrimSpace(q.Get("allowInsecure")); strings.EqualFold(insecure, "1") || strings.EqualFold(insecure, "true") {
			tls["insecure"] = true
		}
		fp := strings.TrimSpace(q.Get("fp"))
		if fp == "" {
			fp = strings.TrimSpace(q.Get("client-fingerprint"))
		}
		if fp != "" {
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
		}
		if security == "reality" {
			reality := map[string]any{"enabled": true}
			if pk := strings.TrimSpace(q.Get("pbk")); pk != "" {
				reality["public_key"] = pk
			}
			if sid := strings.TrimSpace(q.Get("sid")); sid != "" {
				reality["short_id"] = sid
			}
			tls["reality"] = reality
		}
		node["tls"] = tls
	}
	netVal := strings.ToLower(strings.TrimSpace(q.Get("type")))
	if netVal == "ws" {
		transport := map[string]any{"type": "ws"}
		if path := strings.TrimSpace(q.Get("path")); path != "" {
			transport["path"] = path
		}
		if host := strings.TrimSpace(q.Get("host")); host != "" {
			transport["headers"] = map[string]any{"Host": host}
		}
		node["transport"] = transport
	}
}

func splitHostPortOrDefault(hostPort, fallbackPort string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(hostPort)
	if err != nil {
		if strings.Contains(hostPort, ":") {
			parts := strings.Split(hostPort, ":")
			host = strings.Join(parts[:len(parts)-1], ":")
			portStr = parts[len(parts)-1]
		} else {
			host = hostPort
			portStr = fallbackPort
		}
	}
	host = strings.Trim(host, "[]")
	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil || port <= 0 {
		return "", 0, fmt.Errorf("invalid host/port: %s", hostPort)
	}
	return host, port, nil
}

func tagFromURLFragment(u *url.URL, fallback string) string {
	if u == nil {
		return fallback
	}
	frag := strings.TrimSpace(u.Fragment)
	if frag == "" {
		return fallback
	}
	if unescaped, err := url.QueryUnescape(frag); err == nil && strings.TrimSpace(unescaped) != "" {
		return strings.TrimSpace(unescaped)
	}
	return frag
}

func asStringField(v any) (string, bool) {
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(s), true
}

func asIntField(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(x))
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func (s *SingBoxService) ImportSubscriptionFromURL(rawURL string) (*OperationResult, error) {
	startedAt := time.Now()
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("subscription url is empty")
	}
	s.AppendSubscriptionUpdateEventDetailed("info", "import", "start", rawURL, "开始执行订阅导入", 0)
	content, err := s.DownloadSubscription(rawURL)
	if err != nil {
		return nil, err
	}
	finalConfig, summary, err := s.BuildConfigFromSubscription(rawURL, content)
	if err != nil {
		s.AppendSubscriptionUpdateEventDetailed("error", "import", "build", rawURL, err.Error(), time.Since(startedAt))
		return nil, err
	}
	if summary != nil && summary.Mode == "full_config" {
		if err := s.clearManagedTagsState(); err != nil {
			s.AppendSubscriptionUpdateEventDetailed("error", "import", "state", rawURL, "清理旧的托管节点状态失败："+err.Error(), time.Since(startedAt))
			return nil, err
		}
		for _, note := range summary.Compatibility {
			s.AppendSubscriptionUpdateEventDetailed("info", "import", "compat", rawURL, note, time.Since(startedAt))
		}
	}
	res, err := s.ApplySubscriptionContent(rawURL, finalConfig, startedAt)
	if err != nil {
		return nil, err
	}
	if summary != nil {
		msg := fmt.Sprintf("订阅导入完成：解析节点 %d 个，展开组 %d 个", summary.ParsedNodeCount, len(summary.ExpandedGroups))
		if summary.Mode == "full_config" {
			msg = "完整配置订阅导入完成"
		}
		if len(summary.Compatibility) > 0 {
			msg += "；" + strings.Join(summary.Compatibility, "；")
		}
		s.AppendSubscriptionUpdateEventDetailed("ok", "import", "summary", rawURL, msg, time.Since(startedAt))
		if res != nil {
			res.Message = msg + "；" + res.Message
		}
	}
	return res, nil
}

func (s *SingBoxService) ImportSubscriptionsFromURLs(rawURLs []string) (*OperationResult, error) {
	urls := normalizeSubscriptionURLs(strings.Join(rawURLs, "\n"))
	if len(urls) == 0 {
		return nil, fmt.Errorf("subscription url is empty")
	}
	if len(urls) == 1 {
		return s.ImportSubscriptionFromURL(urls[0])
	}

	startedAt := time.Now()
	allNodes := make([]map[string]any, 0)
	totalParsed := 0
	expandedSet := map[string]struct{}{}
	failedCount := 0

	for idx, rawURL := range urls {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			continue
		}
		s.AppendSubscriptionUpdateEventDetailed("info", "import", "start", rawURL, fmt.Sprintf("开始处理第 %d/%d 条节点订阅", idx+1, len(urls)), 0)
		content, err := s.DownloadSubscription(rawURL)
		if err != nil {
			failedCount += 1
			s.AppendSubscriptionUpdateEventDetailed("error", "import", "download", rawURL, "该条节点订阅拉取失败，已忽略："+err.Error(), time.Since(startedAt))
			continue
		}
		trimmed := strings.TrimSpace(content)
		if isSingboxConfigJSON(trimmed) {
			failedCount += 1
			s.AppendSubscriptionUpdateEventDetailed("error", "import", "type", rawURL, "该条内容是完整配置，不属于节点模板入口，已忽略", time.Since(startedAt))
			continue
		}
		if nodesFromJSON, ok := parseSingboxOutboundsOnly(trimmed); ok {
			allNodes = append(allNodes, nodesFromJSON...)
			totalParsed += len(nodesFromJSON)
			s.AppendSubscriptionUpdateEventDetailed("ok", "import", "parse", rawURL, fmt.Sprintf("节点订阅(JSON)解析完成：获取节点 %d 个", len(nodesFromJSON)), time.Since(startedAt))
			continue
		}
		nodes, err := parseSubscriptionNodes(trimmed)
		if err != nil {
			failedCount += 1
			s.AppendSubscriptionUpdateEventDetailed("error", "import", "build", rawURL, "该条节点订阅解析失败，已忽略："+err.Error(), time.Since(startedAt))
			continue
		}
		if len(nodes) == 0 {
			failedCount += 1
			s.AppendSubscriptionUpdateEventDetailed("error", "import", "build", rawURL, "该条节点订阅未解析到可用节点，已忽略", time.Since(startedAt))
			continue
		}
		allNodes = append(allNodes, nodes...)
		totalParsed += len(nodes)
		s.AppendSubscriptionUpdateEventDetailed("ok", "import", "parse", rawURL, fmt.Sprintf("节点订阅解析完成：获取节点 %d 个", len(nodes)), time.Since(startedAt))
	}

	if len(allNodes) == 0 {
		return nil, fmt.Errorf("all node subscriptions failed or produced no supported nodes")
	}
	baseText, err := s.readSubscriptionBaseConfig()
	if err != nil {
		s.AppendSubscriptionUpdateEventDetailed("error", "import", "build", joinSubscriptionURLs(urls), err.Error(), time.Since(startedAt))
		return nil, err
	}
	manualRaw, err := s.ReadManualNodesDraft()
	if err != nil {
		s.AppendSubscriptionUpdateEventDetailed("error", "import", "manual-draft", joinSubscriptionURLs(urls), err.Error(), time.Since(startedAt))
		return nil, err
	}
	manualRaw = strings.TrimSpace(manualRaw)
	manualParsed := 0
	if manualRaw != "" {
		manualNodes, _, err := parseManualNodeLines(manualRaw)
		if err != nil {
			s.AppendSubscriptionUpdateEventDetailed("error", "import", "manual-draft", joinSubscriptionURLs(urls), "手动草稿节点解析失败，已终止更新："+err.Error(), time.Since(startedAt))
			return nil, err
		}
		allNodes = append(allNodes, manualNodes...)
		manualParsed = len(manualNodes)
	}
	finalConfig, summary, err := s.mergeSubscriptionNodesIntoConfig(baseText, allNodes)
	if err != nil {
		s.AppendSubscriptionUpdateEventDetailed("error", "import", "build", joinSubscriptionURLs(urls), err.Error(), time.Since(startedAt))
		return nil, err
	}
	if summary != nil {
		for _, g := range summary.ExpandedGroups {
			expandedSet[g] = struct{}{}
		}
		summary.ParsedNodeCount = totalParsed
	}
	res, err := s.ApplySubscriptionContent(joinSubscriptionURLs(urls), finalConfig, startedAt)
	if err != nil {
		return nil, err
	}
	expanded := make([]string, 0, len(expandedSet))
	for g := range expandedSet {
		expanded = append(expanded, g)
	}
	msg := fmt.Sprintf("节点模板导入完成：订阅 %d 条，成功 %d 条，忽略失败 %d 条，订阅解析节点 %d 个，手动草稿节点 %d 个，最终合并节点 %d 个，展开组 %d 个", len(urls), len(urls)-failedCount, failedCount, totalParsed, manualParsed, len(allNodes), len(expanded))
	s.AppendSubscriptionUpdateEventDetailed("ok", "import", "summary", joinSubscriptionURLs(urls), msg, time.Since(startedAt))
	if res != nil {
		res.Message = msg + "；" + res.Message
	}
	return res, nil
}
