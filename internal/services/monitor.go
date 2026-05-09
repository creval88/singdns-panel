package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode"

	cfgpkg "singdns-panel/internal/config"
)

const (
	monitorCronMarker = "# singdns-panel monitor-cron"
	monitorHistoryCap = 12
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
	LastSwitchAt          string             `json:"last_switch_at,omitempty"`
	LastSwitchReason      string             `json:"last_switch_reason,omitempty"`
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
	GroupTag        string
	GroupExists     bool
	GroupType       string
	Healthy         bool
	DelayMS         int
	CurrentTarget   string
	PreferredTarget string
	Error           string
}

type monitorDecision struct {
	Status        string
	Action        string
	CurrentGroup  string
	CurrentTarget string
	Message       string
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

	if msg := strings.TrimSpace(state.LastMessage); msg != "" {
		st.Message = msg
	} else {
		st.Message = fmt.Sprintf("将按 Clash API 对主组 %s 和备组 %s 进行检测，并根据阈值决定是否切换。", safeMonitorGroup(m.cfg.PrimaryGroup), safeMonitorGroup(m.cfg.FallbackGroup))
	}
	return st, nil
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
	state.LastRunAt = now.Format("2006-01-02 15:04:05")
	state.LastProbeTarget = strings.TrimSpace(m.cfg.APIBase)

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

	primaryEval, err := m.evaluatePrimary(client, proxies, groups.Primary)
	if err != nil && strings.TrimSpace(primaryEval.Error) == "" {
		primaryEval.Error = err.Error()
	}
	fallbackEval, fbErr := m.evaluateFallback(client, groups.Fallback)
	if fbErr != nil && strings.TrimSpace(fallbackEval.Error) == "" {
		fallbackEval.Error = fbErr.Error()
	}

	state.CurrentTarget = currentTarget
	state.CurrentGroup = currentGroup
	state.PrimaryActual = primaryEval.CurrentTarget
	state.LastPrimaryLatencyMS = primaryEval.DelayMS
	state.LastFallbackLatencyMS = fallbackEval.DelayMS
	state.LastProbeLatency = formatMonitorLatency(primaryEval.DelayMS)

	decision, err := m.decideAndApply(client, groups, currentGroup, currentTarget, primaryEval, fallbackEval)
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
	if st.History == nil {
		st.History = []MonitorRunResult{}
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

func (m *MonitorService) evaluatePrimary(client *clashMonitorClient, proxies map[string]clashProxyInfo, groupTag string) (monitorGroupEval, error) {
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
		eval.Healthy = err == nil && delay > 0 && delay <= m.cfg.PrimaryMaxStableDelayMS
		if err != nil {
			eval.Error = err.Error()
			return eval, err
		}
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
	eval.Healthy = bestDelay > 0 && bestDelay <= m.cfg.PrimaryMaxStableDelayMS
	if bestTarget == "" && lastErr != nil {
		eval.Error = lastErr.Error()
		return eval, lastErr
	}
	return eval, nil
}

func (m *MonitorService) evaluateFallback(client *clashMonitorClient, groupTag string) (monitorGroupEval, error) {
	groupTag = strings.TrimSpace(groupTag)
	eval := monitorGroupEval{GroupTag: groupTag, PreferredTarget: groupTag}
	if groupTag == "" {
		eval.Error = "未配置备组"
		return eval, fmt.Errorf(eval.Error)
	}
	delay, err := client.delay(groupTag, m.cfg.TestURL, m.cfg.TimeoutMS)
	eval.DelayMS = delay
	eval.Healthy = err == nil && delay > 0 && delay <= m.cfg.FallbackMaxStableDelayMS
	if err != nil {
		eval.Error = err.Error()
		return eval, err
	}
	return eval, nil
}

func (m *MonitorService) decideAndApply(client *clashMonitorClient, groups monitorRuntimeGroups, currentGroup, currentTarget string, primaryEval, fallbackEval monitorGroupEval) (monitorDecision, error) {
	defaultGroup := strings.TrimSpace(groups.Default)
	recheckInterval := time.Duration(maxInt(m.cfg.RecheckIntervalSec, 1)) * time.Second

	if currentGroup == "fallback" && m.cfg.AutoFailback {
		successes := 0
		bestEval := primaryEval
		for attempt := 1; attempt <= maxInt(m.cfg.SuccessThreshold, 1); attempt++ {
			eval, _ := m.evaluatePrimary(client, mustReadProxiesOnce(client), groups.Primary)
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
					return monitorDecision{Status: "ok", Action: "switch", CurrentGroup: "primary", CurrentTarget: target, Message: msg}, nil
				}
			} else {
				msg := fmt.Sprintf("当前已在备组 %s，主组尚未稳定恢复。最近检测主组延迟 %s。", safeMonitorGroup(groups.Fallback), monitorDelayText(eval.DelayMS, eval.Error))
				return monitorDecision{Status: "ok", Action: "hold", CurrentGroup: "fallback", CurrentTarget: strings.TrimSpace(groups.Fallback), Message: msg}, nil
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
			eval, _ := m.evaluatePrimary(client, mustReadProxiesOnce(client), groups.Primary)
			lastEval = eval
			if eval.Healthy {
				if err := m.applyPrimarySelection(client, defaultGroup, groups.Primary, eval); err != nil {
					msg := fmt.Sprintf("主组可用，但应用主组策略失败：%v", err)
					return monitorDecision{Status: "error", Action: "error", CurrentGroup: currentGroup, CurrentTarget: currentTarget, Message: msg}, fmt.Errorf(msg)
				}
				target := strings.TrimSpace(groups.Primary)
				msg := fmt.Sprintf("主组 %s 健康（%dms），继续使用主组。", safeMonitorGroup(target), eval.DelayMS)
				if eval.PreferredTarget != "" && eval.PreferredTarget != eval.CurrentTarget && !m.cfg.DisablePrimaryGroupOptimization {
					msg += fmt.Sprintf(" 已将主组优选节点切到 %s。", eval.PreferredTarget)
					return monitorDecision{Status: "ok", Action: "optimize", CurrentGroup: "primary", CurrentTarget: target, Message: msg}, nil
				}
				return monitorDecision{Status: "ok", Action: "hold", CurrentGroup: "primary", CurrentTarget: target, Message: msg}, nil
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
			msg := fmt.Sprintf("主组 %s 连续 %d 次检测异常（最近 %s），已切换到备组 %s（%dms）。", safeMonitorGroup(groups.Primary), failures, monitorDelayText(lastEval.DelayMS, lastEval.Error), safeMonitorGroup(target), fallbackEval.DelayMS)
			return monitorDecision{Status: "ok", Action: "switch", CurrentGroup: "fallback", CurrentTarget: target, Message: msg}, nil
		}
		msg := fmt.Sprintf("主组 %s 异常，但备组 %s 也不可用：主组 %s，备组 %s。未执行切换。", safeMonitorGroup(groups.Primary), safeMonitorGroup(groups.Fallback), monitorDelayText(lastEval.DelayMS, lastEval.Error), monitorDelayText(fallbackEval.DelayMS, fallbackEval.Error))
		return monitorDecision{Status: "error", Action: "hold", CurrentGroup: currentGroup, CurrentTarget: currentTarget, Message: msg}, fmt.Errorf(msg)
	}

	msg := fmt.Sprintf("当前保持备组 %s，自动回切 %s。主组最近 %s，备组最近 %s。", safeMonitorGroup(groups.Fallback), ternaryString(m.cfg.AutoFailback, "已开启", "已关闭"), monitorDelayText(primaryEval.DelayMS, primaryEval.Error), monitorDelayText(fallbackEval.DelayMS, fallbackEval.Error))
	return monitorDecision{Status: "ok", Action: "hold", CurrentGroup: "fallback", CurrentTarget: strings.TrimSpace(groups.Fallback), Message: msg}, nil
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

func monitorCronSummary(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "已开启"
	}
	if fields[0] == "*" {
		return "已开启：每 1 分钟执行一次监控"
	}
	if strings.HasPrefix(fields[0], "*/") {
		return fmt.Sprintf("已开启：每 %s 分钟执行一次监控", strings.TrimPrefix(fields[0], "*/"))
	}
	return "已开启：按 cron 表达式执行监控"
}

func monitorCronSetMessage(intervalMinutes int) string {
	if intervalMinutes <= 1 {
		return "节点监控定时任务已保存：每 1 分钟执行一次"
	}
	return fmt.Sprintf("节点监控定时任务已保存：每 %d 分钟执行一次", intervalMinutes)
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

func ternaryString(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}
