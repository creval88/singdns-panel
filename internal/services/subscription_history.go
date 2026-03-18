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
	if p := strings.TrimSpace(s.panelConfigPath); p != "" {
		baseDir = filepath.Dir(p)
	}
	return filepath.Join(baseDir, "subscription-history.log")
}

func (s *SingBoxService) subscriptionUpdateLogPath() string {
	baseDir := filepath.Dir(strings.TrimSpace(s.cfg.ConfigPath))
	if p := strings.TrimSpace(s.panelConfigPath); p != "" {
		baseDir = filepath.Dir(p)
	}
	return filepath.Join(baseDir, "subscription-updates.log")
}

func (s *SingBoxService) AppendSubscriptionHistory(rawURL string) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return
	}
	f, err := os.OpenFile(s.subscriptionHistoryPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s\t%s\n", time.Now().Format("2006-01-02 15:04:05"), rawURL)
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
	f, err := os.OpenFile(s.subscriptionUpdateLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(
		f,
		"%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		status,
		action,
		strings.ReplaceAll(stage, "\t", " "),
		duration.Milliseconds(),
		strings.ReplaceAll(rawURL, "\t", " "),
		strings.ReplaceAll(message, "\t", " "),
	)
}

func (s *SingBoxService) SubscriptionHistory() ([]SubscriptionHistoryItem, error) {
	b, err := os.ReadFile(s.subscriptionHistoryPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	text := strings.TrimSpace(string(b))
	if text == "" {
		return nil, nil
	}
	lines := strings.Split(text, "\n")
	var out []SubscriptionHistoryItem
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i] == "" {
			continue
		}
		parts := strings.SplitN(lines[i], "\t", 2)
		if len(parts) != 2 {
			continue
		}
		out = append(out, SubscriptionHistoryItem{Time: parts[0], URL: parts[1]})
	}
	return out, nil
}

func (s *SingBoxService) SubscriptionUpdateEvents(limit int) ([]SubscriptionUpdateEvent, error) {
	b, err := os.ReadFile(s.subscriptionUpdateLogPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	text := strings.TrimSpace(string(b))
	if text == "" {
		return nil, nil
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
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		switch len(parts) {
		case 7:
			durationMs, _ := strconv.ParseInt(parts[4], 10, 64)
			out = append(out, SubscriptionUpdateEvent{
				Time:         parts[0],
				Status:       parts[1],
				Action:       parts[2],
				Stage:        parts[3],
				DurationMs:   durationMs,
				DurationText: formatDurationMS(durationMs),
				URL:          parts[5],
				Message:      parts[6],
			})
		case 5:
			out = append(out, SubscriptionUpdateEvent{Time: parts[0], Status: parts[1], Action: parts[2], Stage: parts[2], URL: parts[3], Message: parts[4]})
		default:
			continue
		}
	}
	return out, nil
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
