package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	cfgpkg "singdns-panel/internal/config"
)

type fakeClashAPI struct {
	mu         sync.Mutex
	proxies    map[string]clashProxyInfo
	delays     map[string]int
	delayByURL map[string]map[string]int
	puts       []string
}

func (f *fakeClashAPI) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/proxies":
		_ = json.NewEncoder(w).Encode(map[string]any{"proxies": f.proxies})
		return
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/delay"):
		name := strings.TrimPrefix(strings.TrimSuffix(r.URL.Path, "/delay"), "/proxies/")
		if byURL, ok := f.delayByURL[name]; ok {
			testURL := r.URL.Query().Get("url")
			if delay, ok := byURL[testURL]; ok && delay > 0 {
				_ = json.NewEncoder(w).Encode(map[string]any{"delay": delay})
				return
			}
			http.Error(w, `{"error":"unavailable"}`, http.StatusBadGateway)
			return
		}
		if delay, ok := f.delays[name]; ok && delay > 0 {
			_ = json.NewEncoder(w).Encode(map[string]any{"delay": delay})
			return
		}
		http.Error(w, `{"error":"unavailable"}`, http.StatusBadGateway)
		return
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/proxies/"):
		group := strings.TrimPrefix(r.URL.Path, "/proxies/")
		var in struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		proxy := f.proxies[group]
		proxy.Now = in.Name
		f.proxies[group] = proxy
		f.puts = append(f.puts, group+"->"+in.Name)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		return
	default:
		http.NotFound(w, r)
	}
}

