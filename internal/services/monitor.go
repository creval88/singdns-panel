package services

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	cfgpkg "singdns-panel/internal/config"
)

const monitorCronMarker = "# singdns-panel monitor-cron"

type MonitorService struct {
	cfg     cfgpkg.MonitorConfig
	singbox *SingBoxService
}

type MonitorStatus struct {
	Enabled              bool   `json:"enabled"`
	Mode                 string `json:"mode"`
	ActualDefaultTarget  string `json:"actualDefaultTarget,omitempty"`
	ActualDefaultTargetS string `json:"actual_default_target,omitempty"`
	PrimaryGroup         string `json:"primaryGroup,omitempty"`
	PrimaryGroupS        string `json:"primary_group,omitempty"`
	PrimaryActual        string `json:"primaryActual,omitempty"`
	PrimaryActualS       string `json:"primary_actual,omitempty"`
	FallbackGroup        string `json:"fallbackGroup,omitempty"`
	FallbackGroupS       string `json:"fallback_group,omitempty"`
	PrimaryHealthy       bool   `json:"primaryHealthy,omitempty"`
	PrimaryHealthyS      bool   `json:"primary_healthy,omitempty"`
	Message              string `json:"message,omitempty"`
}

type MonitorRunResult struct {
	SwitchedTo string `json:"switched_to,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type MonitorHistorySummary struct {
	Items []MonitorRunResult `json:"items"`
}

type monitorState struct {
	LastRunAt        string `json:"last_run_at,omitempty"`
	LastStatus       string `json:"last_status,omitempty"`
	LastMessage      string `json:"last_message,omitempty"`
	LastProbeTarget  string `json:"last_probe_target,omitempty"`
	LastProbeLatency string `json:"last_probe_latency,omitempty"`
	IntervalMinutes  int    `json:"interval_minutes,omitempty"`
}

func NewMonitorService(cfg cfgpkg.MonitorConfig, singbox *SingBoxService) *MonitorService {
	return &MonitorService{cfg: cfg, singbox: singbox}
}

func (m *MonitorService) Status() (*MonitorStatus, error) {
	state, _ := m.loadState()
	st := &MonitorStatus{
		Enabled:              m.cfg.Enabled,
		ActualDefaultTarget:  safeMonitorGroup(m.cfg.DefaultProxyGroup),
		ActualDefaultTargetS: safeMonitorGroup(m.cfg.DefaultProxyGroup),
		PrimaryGroup:         safeMonitorGroup(m.cfg.PrimaryGroup),
		PrimaryGroupS:        safeMonitorGroup(m.cfg.PrimaryGroup),
		PrimaryActual:        safeMonitorGroup(m.cfg.PrimaryGroup),
		PrimaryActualS:       safeMonitorGroup(m.cfg.PrimaryGroup),
		FallbackGroup:        safeMonitorGroup(m.cfg.FallbackGroup),
		FallbackGroupS:       safeMonitorGroup(m.cfg.FallbackGroup),
	}

	baseMessage := "当前版本提供监控策略管理、手动检查与定时触发记录；不会自动切换默认代理。"
	if !m.cfg.Enabled {
		st.Mode = "manual"
		st.Message = "自动检测切换已关闭。" + baseMessage
		return st, nil
	}

	st.Mode = "policy_only"
	st.PrimaryHealthy = state.LastStatus == "ok"
	st.PrimaryHealthyS = st.PrimaryHealthy
	st.Message = fmt.Sprintf("主组 %s <= %dms，异常参考备组 %s <= %dms。%s", safeMonitorGroup(m.cfg.PrimaryGroup), m.cfg.PrimaryMaxStableDelayMS, safeMonitorGroup(m.cfg.FallbackGroup), m.cfg.FallbackMaxStableDelayMS, baseMessage)
	if state.LastRunAt != "" {
		st.Message += fmt.Sprintf(" 上次检查：%s。", state.LastRunAt)
		if strings.TrimSpace(state.LastMessage) != "" {
			st.Message += " " + strings.TrimSpace(state.LastMessage)
		}
	}
	return st, nil
}

func (m *MonitorService) RunOnce() (*OperationResult, error) {
	state, _ := m.loadState()
	state.LastRunAt = time.Now().Format("2006-01-02 15:04:05")
	state.LastProbeTarget = strings.TrimSpace(m.cfg.APIBase)

	if !m.cfg.Enabled {
		state.LastStatus = "disabled"
		state.LastMessage = "自动检测切换已关闭；本次仅记录检查请求，未执行任何切换。"
		_ = m.saveState(state)
		return &OperationResult{Action: "monitor.run", Message: state.LastMessage}, nil
	}

	dur, err := probeMonitorAPIBase(m.cfg.APIBase, time.Duration(m.cfg.TimeoutMS)*time.Millisecond)
	if err != nil {
		state.LastStatus = "error"
		state.LastProbeLatency = ""
		state.LastMessage = fmt.Sprintf("节点检查失败：Clash API 不可达（%v）。当前版本不会自动切换默认代理。", err)
		_ = m.saveState(state)
		return &OperationResult{Action: "monitor.run", Message: state.LastMessage}, fmt.Errorf(state.LastMessage)
	}

	state.LastStatus = "ok"
	state.LastProbeLatency = dur.String()
	state.LastMessage = fmt.Sprintf("已完成一轮节点检查：Clash API 可连接（耗时 %s）。当前版本仅记录状态，不自动切换默认代理。", dur.Round(time.Millisecond))
	_ = m.saveState(state)
	return &OperationResult{Action: "monitor.run", Message: state.LastMessage}, nil
}

func (m *MonitorService) HistorySummary() (*MonitorHistorySummary, error) {
	state, err := m.loadState()
	if err != nil {
		return &MonitorHistorySummary{Items: []MonitorRunResult{}}, nil
	}
	if strings.TrimSpace(state.LastMessage) == "" {
		return &MonitorHistorySummary{Items: []MonitorRunResult{}}, nil
	}
	return &MonitorHistorySummary{Items: []MonitorRunResult{{
		SwitchedTo: safeMonitorGroup(m.cfg.DefaultProxyGroup),
		Reason:     strings.TrimSpace(state.LastMessage),
	}}}, nil
}

func (m *MonitorService) CronShow() (*CronInfo, error) {
	info := &CronInfo{}
	lines, err := m.singbox.readRootCrontab()
	if err != nil {
		return nil, err
	}
	if line := m.findManagedCronLine(lines); line != "" {
		info.Enabled = true
		info.Raw = line
		info.Summary = monitorCronSummary(line)
	}
	return info, nil
}

func (m *MonitorService) CronSet(intervalMinutes int) (*OperationResult, error) {
	if intervalMinutes <= 0 {
		intervalMinutes = 1
	}
	if intervalMinutes > 30 {
		intervalMinutes = 30
	}
	cmdLine, err := m.cronUpdateCommand()
	if err != nil {
		return nil, err
	}
	lines, err := m.singbox.readRootCrontab()
	if err != nil {
		return nil, err
	}
	lines = m.filterManagedCronLines(lines)
	prefix := "* * * * *"
	if intervalMinutes > 1 {
		prefix = fmt.Sprintf("*/%d * * * *", intervalMinutes)
	}
	lines = append(lines, fmt.Sprintf("%s %s", prefix, cmdLine))
	if err := m.singbox.writeRootCrontab(lines); err != nil {
		return nil, err
	}
	state, _ := m.loadState()
	state.IntervalMinutes = intervalMinutes
	_ = m.saveState(state)
	return &OperationResult{Action: "monitor.cron.set", Message: monitorCronSetMessage(intervalMinutes)}, nil
}

func (m *MonitorService) CronDelete() (*OperationResult, error) {
	lines, err := m.singbox.readRootCrontab()
	if err != nil {
		return nil, err
	}
	filtered := m.filterManagedCronLines(lines)
	if err := m.singbox.writeRootCrontab(filtered); err != nil {
		return nil, err
	}
	state, _ := m.loadState()
	state.IntervalMinutes = 0
	_ = m.saveState(state)
	return &OperationResult{Action: "monitor.cron.delete", Message: "节点检查定时任务已删除"}, nil
}

func (m *MonitorService) cronUpdateCommand() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate current executable: %w", err)
	}
	cfgPath := strings.TrimSpace(m.singbox.panelConfigPath)
	if cfgPath == "" {
		cfgPath = "configs/panel.json"
	}
	return fmt.Sprintf("SINGDNS_CONFIG=%s %s monitor-run %s", shellQuote(cfgPath), shellQuote(exe), monitorCronMarker), nil
}

func (m *MonitorService) findManagedCronLine(lines []string) string {
	for _, line := range lines {
		if strings.Contains(line, monitorCronMarker) {
			return line
		}
	}
	return ""
}

func (m *MonitorService) filterManagedCronLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.Contains(line, monitorCronMarker) {
			continue
		}
		out = append(out, line)
	}
	return out
}

func (m *MonitorService) loadState() (*monitorState, error) {
	path := strings.TrimSpace(m.cfg.StateFile)
	if path == "" {
		path = "data/monitor-state.json"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &monitorState{}, nil
		}
		return nil, err
	}
	var st monitorState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func (m *MonitorService) saveState(st *monitorState) error {
	path := strings.TrimSpace(m.cfg.StateFile)
	if path == "" {
		path = "data/monitor-state.json"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

func monitorCronSummary(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "已开启"
	}
	if fields[0] == "*" {
		return "已开启：每 1 分钟执行一次手动检查"
	}
	if strings.HasPrefix(fields[0], "*/") {
		return fmt.Sprintf("已开启：每 %s 分钟执行一次手动检查", strings.TrimPrefix(fields[0], "*/"))
	}
	return "已开启：按 cron 表达式执行手动检查"
}

func monitorCronSetMessage(intervalMinutes int) string {
	if intervalMinutes <= 1 {
		return "节点检查定时任务已保存：每 1 分钟执行一次"
	}
	return fmt.Sprintf("节点检查定时任务已保存：每 %d 分钟执行一次", intervalMinutes)
}

func probeMonitorAPIBase(raw string, timeout time.Duration) (time.Duration, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("api_base 为空")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return 0, fmt.Errorf("api_base 非法: %w", err)
	}
	host := u.Hostname()
	port := u.Port()
	if host == "" {
		return 0, fmt.Errorf("api_base 缺少主机名")
	}
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		default:
			port = "80"
		}
	}
	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), timeout)
	if err != nil {
		return 0, err
	}
	_ = conn.Close()
	return time.Since(start), nil
}

func safeMonitorGroup(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return strings.TrimSpace(v)
}
