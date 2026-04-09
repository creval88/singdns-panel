package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	cfgpkg "singdns-panel/internal/config"
	svc "singdns-panel/internal/services"
)

func (a *App) SingBoxPage(w http.ResponseWriter, r *http.Request) {
	overview := a.singboxOverview(r)
	overview["Title"] = "Sing-box"
	overview["ActiveNav"] = "singbox"
	overview["PageTitle"] = "Sing-box 管理"
	overview["Eyebrow"] = "Service"
	overview["SidebarSubtitle"] = "sing-box / mosdns 控制台"
	a.render(w, "singbox.html", overview)
}

func (a *App) singboxOverview(r *http.Request) map[string]any {
	status, _ := a.SingBox.Status()
	if status == nil {
		status = &svc.ServiceStatus{Name: "sing-box"}
	}
	config, _ := a.SingBox.ReadConfig()
	templateConfig, _ := a.SingBox.ReadTemplateConfig()
	url, _ := a.SingBox.ReadSubscriptionURL()
	subscription, _ := a.SingBox.SubscriptionStatus()
	if subscription == nil {
		subscription = &svc.SubscriptionStatus{}
	}
	cron, _ := a.SingBox.CronShow()
	if cron == nil {
		cron = &svc.CronInfo{}
	}
	monitorCron, _ := a.Monitor.CronShow()
	if monitorCron == nil {
		monitorCron = &svc.CronInfo{}
	}
	monitorStatus, _ := a.Monitor.Status()
	if monitorStatus == nil {
		monitorStatus = &svc.MonitorStatus{}
	}
	monitorConfig := a.Config.Monitor
	version, _ := a.SingBox.Version()
	latestVersion, _ := a.SingBox.LatestVersion()
	updatedAt, _ := a.SingBox.ConfigUpdatedAt()
	backups, _ := a.SingBox.ListBackups()
	if backups == nil {
		backups = []svc.BackupInfo{}
	}
	backupStatus, _ := a.SingBox.BackupStatus()
	if backupStatus == nil {
		backupStatus = &svc.BackupStatus{}
	}
	history, _ := a.SingBox.SubscriptionHistory()
	if history == nil {
		history = []svc.SubscriptionHistoryItem{}
	}
	updateEvents, _ := a.SingBox.SubscriptionUpdateEvents(12)
	if updateEvents == nil {
		updateEvents = []svc.SubscriptionUpdateEvent{}
	}
	configStatus, _ := a.SingBox.ConfigStatus()
	if configStatus == nil {
		configStatus = &svc.ConfigStatus{}
	}
	clashAPI, _ := a.SingBox.ClashAPIInfo(r.Host)
	if clashAPI == nil {
		clashAPI = &svc.ClashAPIInfo{}
	}
	panel, _ := a.Panel.LatestLocalRelease()
	manualNodesDraft, _ := a.SingBox.ReadManualNodesDraft()
	ipForwardStatus, ipForwardErr := a.SingBox.IPForwardStatus()
	ipForwardMessage := "检测失败"
	if ipForwardStatus != nil && strings.TrimSpace(ipForwardStatus.Message) != "" {
		ipForwardMessage = ipForwardStatus.Message
	}
	if ipForwardErr != nil {
		ipForwardMessage = ipForwardErr.Error()
	}
	return map[string]any{
		"Status":           status,
		"Config":           config,
		"TemplateConfig":   templateConfig,
		"URL":              url,
		"Subscription":     subscription,
		"Cron":             cron,
		"MonitorCron":      monitorCron,
		"MonitorStatus":    monitorStatus,
		"MonitorConfig":    monitorConfig,
		"Version":          version,
		"LatestVersion":    latestVersion,
		"UpdatedAt":        updatedAt,
		"Backups":          backups,
		"BackupStatus":     backupStatus,
		"History":          history,
		"UpdateEvents":     updateEvents,
		"ConfigStatus":     configStatus,
		"ClashAPI":         clashAPI,
		"PanelVersion":     a.Panel.CurrentVersion(),
		"PanelRelease":     panel,
		"ManualNodesDraft": manualNodesDraft,
		"IPForwardStatus":  ipForwardStatus,
		"IPForwardError":   ipForwardErrString(ipForwardErr),
		"IPForwardMessage": ipForwardMessage,

		"status":           status,
		"config":           config,
		"templateConfig":   templateConfig,
		"url":              url,
		"subscription":     subscription,
		"cron":             cron,
		"monitorCron":      monitorCron,
		"monitorStatus":    monitorStatus,
		"monitorConfig":    monitorConfig,
		"version":          version,
		"latestVersion":    latestVersion,
		"updatedAt":        updatedAt,
		"backups":          backups,
		"backupStatus":     backupStatus,
		"history":          history,
		"updateEvents":     updateEvents,
		"configStatus":     configStatus,
		"clashAPI":         clashAPI,
		"panelVersion":     a.Panel.CurrentVersion(),
		"panelRelease":     panel,
		"manualNodesDraft": manualNodesDraft,
		"ipForwardStatus":  ipForwardStatus,
		"ipForwardError":   ipForwardErrString(ipForwardErr),
		"ipForwardMessage": ipForwardMessage,
	}
}

