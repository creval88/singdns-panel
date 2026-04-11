package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strings"

	cfgpkg "singdns-panel/internal/config"
	"singdns-panel/internal/services"
)

func normalizeArch(a string) string {
	switch strings.ToLower(strings.TrimSpace(a)) {
	case "x86_64", "x64":
		return "amd64"
	case "aarch64":
		return "arm64"
	default:
		return strings.ToLower(strings.TrimSpace(a))
	}
}

func normalizeChannel(c string) string {
	c = strings.ToLower(strings.TrimSpace(c))
	if c == "" {
		return "beta"
	}
	return c
}

func validatePanelBaseURL(raw string) (string, error) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return "", fmt.Errorf("base_url 不能为空")
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return "", fmt.Errorf("base_url 格式错误: %v", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("base_url 仅支持 http/https")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("base_url 缺少主机名")
	}
	return parsed.String(), nil
}

func (a *App) HealthAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (a *App) PanelUpdateConfigAPI(w http.ResponseWriter, r *http.Request) {
	cfg := a.Panel.ConfigSnapshot()
	cfg.Channel = normalizeChannel(cfg.Channel)
	if strings.TrimSpace(cfg.Arch) == "" {
		cfg.Arch = normalizeArch(runtime.GOARCH)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":     true,
		"config": cfg,
		"options": map[string]any{
			"channels": []string{"beta", "stable"},
			"arches":   []string{"amd64", "arm64"},
		},
	})
}

func (a *App) PanelUpdateConfigSaveAPI(w http.ResponseWriter, r *http.Request) {
	var in cfgpkg.PanelUpdateConfig
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respondMessage(w, err, "")
		return
	}
	cfg := a.Panel.ConfigSnapshot()
	if v := strings.TrimSpace(in.ReleaseDir); v != "" {
		cfg.ReleaseDir = v
	}
	if v := strings.TrimSpace(in.UpgradeCommand); v != "" {
		cfg.UpgradeCommand = v
	}
	if v := strings.TrimSpace(in.BaseURL); v != "" {
		normalizedURL, err := validatePanelBaseURL(v)
		if err != nil {
			respondMessage(w, err, "")
			return
		}
		cfg.BaseURL = normalizedURL
	}
	if v := strings.TrimSpace(in.Channel); v != "" {
		v = normalizeChannel(v)
		if v != "beta" && v != "stable" {
			respondMessage(w, fmt.Errorf("channel 仅支持 beta/stable"), "")
			return
		}
		cfg.Channel = v
	}
	if v := strings.TrimSpace(in.Arch); v != "" {
		v = normalizeArch(v)
		if v != "amd64" && v != "arm64" {
			respondMessage(w, fmt.Errorf("arch 仅支持 amd64/arm64"), "")
			return
		}
		cfg.Arch = v
	}

	cfg.Channel = normalizeChannel(cfg.Channel)
	if strings.TrimSpace(cfg.Arch) == "" {
		cfg.Arch = normalizeArch(runtime.GOARCH)
	}

	a.Config.PanelUpdate = cfg
	if err := a.Config.Save(a.ConfigPath); err != nil {
		respondMessage(w, err, "")
		return
	}
	a.Panel.UpdateConfig(cfg)
	a.auditMessageFromRequest(r, "panel.update_config", "更新源配置已保存")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"message": "更新源配置已保存",
		"config":  cfg,
	})
}

func (a *App) SystemPage(w http.ResponseWriter, r *http.Request) {
	panel, _ := a.Panel.LatestLocalRelease()
	installStatus := services.CollectSystemInstallStatus(a.SingBox, a.MosDNS)
	networkStatus, _ := a.Network.Status()
	a.render(w, "system.html", map[string]any{
		"Title":           "System Settings",
		"ActiveNav":       "system",
		"PageTitle":       "系统设置与升级",
		"Eyebrow":         "System",
		"SidebarSubtitle": "sing-box / mosdns 控制台",
		"PanelVersion":    a.Panel.CurrentVersion(),
		"PanelRelease":    panel,
		"Arch":            "linux/" + normalizeArch(runtime.GOARCH),
		"Listen":          a.Config.Listen,
		"InstallStatus":   installStatus,
		"NetworkStatus":   networkStatus,
	})
}

func (a *App) SystemInstallStatusAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":     true,
		"status": services.CollectSystemInstallStatus(a.SingBox, a.MosDNS),
	})
}

func (a *App) SystemInstallSingBoxAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		EnableIPForward bool `json:"enable_ip_forward"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if !in.EnableIPForward {
		in.EnableIPForward = true
	}
	res, err := a.SingBox.InstallOfficial(in.EnableIPForward)
	a.respondAudited(w, r, "system.install.singbox", res, err, "Sing-box 安装成功")
}

func (a *App) SystemInstallMosDNSAPI(w http.ResponseWriter, r *http.Request) {
	res, err := a.MosDNS.InstallFromReference()
	a.respondAudited(w, r, "system.install.mosdns", res, err, "MosDNS 安装成功")
}

func (a *App) SystemEnableIPForwardAPI(w http.ResponseWriter, r *http.Request) {
	err := a.SingBox.EnableIPForward()
	if err != nil {
		a.auditFromRequest(r, "system.ip_forward.enable", err)
		respondMessage(w, err, "")
		return
	}
	a.auditMessageFromRequest(r, "system.ip_forward.enable", "已开启 IP 转发")
	respondMessage(w, nil, "已开启 IP 转发")
}

func (a *App) SystemNetworkStatusAPI(w http.ResponseWriter, r *http.Request) {
	st, err := a.Network.Status()
	if err != nil {
		respondMessage(w, err, "")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":     true,
		"status": st,
	})
}

func (a *App) SystemNetworkSaveAPI(w http.ResponseWriter, r *http.Request) {
	var in services.NetworkSettingsInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respondMessage(w, err, "")
		return
	}
	res, err := a.Network.Apply(in)
	a.respondAudited(w, r, "system.network.apply", res, err, "网络配置已保存")
}
