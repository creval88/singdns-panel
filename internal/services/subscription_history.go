package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type SubscriptionHistoryItem struct {
	Time string `json:"time"`
	URL  string `json:"url"`
}

type SubscriptionUpdateEvent struct {
	Time         string `json:"time"`
	Status       string `json:"status"`
	Action       string `json:"action"`
	Stage        string `json:"stage"`
	URL          string `json:"url"`
	Message      string `json:"message"`
	DurationMs   int64  `json:"duration_ms,omitempty"`
	DurationText string `json:"duration_text,omitempty"`
}

func (s *SingBoxService) subscriptionHistoryPath() string {
	baseDir := filepath.Dir(strings.TrimSpace(s.cfg.ConfigPath))
	if baseDir == "" || baseDir == "." {
		baseDir = "/etc/sing-box"
	}
	return filepath.Join(baseDir, "subscription-history.log")
}

func (s *SingBoxService) subscriptionUpdateLogPath() string {
	baseDir := filepath.Dir(strings.TrimSpace(s.cfg.ConfigPath))
	if baseDir == "" || baseDir == "." {
		baseDir = "/etc/sing-box"
	}
	return filepath.Join(baseDir, "subscription-updates.log")
}

func (s *SingBoxService) legacySubscriptionHistoryPath() string {
	if p := strings.TrimSpace(s.panelConfigPath); p != "" {
		return filepath.Join(filepath.Dir(p), "subscription-history.log")
	}
	return ""
}

func (s *SingBoxService) legacySubscriptionUpdateLogPath() string {
	if p := strings.TrimSpace(s.panelConfigPath); p != "" {
		return filepath.Join(filepath.Dir(p), "subscription-updates.log")
	}
	return ""
}

func (s *SingBoxService) appendLine(path, line string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintln(f, line)
}

func (s *SingBoxService) AppendSubscriptionHistory(rawURL string) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return
	}
	line := fmt.Sprintf("%s\t%s", time.Now().Format("2006-01-02 15:04:05"), strings.ReplaceAll(rawURL, "\t", " "))
	// 主路径写入（/etc/sing-box），兼容路径写入（旧版本读 app/configs）
	s.appendLine(s.subscriptionHistoryPath(), line)
	if legacy := s.legacySubscriptionHistoryPath(); legacy != "" && legacy != s.subscriptionHistoryPath() {
		s.appendLine(legacy, line)
	}
}

func (s *SingBoxService) AppendSubscriptionUpdateEvent(status, action, rawURL, message string) {
	s.AppendSubscriptionUpdateEventDetailed(status, action, action, rawURL, message, 0)
}

func (s *SingBoxService) AppendSubscriptionUpdateEventDetailed(status, action, stage, rawURL, message string, duration time.Duration) {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "info"
	}
	action = strings.TrimSpace(action)
	if action == "" {
		action = "update"
	}
	stage = strings.TrimSpace(stage)
	if stage == "" {
		stage = action
	}
	rawURL = strings.TrimSpace(rawURL)
	message = strings.TrimSpace(message)

	line := fmt.Sprintf(
		"%s\t%s\t%s\t%s\t%d\t%s\t%s",
		time.Now().Format("2006-01-02 15:04:05"),
		status,
		action,
		strings.ReplaceAll(stage, "\t", " "),
		duration.Milliseconds(),
		strings.ReplaceAll(rawURL, "\t", " "),
		strings.ReplaceAll(message, "\t", " "),
	)
	// 主路径写入（/etc/sing-box），兼容路径写入（旧版本读 app/configs）
	s.appendLine(s.subscriptionUpdateLogPath(), line)
	if legacy := s.legacySubscriptionUpdateLogPath(); legacy != "" && legacy != s.subscriptionUpdateLogPath() {
		s.appendLine(legacy, line)
	}
}

func readLogFirst(pathA, pathB string) string {
	if b, err := os.ReadFile(pathA); err == nil {
		if text := strings.TrimSpace(string(b)); text != "" {
			return text
		}
	}
	if strings.TrimSpace(pathB) == "" || pathB == pathA {
		return ""
	}
	if b, err := os.ReadFile(pathB); err == nil {
		return strings.TrimSpace(string(b))
	}
	return ""
}

