package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cfgpkg "singdns-panel/internal/config"
	"singdns-panel/internal/services"
)

func TestPanelProbeRemoteAPI_Success(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"channels": map[string]any{
				"beta": map[string]any{
					"amd64": map[string]any{
						"version": "v9.9.9",
						"url":     server.URL + "/pkg.tar.gz",
					},
				},
			},
		})
	})
	mux.HandleFunc("/pkg.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	app := &App{Panel: services.NewPanelService("v1.0.0", cfgpkg.PanelUpdateConfig{
		BaseURL: server.URL + "/manifest.json",
		Channel: "beta",
		Arch:    "amd64",
	})}

	req := httptest.NewRequest(http.MethodGet, "/api/panel/probe-remote", nil)
	rr := httptest.NewRecorder()
	app.PanelProbeRemoteAPI(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rr.Code, rr.Body.String())
	}
	var out map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got body=%v", out)
	}
	probe, _ := out["probe"].(map[string]any)
	if pkgOK, _ := probe["package_ok"].(bool); !pkgOK {
		t.Fatalf("expected package_ok=true, got probe=%v", probe)
	}
}

func TestPanelProbeRemoteAPI_BadRequest(t *testing.T) {
	app := &App{Panel: services.NewPanelService("v1.0.0", cfgpkg.PanelUpdateConfig{})}

	req := httptest.NewRequest(http.MethodGet, "/api/panel/probe-remote", nil)
	rr := httptest.NewRecorder()
	app.PanelProbeRemoteAPI(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "未配置 panel_update.base_url") {
		t.Fatalf("unexpected error body: %s", rr.Body.String())
	}
}

func TestPanelUpdateConfigSaveAPI_InvalidChannel(t *testing.T) {
	app := &App{Panel: services.NewPanelService("v1.0.0", cfgpkg.PanelUpdateConfig{})}

	body := strings.NewReader(`{"channel":"nightly"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/panel/update-config", body)
	rr := httptest.NewRecorder()
	app.PanelUpdateConfigSaveAPI(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "channel 仅支持 beta/stable") {
		t.Fatalf("unexpected error body: %s", rr.Body.String())
	}
}

func TestPanelUpdateConfigSaveAPI_ValidAndPersist(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "panel.json")
	cfg := &cfgpkg.Config{}
	if err := os.WriteFile(cfgPath, []byte(`{"listen":":9999"}`), 0644); err != nil {
		t.Fatalf("prepare config file: %v", err)
	}

	app := &App{
		Config:     cfg,
		ConfigPath: cfgPath,
		Panel:      services.NewPanelService("v1.0.0", cfgpkg.PanelUpdateConfig{}),
	}

	body := strings.NewReader(`{"base_url":"https://example.com/latest.json","channel":"beta","arch":"x86_64"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/panel/update-config", body)
	rr := httptest.NewRecorder()
	app.PanelUpdateConfigSaveAPI(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rr.Code, rr.Body.String())
	}
	if app.Config.PanelUpdate.BaseURL != "https://example.com/latest.json" {
		t.Fatalf("config not updated in memory: %+v", app.Config.PanelUpdate)
	}
	if app.Config.PanelUpdate.Channel != "beta" || app.Config.PanelUpdate.Arch != "amd64" {
		t.Fatalf("unexpected normalized channel/arch: %+v", app.Config.PanelUpdate)
	}
	snap := app.Panel.ConfigSnapshot()
	if snap.BaseURL != "https://example.com/latest.json" || snap.Arch != "amd64" {
		t.Fatalf("panel service config not updated: %+v", snap)
	}
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	if !strings.Contains(string(b), "https://example.com/latest.json") {
		t.Fatalf("persisted config missing base_url: %s", string(b))
	}
}

func TestPanelVersionAPI_RemoteErrorField(t *testing.T) {
	app := &App{Panel: services.NewPanelService("v1.0.0", cfgpkg.PanelUpdateConfig{})}

	req := httptest.NewRequest(http.MethodGet, "/api/panel/version", nil)
	rr := httptest.NewRecorder()
	app.PanelVersionAPI(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rr.Code, rr.Body.String())
	}
	var out map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	re, _ := out["remote_error"].(string)
	if !strings.Contains(re, "未配置 panel_update.base_url") {
		t.Fatalf("expected remote_error to contain base_url message, got: %q", re)
	}
	if msg, _ := out["message"].(string); msg != "远程更新源不可用" {
		t.Fatalf("unexpected message: %q", msg)
	}
}
