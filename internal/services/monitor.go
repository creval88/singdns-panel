package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	cfgpkg "singdns-panel/internal/config"
)

const (
	monitorCronMarker       = "# singdns-panel monitor-cron"
	monitorHistoryCap       = 12
	monitorSwitchHistoryCap = 30
)

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
	CurrentGroup         string `json:"current_group,omitempty"`
	LastRunAt            string `json:"last_run_at,omitempty"`
	LastSwitchAt         string `json:"last_switch_at,omitempty"`
	LastPrimaryLatencyMS int    `json:"last_primary_latency_ms,omitempty"`
	LastFallbackLatency  int    `json:"last_fallback_latency_ms,omitempty"`
	LastPrimaryScore     int    `json:"last_primary_quality_score,omitempty"`
	LastFallbackScore    int    `json:"last_fallback_quality_score,omitempty"`
	LastDownloadKBps     int    `json:"last_download_kbps,omitempty"`
	LastDownloadRequired int    `json:"last_download_required_kbps,omitempty"`
	LastDownloadPhase    string `json:"last_download_phase,omitempty"`
	LastDownloadWindows  int    `json:"last_download_windows,omitempty"`
	LastDownloadLowWin   int    `json:"last_download_low_windows,omitempty"`
	LastDownloadGroup    string `json:"last_download_group,omitempty"`
	LastSchedulerAt      string `json:"last_scheduler_at,omitempty"`
	LastSchedulerMessage string `json:"last_scheduler_message,omitempty"`
	NextRunAfter         string `json:"next_run_after,omitempty"`
	ActiveIntervalMin    int    `json:"active_interval_min,omitempty"`
	ActivePhase          string `json:"active_phase,omitempty"`
	Message              string `json:"message,omitempty"`
}

