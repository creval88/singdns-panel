package services

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	cfgpkg "singdns-panel/internal/config"
	"singdns-panel/internal/utils"
)

type SingBoxService struct {
	cfg             cfgpkg.ServiceConfig
	systemd         *SystemdService
	panelConfigPath string
}

type CronInfo struct {
	Enabled bool   `json:"enabled"`
	Raw     string `json:"raw"`
	Summary string `json:"summary"`
	NextRun string `json:"next_run"`
	Days    int    `json:"days"`
	Hour    int    `json:"hour"`
}

type BackupInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	ModTime   string `json:"mod_time"`
	Size      int64  `json:"size"`
	SizeText  string `json:"size_text"`
	AgeText   string `json:"age_text"`
	IsLatest  bool   `json:"is_latest"`
	IsCurrent bool   `json:"is_current"`
}

type SubscriptionStatus struct {
	URL                    string `json:"url"`
	Host                   string `json:"host"`
	Configured             bool   `json:"configured"`
	HistoryCount           int    `json:"history_count"`
	LastHistoryTime        string `json:"last_history_time"`
	UpdateCount            int    `json:"update_count"`
	LastUpdateTime         string `json:"last_update_time"`
	LastUpdateStatus       string `json:"last_update_status"`
	LastUpdateAction       string `json:"last_update_action"`
	LastUpdateStage        string `json:"last_update_stage"`
	LastUpdateMessage      string `json:"last_update_message"`
	LastUpdateDurationMs   int64  `json:"last_update_duration_ms"`
	LastUpdateDurationText string `json:"last_update_duration_text"`
	LastSuccessTime        string `json:"last_success_time"`
	LastSuccessStage       string `json:"last_success_stage"`
	LastSuccessMessage     string `json:"last_success_message"`
}

type BackupStatus struct {
	Count                int    `json:"count"`
	LatestName           string `json:"latest_name"`
	LatestModTime        string `json:"latest_mod_time"`
	LatestAgeText        string `json:"latest_age_text"`
	LatestSizeText       string `json:"latest_size_text"`
	CurrentMatchesName   string `json:"current_matches_name"`
	CurrentMatchesLatest bool   `json:"current_matches_latest"`
}

type ConfigStatus struct {
	UpdatedAt       string `json:"updated_at"`
	ServerBytes     int    `json:"server_bytes"`
	ServerLines     int    `json:"server_lines"`
	ServerJSONValid bool   `json:"server_json_valid"`
}

type ClashAPIInfo struct {
	Enabled bool
	URL     string
	Secret  string
	Port    string
}

func NewSingBoxService(cfg cfgpkg.ServiceConfig, systemd *SystemdService, panelConfigPath string) *SingBoxService {
	return &SingBoxService{cfg: cfg, systemd: systemd, panelConfigPath: strings.TrimSpace(panelConfigPath)}
}

func (s *SingBoxService) Status() (*ServiceStatus, error) { return s.systemd.Status(s.cfg.ServiceName) }
func (s *SingBoxService) Action(action string) (*ServiceActionResult, error) {
	return s.systemd.Action(s.cfg.ServiceName, action)
}
func (s *SingBoxService) Logs(lines int) (string, error) {
	return s.systemd.Logs(s.cfg.ServiceName, lines)
}

func (s *SingBoxService) ReadConfig() (string, error) {
	if b, err := os.ReadFile(s.cfg.ConfigPath); err == nil {
		return string(b), nil
	}
	if s.cfg.CtlPath != "" {
		res, err := utils.Run(10*time.Second, "sudo", s.cfg.CtlPath, "get-config")
		if err == nil {
			return res.Stdout, nil
		}
	}
	b, err := os.ReadFile(s.cfg.ConfigPath)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (s *SingBoxService) ValidateConfig(content string) error {
	tmp := filepath.Join(os.TempDir(), "singdns-panel-singbox-check.json")
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return err
	}
	defer os.Remove(tmp)
	_, err := utils.Run(10*time.Second, s.cfg.BinPath, "check", "-c", tmp)
	return err
}

