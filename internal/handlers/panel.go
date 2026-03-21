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
	remoteError := ""

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
	} else {
		remoteError = err.Error()
		if localErr == nil && !configured {
			message = "远程更新源不可用"
		}
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
		"remote_error":   remoteError,
		"checked_at":     time.Now().Format(time.RFC3339),
	})
}

func (a *App) PanelUpgradeTaskAPI(w http.ResponseWriter, r *http.Request) {
	taskID := strings.TrimSpace(r.URL.Query().Get("id"))
	if taskID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "缺少任务ID"})
		return
	}
	task := a.Panel.Task(taskID)
	if task == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "任务不存在"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "task": task})
}

func (a *App) PanelProbeRemoteAPI(w http.ResponseWriter, r *http.Request) {
	probe, err := a.Panel.ProbeRemoteRelease()
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error(), "probe": probe})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "probe": probe})
}

func (a *App) PanelUpgradeAPI(w http.ResponseWriter, r *http.Request) {
	user, _ := a.Sessions.Username(r)
	task := a.Panel.NewTask("local", "", "")
	go func(username, taskID string) {
		time.Sleep(200 * time.Millisecond)
		a.Panel.MarkTaskRunning(taskID, "正在执行本地升级脚本")
		err := a.Panel.Upgrade()
		if err != nil {
			a.Panel.MarkTaskFailed(taskID, err)
		} else {
			a.Panel.MarkTaskSuccess(taskID, "本地升级完成，服务已恢复")
		}
		if a.Audit != nil {
			result := "ok"
			if err != nil {
				result = err.Error()
			}
			a.Audit.Log(username, "panel.upgrade", result)
		}
	}(user, task.ID)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"message": "已触发面板升级任务，服务可能会短暂重启，请约 10-30 秒后刷新页面",
		"task":    task,
	})
}

func (a *App) PanelRemoteUpgradeAPI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL    string `json:"url"`
		SHA256 string `json:"sha256"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)

	downloadURL := strings.TrimSpace(in.URL)
	expectedSHA := strings.TrimSpace(in.SHA256)
	resolvedVersion := ""
	if downloadURL == "" {
		rr, err := a.Panel.ResolveRemoteRelease()
		if err != nil {
			respondMessage(w, fmt.Errorf("未提供下载链接，且自动解析远程版本失败: %w", err), "")
			return
		}
		downloadURL = rr.URL
		resolvedVersion = strings.TrimSpace(rr.Version)
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
	if strings.TrimSpace(resolvedVersion) == "" {
		resolvedVersion = strings.TrimSpace(info.Version)
	}

	user, _ := a.Sessions.Username(r)
	task := a.Panel.NewTask("remote", resolvedVersion, info.Path)
	go func(username, taskID, version string) {
		time.Sleep(200 * time.Millisecond)
		msg := "正在执行远程升级脚本"
		if strings.TrimSpace(version) != "" {
			msg = fmt.Sprintf("正在升级到 %s", strings.TrimSpace(version))
		}
		a.Panel.MarkTaskRunning(taskID, msg)

		err := a.Panel.Upgrade()
		if err != nil {
			a.Panel.MarkTaskFailed(taskID, err)
		} else {
			successMsg := "远程升级完成，服务已恢复"
			if strings.TrimSpace(version) != "" {
				successMsg = fmt.Sprintf("已升级到 %s，服务已恢复", strings.TrimSpace(version))
			}
			a.Panel.MarkTaskSuccess(taskID, successMsg)
		}
		if a.Audit != nil {
			result := "ok"
			if err != nil {
				result = err.Error()
			}
			a.Audit.Log(username, "panel.remote_upgrade", result)
		}
	}(user, task.ID, resolvedVersion)

	msg := "已成功下载升级包并开始后台升级，页面可能短暂断开，请约 10-30 秒后刷新。"
	if strings.TrimSpace(resolvedVersion) != "" {
		msg = fmt.Sprintf("已成功下载 %s 版本升级包并开始后台升级，页面可能短暂断开，请约 10-30 秒后刷新。", strings.TrimSpace(resolvedVersion))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"message": msg,
		"task":    task,
	})
}
