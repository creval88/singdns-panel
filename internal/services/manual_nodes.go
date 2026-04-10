package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ManualNodeLineResult struct {
	Line         int    `json:"line"`
	InputRaw     string `json:"input_raw"`
	InputDisplay string `json:"input_display"`
	Status       string `json:"status"`
	Tag          string `json:"tag,omitempty"`
	NodeType     string `json:"node_type,omitempty"`
	Message      string `json:"message,omitempty"`
}

type ManualNodesImportResult struct {
	Total       int                    `json:"total"`
	Success     int                    `json:"success"`
	Failed      int                    `json:"failed"`
	Ignored     int                    `json:"ignored"`
	ParsedNodes int                    `json:"parsed_nodes"`
	Errors      []string               `json:"errors,omitempty"`
	LineResults []ManualNodeLineResult `json:"line_results,omitempty"`
	Message     string                 `json:"message"`
}

func (s *SingBoxService) manualNodesBaseDir() string {
	baseDir := filepath.Dir(strings.TrimSpace(s.cfg.ConfigPath))
	if baseDir == "" || baseDir == "." {
		baseDir = "/etc/sing-box"
	}
	return baseDir
}

func (s *SingBoxService) manualNodesPath() string {
	return filepath.Join(s.manualNodesBaseDir(), "manual-nodes.txt")
}

func (s *SingBoxService) manualNodesLastResultPath() string {
	return filepath.Join(s.manualNodesBaseDir(), "manual-nodes-last-import.json")
}