func ipForwardErrString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (a *App) SingBoxOverviewAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	_ = json.NewEncoder(w).Encode(a.singboxOverview(r))
}

func (a *App) SingBoxStatusAPI(w http.ResponseWriter, r *http.Request) {
	a.serviceStatusAPI(w, a.SingBox)
}
func (a *App) SingBoxConfigAPI(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"config": mustStr(a.SingBox.ReadConfig())})
}

func (a *App) SingBoxTemplateConfigAPI(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"config": mustStr(a.SingBox.ReadTemplateConfig())})
}
func (a *App) SingBoxSubscriptionAPI(w http.ResponseWriter, r *http.Request) {
	fullURL, _ := a.SingBox.ReadFullConfigSubscriptionURL()
	nodeURLs, _ := a.SingBox.ReadNodeSubscriptionURLs()
	json.NewEncoder(w).Encode(map[string]any{"url": fullURL, "urls": nodeURLs, "full_config_url": fullURL, "node_urls": nodeURLs})
}

func (a *App) SingBoxActionAPI(w http.ResponseWriter, r *http.Request) {
	a.serviceActionAPI(w, r, "singbox.action.", a.SingBox, "Sing-box 已操作")
}

func (a *App) SingBoxTemplateConfigValidateAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Config string `json:"config"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if err := a.SingBox.ValidateTemplateConfig(in.Config); err != nil {
		respondMessage(w, err, "")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "模版配置校验通过"})
}

func (a *App) SingBoxTemplateConfigSaveAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Config string `json:"config"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	res, err := a.SingBox.SaveTemplateConfig(in.Config)
	a.respondAudited(w, r, "singbox.template.save", res, err, "模版配置已保存")
}

func (a *App) SingBoxConfigValidateAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Config string `json:"config"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if err := a.SingBox.ValidateConfig(in.Config); err != nil {
		respondMessage(w, err, "")
		return
	}
	risk, err := a.SingBox.ConfigRiskReport(in.Config)
	if err != nil {
		respondMessage(w, err, "")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"message": "配置校验通过",
		"risk":    risk,
	})
}

func (a *App) SingBoxConfigSaveAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Config  string `json:"config"`
		Restart bool   `json:"restart"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	res, err := a.SingBox.SaveConfig(in.Config)
	if err != nil {
		a.respondAudited(w, r, "singbox.config.save", nil, err, "配置保存失败")
		return
	}
	if in.Restart {
		restartRes, err := a.SingBox.Action("restart")
		if err != nil {
			a.respondAudited(w, r, "singbox.config.save_restart", nil, err, "配置已保存，但重启失败")
			return
		}
		msg := "配置已保存并重启 Sing-box"
		if res != nil && res.Message != "" {
			msg = res.Message + "，并已重启 Sing-box"
		}
		if restartRes != nil && restartRes.Message != "" {
			msg = msg + "（" + restartRes.Message + "）"
		}
		a.auditMessageFromRequest(r, "singbox.config.save_restart", msg)
		respondMessage(w, nil, msg)
		return
	}
	a.respondAudited(w, r, "singbox.config.save", res, nil, "配置已保存")
}

func (a *App) SingBoxSubscriptionSaveAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL  string   `json:"url"`
		URLs []string `json:"urls"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	urls := in.URLs
	if len(urls) == 0 && in.URL != "" {
		urls = []string{in.URL}
	}
	res, err := a.SingBox.SaveNodeSubscriptionURLs(urls)
	if err == nil {
		for _, item := range urls {
			a.SingBox.AppendSubscriptionHistory(item)
		}
	}
	a.respondAudited(w, r, "singbox.subscription.save", res, err, "节点订阅已保存")
}

func (a *App) SingBoxSubscriptionSaveFullAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL string `json:"url"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	res, err := a.SingBox.SaveFullConfigSubscriptionURL(in.URL)
	if err == nil && in.URL != "" {
		a.SingBox.AppendSubscriptionHistory(in.URL)
	}
	a.respondAudited(w, r, "singbox.subscription.full.save", res, err, "完整配置订阅已保存")
}

func (a *App) SingBoxSubscriptionUpdateAPI(w http.ResponseWriter, r *http.Request) {
	res, err := a.SingBox.UpdateSubscription()
	a.respondAudited(w, r, "singbox.subscription.update", res, err, "订阅已更新")
}

func (a *App) SingBoxSubscriptionUpdateFullAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL string `json:"url"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	res, err := a.SingBox.UpdateFullConfigSubscriptionFromURL(in.URL)
	a.respondAudited(w, r, "singbox.subscription.full.update", res, err, "完整配置订阅已更新")
}

func (a *App) SingBoxSubscriptionUpdateNodesAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URLs []string `json:"urls"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	res, err := a.SingBox.UpdateNodeSubscriptionsFromURLs(in.URLs)
	a.respondAudited(w, r, "singbox.subscription.nodes.update", res, err, "节点模板订阅已更新")
}

func (a *App) SingBoxManualNodesAPI(w http.ResponseWriter, r *http.Request) {
	text, err := a.SingBox.ReadManualNodesDraft()
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "nodes": text})
}

func (a *App) SingBoxManualNodesSaveAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Nodes string `json:"nodes"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	res, err := a.SingBox.SaveManualNodesDraft(in.Nodes)
	a.respondAudited(w, r, "singbox.manual_nodes.save", res, err, "手动节点草稿已保存")
}

func (a *App) SingBoxManualNodesImportAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Nodes string `json:"nodes"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	res, err := a.SingBox.ImportManualNodes(in.Nodes)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		payload := map[string]any{"ok": false, "error": err.Error()}
		if res != nil {
			payload["result"] = res
		}
		_ = json.NewEncoder(w).Encode(payload)
		return
	}
	msg := "手动节点导入成功"
	if res != nil && strings.TrimSpace(res.Message) != "" {
		msg = strings.TrimSpace(res.Message)
	}
	a.auditMessageFromRequest(r, "singbox.manual_nodes.import", msg)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": msg, "result": res})
}
func (a *App) SingBoxVersionAPI(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"version": mustStr(a.SingBox.Version())})
}
func (a *App) SingBoxUpgradeAPI(w http.ResponseWriter, r *http.Request) {
	err := a.SingBox.Upgrade()
	a.auditFromRequest(r, "singbox.upgrade", err)
	respondMessage(w, err, "Sing-box 核心已更新")
}

func (a *App) SingBoxIPForwardStatusAPI(w http.ResponseWriter, r *http.Request) {
	status, err := a.SingBox.IPForwardStatus()
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":      false,
			"error":   err.Error(),
			"message": "检测 IP 转发失败",
		})
		return
	}
	message := "未开启 IP 转发"
	if status != nil && status.Enabled {
		message = "已开启 IP 转发"
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"message": message,
		"status":  status,
	})
}