func parseHistoryLine(line string) (SubscriptionHistoryItem, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return SubscriptionHistoryItem{}, false
	}
	// 新格式：time\turl
	if strings.Contains(line, "\t") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			return SubscriptionHistoryItem{Time: strings.TrimSpace(parts[0]), URL: strings.TrimSpace(parts[1])}, true
		}
	}
	// 兼容旧格式："YYYY-MM-DD HH:MM:SS URL"
	if len(line) > 20 {
		timePart := strings.TrimSpace(line[:19])
		rest := strings.TrimSpace(line[19:])
		if rest != "" {
			return SubscriptionHistoryItem{Time: timePart, URL: rest}, true
		}
	}
	return SubscriptionHistoryItem{}, false
}

func (s *SingBoxService) SubscriptionHistory() ([]SubscriptionHistoryItem, error) {
	text := readLogFirst(s.subscriptionHistoryPath(), s.legacySubscriptionHistoryPath())
	if text == "" {
		return nil, nil
	}
	lines := strings.Split(text, "\n")
	out := make([]SubscriptionHistoryItem, 0, len(lines))
	for i := len(lines) - 1; i >= 0; i-- {
		if item, ok := parseHistoryLine(lines[i]); ok {
			out = append(out, item)
		}
	}
	return out, nil
}

func parseUpdateLine(line string) (SubscriptionUpdateEvent, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return SubscriptionUpdateEvent{}, false
	}
	parts := strings.Split(line, "\t")
	switch len(parts) {
	case 7:
		durationMs, _ := strconv.ParseInt(parts[4], 10, 64)
		return SubscriptionUpdateEvent{
			Time:         strings.TrimSpace(parts[0]),
			Status:       strings.TrimSpace(parts[1]),
			Action:       strings.TrimSpace(parts[2]),
			Stage:        strings.TrimSpace(parts[3]),
			DurationMs:   durationMs,
			DurationText: formatDurationMS(durationMs),
			URL:          strings.TrimSpace(parts[5]),
			Message:      strings.TrimSpace(parts[6]),
		}, true
	case 5:
		return SubscriptionUpdateEvent{
			Time:    strings.TrimSpace(parts[0]),
			Status:  strings.TrimSpace(parts[1]),
			Action:  strings.TrimSpace(parts[2]),
			Stage:   strings.TrimSpace(parts[2]),
			URL:     strings.TrimSpace(parts[3]),
			Message: strings.TrimSpace(parts[4]),
		}, true
	default:
		// 兼容极旧格式：time + URL（无状态）
		if len(line) > 20 {
			timePart := strings.TrimSpace(line[:19])
			rest := strings.TrimSpace(line[19:])
			if rest != "" {
				return SubscriptionUpdateEvent{
					Time:    timePart,
					Status:  "ok",
					Action:  "update",
					Stage:   "complete",
					URL:     rest,
					Message: "订阅更新成功",
				}, true
			}
		}
		return SubscriptionUpdateEvent{}, false
	}
}

func (s *SingBoxService) SubscriptionUpdateEvents(limit int) ([]SubscriptionUpdateEvent, error) {
	text := readLogFirst(s.subscriptionUpdateLogPath(), s.legacySubscriptionUpdateLogPath())
	if text == "" {
		// 兜底：如果 updates 为空，尝试用 history 映射出简易事件，避免 UI 一直“未执行”
		historyText := readLogFirst(s.subscriptionHistoryPath(), s.legacySubscriptionHistoryPath())
		if historyText == "" {
			return nil, nil
		}
		historyLines := strings.Split(historyText, "\n")
		if limit <= 0 {
			limit = 20
		}
		out := make([]SubscriptionUpdateEvent, 0, minInt(limit, len(historyLines)))
		for i := len(historyLines) - 1; i >= 0 && len(out) < limit; i-- {
			item, ok := parseHistoryLine(historyLines[i])
			if !ok {
				continue
			}
			out = append(out, SubscriptionUpdateEvent{
				Time:    item.Time,
				Status:  "ok",
				Action:  "update",
				Stage:   "complete",
				URL:     item.URL,
				Message: "订阅更新成功（兼容旧日志）",
			})
		}
		return out, nil
	}

	lines := strings.Split(text, "\n")
	if limit <= 0 {
		limit = 20
	}
	capHint := limit
	if len(lines) < capHint {
		capHint = len(lines)
	}
	out := make([]SubscriptionUpdateEvent, 0, capHint)
	for i := len(lines) - 1; i >= 0 && len(out) < limit; i-- {
		if ev, ok := parseUpdateLine(lines[i]); ok {
			out = append(out, ev)
		}
	}
	return out, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func formatDurationMS(ms int64) string {
	if ms <= 0 {
		return ""
	}
	d := time.Duration(ms) * time.Millisecond
	if d < time.Second {
		return d.String()
	}
	return d.Round(time.Millisecond).String()
}
