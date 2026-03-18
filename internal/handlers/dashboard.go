package handlers

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"singdns-panel/internal/services"
)

func (a *App) Dashboard(w http.ResponseWriter, r *http.Request) {
	sb, _ := a.SingBox.Status()
	if sb == nil {
		sb = &services.ServiceStatus{Name: "sing-box"}
	}
	md, _ := a.MosDNS.Status()
	if md == nil {
		md = &services.ServiceStatus{Name: "mosdns"}
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
	audits, _ := a.Audit.List(1)
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
	cron, _ := a.SingBox.CronShow()
	if cron == nil {
		cron = &services.CronInfo{}
	}
	url, _ := a.SingBox.ReadSubscriptionURL()
	subscription, _ := a.SingBox.SubscriptionStatus()
	if subscription == nil {
		subscription = &services.SubscriptionStatus{}
	}
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
	audits, _ := a.Audit.List(1)
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
		"clashAPI":         clashAPI,
		"hostStats":        hostStats,
	})
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
