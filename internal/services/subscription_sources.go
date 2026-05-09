package services

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func (s *SingBoxService) buildSubscriptionSourceSummary(fullURL string, nodeURLs []string) (*SubscriptionSourceSummary, error) {
	managedTags := s.readManagedTagsState()
	summary := &SubscriptionSourceSummary{
		ActiveMode: detectSubscriptionActiveMode(strings.TrimSpace(fullURL), nodeURLs, managedTags),
		Nodes:      []SubscriptionSourceNode{},
		Notes:      []string{},
	}

	manualTagSet, manualNotes := s.manualNodeTagSet()
	summary.Notes = append(summary.Notes, manualNotes...)

	if strings.TrimSpace(fullURL) != "" && len(nodeURLs) > 0 {
		summary.Notes = append(summary.Notes, "当前同时配置了完整配置订阅和节点模板入口；当前活动托管节点以最近一次生成结果为准。")
	}
	if len(managedTags) == 0 {
		switch {
		case strings.TrimSpace(fullURL) != "":
			summary.Notes = append(summary.Notes, "当前启用了完整配置订阅；完整配置模式不会额外标记面板托管节点。")
		case len(nodeURLs) > 0:
			summary.Notes = append(summary.Notes, "已保存节点订阅，但当前配置里还没有面板托管节点；通常需要先执行一次“立即更新节点模板订阅”。")
		}
		return summary, nil
	}

	cfgText, err := s.ReadConfig()
	if err != nil {
		return nil, err
	}

	var cfg map[string]any
	if err := json.Unmarshal([]byte(cfgText), &cfg); err != nil {
		return nil, fmt.Errorf("parse current config json: %w", err)
	}

	outbounds, _ := cfg["outbounds"].([]any)
	if len(outbounds) == 0 {
		summary.Notes = append(summary.Notes, "当前配置未发现 outbounds，无法分析节点来源。")
		return summary, nil
	}

	managedSet := make(map[string]struct{}, len(managedTags))
	for _, tag := range managedTags {
		if tag = strings.TrimSpace(tag); tag != "" {
			managedSet[tag] = struct{}{}
		}
	}

	outboundByTag := make(map[string]map[string]any, len(managedTags))
	nodeGroups := make(map[string][]string, len(managedTags))
	groupSet := map[string]struct{}{}

	for _, item := range outbounds {
		m, ok := item.(map[string]any)
		if !ok || m == nil {
			continue
		}
		tag := strings.TrimSpace(stringValue(m["tag"]))
		if tag != "" {
			if _, ok := managedSet[tag]; ok {
				outboundByTag[tag] = m
			}
		}

		typ := strings.TrimSpace(stringValue(m["type"]))
		if typ != "selector" && typ != "urltest" {
			continue
		}
		groupTag := strings.TrimSpace(stringValue(m["tag"]))
		if groupTag == "" {
			continue
		}
		for _, member := range stringSliceValue(m["outbounds"]) {
			member = strings.TrimSpace(member)
			if member == "" {
				continue
			}
			if _, ok := managedSet[member]; !ok {
				continue
			}
			nodeGroups[member] = appendUniqueString(nodeGroups[member], groupTag)
			groupSet[groupTag] = struct{}{}
		}
	}

	missingCount := 0
	for _, tag := range managedTags {
		node := SubscriptionSourceNode{
			Tag:    tag,
			Source: "unknown",
			Groups: append([]string(nil), nodeGroups[tag]...),
		}
		sort.Strings(node.Groups)

		if cfgNode, ok := outboundByTag[tag]; ok && cfgNode != nil {
			node.Type = strings.TrimSpace(stringValue(cfgNode["type"]))
			node.Server = strings.TrimSpace(stringValue(cfgNode["server"]))
		} else {
			missingCount++
		}

		if _, ok := manualTagSet[tag]; ok {
			node.Source = "manual"
			summary.ManualNodeCount++
		} else if len(nodeURLs) > 0 {
			node.Source = "subscription"
			summary.SubscriptionCount++
		} else {
			summary.UnknownNodeCount++
		}

		for _, groupTag := range node.Groups {
			if isSelfBuiltGroupTag(groupTag) {
				node.SelfBuilt = true
				summary.SelfBuiltCount++
				break
			}
		}

		summary.Nodes = append(summary.Nodes, node)
	}

	summary.ManagedNodeCount = len(summary.Nodes)
	summary.MatchedGroupCount = len(groupSet)
	if missingCount > 0 {
		summary.Notes = append(summary.Notes, fmt.Sprintf("有 %d 个托管节点未在当前配置 outbounds 中找到，可能是旧状态残留或刚完成切换。", missingCount))
	}

	return summary, nil
}

func detectSubscriptionActiveMode(fullURL string, nodeURLs, managedTags []string) string {
	if len(managedTags) > 0 {
		return "nodes_template"
	}
	if strings.TrimSpace(fullURL) != "" {
		return "full_config"
	}
	if len(nodeURLs) > 0 {
		return "nodes_template"
	}
	return "idle"
}

func (s *SingBoxService) manualNodeTagSet() (map[string]struct{}, []string) {
	out := map[string]struct{}{}
	notes := []string{}

	raw, err := s.ReadManualNodesDraft()
	if err != nil {
		notes = append(notes, "读取手动节点草稿失败，来源分析未计入手动节点。")
		return out, notes
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out, notes
	}

	nodes, result, err := parseManualNodeLines(raw)
	if result != nil && result.Failed > 0 {
		notes = append(notes, fmt.Sprintf("手动节点草稿里有 %d 行解析失败，来源统计只按成功解析的节点计算。", result.Failed))
	}
	if err != nil {
		return out, notes
	}

	for _, node := range nodes {
		tag := strings.TrimSpace(stringValue(node["tag"]))
		if tag != "" {
			out[tag] = struct{}{}
		}
	}
	return out, notes
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func isSelfBuiltGroupTag(tag string) bool {
	tag = strings.TrimSpace(strings.ToLower(tag))
	if tag == "" {
		return false
	}
	return strings.Contains(tag, "自建")
}
