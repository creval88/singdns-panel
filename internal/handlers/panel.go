package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

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
	user, _ := a.Sessions.Username(r)
	go func(username string) {
		time.Sleep(200 * time.Millisecond)
		err := a.Panel.Upgrade()
		if a.Audit != nil {
			result := "ok"
			if err != nil {
				result = err.Error()
			}
			a.Audit.Log(username, "panel.upgrade", result)
		}
	}(user)
	respondMessage(w, nil, "已触发面板升级任务，服务可能会短暂重启，请约 10-30 秒后刷新页面")
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

	user, _ := a.Sessions.Username(r)
	go func(username string) {
		time.Sleep(200 * time.Millisecond)
		err := a.Panel.Upgrade()
		if a.Audit != nil {
			result := "ok"
			if err != nil {
				result = err.Error()
			}
			a.Audit.Log(username, "panel.remote_upgrade", result)
		}
	}(user)

	msg := fmt.Sprintf("已成功下载 %s 版本升级包并开始后台升级，页面可能短暂断开，请约 10-30 秒后刷新。", info.Version)
	respondMessage(w, nil, msg)
}
