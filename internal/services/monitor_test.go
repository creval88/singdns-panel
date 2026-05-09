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

	cfgpkg "singdns-panel/internal/config"
)

type fakeClashAPI struct {
	mu      sync.Mutex
	proxies map[string]clashProxyInfo
	delays  map[string]int
	puts    []string
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