func writeMonitorConfig(t *testing.T, dir, apiBase string) string {
	t.Helper()
	cfg := map[string]any{
		"experimental": map[string]any{
			"clash_api": map[string]any{
				"external_controller": strings.TrimPrefix(apiBase, "http://"),
			},
		},
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestMonitorStatusHandlesBrokenStateFile(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	statePath := filepath.Join(tmp, "monitor-state.json")
	if err := os.WriteFile(statePath, []byte(`{broken-json`), 0644); err != nil {
		t.Fatalf("write state: %v", err)
	}
	monitor := NewMonitorService(cfgpkg.MonitorConfig{
		Enabled:       true,
		PrimaryGroup:  "日本手动",
		FallbackGroup: "自建节点",
		StateFile:     statePath,
	}, nil)

	status, err := monitor.Status()
	if err != nil {
		t.Fatalf("Status err: %v", err)
	}
	if status == nil {
		t.Fatalf("Status returned nil")
	}
	if status.PrimaryGroup != "日本手动" || status.FallbackGroup != "自建节点" {
		t.Fatalf("unexpected status groups: %#v", status)
	}
}

func TestMonitorSaveStateIsReadableByPanelUser(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	statePath := filepath.Join(tmp, "monitor-state.json")
	monitor := NewMonitorService(cfgpkg.MonitorConfig{StateFile: statePath}, nil)
	if err := monitor.saveState(&monitorState{LastRunAt: "2026-05-11 10:00:00"}); err != nil {
		t.Fatalf("saveState err: %v", err)
	}
	info, err := os.Stat(statePath)
	if err != nil {
		t.Fatalf("stat state: %v", err)
	}
	if got := info.Mode().Perm(); got&0044 != 0044 {
		t.Fatalf("state file should be group/world readable, mode=%o", got)
	}
}

func TestMonitorActiveDownloadThresholdUsesCustomPeakWindow(t *testing.T) {
	t.Parallel()

	monitor := NewMonitorService(cfgpkg.MonitorConfig{
		MinDownloadKBps:            500,
		VideoCheckEnabled:          true,
		VideoDayMinDownloadKBps:    1200,
		VideoPeakMinDownloadKBps:   3200,
		VideoPeakStart:             "19:30",
		VideoPeakEnd:               "01:00",
		VideoDownloadDurationSec:   12,
		VideoDownloadWindowSec:     3,
		VideoDownloadMaxLowWindows: 1,
	}, nil)

	peak := monitor.activeDownloadThreshold(time.Date(2026, 5, 11, 23, 0, 0, 0, time.Local))
	if peak.Phase != "peak" || peak.RequiredKBps != 3200 || peak.DurationSec != 12 || peak.WindowSec != 3 || peak.MaxLowWindows != 1 {
		t.Fatalf("unexpected peak policy: %#v", peak)
	}
	day := monitor.activeDownloadThreshold(time.Date(2026, 5, 11, 10, 0, 0, 0, time.Local))
	if day.Phase != "day" || day.RequiredKBps != 1200 {
		t.Fatalf("unexpected day policy: %#v", day)
	}
}

func TestMonitorRunPolicyUsesDayAndPeakIntervals(t *testing.T) {
	t.Parallel()

	monitor := NewMonitorService(cfgpkg.MonitorConfig{
		DayCheckIntervalMin:      5,
		PeakCheckIntervalMin:     1,
		DownloadPrecheckDisabled: false,
		VideoCheckEnabled:        true,
		VideoDayCheckEnabled:     false,
		VideoPeakCheckEnabled:    true,
		VideoPeakStart:           "19:00",
		VideoPeakEnd:             "23:59",
	}, nil)

	day := monitor.activeRunPolicy(time.Date(2026, 5, 12, 14, 0, 0, 0, time.Local))
	if day.Phase != "day" || day.IntervalMinutes != 5 || day.VideoDownloadEnabled {
		t.Fatalf("unexpected day policy: %#v", day)
	}
	peak := monitor.activeRunPolicy(time.Date(2026, 5, 12, 20, 0, 0, 0, time.Local))
	if peak.Phase != "peak" || peak.IntervalMinutes != 1 || !peak.VideoDownloadEnabled {
		t.Fatalf("unexpected peak policy: %#v", peak)
	}
}

func TestMonitorRunScheduledSkipsBeforeConfiguredInterval(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	statePath := filepath.Join(tmp, "monitor-state.json")
	now := time.Now()
	monitor := NewMonitorService(cfgpkg.MonitorConfig{
		Enabled:              true,
		DayCheckIntervalMin:  60,
		PeakCheckIntervalMin: 60,
		VideoPeakStart:       "00:00",
		VideoPeakEnd:         "00:01",
		StateFile:            statePath,
	}, nil)
	if err := monitor.saveState(&monitorState{LastRunAt: now.Format("2006-01-02 15:04:05")}); err != nil {
		t.Fatalf("saveState err: %v", err)
	}

	res, err := monitor.RunScheduled()
	if err != nil {
		t.Fatalf("RunScheduled err: %v", err)
	}
	if res == nil || res.Action != "monitor.run.skip" || !strings.Contains(res.Message, "未到下一轮完整检测时间") {
		t.Fatalf("unexpected scheduled result: %#v", res)
	}
	st, err := monitor.loadState()
	if err != nil {
		t.Fatalf("loadState err: %v", err)
	}
	if st.LastRunAt != now.Format("2006-01-02 15:04:05") || st.LastSchedulerAt == "" {
		t.Fatalf("unexpected state after skip: %#v", st)
	}
}

func TestMonitorRunOnceQualityProbeSwitchesToFallback(t *testing.T) {
	t.Parallel()

	api := &fakeClashAPI{
		proxies: map[string]clashProxyInfo{
			"默认代理": {Name: "默认代理", Type: "Selector", Now: "香港手动", All: []string{"香港手动", "自建节点"}},
			"香港手动": {Name: "香港手动", Type: "Selector", Now: "hk-a", All: []string{"hk-a"}},
			"自建节点": {Name: "自建节点", Type: "Selector", Now: "manual-a", All: []string{"manual-a"}},
		},
		delays: map[string]int{
			"自建节点": 120,
		},
		delayByURL: map[string]map[string]int{
			"hk-a": {
				"http://www.gstatic.com/generate_204": 80,
				"https://ok.example/204":              90,
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(api.handler))
	defer srv.Close()

	tmp := t.TempDir()
	cfgPath := writeMonitorConfig(t, tmp, srv.URL)
	singbox := &SingBoxService{cfg: cfgpkg.ServiceConfig{ConfigPath: cfgPath}}
	monitor := NewMonitorService(cfgpkg.MonitorConfig{
		Enabled:                  true,
		APIBase:                  srv.URL,
		DefaultProxyGroup:        "默认代理",
		PrimaryGroup:             "香港手动",
		FallbackGroup:            "自建节点",
		TestURL:                  "http://www.gstatic.com/generate_204",
		TimeoutMS:                1000,
		PrimaryMaxStableDelayMS:  150,
		FallbackMaxStableDelayMS: 300,
		FailThreshold:            1,
		SuccessThreshold:         1,
		RecheckIntervalSec:       1,
		AutoFailback:             true,
		QualityCheckEnabled:      true,
		ProbeURLs:                []string{"https://ok.example/204", "https://blocked.example/204"},
		MinProbeSuccess:          2,
		QualityScoreThreshold:    70,
		StateFile:                filepath.Join(tmp, "monitor-state.json"),
	}, singbox)

	res, err := monitor.RunOnce()
	if err != nil {
		t.Fatalf("RunOnce err: %v", err)
	}
	if res == nil || !strings.Contains(res.Message, "已切换到备组") {
		t.Fatalf("unexpected result: %#v", res)
	}
	if got := api.proxies["默认代理"].Now; got != "自建节点" {
		t.Fatalf("default group now=%q", got)
	}
}

func TestMonitorRunOnceSwitchesToFallback(t *testing.T) {
	t.Parallel()

	api := &fakeClashAPI{
		proxies: map[string]clashProxyInfo{
			"默认代理": {Name: "默认代理", Type: "Selector", Now: "香港手动", All: []string{"香港手动", "自建节点"}},
			"香港手动": {Name: "香港手动", Type: "Selector", Now: "hk-a", All: []string{"hk-a", "hk-b"}},
			"自建节点": {Name: "自建节点", Type: "Selector", Now: "manual-a", All: []string{"manual-a"}},
		},
		delays: map[string]int{
			"自建节点": 180,
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(api.handler))
	defer srv.Close()

	tmp := t.TempDir()
	cfgPath := writeMonitorConfig(t, tmp, srv.URL)
	singbox := &SingBoxService{cfg: cfgpkg.ServiceConfig{ConfigPath: cfgPath}}
	monitor := NewMonitorService(cfgpkg.MonitorConfig{
		Enabled:                  true,
		APIBase:                  srv.URL,
		DefaultProxyGroup:        "默认代理",
		PrimaryGroup:             "香港手动",
		FallbackGroup:            "自建节点",
		TestURL:                  "http://www.gstatic.com/generate_204",
		TimeoutMS:                1000,
		PrimaryMaxStableDelayMS:  150,
		FallbackMaxStableDelayMS: 300,
		FailThreshold:            1,
		SuccessThreshold:         1,
		RecheckIntervalSec:       1,
		AutoFailback:             true,
		StateFile:                filepath.Join(tmp, "monitor-state.json"),
	}, singbox)

	res, err := monitor.RunOnce()
	if err != nil {
		t.Fatalf("RunOnce err: %v", err)
	}
	if res == nil || !strings.Contains(res.Message, "已切换到备组") {
		t.Fatalf("unexpected result: %#v", res)
	}
	if got := api.proxies["默认代理"].Now; got != "自建节点" {
		t.Fatalf("default group now=%q", got)
	}
	if len(api.puts) != 1 || api.puts[0] != "默认代理->自建节点" {
		t.Fatalf("unexpected puts: %#v", api.puts)
	}
}

func TestMonitorRunOnceAutoFailbackAndOptimizePrimary(t *testing.T) {
	t.Parallel()

	api := &fakeClashAPI{
		proxies: map[string]clashProxyInfo{
			"默认代理": {Name: "默认代理", Type: "Selector", Now: "自建节点", All: []string{"香港手动", "自建节点"}},
			"香港手动": {Name: "香港手动", Type: "Selector", Now: "hk-b", All: []string{"hk-a", "hk-b"}},
			"自建节点": {Name: "自建节点", Type: "Selector", Now: "manual-a", All: []string{"manual-a"}},
		},
		delays: map[string]int{
			"hk-a": 80,
			"hk-b": 120,
			"自建节点": 180,
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(api.handler))
	defer srv.Close()

	tmp := t.TempDir()
	cfgPath := writeMonitorConfig(t, tmp, srv.URL)
	statePath := filepath.Join(tmp, "monitor-state.json")
	if err := os.WriteFile(statePath, []byte(`{"current_group":"fallback","current_target":"自建节点"}`), 0644); err != nil {
		t.Fatalf("write state: %v", err)
	}
	singbox := &SingBoxService{cfg: cfgpkg.ServiceConfig{ConfigPath: cfgPath}}
	monitor := NewMonitorService(cfgpkg.MonitorConfig{
		Enabled:                  true,
		APIBase:                  srv.URL,
		DefaultProxyGroup:        "默认代理",
		PrimaryGroup:             "香港手动",
		FallbackGroup:            "自建节点",
		TestURL:                  "http://www.gstatic.com/generate_204",
		TimeoutMS:                1000,
		PrimaryMaxStableDelayMS:  150,
		FallbackMaxStableDelayMS: 300,
		FailThreshold:            1,
		SuccessThreshold:         1,
		RecheckIntervalSec:       1,
		AutoFailback:             true,
		StateFile:                statePath,
	}, singbox)

	res, err := monitor.RunOnce()
	if err != nil {
		t.Fatalf("RunOnce err: %v", err)
	}
	if res == nil || !strings.Contains(res.Message, "已将默认代理切回主组") {
		t.Fatalf("unexpected result: %#v", res)
	}
	if got := api.proxies["香港手动"].Now; got != "hk-a" {
		t.Fatalf("primary group now=%q", got)
	}
	if got := api.proxies["默认代理"].Now; got != "香港手动" {
		t.Fatalf("default group now=%q", got)
	}
	if len(api.puts) != 2 || api.puts[0] != "香港手动->hk-a" || api.puts[1] != "默认代理->香港手动" {
		t.Fatalf("unexpected puts: %#v", api.puts)
	}
}

func TestMonitorRunOnceResolvesEmojiTaggedGroups(t *testing.T) {
	t.Parallel()

	api := &fakeClashAPI{
		proxies: map[string]clashProxyInfo{
			"🚀 默认代理":  {Name: "🚀 默认代理", Type: "Selector", Now: "🇭🇰 香港手动", All: []string{"🇭🇰 香港手动", "🚀 自建节点"}},
			"🇭🇰 香港手动": {Name: "🇭🇰 香港手动", Type: "Selector", Now: "hk-a", All: []string{"hk-a"}},
			"🚀 自建节点":  {Name: "🚀 自建节点", Type: "Selector", Now: "manual-a", All: []string{"manual-a"}},
		},
		delays: map[string]int{
			"hk-a":   260,
			"🚀 自建节点": 150,
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(api.handler))
	defer srv.Close()

	tmp := t.TempDir()
	cfgPath := writeMonitorConfig(t, tmp, srv.URL)
	singbox := &SingBoxService{cfg: cfgpkg.ServiceConfig{ConfigPath: cfgPath}}
	monitor := NewMonitorService(cfgpkg.MonitorConfig{
		Enabled:                  true,
		APIBase:                  srv.URL,
		DefaultProxyGroup:        "默认代理",
		PrimaryGroup:             "香港手动",
		FallbackGroup:            "自建节点",
		TestURL:                  "http://www.gstatic.com/generate_204",
		TimeoutMS:                1000,
		PrimaryMaxStableDelayMS:  200,
		FallbackMaxStableDelayMS: 300,
		FailThreshold:            1,
		SuccessThreshold:         1,
		RecheckIntervalSec:       1,
		AutoFailback:             true,
		StateFile:                filepath.Join(tmp, "monitor-state.json"),
	}, singbox)

	res, err := monitor.RunOnce()
	if err != nil {
		t.Fatalf("RunOnce err: %v", err)
	}
	if res == nil || !strings.Contains(res.Message, "已切换到备组") {
		t.Fatalf("unexpected result: %#v", res)
	}
	if got := api.proxies["🚀 默认代理"].Now; got != "🚀 自建节点" {
		t.Fatalf("default group now=%q", got)
	}
	if len(api.puts) != 1 || api.puts[0] != "🚀 默认代理->🚀 自建节点" {
		t.Fatalf("unexpected puts: %#v", api.puts)
	}
}
