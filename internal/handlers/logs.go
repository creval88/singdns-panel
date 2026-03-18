package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func (a *App) LogsPage(w http.ResponseWriter, r *http.Request) {
	sb, _ := a.SingBox.Logs(100)
	md, _ := a.MosDNS.Logs(100)
	a.render(w, "logs.html", map[string]any{"Title": "日志", "ActiveNav": "logs", "PageTitle": "服务日志", "Eyebrow": "Logs", "HeaderHint": "默认每 5 秒自动刷新，可暂停", "SidebarSubtitle": "服务日志", "SingBoxLogs": sb, "MosDNSLogs": md})
}

func (a *App) ServiceLogsAPI(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	lines := intParam(r, "lines")
	var logs string
	var err error
	switch name {
	case "singbox":
		logs, err = a.SingBox.Logs(lines)
	case "mosdns":
		logs, err = a.MosDNS.Logs(lines)
	default:
		respondMessage(w, http.ErrNotSupported, "")
		return
	}
	lineList := strings.Split(strings.TrimRight(logs, "\n"), "\n")
	errorCount, warnCount := 0, 0
	for _, line := range lineList {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "fatal") || strings.Contains(lower, "failed") {
			errorCount++
		}
		if strings.Contains(lower, "warn") {
			warnCount++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"logs": logs, "error": errString(err), "lineCount": len(lineList), "errorCount": errorCount, "warnCount": warnCount, "fetchedAt": time.Now().Format("15:04:05")})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
