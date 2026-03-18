package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"singdns-panel/internal/services"
)

func (a *App) PanelVersionAPI(w http.ResponseWriter, r *http.Request) {
	current := a.Panel.CurrentVersion()
	release, localErr := a.Panel.LatestLocalRelease()
	configured := a.Panel.Configured()
	message := "尚未配置面板更新源"
	hasUpdate := false
	latestVersion := ""
	var remote *services.RemoteReleaseInfo

	if localErr != nil {
		message = localErr.Error()
	} else if configured {
		if release == nil {
			message = "已配置本地升级目录，但暂未发现可用发布包"
		} else {
			latestVersion = release.Version
			hasUpdate = release.Version != "" && release.Version != current && release.HasUpgrade
			if release.HasUpgrade {
				message = "已检测到本地升级候选"
			} else {
				message = "检测到发布目录，但升级包不完整"
			}
		}
	}

	if rr, err := a.Panel.ResolveRemoteRelease(); err == nil {
		remote = rr
		if strings.TrimSpace(rr.Version) != "" {
			latestVersion = rr.Version
		}
		if rr.Version != "" && rr.Version != current {
			hasUpdate = true
		}
		if rr.Version == "" {
			message = "已连接远程更新源，但未返回版本号"
		} else if rr.Version == current {
			message = "当前已是最新版本"
		} else {
			message = "检测到远程可更新版本"
		}
	} else if remote == nil && localErr == nil {
		// 保留本地消息，不覆盖
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"currentVersion": current,
		"latestVersion":  latestVersion,
		"hasUpdate":      hasUpdate,
		"configured":     configured,
		"message":        message,
		"release":        release,
		"remote":         remote,
	})
}

func (a *App) PanelUpgradeAPI(w http.ResponseWriter, r *http.Request) {
	err := a.Panel.Upgrade()
	a.auditFromRequest(r, "panel.upgrade", err)
	respondMessage(w, err, "面板升级脚本已执行，服务可能会短暂重启")
}

func (a *App) PanelRemoteUpgradeAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL    string `json:"url"`
		SHA256 string `json:"sha256"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)

	downloadURL := strings.TrimSpace(in.URL)
	expectedSHA := strings.TrimSpace(in.SHA256)
	if downloadURL == "" {
		rr, err := a.Panel.ResolveRemoteRelease()
		if err != nil {
			respondMessage(w, fmt.Errorf("未提供下载链接，且自动解析远程版本失败: %w", err), "")
			return
		}
		downloadURL = rr.URL
		if expectedSHA == "" {
			expectedSHA = rr.SHA256
		}
	}

	info, err := a.Panel.DownloadAndExtractWithSHA(downloadURL, expectedSHA)
	if err != nil {
		a.auditFromRequest(r, "panel.remote_download", err)
		respondMessage(w, err, "")
		return
	}

	// 解压成功后直接执行升级
	err = a.Panel.Upgrade()
	a.auditFromRequest(r, "panel.remote_upgrade", err)
	msg := fmt.Sprintf("已成功下载 %s 版本的升级包并触发升级脚本，服务可能会短暂重启。", info.Version)
	respondMessage(w, err, msg)
}
