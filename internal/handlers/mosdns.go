package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (a *App) MosDNSPage(w http.ResponseWriter, r *http.Request) {
	status, _ := a.MosDNS.Status()
	a.render(w, "mosdns.html", map[string]any{
		"Title":           "MosDNS",
		"ActiveNav":       "mosdns",
		"PageTitle":       "MosDNS 管理",
		"Eyebrow":         "Service",
		"SidebarSubtitle": "sing-box / mosdns 控制台",
		"Status":          status,
		"WebURL":          a.MosDNS.WebURL(),
	})
}

func (a *App) MosDNSStatusAPI(w http.ResponseWriter, r *http.Request) {
	a.serviceStatusAPI(w, a.MosDNS)
}

func (a *App) MosDNSActionAPI(w http.ResponseWriter, r *http.Request) {
	a.serviceActionAPI(w, r, "mosdns.action.", a.MosDNS, "MosDNS 已操作")
}

func normalizeMosDNSWebURL(raw string) (string, error) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return "", fmt.Errorf("web_url 不能为空")
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return "", fmt.Errorf("web_url 格式错误: %v", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("web_url 仅支持 http/https")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("web_url 缺少主机名")
	}
	return parsed.String(), nil
}

func (a *App) MosDNSConfigAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": true,
		"config": map[string]any{
			"web_url": a.MosDNS.WebURL(),
		},
	})
}

func (a *App) MosDNSConfigSaveAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		WebURL string `json:"web_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respondMessage(w, err, "")
		return
	}

	webURL, err := normalizeMosDNSWebURL(in.WebURL)
	if err != nil {
		respondMessage(w, err, "")
		return
	}

	a.Config.Services.MosDNS.WebURL = webURL
	if err := a.Config.Save(a.ConfigPath); err != nil {
		respondMessage(w, err, "")
		return
	}
	a.MosDNS.UpdateConfig(a.Config.Services.MosDNS)
	a.auditMessageFromRequest(r, "mosdns.config.save", "MosDNS 面板地址已保存")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"message": "MosDNS 面板地址已保存",
		"config": map[string]any{
			"web_url": webURL,
		},
	})
}
