package handlers

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"singdns-panel/internal/services"
)

type DashboardHealthOverview struct {
	Level                string   `json:"level"`
	Summary              string   `json:"summary"`
	Issues               []string `json:"issues"`
	Suggestions          []string `json:"suggestions"`
	LastUpdateStatus     string   `json:"last_update_status"`
	LastUpdateTime       string   `json:"last_update_time"`
	LastSuccessTime      string   `json:"last_success_time"`
	LastUpdateMessage    string   `json:"last_update_message"`
	SubscriptionStale    bool     `json:"subscription_stale"`
	SubscriptionStaleTip string   `json:"subscription_stale_tip"`
}

type DashboardSubscriptionDigest struct {
	Total       int                                `json:"total"`
	Success     int                                `json:"success"`
	Failed      int                                `json:"failed"`
	Running     int                                `json:"running"`
	LastStatus  string                             `json:"last_status"`
	LastMessage string                             `json:"last_message"`
	LastTime    string                             `json:"last_time"`
	LastStage   string                             `json:"last_stage"`
	LastCost    string                             `json:"last_cost"`
	Recent      []services.SubscriptionUpdateEvent `json:"recent"`
}

func (a *App) Dashboard(w http.ResponseWriter, r *http.Request) {
	sb, _ := a.SingBox.Status()
	if sb == nil {
		sb = &services.ServiceStatus{Name: "sing-box"}
	}
	md, _ := a.MosDNS.Status()
	if md == nil {
		md = &services.ServiceStatus{Name: "mosdns"}
	}
	panelSvc, _ := services.NewSystemdService().Status("singdns-panel")
	if panelSvc == nil {
		panelSvc = &services.ServiceStatus{Name: "singdns-panel"}
	}
	cron, _ := a.SingBox.CronShow()
	if cron == nil {
		cron = &services.CronInfo{}
	}
	url, _ := a.SingBox.ReadSubscriptionURL()
	subscription, _ := a.SingBox.SubscriptionStatus()
	if subscription == nil {
		subscription = &services.SubscriptionStatus{}
	}
	updates, _ := a.SingBox.SubscriptionUpdateEvents(12)
	if updates == nil {
		updates = []services.SubscriptionUpdateEvent{}
	}
	digest := summarizeSubscriptionUpdates(updates)
	health := buildDashboardHealth(sb, md, panelSvc, subscription, updates)

	sbVersion, _ := a.SingBox.Version()
	latestVersion, _ := a.SingBox.LatestVersion()
	updatedAt, _ := a.SingBox.ConfigUpdatedAt()
	backups, _ := a.SingBox.ListBackups()
	if backups == nil {
		backups = []services.BackupInfo{}
	}
	backupStatus, _ := a.SingBox.BackupStatus()
	if backupStatus == nil {
		backupStatus = &services.BackupStatus{}
	}
	configStatus, _ := a.SingBox.ConfigStatus()
	if configStatus == nil {
		configStatus = &services.ConfigStatus{}
	}
	audits, _ := a.Audit.List(20)
	actionTimeline := summarizeActionTimeline(audits, 3)
	clashAPI, _ := a.SingBox.ClashAPIInfo(r.Host)
	if clashAPI == nil {
		clashAPI = &services.ClashAPIInfo{}
	}
	hostStats, _ := services.ReadHostStats()
	latestBackupTime := "暂无"
	latestBackupAgo := "暂无"
	if len(backups) > 0 {
		latestBackupTime = backups[0].ModTime
		latestBackupAgo = humanizeAge(backups[0].AgeText)
	}
	recentAudit := "暂无"
	if len(audits) > 0 {
		recentAudit = audits[0].Action
		if audits[0].Result != "" {
			recentAudit += " · " + audits[0].Result
		}
	}
	a.render(w, "dashboard.html", map[string]any{
		"Title":            "Dashboard",
		"ActiveNav":        "dashboard",
		"PageTitle":        "系统仪表盘",
		"Eyebrow":          "Overview",
		"HeaderHint":       "每 5 秒自动刷新",
		"SingBox":          sb,
		"MosDNS":           md,
		"PanelService":     panelSvc,
		"Health":           health,
		"SubDigest":        digest,
		"SubEvents":        digest.Recent,
		"Cron":             cron,
		"SubURL":           maskURL(url),
		"Subscription":     subscription,
		"MosDNSWeb":        a.MosDNS.WebURL(),
		"SBVersion":        sbVersion,
		"SBVersionShort":   shortVersion(sbVersion),
		"LatestVersion":    latestVersion,
		"UpdatedAt":        updatedAt,
		"BackupCount":      len(backups),
		"BackupStatus":     backupStatus,
		"ConfigStatus":     configStatus,
		"LatestBackupTime": latestBackupTime,
		"LatestBackupAgo":  latestBackupAgo,
		"RecentAudit":      recentAudit,
		"ActionTimeline":   actionTimeline,
		"ClashAPI":         clashAPI,
		"PanelVersion":     prettyPanelVersion(a.PanelVersion),
		"HostStats":        hostStats,
	})
}