func (s *SingBoxService) writeConfigFile(content string) error {
	return s.writeManagedFile(s.cfg.ConfigPath, content)
}

func (s *SingBoxService) SaveConfig(content string) (*OperationResult, error) {
	if err := s.ValidateConfig(content); err != nil {
		return nil, err
	}
	backupName, err := s.CreateBackup()
	if err != nil {
		return nil, err
	}
	if err := s.writeConfigFile(content); err != nil {
		return nil, err
	}
	s.PruneBackups(20)
	msg := fmt.Sprintf("配置已保存，写入 %d 字节", len(content))
	if backupName != "" {
		msg += "，已备份为 " + backupName
	}
	return &OperationResult{Action: "config.save", Message: msg}, nil
}

func (s *SingBoxService) ReadSubscriptionURL() (string, error) {
	b, err := os.ReadFile(s.cfg.URLPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func (s *SingBoxService) SaveSubscriptionURL(rawURL string) (*OperationResult, error) {
	rawURL = strings.TrimSpace(rawURL)
	if err := s.writeManagedFile(s.cfg.URLPath, rawURL+"\n"); err != nil {
		return nil, err
	}
	msg := "订阅链接已保存"
	if rawURL == "" {
		msg = "订阅链接已清空"
	}
	return &OperationResult{Action: "subscription.save", Message: msg}, nil
}

func (s *SingBoxService) UpdateSubscription() (*OperationResult, error) {
	rawURL, err := s.ReadSubscriptionURL()
	if err != nil {
		s.AppendSubscriptionUpdateEventDetailed("error", "update", "read-url", "", err.Error(), 0)
		return nil, err
	}
	return s.UpdateSubscriptionFromURL(rawURL)
}

func (s *SingBoxService) UpdateSubscriptionFromURL(rawURL string) (*OperationResult, error) {
	startedAt := time.Now()
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		err := fmt.Errorf("subscription url is empty")
		s.AppendSubscriptionUpdateEventDetailed("error", "update", "validate-url", rawURL, err.Error(), time.Since(startedAt))
		return nil, err
	}
	s.AppendSubscriptionUpdateEventDetailed("info", "update", "start", rawURL, "开始执行订阅更新", 0)
	content, err := s.DownloadSubscription(rawURL)
	if err != nil {
		return nil, err
	}
	return s.ApplySubscriptionContent(rawURL, content, startedAt)
}

func (s *SingBoxService) DownloadSubscription(rawURL string) (string, error) {
	startedAt := time.Now()
	rawURL = strings.TrimSpace(rawURL)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(rawURL)
	if err != nil {
		s.AppendSubscriptionUpdateEventDetailed("error", "download", "download", rawURL, err.Error(), time.Since(startedAt))
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("download subscription failed: %s", resp.Status)
		s.AppendSubscriptionUpdateEventDetailed("error", "download", "download", rawURL, err.Error(), time.Since(startedAt))
		return "", err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		s.AppendSubscriptionUpdateEventDetailed("error", "download", "read-body", rawURL, err.Error(), time.Since(startedAt))
		return "", err
	}
	content := strings.TrimSpace(string(body))
	if content == "" {
		err := fmt.Errorf("downloaded subscription is empty")
		s.AppendSubscriptionUpdateEventDetailed("error", "download", "read-body", rawURL, err.Error(), time.Since(startedAt))
		return "", err
	}
	s.AppendSubscriptionUpdateEventDetailed("info", "download", "download", rawURL, fmt.Sprintf("订阅下载完成，准备校验并写入 %d 字节", len(content)), time.Since(startedAt))
	return content, nil
}

func (s *SingBoxService) ApplySubscriptionContent(rawURL, content string, startedAt time.Time) (*OperationResult, error) {
	rawURL = strings.TrimSpace(rawURL)
	content = strings.TrimSpace(content)
	if content == "" {
		err := fmt.Errorf("subscription content is empty")
		s.AppendSubscriptionUpdateEventDetailed("error", "update", "validate-content", rawURL, err.Error(), time.Since(startedAt))
		return nil, err
	}
	stageStartedAt := time.Now()
	saveResult, err := s.SaveConfig(content)
	if err != nil {
		s.AppendSubscriptionUpdateEventDetailed("error", "update", "save", rawURL, err.Error(), time.Since(stageStartedAt))
		return nil, err
	}
	s.AppendSubscriptionUpdateEventDetailed("info", "update", "save", rawURL, fmt.Sprintf("配置写入完成，已保存 %d 字节", len(content)), time.Since(stageStartedAt))
	s.AppendSubscriptionHistory(rawURL)
	stageStartedAt = time.Now()
	restartResult, err := s.Action("restart")
	if err != nil {
		s.AppendSubscriptionUpdateEventDetailed("error", "update", "restart", rawURL, err.Error(), time.Since(stageStartedAt))
		return nil, err
	}
	msg := fmt.Sprintf("订阅已更新，写入 %d 字节并重启服务", len(content))
	if saveResult != nil && saveResult.Message != "" {
		msg = saveResult.Message + "，并已重启服务"
	}
	if restartResult != nil && restartResult.Message != "" {
		if saveResult != nil && saveResult.Message != "" {
			msg = saveResult.Message + "，" + restartResult.Message
		} else {
			msg = restartResult.Message
		}
	}
	res := &OperationResult{Action: "subscription.update", Message: msg}
	s.AppendSubscriptionUpdateEventDetailed("ok", "update", "complete", rawURL, msg, time.Since(startedAt))
	return res, nil
}

func (s *SingBoxService) RunScheduledSubscriptionUpdate() error {
	_, err := s.UpdateSubscription()
	return err
}

func (s *SingBoxService) SubscriptionStatus() (*SubscriptionStatus, error) {
	rawURL, err := s.ReadSubscriptionURL()
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	history, err := s.SubscriptionHistory()
	if err != nil {
		return nil, err
	}
	updates, err := s.SubscriptionUpdateEvents(20)
	if err != nil {
		return nil, err
	}
	info := &SubscriptionStatus{URL: rawURL, Configured: strings.TrimSpace(rawURL) != "", HistoryCount: len(history), UpdateCount: len(updates)}
	if u, err := url.Parse(strings.TrimSpace(rawURL)); err == nil {
		info.Host = u.Hostname()
	}
	if len(history) > 0 {
		info.LastHistoryTime = history[0].Time
	}
	if len(updates) > 0 {
		info.LastUpdateTime = updates[0].Time
		info.LastUpdateStatus = updates[0].Status
		info.LastUpdateAction = updates[0].Action
		info.LastUpdateStage = updates[0].Stage
		info.LastUpdateMessage = updates[0].Message
		info.LastUpdateDurationMs = updates[0].DurationMs
		info.LastUpdateDurationText = updates[0].DurationText
		for _, item := range updates {
			if item.Status == "ok" {
				info.LastSuccessTime = item.Time
				info.LastSuccessStage = item.Stage
				info.LastSuccessMessage = item.Message
				break
			}
		}
	}
	return info, nil
}

func (s *SingBoxService) BackupStatus() (*BackupStatus, error) {
	items, err := s.ListBackups()
	if err != nil {
		return nil, err
	}
	info := &BackupStatus{Count: len(items)}
	if len(items) > 0 {
		info.LatestName = items[0].Name
		info.LatestModTime = items[0].ModTime
		info.LatestAgeText = items[0].AgeText
		info.LatestSizeText = items[0].SizeText
	}
	for _, item := range items {
		if item.IsCurrent {
			info.CurrentMatchesName = item.Name
			info.CurrentMatchesLatest = item.IsLatest
			break
		}
	}
	return info, nil
}

func (s *SingBoxService) ConfigStatus() (*ConfigStatus, error) {
	cfgText, err := s.ReadConfig()
	if err != nil {
		return nil, err
	}
	updatedAt, _ := s.ConfigUpdatedAt()
	status := &ConfigStatus{UpdatedAt: updatedAt, ServerBytes: len(cfgText), ServerLines: strings.Count(cfgText, "\n") + 1}
	var raw any
	status.ServerJSONValid = json.Unmarshal([]byte(cfgText), &raw) == nil
	return status, nil
}

func (s *SingBoxService) Version() (string, error) {
	res, err := utils.Run(5*time.Second, s.cfg.BinPath, "version")
	if err != nil {
		return res.Stdout + res.Stderr, err
	}
	return strings.TrimSpace(res.Stdout), nil
}

func (s *SingBoxService) LatestVersion() (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get("https://github.com/SagerNet/sing-box/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	finalURL := resp.Request.URL.String()
	parts := strings.Split(strings.TrimRight(finalURL, "/"), "/")
	if len(parts) == 0 {
		return "", nil
	}
	return strings.TrimSpace(parts[len(parts)-1]), nil
}

func (s *SingBoxService) ConfigUpdatedAt() (string, error) {
	st, err := os.Stat(s.cfg.ConfigPath)
	if err != nil {
		return "", err
	}
	return st.ModTime().Format("2006-01-02 15:04:05"), nil
}

func (s *SingBoxService) Upgrade() error {
	latestVer, err := s.LatestVersion()
	if err != nil {
		if s.cfg.CtlPath != "" {
			_, ctlErr := utils.Run(120*time.Second, "sudo", s.cfg.CtlPath, "upgrade")
			return ctlErr
		}
		return err
	}
	if latestVer == "" {
		return fmt.Errorf("failed to get latest version")
	}

	archVal, err := singboxArchForHost()
	if err != nil {
		if s.cfg.CtlPath != "" {
			_, ctlErr := utils.Run(120*time.Second, "sudo", s.cfg.CtlPath, "upgrade")
			return ctlErr
		}
		return err
	}

	verNum := strings.TrimPrefix(latestVer, "v")
	downloadURL := fmt.Sprintf("https://github.com/SagerNet/sing-box/releases/download/%s/sing-box-%s-linux-%s.tar.gz", latestVer, verNum, archVal)

	tmpTar, err := os.CreateTemp("", "sing-box-*.tar.gz")
	if err != nil {
		return err
	}
	tmpTarPath := tmpTar.Name()
	tmpTar.Close()
	defer os.Remove(tmpTarPath)

	if err := downloadFile(downloadURL, tmpTarPath, 2*time.Minute); err != nil {
		if s.cfg.CtlPath != "" {
			_, ctlErr := utils.Run(120*time.Second, "sudo", s.cfg.CtlPath, "upgrade")
			return ctlErr
		}
		return err
	}

	binPath, err := extractSingboxBinary(tmpTarPath, verNum, archVal)
	if err != nil {
		if s.cfg.CtlPath != "" {
			_, ctlErr := utils.Run(120*time.Second, "sudo", s.cfg.CtlPath, "upgrade")
			return ctlErr
		}
		return err
	}
	defer os.Remove(binPath)

	// 平滑停启
	_, _ = utils.Run(20*time.Second, "sudo", "systemctl", "stop", s.cfg.ServiceName)
	_, _ = utils.Run(10*time.Second, "sudo", "mkdir", "-p", filepath.Dir(s.cfg.BinPath))
	if res, err := utils.Run(30*time.Second, "sudo", "install", "-m", "755", binPath, s.cfg.BinPath); err != nil {
		if s.cfg.CtlPath != "" {
			if _, ctlErr := utils.Run(120*time.Second, "sudo", s.cfg.CtlPath, "upgrade"); ctlErr == nil {
				return nil
			}
		}
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		return fmt.Errorf("安装新内核失败（目标 %s）: %v, detail=%s", s.cfg.BinPath, err, msg)
	}
	if _, err := utils.Run(20*time.Second, "sudo", "systemctl", "start", s.cfg.ServiceName); err != nil {
		return err
	}
	if _, err := utils.Run(10*time.Second, "sudo", s.cfg.BinPath, "version"); err != nil {
		return err
	}
	return nil
}

func (s *SingBoxService) CronShow() (*CronInfo, error) {
	info := &CronInfo{}
	lines, err := s.readRootCrontab()
	if err != nil {
		return nil, err
	}
	if line := s.findManagedCronLine(lines); line != "" {
		info.Enabled = true
		info.Raw = line
		info.Summary = cronLineSummary(line)
		parseCronLine(line, info)
	}
	return info, nil
}

func parseCronLine(line string, info *CronInfo) {
	// Example: "0 3 */2 * *" or "0 3 * * *"
	parts := strings.Fields(line)
	if len(parts) < 5 {
		return
	}
	// Parse days and hour from cron expression
	// minute hour day-of-month month day-of-week
	minute := parts[0]
	hourPart := parts[1]
	dayPart := parts[2] // */n means every n days

	hour := 0
	fmt.Sscanf(hourPart, "%d", &hour)
	info.Hour = hour

	days := 1
	if strings.HasPrefix(dayPart, "*/") {
		fmt.Sscanf(dayPart[2:], "%d", &days)
	}
	info.Days = days

	// Calculate next run time
	now := time.Now()
	next := now
	if days > 1 {
		// Every N days
		next = time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
		if next.Before(now) || next.Equal(now) {
			next = next.AddDate(0, 0, days)
		}
		// Find the next occurrence
		for next.Before(now) {
			next = next.AddDate(0, 0, days)
		}
	} else {
		// Daily at specific hour
		next = time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
		if next.Before(now) {
			next = next.AddDate(0, 0, 1)
		}
	}
	_ = minute // could be used for more precise calculation
	info.NextRun = next.Format("2006-01-02 15:04")
}

func (s *SingBoxService) CronSet(days, hour int) (*OperationResult, error) {
	if days <= 0 {
		days = 1
	}
	if hour < 0 || hour > 23 {
		return nil, fmt.Errorf("invalid hour: %d", hour)
	}
	cmdLine, err := s.cronUpdateCommand()
	if err != nil {
		return nil, err
	}
	lines, err := s.readRootCrontab()
	if err != nil {
		return nil, err
	}
	lines = s.filterManagedCronLines(lines)
	expr := fmt.Sprintf("0 %d * * *", hour)
	if days != 1 {
		expr = fmt.Sprintf("0 %d */%d * *", hour, days)
	}
	lines = append(lines, fmt.Sprintf("%s %s", expr, cmdLine))
	if err := s.writeRootCrontab(lines); err != nil {
		return nil, err
	}
	return &OperationResult{Action: "cron.set", Message: cronSetMessage(days, hour)}, nil
}

func (s *SingBoxService) CronDelete() (*OperationResult, error) {
	lines, err := s.readRootCrontab()
	if err != nil {
		return nil, err
	}
	filtered := s.filterManagedCronLines(lines)
	if err := s.writeRootCrontab(filtered); err != nil {
		return nil, err
	}
	return &OperationResult{Action: "cron.delete", Message: "订阅自动更新任务已删除"}, nil
}

func (s *SingBoxService) readRootCrontab() ([]string, error) {
	res, err := utils.Run(5*time.Second, "sudo", "crontab", "-l")
	if err != nil {
		stderr := strings.TrimSpace(res.Stderr)
		if stderr == "no crontab for root" {
			return nil, nil
		}
		return nil, err
	}
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(res.Stdout, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed == "" {
			continue
		}
		lines = append(lines, trimmed)
	}
	return lines, nil
}

func (s *SingBoxService) writeRootCrontab(lines []string) error {
	content := strings.Join(lines, "\n")
	if content != "" {
		content += "\n"
	}
	_, err := s.runCommandWithInput(10*time.Second, content, "sudo", "crontab", "-")
	return err
}

func (s *SingBoxService) cronUpdateCommand() (string, error) {
	exePath, err := os.Executable()
	if err == nil && strings.TrimSpace(exePath) != "" && strings.TrimSpace(s.panelConfigPath) != "" {
		return fmt.Sprintf("SINGDNS_CONFIG=%s %s subscription-update", shellQuote(s.panelConfigPath), shellQuote(exePath)), nil
	}
	return "", fmt.Errorf("unable to determine subscription update command")
}

func (s *SingBoxService) filterManagedCronLines(lines []string) []string {
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if s.isManagedCronLine(line) {
			continue
		}
		filtered = append(filtered, line)
	}
	return filtered
}

func (s *SingBoxService) findManagedCronLine(lines []string) string {
	for _, line := range lines {
		if s.isManagedCronLine(line) {
			return line
		}
	}
	return ""
}

func (s *SingBoxService) isManagedCronLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return false
	}
	return strings.Contains(line, " subscription-update")
}

