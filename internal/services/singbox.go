package services

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	cfgpkg "singdns-panel/internal/config"
	"singdns-panel/internal/utils"
)

const maxUploadedCoreBytes = 500 * 1024 * 1024 // 500MB

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

type SubscriptionNodeResult struct {
	URL         string `json:"url"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	ParsedNodes int    `json:"parsed_nodes,omitempty"`
}

type SubscriptionNodeSummary struct {
	UpdatedAt     string `json:"updated_at"`
	Total         int    `json:"total"`
	Success       int    `json:"success"`
	Ignored       int    `json:"ignored"`
	Failed        int    `json:"failed"`
	ParsedNodes   int    `json:"parsed_nodes"`
	MatchedGroups int    `json:"matched_groups"`
}

type SubscriptionSourceNode struct {
	Tag       string   `json:"tag"`
	Type      string   `json:"type,omitempty"`
	Server    string   `json:"server,omitempty"`
	Source    string   `json:"source"`
	Groups    []string `json:"groups,omitempty"`
	SelfBuilt bool     `json:"self_built"`
}

type SubscriptionSourceSummary struct {
	ActiveMode        string                   `json:"active_mode"`
	ManagedNodeCount  int                      `json:"managed_node_count"`
	ManualNodeCount   int                      `json:"manual_node_count"`
	SubscriptionCount int                      `json:"subscription_count"`
	UnknownNodeCount  int                      `json:"unknown_node_count"`
	SelfBuiltCount    int                      `json:"self_built_count"`
	MatchedGroupCount int                      `json:"matched_group_count"`
	Nodes             []SubscriptionSourceNode `json:"nodes"`
	Notes             []string                 `json:"notes,omitempty"`
}

type SubscriptionStatus struct {
	URL                    string                     `json:"url"`
	URLs                   []string                   `json:"urls"`
	Host                   string                     `json:"host"`
	Hosts                  []string                   `json:"hosts"`
	FullConfigURL          string                     `json:"full_config_url"`
	NodeURLs               []string                   `json:"node_urls"`
	NodeHosts              []string                   `json:"node_hosts"`
	NodeResults            []SubscriptionNodeResult   `json:"node_results"`
	NodeSummary            *SubscriptionNodeSummary   `json:"node_summary"`
	FullConfigured         bool                       `json:"full_configured"`
	NodeConfigured         bool                       `json:"node_configured"`
	Configured             bool                       `json:"configured"`
	HistoryCount           int                        `json:"history_count"`
	LastHistoryTime        string                     `json:"last_history_time"`
	UpdateCount            int                        `json:"update_count"`
	LastUpdateTime         string                     `json:"last_update_time"`
	LastUpdateStatus       string                     `json:"last_update_status"`
	LastUpdateAction       string                     `json:"last_update_action"`
	LastUpdateStage        string                     `json:"last_update_stage"`
	LastUpdateMessage      string                     `json:"last_update_message"`
	LastUpdateDurationMs   int64                      `json:"last_update_duration_ms"`
	LastUpdateDurationText string                     `json:"last_update_duration_text"`
	LastSuccessTime        string                     `json:"last_success_time"`
	LastSuccessStage       string                     `json:"last_success_stage"`
	LastSuccessMessage     string                     `json:"last_success_message"`
	Sources                *SubscriptionSourceSummary `json:"sources"`
}

func normalizeSubscriptionURLs(raw string) []string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	seen := map[string]struct{}{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		out = append(out, line)
	}
	return out
}

func joinSubscriptionURLs(urls []string) string {
	return strings.Join(normalizeSubscriptionURLs(strings.Join(urls, "\n")), "\n")
}

func (s *SingBoxService) nodeSubscriptionURLPath() string {
	base := strings.TrimSpace(s.cfg.URLPath)
	if base == "" {
		return ""
	}
	ext := filepath.Ext(base)
	if ext == "" {
		return base + ".nodes"
	}
	return strings.TrimSuffix(base, ext) + ".nodes" + ext
}

func parseSubscriptionHosts(urls []string) []string {
	hosts := make([]string, 0, len(urls))
	seen := map[string]struct{}{}
	for _, rawURL := range urls {
		if u, err := url.Parse(strings.TrimSpace(rawURL)); err == nil {
			if host := strings.TrimSpace(u.Hostname()); host != "" {
				if _, ok := seen[host]; !ok {
					seen[host] = struct{}{}
					hosts = append(hosts, host)
				}
			}
		}
	}
	return hosts
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

type IPForwardStatus struct {
	Enabled bool   `json:"enabled"`
	Value   string `json:"value"`
	Source  string `json:"source"`
	Message string `json:"message"`
}

func NewSingBoxService(cfg cfgpkg.ServiceConfig, systemd *SystemdService, panelConfigPath string) *SingBoxService {
	cfg.BinPath = resolveSingBoxBinPath(cfg.BinPath)
	return &SingBoxService{cfg: cfg, systemd: systemd, panelConfigPath: strings.TrimSpace(panelConfigPath)}
}

func resolveSingBoxBinPath(configured string) string {
	candidates := []string{}
	configured = strings.TrimSpace(configured)
	if configured != "" {
		candidates = append(candidates, configured)
	}
	candidates = append(candidates, "/usr/bin/sing-box", "/usr/local/bin/sing-box")
	seen := map[string]struct{}{}
	for _, path := range candidates {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			return path
		}
	}
	if configured != "" {
		return configured
	}
	return "/usr/bin/sing-box"
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
	res, err := utils.Run(10*time.Second, s.cfg.BinPath, "check", "-c", tmp)
	if err != nil {
		detail := strings.TrimSpace((res.Stdout + "\n" + res.Stderr))
		if detail == "" {
			return err
		}
		line, col := extractLineCol(detail)
		if line > 0 {
			if col > 0 {
				return fmt.Errorf("配置校验失败（line %d, col %d）: %s", line, col, detail)
			}
			return fmt.Errorf("配置校验失败（line %d）: %s", line, detail)
		}
		return fmt.Errorf("配置校验失败: %s", detail)
	}
	return nil
}

func extractLineCol(detail string) (line int, col int) {
	re := regexp.MustCompile(`(?i)line\s*(\d+)(?:\s*[:,]?\s*(?:col|column)\s*(\d+))?`)
	m := re.FindStringSubmatch(detail)
	if len(m) >= 2 {
		fmt.Sscanf(m[1], "%d", &line)
	}
	if len(m) >= 3 && m[2] != "" {
		fmt.Sscanf(m[2], "%d", &col)
	}
	if line == 0 {
		re2 := regexp.MustCompile(`(?i)(\d+):(\d+)`)
		m2 := re2.FindStringSubmatch(detail)
		if len(m2) == 3 {
			fmt.Sscanf(m2[1], "%d", &line)
			fmt.Sscanf(m2[2], "%d", &col)
		}
	}
	return
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

func (s *SingBoxService) saveSubscriptionConfig(content string) (*OperationResult, error) {
	if err := s.ValidateConfig(content); err != nil {
		return nil, err
	}
	backupName, err := s.CreateSubscriptionRollbackBackup()
	if err != nil {
		return nil, err
	}
	if err := s.writeConfigFile(content); err != nil {
		return nil, err
	}
	msg := fmt.Sprintf("订阅配置已保存，写入 %d 字节", len(content))
	if backupName != "" {
		msg += "，已生成订阅回滚备份"
	}
	return &OperationResult{Action: "subscription.config.save", Message: msg}, nil
}

func (s *SingBoxService) ReadSubscriptionURLs() ([]string, error) {
	return s.ReadNodeSubscriptionURLs()
}

func (s *SingBoxService) ReadSubscriptionURL() (string, error) {
	return s.ReadFullConfigSubscriptionURL()
}

func (s *SingBoxService) ReadFullConfigSubscriptionURL() (string, error) {
	b, err := os.ReadFile(s.cfg.URLPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func (s *SingBoxService) ReadNodeSubscriptionURLs() ([]string, error) {
	path := s.nodeSubscriptionURLPath()
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return normalizeSubscriptionURLs(string(b)), nil
}

func (s *SingBoxService) writeSubscriptionURLFile(path string, raw string) error {
	content := ""
	if strings.TrimSpace(raw) != "" {
		content = strings.TrimSpace(raw) + "\n"
	}
	return s.writeManagedFile(path, content)
}

func (s *SingBoxService) SaveFullConfigSubscriptionURL(rawURL string) (*OperationResult, error) {
	rawURL = strings.TrimSpace(rawURL)
	if err := s.writeSubscriptionURLFile(s.cfg.URLPath, rawURL); err != nil {
		return nil, err
	}
	msg := "完整配置订阅已保存"
	if rawURL == "" {
		msg = "完整配置订阅已清空"
	}
	return &OperationResult{Action: "subscription.full.save", Message: msg}, nil
}

func (s *SingBoxService) SaveNodeSubscriptionURLs(rawURLs []string) (*OperationResult, error) {
	urls := normalizeSubscriptionURLs(strings.Join(rawURLs, "\n"))
	joined := joinSubscriptionURLs(urls)
	if err := s.writeSubscriptionURLFile(s.nodeSubscriptionURLPath(), joined); err != nil {
		return nil, err
	}
	msg := "节点订阅已保存"
	if joined == "" {
		msg = "节点订阅已清空"
	} else if len(urls) > 1 {
		msg = fmt.Sprintf("节点订阅已保存（%d 条）", len(urls))
	}
	return &OperationResult{Action: "subscription.nodes.save", Message: msg}, nil
}

func (s *SingBoxService) SaveSubscriptionURLs(rawURLs []string) (*OperationResult, error) {
	return s.SaveNodeSubscriptionURLs(rawURLs)
}

func (s *SingBoxService) SaveSubscriptionURL(rawURL string) (*OperationResult, error) {
	return s.SaveFullConfigSubscriptionURL(rawURL)
}

func (s *SingBoxService) UpdateSubscription() (*OperationResult, error) {
	fullURL, fullErr := s.ReadFullConfigSubscriptionURL()
	nodeURLs, nodeErr := s.ReadNodeSubscriptionURLs()
	activeMode := detectSubscriptionActiveMode(strings.TrimSpace(fullURL), nodeURLs, s.readManagedTagsState())

	switch activeMode {
	case "nodes_template":
		if len(nodeURLs) > 0 {
			return s.UpdateNodeSubscriptionsFromURLs(nodeURLs)
		}
		if strings.TrimSpace(fullURL) != "" {
			return s.UpdateFullConfigSubscriptionFromURL(fullURL)
		}
	case "full_config":
		if strings.TrimSpace(fullURL) != "" {
			return s.UpdateFullConfigSubscriptionFromURL(fullURL)
		}
		if len(nodeURLs) > 0 {
			return s.UpdateNodeSubscriptionsFromURLs(nodeURLs)
		}
	}

	switch {
	case fullErr != nil && !os.IsNotExist(fullErr):
		s.AppendSubscriptionUpdateEventDetailed("error", "update", "read-url", "", fullErr.Error(), 0)
		return nil, fullErr
	case nodeErr != nil && !os.IsNotExist(nodeErr):
		s.AppendSubscriptionUpdateEventDetailed("error", "update", "read-node-urls", "", nodeErr.Error(), 0)
		return nil, nodeErr
	case len(nodeURLs) > 0:
		return s.UpdateNodeSubscriptionsFromURLs(nodeURLs)
	case strings.TrimSpace(fullURL) != "":
		return s.UpdateFullConfigSubscriptionFromURL(fullURL)
	case fullErr != nil:
		s.AppendSubscriptionUpdateEventDetailed("error", "update", "read-url", "", fullErr.Error(), 0)
		return nil, fullErr
	case nodeErr != nil:
		s.AppendSubscriptionUpdateEventDetailed("error", "update", "read-node-urls", "", nodeErr.Error(), 0)
		return nil, nodeErr
	default:
		err := fmt.Errorf("subscription url is empty")
		s.AppendSubscriptionUpdateEventDetailed("error", "update", "read-url", "", err.Error(), 0)
		return nil, err
	}
}

func (s *SingBoxService) UpdateSubscriptionFromURL(rawURL string) (*OperationResult, error) {
	return s.UpdateFullConfigSubscriptionFromURL(rawURL)
}

func (s *SingBoxService) UpdateSubscriptionFromURLs(rawURLs []string) (*OperationResult, error) {
	return s.UpdateNodeSubscriptionsFromURLs(rawURLs)
}

func (s *SingBoxService) UpdateFullConfigSubscriptionFromURL(rawURL string) (*OperationResult, error) {
	return s.ImportSubscriptionFromURL(rawURL)
}

func (s *SingBoxService) UpdateNodeSubscriptionsFromURLs(rawURLs []string) (*OperationResult, error) {
	return s.ImportSubscriptionsFromURLs(rawURLs)
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
	saveResult, err := s.saveSubscriptionConfig(content)
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
		if rbErr := s.RestoreSubscriptionRollbackBackup(); rbErr == nil {
			_, _ = s.Action("restart")
			s.AppendSubscriptionUpdateEventDetailed("ok", "rollback", "subscription", rawURL, "订阅更新重启失败，已自动回滚到更新前配置", time.Since(stageStartedAt))
			return nil, fmt.Errorf("订阅更新重启失败，已自动回滚到更新前配置: %w", err)
		}
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

func parseFirstInt(message string, patterns ...string) int {
	for _, p := range patterns {
		re := regexp.MustCompile(p)
		m := re.FindStringSubmatch(message)
		if len(m) < 2 {
			continue
		}
		v, err := strconv.Atoi(strings.TrimSpace(m[1]))
		if err == nil {
			return v
		}
	}
	return 0
}

func buildNodeSubscriptionResultMap(nodeURLs []string, updates []SubscriptionUpdateEvent) (map[string]SubscriptionNodeResult, *SubscriptionNodeSummary) {
	resultMap := make(map[string]SubscriptionNodeResult, len(nodeURLs))
	for _, u := range nodeURLs {
		resultMap[u] = SubscriptionNodeResult{URL: u, Status: "pending", Message: "尚无更新记录"}
	}
	if len(nodeURLs) == 0 {
		return resultMap, nil
	}

	summary := &SubscriptionNodeSummary{Total: len(nodeURLs)}
	for _, ev := range updates {
		if ev.Action != "import" {
			continue
		}
		if summary.UpdatedAt == "" && strings.TrimSpace(ev.Time) != "" {
			summary.UpdatedAt = ev.Time
		}
		if ev.Stage == "summary" {
			summary.ParsedNodes = parseFirstInt(ev.Message, `解析节点\s*(\d+)\s*个`)
			summary.MatchedGroups = parseFirstInt(ev.Message, `展开组\s*(\d+)\s*个`)
			continue
		}

		urlKey := strings.TrimSpace(ev.URL)
		if urlKey == "" {
			continue
		}
		curr, ok := resultMap[urlKey]
		if !ok || curr.Status != "pending" {
			continue
		}
		if ev.Status == "ok" {
			curr.Status = "success"
			curr.Message = ev.Message
			if ev.Stage == "parse" {
				curr.ParsedNodes = parseFirstInt(ev.Message, `获取节点\s*(\d+)\s*个`)
			}
			resultMap[urlKey] = curr
			continue
		}
		if ev.Status == "error" {
			if strings.Contains(ev.Message, "已忽略") {
				curr.Status = "ignored"
			} else {
				curr.Status = "failed"
			}
			curr.Message = ev.Message
			resultMap[urlKey] = curr
		}
	}

	for _, u := range nodeURLs {
		item := resultMap[u]
		switch item.Status {
		case "success":
			summary.Success++
			summary.ParsedNodes += item.ParsedNodes
		case "ignored":
			summary.Ignored++
		case "failed":
			summary.Failed++
		}
	}
	if summary.Total > 0 && summary.Success == 0 && summary.Ignored == 0 && summary.Failed == 0 {
		for _, item := range resultMap {
			if item.Status == "pending" {
				summary.Failed++
			}
		}
	}
	return resultMap, summary
}

func (s *SingBoxService) SubscriptionStatus() (*SubscriptionStatus, error) {
	fullURL, err := s.ReadFullConfigSubscriptionURL()
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	nodeURLs, err := s.ReadNodeSubscriptionURLs()
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
	hosts := parseSubscriptionHosts(nodeURLs)
	resultMap, nodeSummary := buildNodeSubscriptionResultMap(nodeURLs, updates)
	nodeResults := make([]SubscriptionNodeResult, 0, len(nodeURLs))
	for _, u := range nodeURLs {
		nodeResults = append(nodeResults, resultMap[u])
	}

	info := &SubscriptionStatus{
		URL:            fullURL,
		URLs:           nodeURLs,
		FullConfigURL:  fullURL,
		NodeURLs:       nodeURLs,
		NodeHosts:      hosts,
		NodeResults:    nodeResults,
		NodeSummary:    nodeSummary,
		FullConfigured: strings.TrimSpace(fullURL) != "",
		NodeConfigured: len(nodeURLs) > 0,
		Configured:     strings.TrimSpace(fullURL) != "" || len(nodeURLs) > 0,
		HistoryCount:   len(history),
		UpdateCount:    len(updates),
	}
	info.Hosts = hosts
	if len(hosts) > 0 {
		info.Host = hosts[0]
		if len(hosts) > 1 {
			info.Host = fmt.Sprintf("%s 等 %d 个", hosts[0], len(hosts))
		}
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
	if sources, err := s.buildSubscriptionSourceSummary(fullURL, nodeURLs); err == nil && sources != nil {
		info.Sources = sources
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
	startedAt := time.Now()
	latestVer, err := s.LatestVersion()
	if err != nil {
		if s.cfg.CtlPath != "" {
			_, ctlErr := utils.Run(120*time.Second, "sudo", s.cfg.CtlPath, "upgrade")
			if ctlErr != nil {
				s.logCoreEvent("error", "upgrade", "latest-version", fmt.Sprintf("获取最新版本失败，fallback ctl 也失败: %v", ctlErr), time.Since(startedAt))
				return ctlErr
			}
			s.logCoreEvent("ok", "upgrade", "ctl-fallback", "已通过 ctl fallback 完成内核升级", time.Since(startedAt))
			return nil
		}
		s.logCoreEvent("error", "upgrade", "latest-version", err.Error(), time.Since(startedAt))
		return err
	}
	if latestVer == "" {
		err := fmt.Errorf("failed to get latest version")
		s.logCoreEvent("error", "upgrade", "latest-version", err.Error(), time.Since(startedAt))
		return err
	}

	archVal, err := singboxArchForHost()
	if err != nil {
		if s.cfg.CtlPath != "" {
			_, ctlErr := utils.Run(120*time.Second, "sudo", s.cfg.CtlPath, "upgrade")
			if ctlErr != nil {
				s.logCoreEvent("error", "upgrade", "arch", fmt.Sprintf("识别架构失败，fallback ctl 也失败: %v", ctlErr), time.Since(startedAt))
				return ctlErr
			}
			s.logCoreEvent("ok", "upgrade", "ctl-fallback", "识别架构失败，已通过 ctl fallback 完成升级", time.Since(startedAt))
			return nil
		}
		s.logCoreEvent("error", "upgrade", "arch", err.Error(), time.Since(startedAt))
		return err
	}

	// Try to prepare rollback only if an existing binary is present; do not fail upgrade when absent
	rollbackBin := ""
	if st, statErr := os.Stat(s.cfg.BinPath); statErr == nil && !st.IsDir() {
		if rb, rbErr := s.prepareCoreRollbackBinary(); rbErr != nil {
			s.logCoreEvent("warn", "upgrade", "prepare-rollback", fmt.Sprintf("升级前备份当前内核失败，将在失败时跳过自动回滚: %v", rbErr), time.Since(startedAt))
		} else {
			rollbackBin = rb
		}
	} else if statErr != nil && !os.IsNotExist(statErr) {
		s.logCoreEvent("warn", "upgrade", "prepare-rollback", fmt.Sprintf("检查当前内核失败: %v", statErr), time.Since(startedAt))
	} else {
		s.logCoreEvent("info", "upgrade", "prepare-rollback", "未检测到已安装内核，将直接安装最新版本", time.Since(startedAt))
	}

	verNum := strings.TrimPrefix(latestVer, "v")
	downloadURL := fmt.Sprintf("https://github.com/SagerNet/sing-box/releases/download/%s/sing-box-%s-linux-%s.tar.gz", latestVer, verNum, archVal)

	tmpTar, err := os.CreateTemp("", "sing-box-*.tar.gz")
	if err != nil {
		s.logCoreEvent("error", "upgrade", "prepare-download", err.Error(), time.Since(startedAt))
		return err
	}
	tmpTarPath := tmpTar.Name()
	tmpTar.Close()
	defer os.Remove(tmpTarPath)

	if err := downloadFile(downloadURL, tmpTarPath, 2*time.Minute); err != nil {
		if s.cfg.CtlPath != "" {
			_, ctlErr := utils.Run(120*time.Second, "sudo", s.cfg.CtlPath, "upgrade")
			if ctlErr != nil {
				s.logCoreEvent("error", "upgrade", "download", fmt.Sprintf("下载失败，fallback ctl 也失败: %v", ctlErr), time.Since(startedAt))
				return ctlErr
			}
			s.logCoreEvent("ok", "upgrade", "ctl-fallback", "下载失败，已通过 ctl fallback 完成升级", time.Since(startedAt))
			return nil
		}
		s.logCoreEvent("error", "upgrade", "download", err.Error(), time.Since(startedAt))
		return err
	}

	binPath, err := extractSingboxBinary(tmpTarPath, verNum, archVal)
	if err != nil {
		if s.cfg.CtlPath != "" {
			_, ctlErr := utils.Run(120*time.Second, "sudo", s.cfg.CtlPath, "upgrade")
			if ctlErr != nil {
				s.logCoreEvent("error", "upgrade", "extract", fmt.Sprintf("解包失败，fallback ctl 也失败: %v", ctlErr), time.Since(startedAt))
				return ctlErr
			}
			s.logCoreEvent("ok", "upgrade", "ctl-fallback", "解包失败，已通过 ctl fallback 完成升级", time.Since(startedAt))
			return nil
		}
		s.logCoreEvent("error", "upgrade", "extract", err.Error(), time.Since(startedAt))
		return err
	}
	defer os.Remove(binPath)

	_, _ = runSudoNoPrompt(20*time.Second, "/bin/systemctl", "stop", s.cfg.ServiceName)
	_, _ = runSudoNoPrompt(10*time.Second, "/bin/mkdir", "-p", filepath.Dir(s.cfg.BinPath))
	if err := s.installCoreBinary(binPath); err != nil {
		s.logCoreEvent("error", "upgrade", "install", err.Error(), time.Since(startedAt))
		if strings.TrimSpace(rollbackBin) == "" {
			return fmt.Errorf("安装新内核失败: %w", err)
		}
		if rbErr := s.rollbackCoreFromBinary(rollbackBin); rbErr == nil {
			s.logCoreEvent("ok", "rollback", "auto", "升级安装失败，已自动回退到升级前版本", time.Since(startedAt))
			return fmt.Errorf("安装新内核失败，已自动回退到升级前版本: %w", err)
		} else {
			s.logCoreEvent("error", "rollback", "auto", fmt.Sprintf("升级安装失败，自动回退失败: %v", rbErr), time.Since(startedAt))
			return fmt.Errorf("安装新内核失败，且自动回退失败: install_err=%v, rollback_err=%v", err, rbErr)
		}
	}
	if res, err := runSudoNoPrompt(20*time.Second, "/bin/systemctl", "start", s.cfg.ServiceName); err != nil {
		s.logCoreEvent("error", "upgrade", "start", commandOutputOrError(res, err), time.Since(startedAt))
		if strings.TrimSpace(rollbackBin) == "" {
			return fmt.Errorf("新内核启动失败: %w", err)
		}
		if rbErr := s.rollbackCoreFromBinary(rollbackBin); rbErr == nil {
			s.logCoreEvent("ok", "rollback", "auto", "新内核启动失败，已自动回退到升级前版本", time.Since(startedAt))
			return fmt.Errorf("新内核启动失败，已自动回退到升级前版本: %w", err)
		} else {
			s.logCoreEvent("error", "rollback", "auto", fmt.Sprintf("新内核启动失败，自动回退失败: %v", rbErr), time.Since(startedAt))
			return fmt.Errorf("新内核启动失败，且自动回退失败: start_err=%v, rollback_err=%v", err, rbErr)
		}
	}
	if err := s.verifyCoreVersion(); err != nil {
		s.logCoreEvent("error", "upgrade", "verify", err.Error(), time.Since(startedAt))
		if strings.TrimSpace(rollbackBin) == "" {
			return fmt.Errorf("新内核版本校验失败: %w", err)
		}
		if rbErr := s.rollbackCoreFromBinary(rollbackBin); rbErr == nil {
			s.logCoreEvent("ok", "rollback", "auto", "新内核校验失败，已自动回退到升级前版本", time.Since(startedAt))
			return fmt.Errorf("新内核版本校验失败，已自动回退到升级前版本: %w", err)
		} else {
			s.logCoreEvent("error", "rollback", "auto", fmt.Sprintf("新内核校验失败，自动回退失败: %v", rbErr), time.Since(startedAt))
			return fmt.Errorf("新内核版本校验失败，且自动回退失败: verify_err=%v, rollback_err=%v", err, rbErr)
		}
	}
	s.logCoreEvent("ok", "upgrade", "complete", fmt.Sprintf("Sing-box 核心已升级到 %s", latestVer), time.Since(startedAt))
	return nil
}

func (s *SingBoxService) RollbackCoreUpgrade() error {
	startedAt := time.Now()
	err := s.rollbackCoreFromBinary(s.coreRollbackBinaryPath())
	if err != nil {
		s.logCoreEvent("error", "rollback", "manual", err.Error(), time.Since(startedAt))
		return err
	}
	s.logCoreEvent("ok", "rollback", "manual", "已手动回退到最近一次升级前内核", time.Since(startedAt))
	return nil
}

func (s *SingBoxService) UpgradeFromUploadedCore(uploadPath, originalName string) (*OperationResult, error) {
	startedAt := time.Now()
	uploadPath = strings.TrimSpace(uploadPath)
	if uploadPath == "" {
		err := fmt.Errorf("未收到上传文件")
		s.logCoreEvent("error", "upgrade.upload", "input", err.Error(), time.Since(startedAt))
		return nil, err
	}
	if st, err := os.Stat(uploadPath); err != nil {
		err = fmt.Errorf("读取上传文件失败: %w", err)
		s.logCoreEvent("error", "upgrade.upload", "input", err.Error(), time.Since(startedAt))
		return nil, err
	} else if st.Size() == 0 {
		err := fmt.Errorf("上传文件为空")
		s.logCoreEvent("error", "upgrade.upload", "input", err.Error(), time.Since(startedAt))
		return nil, err
	} else if st.Size() > maxUploadedCoreBytes {
		err := fmt.Errorf("上传文件过大（>%dMB）", maxUploadedCoreBytes/1024/1024)
		s.logCoreEvent("error", "upgrade.upload", "input", err.Error(), time.Since(startedAt))
		return nil, err
	}

	rollbackBin, err := s.prepareCoreRollbackBinary()
	if err != nil {
		err = fmt.Errorf("替换前备份当前内核失败: %w", err)
		s.logCoreEvent("error", "upgrade.upload", "backup", err.Error(), time.Since(startedAt))
		return nil, err
	}

	resolvedBin, cleanup, err := resolveUploadedCoreBinary(uploadPath, originalName)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		s.logCoreEvent("error", "upgrade.upload", "resolve", err.Error(), time.Since(startedAt))
		return nil, err
	}

	if err := validateUploadedCoreBinary(resolvedBin, s.cfg.BinPath); err != nil {
		s.logCoreEvent("error", "upgrade.upload", "validate", err.Error(), time.Since(startedAt))
		return nil, err
	}

	_, _ = runSudoNoPrompt(20*time.Second, "/bin/systemctl", "stop", s.cfg.ServiceName)
	_, _ = runSudoNoPrompt(10*time.Second, "/bin/mkdir", "-p", filepath.Dir(s.cfg.BinPath))
	if err := s.installCoreBinary(resolvedBin); err != nil {
		s.logCoreEvent("error", "upgrade.upload", "install", err.Error(), time.Since(startedAt))
		if rbErr := s.rollbackCoreFromBinary(rollbackBin); rbErr == nil {
			s.logCoreEvent("ok", "rollback", "auto", "上传内核安装失败，已自动回退到升级前版本", time.Since(startedAt))
			return nil, fmt.Errorf("安装上传内核失败，已自动回退到升级前版本: %w", err)
		}
		return nil, fmt.Errorf("安装上传内核失败，且自动回退失败: install_err=%v", err)
	}
	if res, err := runSudoNoPrompt(20*time.Second, "/bin/systemctl", "start", s.cfg.ServiceName); err != nil {
		s.logCoreEvent("error", "upgrade.upload", "start", commandOutputOrError(res, err), time.Since(startedAt))
		if rbErr := s.rollbackCoreFromBinary(rollbackBin); rbErr == nil {
			s.logCoreEvent("ok", "rollback", "auto", "上传内核启动失败，已自动回退到升级前版本", time.Since(startedAt))
			return nil, fmt.Errorf("上传内核启动失败，已自动回退到升级前版本: %w", err)
		}
		return nil, fmt.Errorf("上传内核启动失败，且自动回退失败: start_err=%v", err)
	}
	if err := s.verifyCoreVersion(); err != nil {
		s.logCoreEvent("error", "upgrade.upload", "verify", err.Error(), time.Since(startedAt))
		if rbErr := s.rollbackCoreFromBinary(rollbackBin); rbErr == nil {
			s.logCoreEvent("ok", "rollback", "auto", "上传内核校验失败，已自动回退到升级前版本", time.Since(startedAt))
			return nil, fmt.Errorf("上传内核版本校验失败，已自动回退到升级前版本: %w", err)
		}
		return nil, fmt.Errorf("上传内核版本校验失败，且自动回退失败: verify_err=%v", err)
	}

	msg := "上传内核替换成功并已重启 sing-box"
	s.logCoreEvent("ok", "upgrade.upload", "complete", msg, time.Since(startedAt))
	return &OperationResult{Action: "upgrade.upload", Message: msg}, nil
}

func resolveUploadedCoreBinary(uploadPath, originalName string) (string, func(), error) {
	lname := strings.ToLower(strings.TrimSpace(originalName))
	if lname == "" {
		lname = strings.ToLower(filepath.Base(uploadPath))
	}
	switch {
	case strings.HasSuffix(lname, ".tar.gz"), strings.HasSuffix(lname, ".tgz"):
		return extractUploadedCoreFromTarGz(uploadPath)
	case strings.HasSuffix(lname, ".zip"):
		return extractUploadedCoreFromZip(uploadPath)
	case strings.HasSuffix(lname, ".tar"):
		return extractUploadedCoreFromTar(uploadPath)
	default:
		return uploadPath, nil, nil
	}
}

func extractUploadedCoreFromTarGz(path string) (string, func(), error) {
	f, err := os.Open(path)
	if err != nil {
		return "", nil, fmt.Errorf("打开 tar.gz 失败: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", nil, fmt.Errorf("解析 gzip 失败: %w", err)
	}
	defer gz.Close()
	return extractCoreFromTarReader(tar.NewReader(gz))
}

func extractUploadedCoreFromTar(path string) (string, func(), error) {
	f, err := os.Open(path)
	if err != nil {
		return "", nil, fmt.Errorf("打开 tar 失败: %w", err)
	}
	defer f.Close()
	return extractCoreFromTarReader(tar.NewReader(f))
}

func extractCoreFromTarReader(tr *tar.Reader) (string, func(), error) {
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", nil, fmt.Errorf("读取归档失败: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if !looksLikeSingBoxBinaryName(hdr.Name) {
			continue
		}
		tmpBin, err := os.CreateTemp("", "sing-box-bin-*")
		if err != nil {
			return "", nil, err
		}
		n, err := io.Copy(tmpBin, io.LimitReader(tr, maxUploadedCoreBytes+1))
		if err != nil {
			tmpBin.Close()
			os.Remove(tmpBin.Name())
			return "", nil, fmt.Errorf("写入临时内核失败: %w", err)
		}
		if n > maxUploadedCoreBytes {
			tmpBin.Close()
			os.Remove(tmpBin.Name())
			return "", nil, fmt.Errorf("上传内核文件过大（>%dMB）", maxUploadedCoreBytes/1024/1024)
		}
		if err := tmpBin.Close(); err != nil {
			os.Remove(tmpBin.Name())
			return "", nil, err
		}
		if err := os.Chmod(tmpBin.Name(), 0755); err != nil {
			os.Remove(tmpBin.Name())
			return "", nil, err
		}
		return tmpBin.Name(), func() { _ = os.Remove(tmpBin.Name()) }, nil
	}
	return "", nil, fmt.Errorf("压缩包中未找到 sing-box 可执行文件")
}

func extractUploadedCoreFromZip(path string) (string, func(), error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", nil, fmt.Errorf("打开 zip 失败: %w", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if !looksLikeSingBoxBinaryName(f.Name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", nil, fmt.Errorf("读取 zip 条目失败: %w", err)
		}
		tmpBin, err := os.CreateTemp("", "sing-box-bin-*")
		if err != nil {
			rc.Close()
			return "", nil, err
		}
		n, err := io.Copy(tmpBin, io.LimitReader(rc, maxUploadedCoreBytes+1))
		if err != nil {
			rc.Close()
			tmpBin.Close()
			os.Remove(tmpBin.Name())
			return "", nil, fmt.Errorf("写入临时内核失败: %w", err)
		}
		if n > maxUploadedCoreBytes {
			rc.Close()
			tmpBin.Close()
			os.Remove(tmpBin.Name())
			return "", nil, fmt.Errorf("上传内核文件过大（>%dMB）", maxUploadedCoreBytes/1024/1024)
		}
		rc.Close()
		if err := tmpBin.Close(); err != nil {
			os.Remove(tmpBin.Name())
			return "", nil, err
		}
		if err := os.Chmod(tmpBin.Name(), 0755); err != nil {
			os.Remove(tmpBin.Name())
			return "", nil, err
		}
		return tmpBin.Name(), func() { _ = os.Remove(tmpBin.Name()) }, nil
	}
	return "", nil, fmt.Errorf("zip 包中未找到 sing-box 可执行文件")
}

func looksLikeSingBoxBinaryName(name string) bool {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(name)))
	if base == "sing-box" || base == "sing-box.exe" {
		return true
	}
	if strings.HasPrefix(base, "sing-box-") {
		return true
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(name)), "/sing-box") {
		return true
	}
	return false
}

func validateUploadedCoreBinary(path, installPath string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("上传内核不存在: %w", err)
	}
	if st.Size() == 0 {
		return fmt.Errorf("上传内核文件为空")
	}
	if st.Size() > maxUploadedCoreBytes {
		return fmt.Errorf("上传内核文件过大（>%dMB）", maxUploadedCoreBytes/1024/1024)
	}
	if st.Mode()&0111 == 0 {
		if err := os.Chmod(path, st.Mode()|0755); err != nil {
			return fmt.Errorf("上传内核不可执行，且赋予执行权限失败: %w", err)
		}
	}

	checkCmds := [][]string{{path, "version"}}
	var lastErr error
	for _, cmd := range checkCmds {
		if _, err := utils.Run(10*time.Second, cmd[0], cmd[1:]...); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}

	msg := "上传文件无法执行 `sing-box version` 校验"
	arch := runtime.GOARCH
	suspect := strings.ToLower(filepath.Base(path))
	if strings.Contains(suspect, "amd64") && arch != "amd64" {
		msg = fmt.Sprintf("上传内核疑似 amd64，但当前机器是 %s", arch)
	}
	if strings.Contains(suspect, "arm64") && arch != "arm64" {
		msg = fmt.Sprintf("上传内核疑似 arm64，但当前机器是 %s", arch)
	}
	if strings.TrimSpace(installPath) != "" {
		msg += fmt.Sprintf("（目标路径: %s）", installPath)
	}
	if lastErr != nil {
		msg += fmt.Sprintf(": %v", lastErr)
	}
	return fmt.Errorf(msg)
}

func (s *SingBoxService) logCoreEvent(status, action, stage, message string, duration time.Duration) {
	s.AppendSubscriptionUpdateEventDetailed(status, action, stage, "", message, duration)
}

func (s *SingBoxService) coreRollbackBinaryPath() string {
	return filepath.Join(os.TempDir(), "sing-box-bin-last-good")
}

func (s *SingBoxService) prepareCoreRollbackBinary() (string, error) {
	backupBin, err := copyFileToTempWithPattern(s.cfg.BinPath, os.TempDir(), "sing-box-bin-*")
	if err != nil {
		return "", err
	}
	if err := copyFile(backupBin, s.coreRollbackBinaryPath(), 0755); err != nil {
		return "", err
	}
	return backupBin, nil
}

func runSudoNoPrompt(timeout time.Duration, name string, args ...string) (*utils.CommandResult, error) {
	// If already running as root, execute the target command directly
	if os.Geteuid() == 0 {
		return utils.Run(timeout, name, args...)
	}
	// Otherwise, invoke via sudo with non-interactive flag
	allArgs := append([]string{"-n", name}, args...)
	return utils.Run(timeout, "sudo", allArgs...)
}

func commandOutputOrError(res *utils.CommandResult, err error) string {
	if res != nil {
		if msg := strings.TrimSpace(res.Stderr); msg != "" {
			return msg
		}
		if msg := strings.TrimSpace(res.Stdout); msg != "" {
			return msg
		}
	}
	if err != nil {
		return err.Error()
	}
	return "unknown error"
}

func (s *SingBoxService) installCoreBinary(binPath string) error {
	res, err := runSudoNoPrompt(30*time.Second, "/usr/bin/install", "-m", "755", binPath, s.cfg.BinPath)
	if err == nil {
		return nil
	}
	return fmt.Errorf("安装内核失败（目标 %s）: %s", s.cfg.BinPath, commandOutputOrError(res, err))
}

func (s *SingBoxService) rollbackCoreFromBinary(binPath string) error {
	if strings.TrimSpace(binPath) == "" {
		return fmt.Errorf("回退失败：缺少回退二进制路径")
	}
	if st, err := os.Stat(binPath); err != nil || st.Size() == 0 {
		if err != nil {
			return fmt.Errorf("回退失败：未找到可用备份 %s: %w", binPath, err)
		}
		return fmt.Errorf("回退失败：备份文件为空 %s", binPath)
	}
	_, _ = runSudoNoPrompt(20*time.Second, "/bin/systemctl", "stop", s.cfg.ServiceName)
	if err := s.installCoreBinary(binPath); err != nil {
		return err
	}
	if res, err := runSudoNoPrompt(20*time.Second, "/bin/systemctl", "start", s.cfg.ServiceName); err != nil {
		return fmt.Errorf("回退后启动服务失败: %s", commandOutputOrError(res, err))
	}
	if err := s.verifyCoreVersion(); err != nil {
		return err
	}
	return nil
}

func (s *SingBoxService) verifyCoreVersion() error {
	if _, err := utils.Run(10*time.Second, s.cfg.BinPath, "version"); err == nil {
		return nil
	}
	if _, err := runSudoNoPrompt(10*time.Second, s.cfg.BinPath, "version"); err == nil {
		return nil
	}
	return fmt.Errorf("version check failed")
}

func copyFileToTempWithPattern(srcPath, dir, pattern string) (string, error) {
	tmp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	if err := copyFile(srcPath, tmpPath, 0755); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

func copyFile(srcPath, dstPath string, mode os.FileMode) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	if mode == 0 {
		mode = 0644
	}
	return os.Chmod(dstPath, mode)
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

func (s *SingBoxService) IPForwardStatus() (*IPForwardStatus, error) {
	status := &IPForwardStatus{}
	if b, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward"); err == nil {
		value := strings.TrimSpace(string(b))
		status.Value = value
		status.Source = "/proc/sys/net/ipv4/ip_forward"
		status.Enabled = value == "1"
		if status.Enabled {
			status.Message = "已开启 IP 转发"
		} else {
			status.Message = "未开启 IP 转发"
		}
		return status, nil
	}

	cmds := [][]string{{"sysctl", "-n", "net.ipv4.ip_forward"}, {"sysctl", "net.ipv4.ip_forward"}}
	var lastErr error
	for _, cmd := range cmds {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", strings.Join(cmd, " "), err)
			continue
		}
		text := strings.TrimSpace(string(out))
		status.Source = strings.Join(cmd, " ")
		status.Value = text
		status.Enabled = strings.HasSuffix(text, "= 1") || text == "1"
		if status.Enabled {
			status.Message = "已开启 IP 转发"
		} else {
			status.Message = "未开启 IP 转发"
		}
		return status, nil
	}

	return nil, fmt.Errorf("检测 IP 转发失败: %w", lastErr)
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