func (a *App) DashboardAPI(w http.ResponseWriter, r *http.Request) {
	sb, _ := a.SingBox.Status()
	if sb == nil {
		sb = &services.ServiceStatus{Name: "sing-box"}
	}
	md, _ := a.MosDNS.Status()
	if md == nil {
		md = &services.ServiceStatus{Name: "mosdns"}
	}
	panelSvc, _ := services.NewSystemdService().Status("singdns-panel")
	if panelSvc == nil {
		panelSvc = &services.ServiceStatus{Name: "singdns-panel"}
	}
	cron, _ := a.SingBox.CronShow()
	if cron == nil {
		cron = &services.CronInfo{}
	}
	url, _ := a.SingBox.ReadSubscriptionURL()
	subscription, _ := a.SingBox.SubscriptionStatus()
	if subscription == nil {
		subscription = &services.SubscriptionStatus{}
	}
	updates, _ := a.SingBox.SubscriptionUpdateEvents(12)
	if updates == nil {
		updates = []services.SubscriptionUpdateEvent{}
	}
	digest := summarizeSubscriptionUpdates(updates)
	health := buildDashboardHealth(sb, md, panelSvc, subscription, updates)

	latestVersion, _ := a.SingBox.LatestVersion()
	updatedAt, _ := a.SingBox.ConfigUpdatedAt()
	backups, _ := a.SingBox.ListBackups()
	if backups == nil {
		backups = []services.BackupInfo{}
	}
	backupStatus, _ := a.SingBox.BackupStatus()
	if backupStatus == nil {
		backupStatus = &services.BackupStatus{}
	}
	configStatus, _ := a.SingBox.ConfigStatus()
	if configStatus == nil {
		configStatus = &services.ConfigStatus{}
	}
	audits, _ := a.Audit.List(20)
	actionTimeline := summarizeActionTimeline(audits, 3)
	clashAPI, _ := a.SingBox.ClashAPIInfo(r.Host)
	if clashAPI == nil {
		clashAPI = &services.ClashAPIInfo{}
	}
	hostStats, _ := services.ReadHostStats()
	latestBackupTime := "暂无"
	latestBackupAgo := "暂无"
	if len(backups) > 0 {
		latestBackupTime = backups[0].ModTime
		latestBackupAgo = humanizeAge(backups[0].AgeText)
	}
	recentAudit := "暂无"
	if len(audits) > 0 {
		recentAudit = audits[0].Action
		if audits[0].Result != "" {
			recentAudit += " · " + audits[0].Result
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"singbox":          sb,
		"mosdns":           md,
		"panel":            panelSvc,
		"health":           health,
		"subDigest":        digest,
		"subEvents":        digest.Recent,
		"cron":             cron,
		"subURL":           maskURL(url),
		"subscription":     subscription,
		"latestVersion":    latestVersion,
		"updatedAt":        updatedAt,
		"backupCount":      len(backups),
		"backupStatus":     backupStatus,
		"configStatus":     configStatus,
		"latestBackupTime": latestBackupTime,
		"latestBackupAgo":  latestBackupAgo,
		"recentAudit":      recentAudit,
		"actionTimeline":   actionTimeline,
		"clashAPI":         clashAPI,
		"hostStats":        hostStats,
	})
}