func cronLineSummary(line string) string {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return strings.TrimSpace(line)
	}
	hour := fields[1]
	day := fields[2]
	switch {
	case day == "*":
		return fmt.Sprintf("每天 %s:00", hour)
	case strings.HasPrefix(day, "*/"):
		return fmt.Sprintf("每隔 %s 天 %s:00", strings.TrimPrefix(day, "*/"), hour)
	default:
		return fmt.Sprintf("cron: %s %s %s %s %s", fields[0], fields[1], fields[2], fields[3], fields[4])
	}
}

func cronSetMessage(days, hour int) string {
	if days <= 1 {
		return fmt.Sprintf("已设置为每天 %02d:00 自动更新订阅", hour)
	}
	return fmt.Sprintf("已设置为每隔 %d 天 %02d:00 自动更新订阅", days, hour)
}

func shellQuote(v string) string {
	if v == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'"
}

func singboxArchForHost() (string, error) {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64", nil
	case "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported arch: %s", runtime.GOARCH)
	}
}

func downloadFile(downloadURL, targetPath string, timeout time.Duration) error {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	f, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	return nil
}

func extractSingboxBinary(tarGzPath, verNum, arch string) (string, error) {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	expected := fmt.Sprintf("sing-box-%s-linux-%s/sing-box", verNum, arch)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if hdr.Name != expected {
			continue
		}
		tmpBin, err := os.CreateTemp("", "sing-box-bin-*")
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(tmpBin, tr); err != nil {
			tmpBin.Close()
			os.Remove(tmpBin.Name())
			return "", err
		}
		tmpBin.Close()
		if err := os.Chmod(tmpBin.Name(), 0755); err != nil {
			os.Remove(tmpBin.Name())
			return "", err
		}
		return tmpBin.Name(), nil
	}
	return "", fmt.Errorf("sing-box binary not found in archive")
}