type MonitorRunResult struct {
	Time       string `json:"time,omitempty"`
	Status     string `json:"status,omitempty"`
	Action     string `json:"action,omitempty"`
	Group      string `json:"group,omitempty"`
	SwitchedTo string `json:"switched_to,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type MonitorHistorySummary struct {
	Items []MonitorRunResult `json:"items"`
}

type MonitorSwitchHistorySummary struct {
	Items []MonitorRunResult `json:"items"`
}

type monitorState struct {
	LastRunAt             string             `json:"last_run_at,omitempty"`
	LastStatus            string             `json:"last_status,omitempty"`
	LastMessage           string             `json:"last_message,omitempty"`
	LastProbeTarget       string             `json:"last_probe_target,omitempty"`
	LastProbeLatency      string             `json:"last_probe_latency,omitempty"`
	IntervalMinutes       int                `json:"interval_minutes,omitempty"`
	CurrentGroup          string             `json:"current_group,omitempty"`
	CurrentTarget         string             `json:"current_target,omitempty"`
	PrimaryActual         string             `json:"primary_actual,omitempty"`
	LastPrimaryLatencyMS  int                `json:"last_primary_latency_ms,omitempty"`
	LastFallbackLatencyMS int                `json:"last_fallback_latency_ms,omitempty"`
	LastPrimaryScore      int                `json:"last_primary_quality_score,omitempty"`
	LastFallbackScore     int                `json:"last_fallback_quality_score,omitempty"`
	LastDownloadKBps      int                `json:"last_download_kbps,omitempty"`
	LastDownloadRequired  int                `json:"last_download_required_kbps,omitempty"`
	LastDownloadPhase     string             `json:"last_download_phase,omitempty"`
	LastDownloadWindows   int                `json:"last_download_windows,omitempty"`
	LastDownloadLowWin    int                `json:"last_download_low_windows,omitempty"`
	LastDownloadGroup     string             `json:"last_download_group,omitempty"`
	LastSwitchAt          string             `json:"last_switch_at,omitempty"`
	LastSwitchReason      string             `json:"last_switch_reason,omitempty"`
	LastSchedulerAt       string             `json:"last_scheduler_at,omitempty"`
	LastSchedulerMessage  string             `json:"last_scheduler_message,omitempty"`
	LastCheckPhase        string             `json:"last_check_phase,omitempty"`
	LastCheckIntervalMin  int                `json:"last_check_interval_min,omitempty"`
	SwitchHistory         []MonitorRunResult `json:"switch_history,omitempty"`
	History               []MonitorRunResult `json:"history,omitempty"`
}

type clashProxyInfo struct {
	Name string   `json:"name"`
	Type string   `json:"type"`
	Now  string   `json:"now"`
	All  []string `json:"all"`
}

type clashProxiesResponse struct {
	Proxies map[string]clashProxyInfo `json:"proxies"`
}

type clashDelayResponse struct {
	Delay int `json:"delay"`
}

type monitorGroupEval struct {
	GroupTag         string
	GroupExists      bool
	GroupType        string
	Healthy          bool
	DelayMS          int
	CurrentTarget    string
	PreferredTarget  string
	ProbeSuccess     int
	ProbeTotal       int
	QualityScore     int
	DownloadKBps     int
	DownloadRequired int
	DownloadPhase    string
	DownloadWindows  int
	DownloadLowWin   int
	QualityReason    string
	DownloadError    string
	Error            string
}

type monitorDownloadResult struct {
	KBps          int
	RequiredKBps  int
	Phase         string
	DurationSec   int
	WindowSec     int
	Windows       int
	LowWindows    int
	MaxLowWindows int
}

type monitorRunPolicy struct {
	Phase                   string
	IntervalMinutes         int
	VideoDownloadEnabled    bool
	DownloadPrecheckEnabled bool
}

type monitorDecision struct {
	Status        string
	Action        string
	CurrentGroup  string
	CurrentTarget string
	Message       string
	DownloadEval  monitorGroupEval
	DownloadGroup string
}

type monitorRuntimeGroups struct {
	Default  string
	Primary  string
	Fallback string
}

type clashMonitorClient struct {
	base   string
	secret string
	client *http.Client
}

func NewMonitorService(cfg cfgpkg.MonitorConfig, singbox *SingBoxService) *MonitorService {
	return &MonitorService{cfg: cfg, singbox: singbox}
}

func (m *MonitorService) Status() (*MonitorStatus, error) {
	state, _ := m.loadState()
	policy := m.activeRunPolicy(time.Now())
	nextRunAfter := ""
	if lastRun, ok := parseMonitorTimestamp(state.LastRunAt); ok && policy.IntervalMinutes > 0 {
		nextRunAfter = lastRun.Add(time.Duration(policy.IntervalMinutes) * time.Minute).Format("2006-01-02 15:04:05")
	}
	st := &MonitorStatus{
		Enabled:              m.cfg.Enabled,
		ActualDefaultTarget:  safeMonitorGroup(state.CurrentTarget),
		ActualDefaultTargetS: safeMonitorGroup(state.CurrentTarget),
		PrimaryGroup:         safeMonitorGroup(m.cfg.PrimaryGroup),
		PrimaryGroupS:        safeMonitorGroup(m.cfg.PrimaryGroup),
		PrimaryActual:        safeMonitorGroup(state.PrimaryActual),
		PrimaryActualS:       safeMonitorGroup(state.PrimaryActual),
		FallbackGroup:        safeMonitorGroup(m.cfg.FallbackGroup),
		FallbackGroupS:       safeMonitorGroup(m.cfg.FallbackGroup),
		CurrentGroup:         safeMonitorGroup(state.CurrentGroup),
		LastRunAt:            strings.TrimSpace(state.LastRunAt),
		LastSwitchAt:         strings.TrimSpace(state.LastSwitchAt),
		LastPrimaryLatencyMS: state.LastPrimaryLatencyMS,
		LastFallbackLatency:  state.LastFallbackLatencyMS,
		LastPrimaryScore:     state.LastPrimaryScore,
		LastFallbackScore:    state.LastFallbackScore,
		LastDownloadKBps:     state.LastDownloadKBps,
		LastDownloadRequired: state.LastDownloadRequired,
		LastDownloadPhase:    strings.TrimSpace(state.LastDownloadPhase),
		LastDownloadWindows:  state.LastDownloadWindows,
		LastDownloadLowWin:   state.LastDownloadLowWin,
		LastDownloadGroup:    strings.TrimSpace(state.LastDownloadGroup),
		LastSchedulerAt:      strings.TrimSpace(state.LastSchedulerAt),
		LastSchedulerMessage: strings.TrimSpace(state.LastSchedulerMessage),
		NextRunAfter:         nextRunAfter,
		ActiveIntervalMin:    policy.IntervalMinutes,
		ActivePhase:          policy.Phase,
		PrimaryHealthy:       strings.TrimSpace(state.LastStatus) == "ok" && strings.TrimSpace(state.CurrentGroup) != "fallback",
	}
	st.PrimaryHealthyS = st.PrimaryHealthy

	if !m.cfg.Enabled {
		st.Mode = "disabled"
		st.Message = "自动监控已关闭，不会执行测速或切换。"
		return st, nil
	}

	switch strings.TrimSpace(state.CurrentGroup) {
	case "primary":
		st.Mode = "primary_active"
	case "fallback":
		st.Mode = "fallback_active"
	default:
		st.Mode = "auto_switch"
	}

	if schedMsg := strings.TrimSpace(state.LastSchedulerMessage); schedMsg != "" && strings.TrimSpace(state.LastSchedulerAt) != "" && strings.TrimSpace(state.LastSchedulerAt) != strings.TrimSpace(state.LastRunAt) {
		st.Message = schedMsg
	} else if msg := strings.TrimSpace(state.LastMessage); msg != "" {
		st.Message = msg
	} else {
		st.Message = fmt.Sprintf("将按 Clash API 对主组 %s 和备组 %s 进行检测，并根据阈值决定是否切换。", safeMonitorGroup(m.cfg.PrimaryGroup), safeMonitorGroup(m.cfg.FallbackGroup))
	}
	return st, nil
}

func (m *MonitorService) RunScheduled() (*OperationResult, error) {
	unlock, err := m.lockRun()
	if err != nil {
		msg := fmt.Sprintf("监控任务已在执行中，请稍后再试: %v", err)
		return &OperationResult{Action: "monitor.run.skip", Message: msg}, fmt.Errorf(msg)
	}
	defer unlock()

	now := time.Now()
	policy := m.activeRunPolicy(now)
	state, _ := m.loadState()
	if m.cfg.Enabled && !m.monitorScheduledDue(state, now, policy) {
		nextRun := "稍后"
		if lastRun, ok := parseMonitorTimestamp(state.LastRunAt); ok {
			nextRun = lastRun.Add(time.Duration(policy.IntervalMinutes) * time.Minute).Format("2006-01-02 15:04:05")
		}
		state.LastSchedulerAt = now.Format("2006-01-02 15:04:05")
		state.LastSchedulerMessage = fmt.Sprintf("%s策略为每 %d 分钟检查一次，未到下一轮完整检测时间（%s）。", monitorPhaseText(policy.Phase), policy.IntervalMinutes, nextRun)
		_ = m.saveState(state)
		return &OperationResult{Action: "monitor.run.skip", Message: state.LastSchedulerMessage}, nil
	}
	return m.runOnceLocked(policy, state, now)
}

func (m *MonitorService) RunOnce() (*OperationResult, error) {
	unlock, err := m.lockRun()
	if err != nil {
		msg := fmt.Sprintf("监控任务已在执行中，请稍后再试: %v", err)
		return &OperationResult{Action: "monitor.run", Message: msg}, fmt.Errorf(msg)
	}
	defer unlock()

	state, _ := m.loadState()
	now := time.Now()
	return m.runOnceLocked(m.activeRunPolicy(now), state, now)
}

func (m *MonitorService) runOnceLocked(policy monitorRunPolicy, state *monitorState, now time.Time) (*OperationResult, error) {
	state.LastRunAt = now.Format("2006-01-02 15:04:05")
	state.LastProbeTarget = strings.TrimSpace(m.cfg.APIBase)
	state.LastCheckPhase = policy.Phase
	state.LastCheckIntervalMin = policy.IntervalMinutes
	state.LastSchedulerAt = state.LastRunAt
	state.LastSchedulerMessage = fmt.Sprintf("%s策略执行完整检测。", monitorPhaseText(policy.Phase))

	if !m.cfg.Enabled {
		state.LastStatus = "disabled"
		state.LastMessage = "自动监控已关闭；本次仅记录执行请求，未执行测速或切换。"
		m.appendHistory(state, MonitorRunResult{
			Time:   state.LastRunAt,
			Status: "disabled",
			Action: "skip",
			Group:  strings.TrimSpace(state.CurrentGroup),
			Reason: state.LastMessage,
		})
		_ = m.saveState(state)
		return &OperationResult{Action: "monitor.run", Message: state.LastMessage}, nil
	}

	client, err := m.newClashClient()
	if err != nil {
		state.LastStatus = "error"
		state.LastProbeLatency = ""
		state.LastMessage = fmt.Sprintf("节点检查失败：无法连接 Clash API（%v）。", err)
		m.appendHistory(state, MonitorRunResult{
			Time:   state.LastRunAt,
			Status: "error",
			Action: "error",
			Group:  strings.TrimSpace(state.CurrentGroup),
			Reason: state.LastMessage,
		})
		_ = m.saveState(state)
		return &OperationResult{Action: "monitor.run", Message: state.LastMessage}, fmt.Errorf(state.LastMessage)
	}

	proxies, err := client.proxies()
	if err != nil {
		state.LastStatus = "error"
		state.LastProbeLatency = ""
		state.LastMessage = fmt.Sprintf("节点检查失败：读取代理组信息失败（%v）。", err)
		m.appendHistory(state, MonitorRunResult{
			Time:   state.LastRunAt,
			Status: "error",
			Action: "error",
			Group:  strings.TrimSpace(state.CurrentGroup),
			Reason: state.LastMessage,
		})
		_ = m.saveState(state)
		return &OperationResult{Action: "monitor.run", Message: state.LastMessage}, fmt.Errorf(state.LastMessage)
	}

	groups, err := m.resolveRuntimeGroups(proxies)
	if err != nil {
		state.LastStatus = "error"
		state.LastMessage = fmt.Sprintf("节点检查失败：%v。", err)
		m.appendHistory(state, MonitorRunResult{
			Time:   state.LastRunAt,
			Status: "error",
			Action: "error",
			Group:  strings.TrimSpace(state.CurrentGroup),
			Reason: state.LastMessage,
		})
		_ = m.saveState(state)
		return &OperationResult{Action: "monitor.run", Message: state.LastMessage}, fmt.Errorf(state.LastMessage)
	}
	defaultProxy := proxies[groups.Default]

	currentTarget := strings.TrimSpace(defaultProxy.Now)
	currentGroup := classifyMonitorGroup(currentTarget, groups.Primary, groups.Fallback)

	primaryEval, err := m.evaluatePrimary(client, proxies, groups.Primary, currentGroup != "fallback", policy)
	if err != nil && strings.TrimSpace(primaryEval.Error) == "" {
		primaryEval.Error = err.Error()
	}
	fallbackEval, fbErr := m.evaluateFallback(client, groups.Fallback, currentGroup == "fallback", policy)
	if fbErr != nil && strings.TrimSpace(fallbackEval.Error) == "" {
		fallbackEval.Error = fbErr.Error()
	}

	state.CurrentTarget = currentTarget
	state.CurrentGroup = currentGroup
	state.PrimaryActual = primaryEval.CurrentTarget
	state.LastPrimaryLatencyMS = primaryEval.DelayMS
	state.LastFallbackLatencyMS = fallbackEval.DelayMS
	state.LastPrimaryScore = primaryEval.QualityScore
	state.LastFallbackScore = fallbackEval.QualityScore
	state.LastProbeLatency = formatMonitorLatency(primaryEval.DelayMS)

	decision, err := m.decideAndApply(client, groups, currentGroup, currentTarget, primaryEval, fallbackEval, policy)
	downloadEval, downloadGroup := monitorDownloadDisplayEval(currentGroup, primaryEval, fallbackEval, decision)
	state.LastDownloadKBps = downloadEval.DownloadKBps
	state.LastDownloadRequired = downloadEval.DownloadRequired
	state.LastDownloadPhase = strings.TrimSpace(downloadEval.DownloadPhase)
	state.LastDownloadWindows = downloadEval.DownloadWindows
	state.LastDownloadLowWin = downloadEval.DownloadLowWin
	state.LastDownloadGroup = downloadGroup
	state.LastStatus = decision.Status
	state.LastMessage = decision.Message
	if strings.TrimSpace(decision.CurrentGroup) != "" {
		state.CurrentGroup = decision.CurrentGroup
	}
	if strings.TrimSpace(decision.CurrentTarget) != "" {
		state.CurrentTarget = decision.CurrentTarget
	}
	if decision.Action == "switch" || decision.Action == "optimize" {
		state.LastSwitchAt = state.LastRunAt
		state.LastSwitchReason = decision.Message
		m.appendSwitchHistory(state, MonitorRunResult{
			Time:       state.LastRunAt,
			Status:     decision.Status,
			Action:     decision.Action,
			Group:      state.CurrentGroup,
			SwitchedTo: state.CurrentTarget,
			Reason:     decision.Message,
		})
	}
	m.appendHistory(state, MonitorRunResult{
		Time:       state.LastRunAt,
		Status:     decision.Status,
		Action:     decision.Action,
		Group:      state.CurrentGroup,
		SwitchedTo: state.CurrentTarget,
		Reason:     decision.Message,
	})
	_ = m.saveState(state)

	if err != nil {
		return &OperationResult{Action: "monitor.run", Message: decision.Message}, err
	}
	return &OperationResult{Action: "monitor.run", Message: decision.Message}, nil
}

func (m *MonitorService) HistorySummary() (*MonitorHistorySummary, error) {
	state, err := m.loadState()
	if err != nil {
		return &MonitorHistorySummary{Items: []MonitorRunResult{}}, nil
	}
	items := append([]MonitorRunResult{}, state.History...)
	return &MonitorHistorySummary{Items: items}, nil
}

func (m *MonitorService) SwitchHistorySummary() (*MonitorSwitchHistorySummary, error) {
	state, err := m.loadState()
	if err != nil {
		return &MonitorSwitchHistorySummary{Items: []MonitorRunResult{}}, nil
	}
	items := append([]MonitorRunResult{}, state.SwitchHistory...)
	if len(items) == 0 && strings.TrimSpace(state.LastSwitchAt) != "" {
		items = append(items, MonitorRunResult{
			Time:       strings.TrimSpace(state.LastSwitchAt),
			Status:     strings.TrimSpace(state.LastStatus),
			Action:     "switch",
			Group:      strings.TrimSpace(state.CurrentGroup),
			SwitchedTo: strings.TrimSpace(state.CurrentTarget),
			Reason:     strings.TrimSpace(state.LastSwitchReason),
		})
	}
	return &MonitorSwitchHistorySummary{Items: items}, nil
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
		info.Summary = m.monitorCronSummary(line)
	}
	return info, nil
}

func (m *MonitorService) CronSet(intervalMinutes int) (*OperationResult, error) {
	cmdLine, err := m.cronUpdateCommand()
	if err != nil {
		return nil, err
	}
	lines, err := m.singbox.readRootCrontab()
	if err != nil {
		return nil, err
	}
	lines = m.filterManagedCronLines(lines)
	lines = append(lines, fmt.Sprintf("* * * * * %s", cmdLine))
	if err := m.singbox.writeRootCrontab(lines); err != nil {
		return nil, err
	}
	state, _ := m.loadState()
	state.IntervalMinutes = 1
	_ = m.saveState(state)
	return &OperationResult{Action: "monitor.cron.set", Message: m.monitorCronSetMessage()}, nil
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
			return newMonitorState(), nil
		}
		return newMonitorState(), err
	}
	var st monitorState
	if err := json.Unmarshal(b, &st); err != nil {
		return newMonitorState(), err
	}
	if st.History == nil {
		st.History = []MonitorRunResult{}
	}
	if st.SwitchHistory == nil {
		st.SwitchHistory = []MonitorRunResult{}
	}
	return &st, nil
}

func newMonitorState() *monitorState {
	return &monitorState{History: []MonitorRunResult{}, SwitchHistory: []MonitorRunResult{}}
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
	tmp, err := os.CreateTemp(filepath.Dir(path), ".monitor-state-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (m *MonitorService) lockRun() (func(), error) {
	lockPath := strings.TrimSpace(m.cfg.StateFile)
	if lockPath == "" {
		lockPath = "data/monitor-state.json"
	}
	lockPath += ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

func (m *MonitorService) appendHistory(state *monitorState, item MonitorRunResult) {
	state.History = append([]MonitorRunResult{item}, state.History...)
	if len(state.History) > monitorHistoryCap {
		state.History = state.History[:monitorHistoryCap]
	}
}

func (m *MonitorService) appendSwitchHistory(state *monitorState, item MonitorRunResult) {
	state.SwitchHistory = append([]MonitorRunResult{item}, state.SwitchHistory...)
	if len(state.SwitchHistory) > monitorSwitchHistoryCap {
		state.SwitchHistory = state.SwitchHistory[:monitorSwitchHistoryCap]
	}
}

func (m *MonitorService) newClashClient() (*clashMonitorClient, error) {
	base := strings.TrimSpace(m.cfg.APIBase)
	if base == "" {
		return nil, fmt.Errorf("api_base 为空")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("api_base 非法: %w", err)
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "http"
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("api_base 缺少主机名")
	}
	info, err := m.singbox.ClashAPIInfo("")
	if err != nil {
		return nil, err
	}
	return &clashMonitorClient{
		base:   strings.TrimRight(parsed.String(), "/"),
		secret: strings.TrimSpace(info.Secret),
		client: &http.Client{Timeout: time.Duration(maxInt(m.cfg.TimeoutMS, 1000)) * time.Millisecond},
	}, nil
}

func (m *MonitorService) evaluatePrimary(client *clashMonitorClient, proxies map[string]clashProxyInfo, groupTag string, activeRoute bool, policy monitorRunPolicy) (monitorGroupEval, error) {
	groupTag = strings.TrimSpace(groupTag)
	eval := monitorGroupEval{GroupTag: groupTag}
	if groupTag == "" {
		eval.Error = "未配置主组"
		return eval, fmt.Errorf(eval.Error)
	}
	group, ok := proxies[groupTag]
	if !ok {
		eval.Error = fmt.Sprintf("未找到主组 %s", groupTag)
		return eval, fmt.Errorf(eval.Error)
	}
	eval.GroupExists = true
	eval.GroupType = group.Type
	eval.CurrentTarget = strings.TrimSpace(group.Now)

	if m.cfg.DisablePrimaryGroupOptimization || len(group.All) == 0 {
		delay, err := client.delay(groupTag, m.cfg.TestURL, m.cfg.TimeoutMS)
		eval.DelayMS = delay
		eval.PreferredTarget = eval.CurrentTarget
		if err != nil {
			eval.Error = err.Error()
			return eval, err
		}
		m.applyQualityCheck(client, &eval, m.cfg.PrimaryMaxStableDelayMS, activeRoute, policy)
		return eval, nil
	}

	bestDelay := 0
	bestTarget := ""
	var lastErr error
	for _, member := range group.All {
		member = strings.TrimSpace(member)
		if member == "" {
			continue
		}
		delay, err := client.delay(member, m.cfg.TestURL, m.cfg.TimeoutMS)
		if err != nil {
			lastErr = err
			continue
		}
		if bestDelay == 0 || delay < bestDelay {
			bestDelay = delay
			bestTarget = member
		}
	}
	eval.DelayMS = bestDelay
	eval.PreferredTarget = bestTarget
	if bestTarget == "" && lastErr != nil {
		eval.Error = lastErr.Error()
		return eval, lastErr
	}
	m.applyQualityCheck(client, &eval, m.cfg.PrimaryMaxStableDelayMS, activeRoute, policy)
	return eval, nil
}

func (m *MonitorService) evaluateFallback(client *clashMonitorClient, groupTag string, activeRoute bool, policy monitorRunPolicy) (monitorGroupEval, error) {
	groupTag = strings.TrimSpace(groupTag)
	eval := monitorGroupEval{GroupTag: groupTag, PreferredTarget: groupTag}
	if groupTag == "" {
		eval.Error = "未配置备组"
		return eval, fmt.Errorf(eval.Error)
	}
	delay, err := client.delay(groupTag, m.cfg.TestURL, m.cfg.TimeoutMS)
	eval.DelayMS = delay
	if err != nil {
		eval.Error = err.Error()
		return eval, err
	}
	m.applyQualityCheck(client, &eval, m.cfg.FallbackMaxStableDelayMS, activeRoute, policy)
	return eval, nil
}

func (m *MonitorService) applyQualityCheck(client *clashMonitorClient, eval *monitorGroupEval, maxStableDelayMS int, activeRoute bool, policy monitorRunPolicy) {
	if eval == nil {
		return
	}
	delayOK := eval.DelayMS > 0 && eval.DelayMS <= maxStableDelayMS
	if !m.cfg.QualityCheckEnabled {
		eval.Healthy = delayOK
		return
	}

	target := strings.TrimSpace(eval.PreferredTarget)
	if target == "" {
		target = strings.TrimSpace(eval.GroupTag)
	}

	probeURLs := m.qualityProbeURLs()
	probeSuccess := 0
	probeTotal := 0
	for _, probeURL := range probeURLs {
		probeTotal++
		if delay, err := client.delay(target, probeURL, m.cfg.TimeoutMS); err == nil && delay > 0 {
			probeSuccess++
		}
	}
	eval.ProbeSuccess = probeSuccess
	eval.ProbeTotal = probeTotal

	minProbeSuccess := m.cfg.MinProbeSuccess
	if minProbeSuccess <= 0 {
		minProbeSuccess = minMonitorInt(2, probeTotal)
	}
	if probeTotal > 0 && minProbeSuccess > probeTotal {
		minProbeSuccess = probeTotal
	}

	downloadAttempted := false
	downloadOK := true
	downloadRequired := m.activeDownloadThreshold(time.Now())
	downloadProxyURL := m.downloadProxyURLForEval(eval, activeRoute)
	precheckOK := delayOK && probeSuccess >= minProbeSuccess
	shouldDownload := policy.VideoDownloadEnabled
	if shouldDownload && policy.DownloadPrecheckEnabled && !precheckOK {
		shouldDownload = false
	}
	if shouldDownload && strings.TrimSpace(m.cfg.DownloadTestURL) != "" && strings.TrimSpace(downloadProxyURL) != "" {
		downloadAttempted = true
		result, err := m.downloadThroughLocalProxy(downloadRequired, downloadProxyURL)
		eval.DownloadKBps = result.KBps
		eval.DownloadRequired = result.RequiredKBps
		eval.DownloadPhase = result.Phase
		eval.DownloadWindows = result.Windows
		eval.DownloadLowWin = result.LowWindows
		if err != nil {
			eval.DownloadError = err.Error()
			downloadOK = false
		} else if result.KBps < result.RequiredKBps || result.LowWindows > result.MaxLowWindows {
			downloadOK = false
		}
	}

	delayScore := 0
	if eval.DelayMS > 0 {
		if eval.DelayMS <= maxStableDelayMS {
			delayScore = 30
		} else {
			delayScore = maxInt(1, (maxStableDelayMS*30)/eval.DelayMS)
		}
	}
	probeScore := 40
	if probeTotal > 0 {
		probeScore = (probeSuccess * 40) / probeTotal
	}
	downloadScore := 30
	if downloadAttempted {
		downloadScore = (eval.DownloadKBps * 30) / maxInt(downloadRequired.RequiredKBps, 1)
		if downloadScore > 30 {
			downloadScore = 30
		}
	}
	eval.QualityScore = delayScore + probeScore + downloadScore

	threshold := maxInt(m.cfg.QualityScoreThreshold, 1)
	eval.Healthy = delayOK && probeSuccess >= minProbeSuccess && eval.QualityScore >= threshold && downloadOK
	if eval.Healthy {
		eval.QualityReason = fmt.Sprintf("score=%d, probes=%d/%d", eval.QualityScore, probeSuccess, probeTotal)
		if downloadAttempted {
			eval.QualityReason += fmt.Sprintf(", download=%dKB/s", eval.DownloadKBps)
		}
		return
	}

	parts := make([]string, 0, 4)
	if !delayOK {
		parts = append(parts, fmt.Sprintf("延迟 %s 超过阈值 %dms", monitorDelayText(eval.DelayMS, eval.Error), maxStableDelayMS))
	}
	if probeSuccess < minProbeSuccess {
		parts = append(parts, fmt.Sprintf("真实访问 %d/%d 低于要求 %d", probeSuccess, probeTotal, minProbeSuccess))
	}
	if eval.QualityScore < threshold {
		parts = append(parts, fmt.Sprintf("质量分 %d 低于阈值 %d", eval.QualityScore, threshold))
	}
	if !downloadOK {
		if strings.TrimSpace(eval.DownloadError) != "" && eval.DownloadKBps <= 0 {
			parts = append(parts, fmt.Sprintf("下载测速不可用: %s", strings.TrimSpace(eval.DownloadError)))
		} else {
			parts = append(parts, fmt.Sprintf("%s下载速度 %dKB/s 低于要求 %dKB/s", downloadPhaseText(eval.DownloadPhase), eval.DownloadKBps, maxInt(eval.DownloadRequired, 1)))
			if eval.DownloadWindows > 0 && eval.DownloadLowWin > downloadRequired.MaxLowWindows {
				parts = append(parts, fmt.Sprintf("低速窗口 %d/%d 超过容忍 %d", eval.DownloadLowWin, eval.DownloadWindows, downloadRequired.MaxLowWindows))
			}
		}
	} else if downloadAttempted && eval.DownloadKBps <= 0 && strings.TrimSpace(eval.DownloadError) != "" {
		parts = append(parts, fmt.Sprintf("下载测速不可用: %s", strings.TrimSpace(eval.DownloadError)))
	}
	eval.QualityReason = strings.Join(parts, "；")
	if eval.QualityReason != "" {
		eval.Error = eval.QualityReason
	}
}

func (m *MonitorService) qualityProbeURLs() []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(m.cfg.ProbeURLs)+1)
	for _, item := range m.cfg.ProbeURLs {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	if len(out) == 0 {
		if fallback := strings.TrimSpace(m.cfg.TestURL); fallback != "" {
			out = append(out, fallback)
		}
	}
	return out
}

func (m *MonitorService) activeRunPolicy(now time.Time) monitorRunPolicy {
	phase := "day"
	interval := m.cfg.DayCheckIntervalMin
	videoDownload := m.cfg.VideoCheckEnabled && m.cfg.VideoDayCheckEnabled
	if monitorInTimeRange(now, m.cfg.VideoPeakStart, m.cfg.VideoPeakEnd) {
		phase = "peak"
		interval = m.cfg.PeakCheckIntervalMin
		videoDownload = m.cfg.VideoCheckEnabled && m.cfg.VideoPeakCheckEnabled
	}
	if interval <= 0 {
		if phase == "peak" {
			interval = 1
		} else {
			interval = 5
		}
	}
	return monitorRunPolicy{
		Phase:                   phase,
		IntervalMinutes:         interval,
		VideoDownloadEnabled:    videoDownload,
		DownloadPrecheckEnabled: !m.cfg.DownloadPrecheckDisabled,
	}
}

func (m *MonitorService) monitorScheduledDue(state *monitorState, now time.Time, policy monitorRunPolicy) bool {
	if state == nil {
		return true
	}
	lastRun, ok := parseMonitorTimestamp(state.LastRunAt)
	if !ok {
		return true
	}
	interval := policy.IntervalMinutes
	if interval <= 0 {
		interval = 1
	}
	return !now.Before(lastRun.Add(time.Duration(interval) * time.Minute))
}

func (m *MonitorService) activeDownloadThreshold(now time.Time) monitorDownloadResult {
	phase := "day"
	required := maxInt(m.cfg.MinDownloadKBps, 1)
	if m.cfg.VideoCheckEnabled {
		if monitorInTimeRange(now, m.cfg.VideoPeakStart, m.cfg.VideoPeakEnd) {
			phase = "peak"
			required = maxInt(m.cfg.VideoPeakMinDownloadKBps, required)
		} else {
			required = maxInt(m.cfg.VideoDayMinDownloadKBps, required)
		}
	}
	durationSec := m.cfg.VideoDownloadDurationSec
	if durationSec <= 0 {
		durationSec = 10
	}
	windowSec := m.cfg.VideoDownloadWindowSec
	if windowSec <= 0 {
		windowSec = 2
	}
	if windowSec > durationSec {
		windowSec = durationSec
	}
	maxLowWindows := m.cfg.VideoDownloadMaxLowWindows
	if maxLowWindows < 0 {
		maxLowWindows = 0
	}
	return monitorDownloadResult{
		RequiredKBps:  required,
		Phase:         phase,
		DurationSec:   durationSec,
		WindowSec:     windowSec,
		MaxLowWindows: maxLowWindows,
	}
}

func (m *MonitorService) downloadProxyURLForEval(eval *monitorGroupEval, activeRoute bool) string {
	if eval == nil {
		return ""
	}
	groupTag := strings.TrimSpace(eval.GroupTag)
	switch groupTag {
	case strings.TrimSpace(m.cfg.PrimaryGroup):
		return monitorPrimaryProxyURL()
	case strings.TrimSpace(m.cfg.FallbackGroup):
		return monitorFallbackProxyURL()
	}
	if activeRoute {
		return strings.TrimSpace(m.cfg.LocalProxyURL)
	}
	return ""
}

func monitorInTimeRange(now time.Time, startText, endText string) bool {
	start, okStart := parseMonitorClock(startText)
	end, okEnd := parseMonitorClock(endText)
	if !okStart || !okEnd || start == end {
		return false
	}
	current := now.Hour()*60 + now.Minute()
	if start < end {
		return current >= start && current < end
	}
	return current >= start || current < end
}

func parseMonitorTimestamp(s string) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(s), time.Local)
	return t, err == nil
}

func monitorPhaseText(phase string) string {
	if strings.TrimSpace(phase) == "peak" {
		return "晚高峰"
	}
	return "白天"
}

func parseMonitorClock(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "24:00" {
		return 24 * 60, true
	}
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, false
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, false
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}

func (m *MonitorService) downloadThroughLocalProxy(policy monitorDownloadResult, proxyURL string) (monitorDownloadResult, error) {
	result := policy
	timeout := time.Duration(maxInt(policy.DurationSec+5, 6)) * time.Second
	minTimeout := time.Duration(maxInt(m.cfg.TimeoutMS, 1000)) * time.Millisecond
	if timeout < minTimeout {
		timeout = minTimeout
	}
	client, err := m.qualityHTTPClient(timeout, proxyURL)
	if err != nil {
		return result, err
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimSpace(m.cfg.DownloadTestURL), nil)
	if err != nil {
		return result, err
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return result, fmt.Errorf("download probe http %d", resp.StatusCode)
	}
	deadline := start.Add(time.Duration(maxInt(policy.DurationSec, 1)) * time.Second)
	windowSec := maxInt(policy.WindowSec, 1)
	windowStart := start
	windowBytes := int64(0)
	totalBytes := int64(0)
	buf := make([]byte, 32*1024)
	for time.Now().Before(deadline) {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			totalBytes += int64(n)
			windowBytes += int64(n)
		}
		now := time.Now()
		if now.Sub(windowStart) >= time.Duration(windowSec)*time.Second {
			result.Windows++
			kbps := kbpsForBytes(windowBytes, now.Sub(windowStart))
			if kbps < result.RequiredKBps {
				result.LowWindows++
			}
			windowStart = now
			windowBytes = 0
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return result, readErr
		}
	}
	if windowBytes > 0 {
		result.Windows++
		kbps := kbpsForBytes(windowBytes, time.Since(windowStart))
		if kbps < result.RequiredKBps {
			result.LowWindows++
		}
	}
	if totalBytes <= 0 {
		return result, fmt.Errorf("download probe empty response")
	}
	result.KBps = kbpsForBytes(totalBytes, time.Since(start))
	return result, nil
}

func kbpsForBytes(n int64, d time.Duration) int {
	seconds := d.Seconds()
	if seconds <= 0 {
		seconds = 0.001
	}
	return int((float64(n) / 1024.0) / seconds)
}

func (m *MonitorService) qualityHTTPClient(timeout time.Duration, proxyURL string) (*http.Client, error) {
	raw := strings.TrimSpace(proxyURL)
	if raw == "" {
		return nil, fmt.Errorf("下载代理为空")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		transport.Proxy = http.ProxyURL(parsed)
	case "socks5", "socks5h":
		addr := parsed.Host
		if addr == "" {
			return nil, fmt.Errorf("下载代理缺少 SOCKS5 地址")
		}
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialSOCKS5(ctx, addr, network, address)
		}
	default:
		return nil, fmt.Errorf("不支持的下载代理协议: %s", parsed.Scheme)
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}, nil
}

func dialSOCKS5(ctx context.Context, proxyAddr, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("unsupported socks network: %s", network)
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		conn.Close()
		return nil, err
	}
	buf := make([]byte, 260)
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		conn.Close()
		return nil, err
	}
	if buf[0] != 0x05 || buf[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("socks5 auth rejected")
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		conn.Close()
		return nil, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		conn.Close()
		return nil, fmt.Errorf("invalid target port: %s", portText)
	}
	req := []byte{0x05, 0x01, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			req = append(req, 0x01)
			req = append(req, ip4...)
		} else {
			req = append(req, 0x04)
			req = append(req, ip.To16()...)
		}
	} else {
		if len(host) > 255 {
			conn.Close()
			return nil, fmt.Errorf("target host too long")
		}
		req = append(req, 0x03, byte(len(host)))
		req = append(req, []byte(host)...)
	}
	req = append(req, byte(port>>8), byte(port))
	if _, err := conn.Write(req); err != nil {
		conn.Close()
		return nil, err
	}
	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		conn.Close()
		return nil, err
	}
	if buf[0] != 0x05 || buf[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("socks5 connect failed: reply %d", buf[1])
	}
	switch buf[3] {
	case 0x01:
		_, err = io.ReadFull(conn, buf[:6])
	case 0x03:
		if _, err = io.ReadFull(conn, buf[:1]); err == nil {
			_, err = io.ReadFull(conn, buf[:int(buf[0])+2])
		}
	case 0x04:
		_, err = io.ReadFull(conn, buf[:18])
	default:
		err = fmt.Errorf("socks5 invalid address type: %d", buf[3])
	}
	if err != nil {
		conn.Close()
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	return conn, nil
}

func (m *MonitorService) decideAndApply(client *clashMonitorClient, groups monitorRuntimeGroups, currentGroup, currentTarget string, primaryEval, fallbackEval monitorGroupEval, policy monitorRunPolicy) (monitorDecision, error) {
	defaultGroup := strings.TrimSpace(groups.Default)
	recheckInterval := time.Duration(maxInt(m.cfg.RecheckIntervalSec, 1)) * time.Second

	if currentGroup == "fallback" && m.cfg.AutoFailback {
		successes := 0
		bestEval := primaryEval
		for attempt := 1; attempt <= maxInt(m.cfg.SuccessThreshold, 1); attempt++ {
			eval, _ := m.evaluatePrimary(client, mustReadProxiesOnce(client), groups.Primary, false, policy)
			if eval.DelayMS > 0 && (bestEval.DelayMS == 0 || eval.DelayMS < bestEval.DelayMS) {
				bestEval = eval
			}
			if eval.Healthy {
				successes++
				if successes >= maxInt(m.cfg.SuccessThreshold, 1) {
					if err := m.applyPrimarySelection(client, defaultGroup, groups.Primary, bestEval); err != nil {
						msg := fmt.Sprintf("主组已恢复，但切回失败：%v", err)
						return monitorDecision{Status: "error", Action: "error", CurrentGroup: "fallback", CurrentTarget: currentTarget, Message: msg}, fmt.Errorf(msg)
					}
					target := strings.TrimSpace(groups.Primary)
					msg := fmt.Sprintf("主组 %s 连续 %d 次检测恢复，已将默认代理切回主组。", safeMonitorGroup(target), successes)
					if bestEval.PreferredTarget != "" && bestEval.PreferredTarget != bestEval.CurrentTarget {
						msg += fmt.Sprintf(" 同时将主组优选节点切到 %s（%dms）。", bestEval.PreferredTarget, bestEval.DelayMS)
					}
					return monitorDecision{Status: "ok", Action: "switch", CurrentGroup: "primary", CurrentTarget: target, Message: msg, DownloadEval: bestEval, DownloadGroup: "primary"}, nil
				}
			} else {
				msg := fmt.Sprintf("当前已在备组 %s，主组尚未稳定恢复。最近检测主组 %s。", safeMonitorGroup(groups.Fallback), monitorEvalText(eval))
				return monitorDecision{Status: "ok", Action: "hold", CurrentGroup: "fallback", CurrentTarget: strings.TrimSpace(groups.Fallback), Message: msg, DownloadEval: eval, DownloadGroup: "primary"}, nil
			}
			if attempt < maxInt(m.cfg.SuccessThreshold, 1) {
				time.Sleep(recheckInterval)
			}
		}
	}

	if currentGroup != "fallback" {
		failures := 0
		lastEval := primaryEval
		for attempt := 1; attempt <= maxInt(m.cfg.FailThreshold, 1); attempt++ {
			eval, _ := m.evaluatePrimary(client, mustReadProxiesOnce(client), groups.Primary, true, policy)
			lastEval = eval
			if eval.Healthy {
				if err := m.applyPrimarySelection(client, defaultGroup, groups.Primary, eval); err != nil {
					msg := fmt.Sprintf("主组可用，但应用主组策略失败：%v", err)
					return monitorDecision{Status: "error", Action: "error", CurrentGroup: currentGroup, CurrentTarget: currentTarget, Message: msg}, fmt.Errorf(msg)
				}
				target := strings.TrimSpace(groups.Primary)
				msg := fmt.Sprintf("主组 %s 健康（%s），继续使用主组。", safeMonitorGroup(target), monitorEvalText(eval))
				if eval.PreferredTarget != "" && eval.PreferredTarget != eval.CurrentTarget && !m.cfg.DisablePrimaryGroupOptimization {
					msg += fmt.Sprintf(" 已将主组优选节点切到 %s。", eval.PreferredTarget)
					return monitorDecision{Status: "ok", Action: "optimize", CurrentGroup: "primary", CurrentTarget: target, Message: msg, DownloadEval: eval, DownloadGroup: "primary"}, nil
				}
				return monitorDecision{Status: "ok", Action: "hold", CurrentGroup: "primary", CurrentTarget: target, Message: msg, DownloadEval: eval, DownloadGroup: "primary"}, nil
			}
			failures++
			if attempt < maxInt(m.cfg.FailThreshold, 1) {
				time.Sleep(recheckInterval)
			}
		}

		if fallbackEval.Healthy {
			if err := client.selectProxy(defaultGroup, strings.TrimSpace(groups.Fallback)); err != nil {
				msg := fmt.Sprintf("主组连续 %d 次异常，尝试切到备组失败：%v", failures, err)
				return monitorDecision{Status: "error", Action: "error", CurrentGroup: currentGroup, CurrentTarget: currentTarget, Message: msg}, fmt.Errorf(msg)
			}
			target := strings.TrimSpace(groups.Fallback)
			msg := fmt.Sprintf("主组 %s 连续 %d 次检测异常（最近 %s），已切换到备组 %s（%s）。", safeMonitorGroup(groups.Primary), failures, monitorEvalText(lastEval), safeMonitorGroup(target), monitorEvalText(fallbackEval))
			return monitorDecision{Status: "ok", Action: "switch", CurrentGroup: "fallback", CurrentTarget: target, Message: msg, DownloadEval: lastEval, DownloadGroup: "primary"}, nil
		}
		msg := fmt.Sprintf("主组 %s 异常，但备组 %s 也不可用：主组 %s，备组 %s。未执行切换。", safeMonitorGroup(groups.Primary), safeMonitorGroup(groups.Fallback), monitorEvalText(lastEval), monitorEvalText(fallbackEval))
		return monitorDecision{Status: "error", Action: "hold", CurrentGroup: currentGroup, CurrentTarget: currentTarget, Message: msg, DownloadEval: lastEval, DownloadGroup: "primary"}, fmt.Errorf(msg)
	}

	msg := fmt.Sprintf("当前保持备组 %s，自动回切 %s。主组最近 %s，备组最近 %s。", safeMonitorGroup(groups.Fallback), ternaryString(m.cfg.AutoFailback, "已开启", "已关闭"), monitorEvalText(primaryEval), monitorEvalText(fallbackEval))
	return monitorDecision{Status: "ok", Action: "hold", CurrentGroup: "fallback", CurrentTarget: strings.TrimSpace(groups.Fallback), Message: msg, DownloadEval: fallbackEval, DownloadGroup: "fallback"}, nil
}

func monitorDownloadDisplayEval(currentGroup string, primaryEval, fallbackEval monitorGroupEval, decision monitorDecision) (monitorGroupEval, string) {
	if strings.TrimSpace(decision.DownloadGroup) != "" && monitorEvalHasDownload(decision.DownloadEval) {
		return decision.DownloadEval, strings.TrimSpace(decision.DownloadGroup)
	}
	if currentGroup != "fallback" && monitorEvalHasDownload(primaryEval) {
		return primaryEval, "primary"
	}
	if currentGroup == "fallback" && monitorEvalHasDownload(fallbackEval) {
		return fallbackEval, "fallback"
	}
	if monitorEvalHasDownload(primaryEval) {
		return primaryEval, "primary"
	}
	if monitorEvalHasDownload(fallbackEval) {
		return fallbackEval, "fallback"
	}
	return monitorGroupEval{}, ""
}

func monitorEvalHasDownload(eval monitorGroupEval) bool {
	return eval.DownloadKBps > 0 || eval.DownloadRequired > 0 || eval.DownloadWindows > 0 || strings.TrimSpace(eval.DownloadPhase) != "" || strings.TrimSpace(eval.DownloadError) != ""
}

func (m *MonitorService) applyPrimarySelection(client *clashMonitorClient, defaultGroup, primaryGroup string, eval monitorGroupEval) error {
	primaryGroup = strings.TrimSpace(primaryGroup)
	if defaultGroup == "" || primaryGroup == "" {
		return fmt.Errorf("默认组或主组为空")
	}
	if !m.cfg.DisablePrimaryGroupOptimization && eval.PreferredTarget != "" && eval.PreferredTarget != eval.CurrentTarget {
		if err := client.selectProxy(primaryGroup, eval.PreferredTarget); err != nil {
			return err
		}
	}
	return client.selectProxy(defaultGroup, primaryGroup)
}

func (m *MonitorService) resolveRuntimeGroups(proxies map[string]clashProxyInfo) (monitorRuntimeGroups, error) {
	defaultGroup, err := resolveMonitorGroupTag(m.cfg.DefaultProxyGroup, proxies)
	if err != nil {
		return monitorRuntimeGroups{}, fmt.Errorf("未找到默认代理组 %s", safeMonitorGroup(m.cfg.DefaultProxyGroup))
	}
	primaryGroup, err := resolveMonitorGroupTag(m.cfg.PrimaryGroup, proxies)
	if err != nil {
		return monitorRuntimeGroups{}, fmt.Errorf("未找到主组 %s", safeMonitorGroup(m.cfg.PrimaryGroup))
	}
	fallbackGroup, err := resolveMonitorGroupTag(m.cfg.FallbackGroup, proxies)
	if err != nil {
		return monitorRuntimeGroups{}, fmt.Errorf("未找到备组 %s", safeMonitorGroup(m.cfg.FallbackGroup))
	}
	return monitorRuntimeGroups{
		Default:  defaultGroup,
		Primary:  primaryGroup,
		Fallback: fallbackGroup,
	}, nil
}

func mustReadProxiesOnce(client *clashMonitorClient) map[string]clashProxyInfo {
	proxies, err := client.proxies()
	if err != nil {
		return map[string]clashProxyInfo{}
	}
	return proxies
}

func (c *clashMonitorClient) proxies() (map[string]clashProxyInfo, error) {
	var resp clashProxiesResponse
	if err := c.doJSON(http.MethodGet, "/proxies", nil, &resp); err != nil {
		return nil, err
	}
	if resp.Proxies == nil {
		return nil, fmt.Errorf("empty proxies response")
	}
	return resp.Proxies, nil
}

func (c *clashMonitorClient) delay(name, testURL string, timeoutMS int) (int, error) {
	if strings.TrimSpace(testURL) == "" {
		testURL = "http://www.gstatic.com/generate_204"
	}
	if timeoutMS <= 0 {
		timeoutMS = 5000
	}
	q := url.Values{}
	q.Set("url", testURL)
	q.Set("timeout", fmt.Sprintf("%d", timeoutMS))
	var resp clashDelayResponse
	if err := c.doJSON(http.MethodGet, "/proxies/"+url.PathEscape(strings.TrimSpace(name))+"/delay?"+q.Encode(), nil, &resp); err != nil {
		return 0, err
	}
	if resp.Delay <= 0 {
		return 0, fmt.Errorf("delay unavailable")
	}
	return resp.Delay, nil
}

func (c *clashMonitorClient) selectProxy(group, target string) error {
	payload := map[string]string{"name": strings.TrimSpace(target)}
	return c.doJSON(http.MethodPut, "/proxies/"+url.PathEscape(strings.TrimSpace(group)), payload, nil)
}

func (c *clashMonitorClient) doJSON(method, path string, body any, out any) error {
	fullURL := strings.TrimRight(c.base, "/") + path
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, fullURL, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (m *MonitorService) monitorCronSummary(line string) string {
	trigger := "已开启：按 cron 表达式触发"
	fields := strings.Fields(line)
	if len(fields) > 0 {
		if fields[0] == "*" {
			trigger = "已开启：每 1 分钟触发"
		} else if strings.HasPrefix(fields[0], "*/") {
			trigger = fmt.Sprintf("已开启：每 %s 分钟触发", strings.TrimPrefix(fields[0], "*/"))
		}
	}
	return trigger + "；" + m.monitorSchedulePolicySummary()
}

func (m *MonitorService) monitorCronSetMessage() string {
	return "节点监控定时任务已保存：每 1 分钟触发；" + m.monitorSchedulePolicySummary()
}

func (m *MonitorService) monitorSchedulePolicySummary() string {
	dayInterval := m.cfg.DayCheckIntervalMin
	if dayInterval <= 0 {
		dayInterval = 5
	}
	peakInterval := m.cfg.PeakCheckIntervalMin
	if peakInterval <= 0 {
		peakInterval = 1
	}
	dayDownload := "下载测速关闭"
	if m.cfg.VideoCheckEnabled && m.cfg.VideoDayCheckEnabled {
		dayDownload = fmt.Sprintf("下载测速开启，要求 %dKB/s", maxInt(m.cfg.VideoDayMinDownloadKBps, maxInt(m.cfg.MinDownloadKBps, 1)))
	}
	peakDownload := "下载测速关闭"
	if m.cfg.VideoCheckEnabled && m.cfg.VideoPeakCheckEnabled {
		peakDownload = fmt.Sprintf("下载测速开启，要求 %dKB/s", maxInt(m.cfg.VideoPeakMinDownloadKBps, maxInt(m.cfg.MinDownloadKBps, 1)))
	}
	precheck := "异常预检开启"
	if m.cfg.DownloadPrecheckDisabled {
		precheck = "异常预检关闭"
	}
	peakStart := strings.TrimSpace(m.cfg.VideoPeakStart)
	if peakStart == "" {
		peakStart = "19:00"
	}
	peakEnd := strings.TrimSpace(m.cfg.VideoPeakEnd)
	if peakEnd == "" {
		peakEnd = "23:59"
	}
	return fmt.Sprintf("白天每 %d 分钟完整检测（%s）；晚高峰 %s-%s 每 %d 分钟完整检测（%s）；%s", dayInterval, dayDownload, peakStart, peakEnd, peakInterval, peakDownload, precheck)
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

func classifyMonitorGroup(currentTarget, primaryGroup, fallbackGroup string) string {
	switch strings.TrimSpace(currentTarget) {
	case strings.TrimSpace(primaryGroup):
		return "primary"
	case strings.TrimSpace(fallbackGroup):
		return "fallback"
	default:
		return "unknown"
	}
}

func safeMonitorGroup(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return strings.TrimSpace(v)
}

func resolveMonitorGroupTag(configured string, proxies map[string]clashProxyInfo) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return "", fmt.Errorf("empty group")
	}
	if _, ok := proxies[configured]; ok {
		return configured, nil
	}
	want := normalizeMonitorGroupKey(configured)
	if want == "" {
		return "", fmt.Errorf("empty normalized group")
	}
	matches := make([]string, 0, 2)
	for tag, info := range proxies {
		candidates := []string{strings.TrimSpace(tag), strings.TrimSpace(info.Name)}
		for _, candidate := range candidates {
			if normalizeMonitorGroupKey(candidate) == want {
				matches = append(matches, strings.TrimSpace(tag))
				break
			}
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("ambiguous group")
	}
	return "", fmt.Errorf("group not found")
}

func normalizeMonitorGroupKey(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range v {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r):
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minMonitorInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func formatMonitorLatency(delayMS int) string {
	if delayMS <= 0 {
		return ""
	}
	return fmt.Sprintf("%dms", delayMS)
}

func monitorDelayText(delayMS int, errText string) string {
	if delayMS > 0 {
		return fmt.Sprintf("%dms", delayMS)
	}
	if strings.TrimSpace(errText) != "" {
		return strings.TrimSpace(errText)
	}
	return "不可用"
}

func monitorEvalText(eval monitorGroupEval) string {
	base := monitorDelayText(eval.DelayMS, eval.Error)
	if eval.QualityScore <= 0 && eval.ProbeTotal <= 0 && eval.DownloadKBps <= 0 {
		return base
	}
	parts := []string{base}
	if eval.QualityScore > 0 {
		parts = append(parts, fmt.Sprintf("质量分 %d", eval.QualityScore))
	}
	if eval.ProbeTotal > 0 {
		parts = append(parts, fmt.Sprintf("访问 %d/%d", eval.ProbeSuccess, eval.ProbeTotal))
	}
	if eval.DownloadKBps > 0 {
		if eval.DownloadRequired > 0 {
			parts = append(parts, fmt.Sprintf("%s下载 %d/%dKB/s", downloadPhaseText(eval.DownloadPhase), eval.DownloadKBps, eval.DownloadRequired))
		} else {
			parts = append(parts, fmt.Sprintf("下载 %dKB/s", eval.DownloadKBps))
		}
	}
	if eval.DownloadWindows > 0 {
		parts = append(parts, fmt.Sprintf("低速窗口 %d/%d", eval.DownloadLowWin, eval.DownloadWindows))
	}
	if eval.DownloadKBps <= 0 && strings.TrimSpace(eval.DownloadError) != "" {
		parts = append(parts, "下载测速不可用")
	}
	if strings.TrimSpace(eval.QualityReason) != "" && !eval.Healthy {
		parts = append(parts, strings.TrimSpace(eval.QualityReason))
	}
	return strings.Join(parts, "，")
}

func downloadPhaseText(phase string) string {
	switch strings.TrimSpace(phase) {
	case "peak":
		return "晚高峰"
	case "day":
		return "白天"
	default:
		return ""
	}
}

func monitorFirstNonEmpty(items ...string) string {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return strings.TrimSpace(item)
		}
	}
	return ""
}

func ternaryString(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}
