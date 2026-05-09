package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

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

func (a *App) SystemSwitchSingBoxBinaryAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respondMessage(w, err, "")
		return
	}
	res, err := a.SingBox.SwitchServiceBinary(in.Path)
	if err == nil {
		a.Config.Services.SingBox.BinPath = a.SingBox.BinPath()
	}
	a.respondAudited(w, r, "system.install.singbox.switch_binary", res, err, "Sing-box 服务内核路径已切换")
}

func adviceForInstallError(err string) []string {
	if strings.TrimSpace(err) == "" {
		return nil
	}
	a := make([]string, 0, 4)
	if strings.Contains(err, "rsync: command not found") {
		a = append(a, "目标机缺少 rsync：apt-get install -y rsync；或升级到带 cp -a 兜底的最新面板后重试")
	}
	if strings.Contains(err, "/usr/bin/sing-box: no such file or directory") || strings.Contains(err, "fork/exec /usr/bin/sing-box: no such file or directory") {
		a = append(a, "未找到 /usr/bin/sing-box：到系统设置→组件安装先安装 Sing-box，或在 panel.json 设置正确 BinPath")
	}
	if regexp.MustCompile(`(?i)(connection refused|TLS handshake timeout|i/o timeout|dial tcp .*: connect: operation timed out)`).MatchString(err) {
		a = append(a, "出网异常：检查网络/代理，或改用“离线安装”上传包")
	}
	if strings.Contains(strings.ToLower(err), "permission denied") {
		a = append(a, "权限不足：检查 sudoers.singdns-panel 与运行用户权限")
	}
	return a
}

func (a *App) SystemInstallSingBoxAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		EnableIPForward bool `json:"enable_ip_forward"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if !in.EnableIPForward {
		in.EnableIPForward = true
	}
	// 分阶段审计（v1 粗粒度）
	t0 := time.Now()
	steps := make([]map[string]any, 0, 4)
	step := func(code, msg, status string, since time.Time) map[string]any {
		return map[string]any{
			"code":        code,
			"message":     msg,
			"status":      status,
			"duration_ms": time.Since(since).Milliseconds(),
			"ended_at":    time.Now().Format(time.RFC3339),
		}
	}
	steps = append(steps, step("precheck", "参数校验与环境快照", "ok", t0))

	t1 := time.Now()
	res, err := a.SingBox.InstallOfficial(in.EnableIPForward)
	if err != nil {
		steps = append(steps, step("install", "执行官方安装脚本", "failed", t1))
		a.auditFromRequest(r, "system.install.singbox", err)
		hints := adviceForInstallError(err.Error())
		respondJSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"error":   err.Error(),
			"message": "安装失败",
			"steps":   steps,
			"hints":   hints,
		})
		return
	}
	steps = append(steps, step("install", "执行官方安装脚本", "ok", t1))

	t2 := time.Now()
	st, _ := a.SingBox.Status()
	statusMsg := ""
	if st != nil {
		if st.Active {
			statusMsg = "服务已启动"
		} else {
			statusMsg = "服务未启动"
		}
	}
	steps = append(steps, step("verify", "服务状态校验："+statusMsg, "ok", t2))

	a.auditMessageFromRequest(r, "system.install.singbox", res.AuditText())
	respondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": res.AuditText(),
		"steps":   steps,
	})
}

func (a *App) SystemInstallMosDNSAPI(w http.ResponseWriter, r *http.Request) {
	// 分阶段审计（v1 粗粒度）
	t0 := time.Now()
	steps := make([]map[string]any, 0, 3)
	step := func(code, msg, status string, since time.Time) map[string]any {
		return map[string]any{
			"code":        code,
			"message":     msg,
			"status":      status,
			"duration_ms": time.Since(since).Milliseconds(),
			"ended_at":    time.Now().Format(time.RFC3339),
		}
	}
	steps = append(steps, step("precheck", "环境快照", "ok", t0))

	t1 := time.Now()
	res, err := a.MosDNS.InstallFromReference()
	if err != nil {
		steps = append(steps, step("install", "执行参考安装逻辑", "failed", t1))
		a.auditFromRequest(r, "system.install.mosdns", err)
		hints := adviceForInstallError(err.Error())
		respondJSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"error":   err.Error(),
			"message": "安装失败",
			"steps":   steps,
			"hints":   hints,
		})
		return
	}
	steps = append(steps, step("install", "执行参考安装逻辑", "ok", t1))

	t2 := time.Now()
	st, _ := a.MosDNS.Status()
	statusMsg := ""
	if st != nil {
		if st.Active {
			statusMsg = "服务已启动"
		} else {
			statusMsg = "服务未启动"
		}
	}
	steps = append(steps, step("verify", "服务状态校验："+statusMsg, "ok", t2))

	a.auditMessageFromRequest(r, "system.install.mosdns", res.AuditText())
	respondJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": res.AuditText(),
		"steps":   steps,
	})
}

// 安装前出网预检（最小集）
func (a *App) SystemInstallPreflightAPI(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 5 * time.Second}
	tests := []string{
		"https://raw.githubusercontent.com/", // manifest/原始内容
		"https://github.com/",                // release 页面
		"https://api.github.com/",            // API 速测
	}
	results := make(map[string]string)
	okAll := true
	for _, u := range tests {
		req, _ := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
		resp, err := client.Do(req)
		if err != nil {
			results[u] = err.Error()
			okAll = false
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 400 {
			results[u] = fmt.Sprintf("HTTP %d", resp.StatusCode)
			okAll = false
		} else {
			results[u] = "ok"
		}
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"ok":      okAll,
		"results": results,
	})
}

// 离线安装：上传 mosdns 核心与（可选）配置包
func (a *App) SystemInstallMosDNSUploadAPI(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(512 << 20); err != nil { // 512MB
		respondMessage(w, fmt.Errorf("解析上传失败: %w", err), "")
		return
	}
	coreFile, coreHdr, err := r.FormFile("file")
	if err != nil {
		respondMessage(w, fmt.Errorf("缺少 file 文件: %w", err), "")
		return
	}
	defer coreFile.Close()
	tmpCore, err := os.CreateTemp("", "mosdns-core-*")
	if err != nil {
		respondMessage(w, err, "")
		return
	}
	defer os.Remove(tmpCore.Name())
	if _, err := io.Copy(tmpCore, coreFile); err != nil {
		tmpCore.Close()
		respondMessage(w, err, "")
		return
	}
	if err := tmpCore.Close(); err != nil {
		respondMessage(w, err, "")
		return
	}
	cfgPath := ""
	if cfgFile, _, err2 := r.FormFile("config"); err2 == nil {
		defer cfgFile.Close()
		tmpCfg, err3 := os.CreateTemp("", "mosdns-cfg-*")
		if err3 != nil {
			respondMessage(w, err3, "")
			return
		}
		defer os.Remove(tmpCfg.Name())
		if _, err := io.Copy(tmpCfg, cfgFile); err != nil {
			tmpCfg.Close()
			respondMessage(w, err, "")
			return
		}
		if err := tmpCfg.Close(); err != nil {
			respondMessage(w, err, "")
			return
		}
		cfgPath = tmpCfg.Name()
	}
	res, err := a.MosDNS.InstallFromUploaded(tmpCore.Name(), cfgPath, coreHdr.Filename)
	a.respondAudited(w, r, "system.install.mosdns.upload", res, err, "MosDNS 离线安装成功")
}

// 离线安装：上传 sing-box 核心包
func (a *App) SystemInstallSingBoxUploadAPI(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(512 << 20); err != nil { // 512MB
		respondMessage(w, fmt.Errorf("解析上传失败: %w", err), "")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		respondMessage(w, fmt.Errorf("缺少 file 文件: %w", err), "")
		return
	}
	defer file.Close()
	tmp, err := os.CreateTemp("", "singbox-core-*")
	if err != nil {
		respondMessage(w, err, "")
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, file); err != nil {
		tmp.Close()
		respondMessage(w, err, "")
		return
	}
	if err := tmp.Close(); err != nil {
		respondMessage(w, err, "")
		return
	}
	res, err := a.SingBox.UpgradeFromUploadedCore(tmp.Name(), hdr.Filename)
	a.respondAudited(w, r, "system.install.singbox.upload", res, err, "Sing-box 离线安装成功")
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