func (s *SingBoxService) ClashAPIInfo(panelHost string) (*ClashAPIInfo, error) {
	cfgText, err := s.ReadConfig()
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(cfgText), &raw); err != nil {
		return nil, err
	}
	info := &ClashAPIInfo{}
	experimental, _ := raw["experimental"].(map[string]any)
	clash, _ := experimental["clash_api"].(map[string]any)
	controller, _ := clash["external_controller"].(string)
	secret, _ := clash["secret"].(string)
	if controller == "" {
		if v, _ := raw["clash_api"].(map[string]any); v != nil {
			if controller == "" {
				controller, _ = v["external_controller"].(string)
			}
			if secret == "" {
				secret, _ = v["secret"].(string)
			}
		}
	}
	if controller == "" {
		return info, nil
	}
	_, port, err := net.SplitHostPort(controller)
	if err != nil {
		parts := strings.Split(controller, ":")
		port = parts[len(parts)-1]
	}
	if port == "" {
		port = "9090"
	}
	host := panelHost
	if strings.Contains(host, ":") {
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		} else {
			host = strings.Split(host, ":")[0]
		}
	}
	q := url.Values{}
	q.Set("host", host)
	q.Set("hostname", host)
	q.Set("port", port)
	if secret != "" {
		q.Set("secret", secret)
	}
	info.Enabled = true
	info.Secret = secret
	info.Port = port
	info.URL = fmt.Sprintf("http://%s:%s/ui/?%s#/proxies", host, port, q.Encode())
	return info, nil
}