func (s *SingBoxService) ReadManualNodesDraft() (string, error) {
	b, err := os.ReadFile(s.manualNodesPath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func (s *SingBoxService) SaveManualNodesDraft(raw string) (*OperationResult, error) {
	raw = strings.TrimSpace(raw)
	content := ""
	if raw != "" {
		content = raw + "\n"
	}
	if err := s.writeManagedFile(s.manualNodesPath(), content); err != nil {
		return nil, err
	}
	msg := "手动节点草稿已保存"
	if raw == "" {
		msg = "手动节点草稿已清空"
	}
	return &OperationResult{Action: "manual_nodes.save", Message: msg}, nil
}

func (s *SingBoxService) ReadLastManualNodesImportResult() (*ManualNodesImportResult, error) {
	b, err := os.ReadFile(s.manualNodesLastResultPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out ManualNodesImportResult
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *SingBoxService) SaveLastManualNodesImportResult(result *ManualNodesImportResult) error {
	if result == nil {
		return nil
	}
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return s.writeManagedFile(s.manualNodesLastResultPath(), string(b)+"\n")
}

func parseManualNodeLines(raw string) ([]map[string]any, *ManualNodesImportResult, error) {
	lines := splitSubscriptionLines(raw)
	if len(lines) == 0 {
		return nil, &ManualNodesImportResult{Total: 0, Success: 0, Failed: 0, Ignored: 0, ParsedNodes: 0, Message: "手动节点为空"}, fmt.Errorf("manual nodes are empty")
	}

	result := &ManualNodesImportResult{Total: len(lines), Errors: make([]string, 0), LineResults: make([]ManualNodeLineResult, 0, len(lines))}
	nodes := make([]map[string]any, 0, len(lines))
	seen := map[string]struct{}{}
	for i, line := range lines {
		key := strings.TrimSpace(line)
		if _, ok := seen[key]; ok {
			result.Ignored++
			result.LineResults = append(result.LineResults, ManualNodeLineResult{
				Line:         i + 1,
				InputRaw:     line,
				InputDisplay: truncateManualInput(line, 120),
				Status:       "ignored",
				Message:      "重复输入，已忽略",
			})
			continue
		}
		seen[key] = struct{}{}
		node, err := parseNodeLine(line, i+1)
		if err != nil {
			result.Failed++
			msg := fmt.Sprintf("第 %d 行解析失败: %v", i+1, err)
			result.LineResults = append(result.LineResults, ManualNodeLineResult{
				Line:         i + 1,
				InputRaw:     line,
				InputDisplay: truncateManualInput(line, 120),
				Status:       "failed",
				Message:      err.Error(),
			})
			if len(result.Errors) < 8 {
				result.Errors = append(result.Errors, msg)
			}
			continue
		}
		if node == nil {
			result.Failed++
			msg := fmt.Sprintf("第 %d 行解析失败: 空节点", i+1)
			result.LineResults = append(result.LineResults, ManualNodeLineResult{
				Line:         i + 1,
				InputRaw:     line,
				InputDisplay: truncateManualInput(line, 120),
				Status:       "failed",
				Message:      "空节点",
			})
			if len(result.Errors) < 8 {
				result.Errors = append(result.Errors, msg)
			}
			continue
		}
		result.Success++
		tag, _ := node["tag"].(string)
		typ, _ := node["type"].(string)
		result.LineResults = append(result.LineResults, ManualNodeLineResult{
			Line:         i + 1,
			InputRaw:     line,
			InputDisplay: truncateManualInput(line, 120),
			Status:       "success",
			Tag:          strings.TrimSpace(tag),
			NodeType:     strings.TrimSpace(typ),
			Message:      "解析成功",
		})
		nodes = append(nodes, node)
	}
	result.ParsedNodes = len(nodes)
	if len(nodes) == 0 {
		result.Message = "手动节点导入失败：未解析到可用节点"
		return nil, result, fmt.Errorf("no supported manual nodes parsed")
	}
	result.Message = fmt.Sprintf("手动节点解析完成：输入 %d 行，成功 %d，失败 %d，忽略 %d，解析节点 %d 个", result.Total, result.Success, result.Failed, result.Ignored, result.ParsedNodes)
	return nodes, result, nil
}

func (s *SingBoxService) loadExistingNodeSubscriptionNodes() ([]map[string]any, int, int, error) {
	urls, err := s.ReadNodeSubscriptionURLs()
	if err != nil || len(urls) == 0 {
		return nil, 0, 0, nil
	}

	allNodes := make([]map[string]any, 0)
	parsedCount := 0
	ignoredCount := 0
	startedAt := time.Now()
	for _, rawURL := range urls {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			continue
		}
		content, err := s.DownloadSubscription(rawURL)
		if err != nil {
			ignoredCount++
			continue
		}
		trimmed := strings.TrimSpace(content)
		if isSingboxConfigJSON(trimmed) {
			ignoredCount++
			s.AppendSubscriptionUpdateEventDetailed("error", "manual-import", "type", rawURL, "手动节点导入时检测到完整配置订阅，已忽略该节点订阅来源", time.Since(startedAt))
			continue
		}
		if nodesFromJSON, ok := parseSingboxOutboundsOnly(trimmed); ok {
			allNodes = append(allNodes, nodesFromJSON...)
			parsedCount += len(nodesFromJSON)
			continue
		}
		nodes, err := parseSubscriptionNodes(trimmed)
		if err != nil {
			ignoredCount++
			s.AppendSubscriptionUpdateEventDetailed("error", "manual-import", "parse", rawURL, "手动节点导入时解析节点订阅失败，已忽略："+err.Error(), time.Since(startedAt))
			continue
		}
		if len(nodes) == 0 {
			ignoredCount++
			continue
		}
		allNodes = append(allNodes, nodes...)
		parsedCount += len(nodes)
	}
	return allNodes, parsedCount, ignoredCount, nil
}

func (s *SingBoxService) ImportManualNodes(raw string) (*ManualNodesImportResult, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		stored, err := s.ReadManualNodesDraft()
		if err != nil {
			return nil, err
		}
		raw = stored
	}
	manualNodes, result, err := parseManualNodeLines(raw)
	if err != nil {
		_ = s.SaveLastManualNodesImportResult(result)
		return result, err
	}

	existingNodes, existingParsed, existingIgnored, err := s.loadExistingNodeSubscriptionNodes()
	if err != nil {
		_ = s.SaveLastManualNodesImportResult(result)
		return result, err
	}
	mergedNodes := make([]map[string]any, 0, len(existingNodes)+len(manualNodes))
	mergedNodes = append(mergedNodes, existingNodes...)
	mergedNodes = append(mergedNodes, manualNodes...)

	baseText, err := s.readSubscriptionBaseConfig()
	if err != nil {
		_ = s.SaveLastManualNodesImportResult(result)
		return result, err
	}
	finalConfig, summary, err := s.mergeSubscriptionNodesIntoConfig(baseText, mergedNodes)
	if err != nil {
		_ = s.SaveLastManualNodesImportResult(result)
		return result, err
	}
	saveResult, err := s.SaveConfig(finalConfig)
	if err != nil {
		_ = s.SaveLastManualNodesImportResult(result)
		return result, err
	}
	restartResult, err := s.Action("restart")
	if err != nil {
		_ = s.SaveLastManualNodesImportResult(result)
		return result, err
	}

	groups := 0
	if summary != nil {
		groups = len(summary.ExpandedGroups)
	}
	result.Message = fmt.Sprintf("手动节点导入完成：手动输入 %d 行，成功 %d，失败 %d，忽略 %d，手动解析节点 %d 个；叠加节点订阅 %d 个（忽略 %d 条订阅）；最终合并节点 %d 个，展开组 %d 个", result.Total, result.Success, result.Failed, result.Ignored, result.ParsedNodes, existingParsed, existingIgnored, len(mergedNodes), groups)
	if saveResult != nil && strings.TrimSpace(saveResult.Message) != "" {
		result.Message += "；" + strings.TrimSpace(saveResult.Message)
	}
	if restartResult != nil && strings.TrimSpace(restartResult.Message) != "" {
		result.Message += "；" + strings.TrimSpace(restartResult.Message)
	}
	_ = s.SaveLastManualNodesImportResult(result)
	return result, nil
}

func truncateManualInput(raw string, max int) string {
	raw = strings.TrimSpace(raw)
	if max <= 0 {
		max = 120
	}
	r := []rune(raw)
	if len(r) <= max {
		return raw
	}
	if max <= 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}