func (a *App) SingBoxUpgradeUploadAPI(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		a.respondAudited(w, r, "singbox.upgrade.upload", nil, fmt.Errorf("解析上传请求失败: %w", err), "")
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		a.respondAudited(w, r, "singbox.upgrade.upload", nil, fmt.Errorf("请上传内核文件(file): %w", err), "")
		return
	}
	defer f.Close()

	tmp, err := os.CreateTemp("", "sing-box-upload-*")
	if err != nil {
		a.respondAudited(w, r, "singbox.upgrade.upload", nil, fmt.Errorf("创建临时文件失败: %w", err), "")
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmp, f); err != nil {
		tmp.Close()
		a.respondAudited(w, r, "singbox.upgrade.upload", nil, fmt.Errorf("保存上传文件失败: %w", err), "")
		return
	}
	if err := tmp.Close(); err != nil {
		a.respondAudited(w, r, "singbox.upgrade.upload", nil, fmt.Errorf("写入上传文件失败: %w", err), "")
		return
	}

	res, err := a.SingBox.UpgradeFromUploadedCore(tmpPath, hdr.Filename)
	a.respondAudited(w, r, "singbox.upgrade.upload", res, err, "上传内核已替换")
}

func (a *App) SingBoxUpgradeRollbackAPI(w http.ResponseWriter, r *http.Request) {
	err := a.SingBox.RollbackCoreUpgrade()
	a.auditFromRequest(r, "singbox.upgrade.rollback", err)
	respondMessage(w, err, "Sing-box 核心已回退到升级前版本")
}

func (a *App) SingBoxCronGetAPI(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(must(a.SingBox.CronShow()))
}

func (a *App) SingBoxCronSetAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Days int `json:"days"`
		Hour int `json:"hour"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	res, err := a.SingBox.CronSet(in.Days, in.Hour)
	a.respondAudited(w, r, "singbox.cron.set", res, err, "操作成功")
}

func (a *App) SingBoxCronDeleteAPI(w http.ResponseWriter, r *http.Request) {
	res, err := a.SingBox.CronDelete()
	a.respondAudited(w, r, "singbox.cron.delete", res, err, "操作成功")
}

func (a *App) MonitorConfigAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":     true,
		"config": a.Config.Monitor,
	})
}

func (a *App) MonitorConfigSaveAPI(w http.ResponseWriter, r *http.Request) {
	var in cfgpkg.MonitorConfig
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respondMessage(w, err, "")
		return
	}
	if strings.TrimSpace(in.DefaultProxyGroup) == "" {
		respondMessage(w, fmt.Errorf("default_proxy_group 不能为空"), "")
		return
	}
	if strings.TrimSpace(in.PrimaryGroup) == "" {
		respondMessage(w, fmt.Errorf("primary_group 不能为空"), "")
		return
	}
	if strings.TrimSpace(in.FallbackGroup) == "" {
		respondMessage(w, fmt.Errorf("fallback_group 不能为空"), "")
		return
	}
	if strings.TrimSpace(in.TestURL) == "" {
		in.TestURL = a.Config.Monitor.TestURL
		if strings.TrimSpace(in.TestURL) == "" {
			in.TestURL = "http://www.gstatic.com/generate_204"
		}
	}
	if in.TimeoutMS <= 0 {
		in.TimeoutMS = 5000
	}
	if in.PrimaryMaxStableDelayMS <= 0 {
		respondMessage(w, fmt.Errorf("primary_max_stable_delay_ms 必须 > 0"), "")
		return
	}
	if in.FallbackMaxStableDelayMS <= 0 {
		respondMessage(w, fmt.Errorf("fallback_max_stable_delay_ms 必须 > 0"), "")
		return
	}
	if in.FailThreshold <= 0 {
		in.FailThreshold = 2
	}
	if in.SuccessThreshold <= 0 {
		in.SuccessThreshold = 2
	}
	if in.RecheckIntervalSec <= 0 {
		in.RecheckIntervalSec = 1
	}
	if strings.TrimSpace(in.StateFile) == "" {
		in.StateFile = a.Config.Monitor.StateFile
		if strings.TrimSpace(in.StateFile) == "" {
			in.StateFile = "/opt/singdns-panel/data/monitor-state.json"
		}
	}
	if strings.TrimSpace(in.APIBase) == "" {
		in.APIBase = a.Config.Monitor.APIBase
		if strings.TrimSpace(in.APIBase) == "" {
			in.APIBase = "http://127.0.0.1:9090"
		}
	}
	cfg := a.Config.Monitor
	cfg.Enabled = in.Enabled
	cfg.APIBase = strings.TrimSpace(in.APIBase)
	cfg.DefaultProxyGroup = strings.TrimSpace(in.DefaultProxyGroup)
	cfg.PrimaryGroup = strings.TrimSpace(in.PrimaryGroup)
	cfg.FallbackGroup = strings.TrimSpace(in.FallbackGroup)
	cfg.TestURL = strings.TrimSpace(in.TestURL)
	cfg.TimeoutMS = in.TimeoutMS
	cfg.PrimaryMaxStableDelayMS = in.PrimaryMaxStableDelayMS
	cfg.FallbackMaxStableDelayMS = in.FallbackMaxStableDelayMS
	cfg.DisablePrimaryGroupOptimization = in.DisablePrimaryGroupOptimization
	cfg.FailThreshold = in.FailThreshold
	cfg.SuccessThreshold = in.SuccessThreshold
	cfg.RecheckIntervalSec = in.RecheckIntervalSec
	cfg.AutoFailback = in.AutoFailback
	cfg.StateFile = strings.TrimSpace(in.StateFile)
	a.Config.Monitor = cfg
	if err := a.Config.Save(a.ConfigPath); err != nil {
		respondMessage(w, err, "")
		return
	}
	a.Monitor = svc.NewMonitorService(cfg, a.SingBox)
	a.auditMessageFromRequest(r, "monitor.config.save", "节点监控策略已保存")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "节点监控策略已保存", "config": cfg})
}

func (a *App) MonitorCronGetAPI(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(must(a.Monitor.CronShow()))
}

func (a *App) MonitorCronSetAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IntervalMinutes int `json:"interval_minutes"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	res, err := a.Monitor.CronSet(in.IntervalMinutes)
	a.respondAudited(w, r, "monitor.cron.set", res, err, "操作成功")
}