func buildDashboardHealth(sb, md, panel *services.ServiceStatus, sub *services.SubscriptionStatus, events []services.SubscriptionUpdateEvent) DashboardHealthOverview {
	issues := make([]string, 0, 6)
	suggestions := make([]string, 0, 6)
	levelRank := 0 // 0 ok, 1 warn, 2 bad
	setLevel := func(rank int) {
		if rank > levelRank {
			levelRank = rank
		}
	}

	if sb == nil || !sb.Active {
		issues = append(issues, "sing-box 服务异常")
		suggestions = append(suggestions, "进入 Sing-box 页面执行重启并检查日志")
		setLevel(2)
	}
	if md == nil || !md.Active {
		issues = append(issues, "MosDNS 服务异常")
		suggestions = append(suggestions, "进入 MosDNS 页面执行重启并确认端口监听")
		setLevel(2)
	}
	if panel == nil || !panel.Active {
		issues = append(issues, "singdns-panel 服务异常")
		suggestions = append(suggestions, "进入 System 页面点击“重启面板服务”")
		setLevel(2)
	}
	if sub == nil || !sub.Configured {
		issues = append(issues, "订阅链接未配置")
		suggestions = append(suggestions, "在 Sing-box 页面配置订阅 URL")
		setLevel(1)
	}

	res := DashboardHealthOverview{
		Level:            "ok",
		Summary:          "系统健康",
		Issues:           issues,
		Suggestions:      suggestions,
		LastUpdateStatus: "-",
		LastUpdateTime:   "-",
		LastSuccessTime:  "-",
	}

	if len(events) == 0 {
		res.LastUpdateStatus = "none"
		res.LastUpdateMessage = "尚无订阅更新记录"
		if sub != nil && sub.Configured {
			issues = append(issues, "订阅已配置但尚无执行记录")
			suggestions = append(suggestions, "手动执行一次订阅更新并观察结果")
			setLevel(1)
		}
	} else {
		latest := events[0]
		res.LastUpdateStatus = strings.TrimSpace(latest.Status)
		res.LastUpdateTime = strings.TrimSpace(latest.Time)
		res.LastUpdateMessage = strings.TrimSpace(latest.Message)
		if latest.Status == "error" {
			issues = append(issues, "最近一次订阅更新失败")
			suggestions = append(suggestions, "查看订阅结果卡中的失败详情并重试")
			setLevel(2)
		}
		for _, ev := range events {
			if ev.Status == "ok" {
				res.LastSuccessTime = strings.TrimSpace(ev.Time)
				break
			}
		}
		if res.LastSuccessTime == "-" && sub != nil && sub.Configured {
			issues = append(issues, "近期无订阅更新成功记录")
			suggestions = append(suggestions, "检查订阅源连通性与配置校验错误")
			setLevel(1)
		}
	}

	if sub != nil && strings.TrimSpace(sub.LastSuccessTime) != "" {
		if t, ok := parsePanelTime(sub.LastSuccessTime); ok {
			if time.Since(t) > 24*time.Hour {
				res.SubscriptionStale = true
				res.SubscriptionStaleTip = "距上次成功更新已超过 24 小时"
				issues = append(issues, "订阅更新新鲜度不足")
				suggestions = append(suggestions, "建议立即手动更新一次订阅")
				setLevel(1)
			}
		}
	}

	res.Issues = issues
	res.Suggestions = suggestions
	switch levelRank {
	case 2:
		res.Level = "bad"
		res.Summary = "系统异常，需要立即处理"
	case 1:
		res.Level = "warn"
		res.Summary = "系统可用，但存在风险项"
	default:
		res.Level = "ok"
		res.Summary = "系统健康"
	}
	return res
}

func summarizeSubscriptionUpdates(events []services.SubscriptionUpdateEvent) DashboardSubscriptionDigest {
	out := DashboardSubscriptionDigest{Recent: events}
	for _, ev := range events {
		out.Total++
		switch strings.TrimSpace(ev.Status) {
		case "ok":
			out.Success++
		case "error":
			out.Failed++
		default:
			out.Running++
		}
	}
	if len(events) > 0 {
		out.LastStatus = strings.TrimSpace(events[0].Status)
		out.LastMessage = strings.TrimSpace(events[0].Message)
		out.LastTime = strings.TrimSpace(events[0].Time)
		out.LastStage = strings.TrimSpace(events[0].Stage)
		out.LastCost = strings.TrimSpace(events[0].DurationText)
	}
	if out.LastStatus == "" {
		out.LastStatus = "none"
	}
	if out.LastStage == "" {
		out.LastStage = "-"
	}
	if out.LastCost == "" {
		out.LastCost = "-"
	}
	if out.LastTime == "" {
		out.LastTime = "-"
	}
	if out.LastMessage == "" {
		out.LastMessage = "-"
	}
	return out
}

func summarizeActionTimeline(items []services.AuditEntry, limit int) []services.AuditEntry {
	if limit <= 0 {
		limit = 3
	}
	if len(items) == 0 {
		return []services.AuditEntry{}
	}
	out := make([]services.AuditEntry, 0, limit)
	for _, it := range items {
		a := strings.TrimSpace(it.Action)
		if a == "" {
			continue
		}
		if strings.HasPrefix(a, "dashboard.") || strings.HasPrefix(a, "auth.") {
			continue
		}
		out = append(out, it)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func parsePanelTime(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, false
	}
	formats := []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05"}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, v, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func maskURL(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12] + "****"
}

func prettyPanelVersion(v string) string {
	if strings.TrimSpace(v) == "" || v == "dev" {
		return "dev build"
	}
	return v
}

func shortVersion(v string) string {
	re := regexp.MustCompile(`version\s+([^\s]+)`)
	m := re.FindStringSubmatch(v)
	if len(m) > 1 {
		return m[1]
	}
	return strings.TrimSpace(strings.Split(v, "\n")[0])
}

func humanizeAge(s string) string {
	s = strings.TrimSpace(strings.TrimSuffix(s, " 前"))
	s = strings.ReplaceAll(s, "0s", "")
	s = strings.ReplaceAll(s, "0m", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return "刚刚"
	}
	return s + " 前"
}
