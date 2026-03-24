package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (a *App) QuickDiagnosticsAPI(w http.ResponseWriter, r *http.Request) {
	sb, _ := a.SingBox.Status()
	md, _ := a.MosDNS.Status()
	panel := a.PanelVersionAPIData()
	sub, _ := a.SingBox.SubscriptionStatus()
	updates, _ := a.SingBox.SubscriptionUpdateEvents(3)
	audits, _ := a.Audit.List(3)
	sbLogs, _ := a.SingBox.Logs(80)
	mdLogs, _ := a.MosDNS.Logs(80)

	sbErr, sbWarn := countLogLevel(sbLogs)
	mdErr, mdWarn := countLogLevel(mdLogs)

	lines := []string{
		"diag.generated_at=" + time.Now().Format("2006-01-02 15:04:05"),
		"service.singbox.active=" + boolText(sb != nil && sb.Active),
		"service.mosdns.active=" + boolText(md != nil && md.Active),
		"service.panel.version=" + strings.TrimSpace(panel.CurrentVersion),
		"service.panel.latest=" + strings.TrimSpace(panel.LatestVersion),
		"subscription.configured=" + boolText(sub != nil && sub.Configured),
	}
	if sub != nil {
		lines = append(lines,
			"subscription.last_update_status="+cleanText(sub.LastUpdateStatus),
			"subscription.last_update_time="+cleanText(sub.LastUpdateTime),
			"subscription.last_success_time="+cleanText(sub.LastSuccessTime),
		)
	}
	if len(updates) > 0 {
		u := updates[0]
		lines = append(lines,
			"subscription.latest_event.status="+cleanText(u.Status),
			"subscription.latest_event.stage="+cleanText(u.Stage),
			"subscription.latest_event.time="+cleanText(u.Time),
			"subscription.latest_event.message="+cleanText(u.Message),
		)
	}
	lines = append(lines,
		fmt.Sprintf("logs.singbox.error=%d", sbErr),
		fmt.Sprintf("logs.singbox.warn=%d", sbWarn),
		fmt.Sprintf("logs.mosdns.error=%d", mdErr),
		fmt.Sprintf("logs.mosdns.warn=%d", mdWarn),
	)
	for i, item := range audits {
		idx := i + 1
		lines = append(lines,
			fmt.Sprintf("audit.%d.time=%s", idx, cleanText(item.Time)),
			fmt.Sprintf("audit.%d.action=%s", idx, cleanText(item.Action)),
			fmt.Sprintf("audit.%d.result=%s", idx, cleanText(item.Result)),
		)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":   true,
		"text": strings.Join(lines, "\n"),
	})
}

func countLogLevel(logs string) (errorCount int, warnCount int) {
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "fatal") || strings.Contains(lower, "failed") {
			errorCount++
		}
		if strings.Contains(lower, "warn") {
			warnCount++
		}
	}
	return
}

func boolText(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func cleanText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "-"
	}
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}

type panelVersionData struct {
	CurrentVersion string
	LatestVersion  string
}

func (a *App) PanelVersionAPIData() panelVersionData {
	latestVersion := ""
	if rr, err := a.Panel.ResolveRemoteRelease(); err == nil && rr != nil {
		latestVersion = strings.TrimSpace(rr.Version)
	}
	if latestVersion == "" {
		if rel, err := a.Panel.LatestLocalRelease(); err == nil && rel != nil {
			latestVersion = strings.TrimSpace(rel.Version)
		}
	}
	return panelVersionData{
		CurrentVersion: a.Panel.CurrentVersion(),
		LatestVersion:  latestVersion,
	}
}