func (a *App) MonitorCronDeleteAPI(w http.ResponseWriter, r *http.Request) {
	res, err := a.Monitor.CronDelete()
	a.respondAudited(w, r, "monitor.cron.delete", res, err, "操作成功")
}

func (a *App) MonitorRunAPI(w http.ResponseWriter, r *http.Request) {
	res, err := a.Monitor.RunOnce()
	a.respondAudited(w, r, "monitor.run", res, err, "节点监控已执行")
}

func (a *App) SingBoxBackupsAPI(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(must(a.SingBox.ListBackups()))
}

func (a *App) SingBoxCreateBackupAPI(w http.ResponseWriter, r *http.Request) {
	name, err := a.SingBox.CreateBackup()
	if err != nil {
		a.respondAudited(w, r, "singbox.backup.create", nil, err, "创建配置备份失败")
		return
	}
	msg := "已创建配置备份"
	if name != "" {
		msg += "：" + name
	}
	a.auditMessageFromRequest(r, "singbox.backup.create", msg)
	respondMessage(w, nil, msg)
}

func (a *App) SingBoxBackupDiffAPI(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	diff, err := a.SingBox.BackupDiff(name)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "diff": diff})
}

func (a *App) SingBoxDeleteBackupAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	res, err := a.SingBox.DeleteBackup(in.Name)
	a.respondAudited(w, r, "singbox.backup.delete", res, err, "备份已删除")
}

func (a *App) SingBoxRestoreBackupAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	res, err := a.SingBox.RestoreBackup(in.Name)
	if err != nil {
		a.respondAudited(w, r, "singbox.backup.restore", nil, err, "回滚失败")
		return
	}
	restartRes, err := a.SingBox.Action("restart")
	if err != nil {
		a.respondAudited(w, r, "singbox.backup.restore", nil, err, "配置已回滚，但重启失败")
		return
	}
	msg := "已回滚并重启 Sing-box"
	if res != nil && res.Message != "" {
		msg = res.Message + "，并已重启 Sing-box"
	}
	if restartRes != nil && restartRes.Message != "" {
		msg = msg + "（" + restartRes.Message + "）"
	}
	a.auditMessageFromRequest(r, "singbox.backup.restore", msg)
	respondMessage(w, nil, msg)
}

func must[T any](v T, _ error) T       { return v }
func mustStr(v string, _ error) string { return v }

func respondOK(w http.ResponseWriter, err error) {
	respondMessage(w, err, "操作成功")
}

func respondMessage(w http.ResponseWriter, err error, msg string) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": msg})
}

func intParam(r *http.Request, key string) int {
	v, _ := strconv.Atoi(r.URL.Query().Get(key))
	return v
}
